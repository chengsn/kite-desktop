package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chengsn/kite-desktop/pkg/common"
	"github.com/chengsn/kite-desktop/pkg/model"
	"github.com/chengsn/kite-desktop/pkg/prometheus"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type clusterRequest struct {
	ID                 uint    `json:"id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	Config             string  `json:"config"`
	PrometheusURL      string  `json:"prometheusURL"`
	PrometheusUsername *string `json:"prometheusUsername"`
	PrometheusPassword *string `json:"prometheusPassword"`
	InCluster          bool    `json:"inCluster"`
	IsDefault          bool    `json:"isDefault"`
	Enabled            bool    `json:"enabled"`
}

// resolvePrometheusAuth merges the request credentials with the ones already
// stored for the cluster. The stored username and password are only replaced
// when the request carries a value, so the UI can submit the form without ever
// reading the secret back. Clearing the URL removes the credentials entirely.
func resolvePrometheusAuth(req *clusterRequest, existing *model.Cluster) (prometheus.BasicAuth, error) {
	if strings.TrimSpace(req.PrometheusURL) == "" {
		return prometheus.BasicAuth{}, nil
	}

	var username, password string
	if existing != nil {
		username = existing.PrometheusUsername
		password = string(existing.PrometheusPassword)
	}
	if req.PrometheusUsername != nil {
		username = strings.TrimSpace(*req.PrometheusUsername)
	}
	if req.PrometheusPassword != nil && *req.PrometheusPassword != "" {
		password = *req.PrometheusPassword
	}

	// Basic auth needs both halves; without a username the credentials are cleared.
	if username == "" {
		return prometheus.BasicAuth{}, nil
	}
	if password == "" {
		return prometheus.BasicAuth{}, fmt.Errorf("prometheusPassword is required when prometheusUsername is set")
	}

	return prometheus.BasicAuth{Username: username, Password: password}, nil
}

var clusterConnectionTester = validateClusterConnection

const clusterConnectionTestTimeout = 12 * time.Second

const (
	clusterConnectionErrorTimeout              = "CLUSTER_CONNECTION_TIMEOUT"
	clusterConnectionErrorDNSResolutionFailed  = "CLUSTER_CONNECTION_DNS_RESOLUTION_FAILED"
	clusterConnectionErrorRefused              = "CLUSTER_CONNECTION_REFUSED"
	clusterConnectionErrorTLSFailed            = "CLUSTER_CONNECTION_TLS_FAILED"
	clusterConnectionErrorUnauthorized         = "CLUSTER_CONNECTION_UNAUTHORIZED"
	clusterConnectionErrorForbidden            = "CLUSTER_CONNECTION_FORBIDDEN"
	clusterConnectionErrorInvalidConfig        = "CLUSTER_CONNECTION_INVALID_CONFIG"
	clusterConnectionErrorInClusterUnavailable = "CLUSTER_CONNECTION_IN_CLUSTER_UNAVAILABLE"
	clusterConnectionErrorUnknown              = "CLUSTER_CONNECTION_UNKNOWN"
)

type clusterConnectionError struct {
	Code    string
	Message string
	Detail  string
}

func (e *clusterConnectionError) Error() string {
	return e.Message
}

func (cm *ClusterManager) GetClusters(c *gin.Context) {
	result := make([]common.ClusterInfo, 0, len(cm.clusters))
	for name, cluster := range cm.clusters {
		result = append(result, common.ClusterInfo{
			Name:      name,
			Version:   cluster.Version,
			IsDefault: name == cm.defaultContext,
		})
	}
	for name, errMsg := range cm.errors {
		result = append(result, common.ClusterInfo{
			Name:      name,
			Version:   "",
			IsDefault: false,
			Error:     errMsg,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	c.JSON(200, result)
}

func (cm *ClusterManager) GetClusterList(c *gin.Context) {
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(clusters))
	for _, cluster := range clusters {
		clusterInfo := gin.H{
			"id":                           cluster.ID,
			"name":                         cluster.Name,
			"description":                  cluster.Description,
			"enabled":                      cluster.Enable,
			"inCluster":                    cluster.InCluster,
			"isDefault":                    cluster.IsDefault,
			"prometheusURL":                cluster.PrometheusURL,
			"prometheusUsername":           cluster.PrometheusUsername,
			"prometheusPasswordConfigured": string(cluster.PrometheusPassword) != "",
			"config":                       "",
		}

		if clientSet, exists := cm.clusters[cluster.Name]; exists {
			clusterInfo["version"] = clientSet.Version
		}
		if errMsg, exists := cm.errors[cluster.Name]; exists {
			clusterInfo["error"] = errMsg
		}

		result = append(result, clusterInfo)
	}

	c.JSON(http.StatusOK, result)
}

func (cm *ClusterManager) CreateCluster(c *gin.Context) {
	var req clusterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if _, err := model.GetClusterByName(req.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "cluster already exists"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.IsDefault {
		if err := model.ClearDefaultCluster(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	auth, err := resolvePrometheusAuth(&req, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cluster := &model.Cluster{
		Name:               req.Name,
		Description:        req.Description,
		Config:             model.SecretString(req.Config),
		PrometheusURL:      req.PrometheusURL,
		PrometheusUsername: auth.Username,
		PrometheusPassword: model.SecretString(auth.Password),
		InCluster:          req.InCluster,
		IsDefault:          req.IsDefault,
		Enable:             true,
	}

	if err := model.AddCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = requestClusterSync(true)

	c.JSON(http.StatusCreated, gin.H{
		"id":      cluster.ID,
		"message": "cluster created successfully",
	})
}

func (cm *ClusterManager) UpdateCluster(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return
	}

	var req clusterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cluster, err := model.GetClusterByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	if req.IsDefault && !cluster.IsDefault {
		if err := model.ClearDefaultCluster(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	auth, err := resolvePrometheusAuth(&req, cluster)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"description":         req.Description,
		"prometheus_url":      req.PrometheusURL,
		"prometheus_username": auth.Username,
		"prometheus_password": model.SecretString(auth.Password),
		"in_cluster":          req.InCluster,
		"is_default":          req.IsDefault,
		"enable":              req.Enabled,
	}

	if req.Name != "" && req.Name != cluster.Name {
		updates["name"] = req.Name
	}

	if req.Config != "" {
		updates["config"] = model.SecretString(req.Config)
	}

	if err := model.UpdateCluster(cluster, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = requestClusterSync(true)

	c.JSON(http.StatusOK, gin.H{"message": "cluster updated successfully"})
}

func (cm *ClusterManager) DeleteCluster(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return
	}

	cluster, err := model.GetClusterByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	if cluster.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete default cluster"})
		return
	}

	if err := model.DeleteCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = requestClusterSync(true)

	c.JSON(http.StatusOK, gin.H{"message": "cluster deleted successfully"})
}

func validateClusterConnection(cluster *model.Cluster) (*ClientSet, error) {
	restConfig, err := buildClusterRESTConfig(cluster)
	if err != nil {
		return nil, formatClusterConnectionError(err)
	}

	restConfig = rest.CopyConfig(restConfig)
	restConfig.Timeout = clusterConnectionTestTimeout

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, formatClusterConnectionError(err)
	}

	version, err := clientSet.Discovery().ServerVersion()
	if err != nil {
		return nil, formatClusterConnectionError(err)
	}

	return &ClientSet{
		Name:    cluster.Name,
		Version: version.String(),
	}, nil
}

func buildClusterRESTConfig(cluster *model.Cluster) (*rest.Config, error) {
	if cluster.InCluster {
		return rest.InClusterConfig()
	}

	return clientcmd.RESTConfigFromKubeConfig([]byte(cluster.Config))
}

func formatClusterConnectionError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	lowerMessage := strings.ToLower(message)

	switch {
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(lowerMessage, "context deadline exceeded"),
		strings.Contains(lowerMessage, "client.timeout exceeded"),
		strings.Contains(lowerMessage, "i/o timeout"),
		strings.Contains(lowerMessage, "tls handshake timeout"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorTimeout,
			Message: fmt.Sprintf("Connection test timed out after %s.", clusterConnectionTestTimeout),
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "no such host"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorDNSResolutionFailed,
			Message: "Failed to resolve the Kubernetes API Server host.",
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "connection refused"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorRefused,
			Message: "The Kubernetes API Server refused the connection.",
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "x509:"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorTLSFailed,
			Message: "TLS certificate validation failed.",
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "unauthorized"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorUnauthorized,
			Message: "Authentication failed.",
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "forbidden"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorForbidden,
			Message: "The current kubeconfig can reach the cluster, but it is not allowed to query cluster version.",
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "invalid configuration"),
		strings.Contains(lowerMessage, "no configuration has been provided"),
		strings.Contains(lowerMessage, "no server found for cluster"),
		strings.Contains(lowerMessage, "failed to parse kubeconfig"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorInvalidConfig,
			Message: "The kubeconfig is invalid or incomplete.",
			Detail:  message,
		}
	case strings.Contains(lowerMessage, "inclusterconfig"),
		strings.Contains(lowerMessage, "serviceaccount"),
		strings.Contains(lowerMessage, "kubernetes service host"):
		return &clusterConnectionError{
			Code:    clusterConnectionErrorInClusterUnavailable,
			Message: "In-cluster configuration is unavailable in the current environment.",
			Detail:  message,
		}
	default:
		return &clusterConnectionError{
			Code:    clusterConnectionErrorUnknown,
			Message: "Cluster connection test failed.",
			Detail:  message,
		}
	}
}

func writeClusterConnectionError(c *gin.Context, err error) {
	var connectionErr *clusterConnectionError
	if errors.As(err, &connectionErr) {
		response := gin.H{
			"error":     connectionErr.Message,
			"errorCode": connectionErr.Code,
		}
		if detail := strings.TrimSpace(connectionErr.Detail); detail != "" && detail != connectionErr.Message {
			response["errorDetail"] = detail
		}
		c.JSON(http.StatusBadRequest, response)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func (cm *ClusterManager) TestClusterConnection(c *gin.Context) {
	var req clusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clusterName := strings.TrimSpace(req.Name)
	if clusterName == "" {
		clusterName = "test-connection"
	}

	// Editing an existing cluster only sends the fields the user touched, so the
	// stored kubeconfig, Prometheus URL and credentials fill in the blanks.
	var existingCluster *model.Cluster
	if req.ID > 0 {
		existing, err := model.GetClusterByID(req.ID)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if !req.InCluster && strings.TrimSpace(req.Config) == "" {
				c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
				return
			}
		} else {
			existingCluster = existing
		}
	}

	config := strings.TrimSpace(req.Config)
	if !req.InCluster && config == "" && existingCluster != nil {
		config = strings.TrimSpace(string(existingCluster.Config))
	}

	if !req.InCluster && config == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config is required when inCluster is false"})
		return
	}

	prometheusURL := strings.TrimSpace(req.PrometheusURL)
	if prometheusURL == "" && existingCluster != nil {
		prometheusURL = existingCluster.PrometheusURL
	}

	authReq := req
	authReq.PrometheusURL = prometheusURL
	auth, err := resolvePrometheusAuth(&authReq, existingCluster)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cluster := &model.Cluster{
		Name:               clusterName,
		Config:             model.SecretString(config),
		PrometheusURL:      prometheusURL,
		PrometheusUsername: auth.Username,
		PrometheusPassword: model.SecretString(auth.Password),
		InCluster:          req.InCluster,
	}

	cs, err := clusterConnectionTester(cluster)
	if err != nil {
		writeClusterConnectionError(c, err)
		return
	}
	if cs != nil && cs.K8sClient != nil {
		defer cs.K8sClient.Stop(cluster.Name)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "cluster connection successful",
		"version": cs.Version,
	})
}

func (cm *ClusterManager) ImportClustersFromKubeconfig(c *gin.Context) {
	var clusterReq common.ImportClustersRequest
	if err := c.ShouldBindJSON(&clusterReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !clusterReq.InCluster && clusterReq.Config == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config is required when inCluster is false"})
		return
	}

	cc, err := model.CountClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cc > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "import not allowed when clusters exist"})
		return
	}

	if clusterReq.InCluster {
		// In-cluster config
		cluster := &model.Cluster{
			Name:        "in-cluster",
			InCluster:   true,
			Description: "Kubernetes in-cluster config",
			IsDefault:   true,
			Enable:      true,
		}
		if err := model.AddCluster(cluster); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = requestClusterSync(true)
		c.JSON(http.StatusCreated, gin.H{"message": fmt.Sprintf("imported %d clusters successfully", 1)})
		return
	}

	kubeconfig, err := clientcmd.Load([]byte(clusterReq.Config))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	importedCount := ImportClustersFromKubeconfig(kubeconfig)
	_ = requestClusterSync(true)
	c.JSON(http.StatusCreated, gin.H{"message": fmt.Sprintf("imported %d clusters successfully", importedCount)})
}
