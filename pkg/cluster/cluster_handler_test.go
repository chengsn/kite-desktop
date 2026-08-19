package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/chengsn/kite-desktop/pkg/model"
	"github.com/chengsn/kite-desktop/pkg/prometheus"
	"github.com/gin-gonic/gin"
)

func TestFormatClusterConnectionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		contains    string
		detail      string
		notContains string
	}{
		{
			name:     "timeout",
			err:      context.DeadlineExceeded,
			contains: "timed out after 12s",
			detail:   context.DeadlineExceeded.Error(),
		},
		{
			name:     "dns",
			err:      errors.New("lookup demo.example.invalid: no such host"),
			contains: "Failed to resolve the Kubernetes API Server host.",
			detail:   "lookup demo.example.invalid: no such host",
		},
		{
			name:     "tls",
			err:      errors.New("x509: certificate signed by unknown authority"),
			contains: "TLS certificate validation failed.",
			detail:   "x509: certificate signed by unknown authority",
		},
		{
			name:     "default passthrough",
			err:      errors.New("plain failure"),
			contains: "Cluster connection test failed.",
			detail:   "plain failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatClusterConnectionError(tt.err)
			if got == nil {
				t.Fatal("expected formatted error, got nil")
			}
			if !strings.Contains(got.Error(), tt.contains) {
				t.Fatalf("formatted error = %q, want substring %q", got.Error(), tt.contains)
			}
			var connectionErr *clusterConnectionError
			if !errors.As(got, &connectionErr) {
				t.Fatalf("expected clusterConnectionError, got %T", got)
			}
			if connectionErr.Code == "" {
				t.Fatalf("expected error code, got %+v", connectionErr)
			}
			if tt.detail != "" && !strings.Contains(connectionErr.Detail, tt.detail) {
				t.Fatalf("error detail = %q, want substring %q", connectionErr.Detail, tt.detail)
			}
			if tt.notContains != "" && strings.Contains(connectionErr.Detail, tt.notContains) {
				t.Fatalf("error detail = %q, unexpected substring %q", connectionErr.Detail, tt.notContains)
			}
		})
	}
}

func TestTestClusterConnectionSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalTester := clusterConnectionTester
	t.Cleanup(func() {
		clusterConnectionTester = originalTester
	})

	clusterConnectionTester = func(cluster *model.Cluster) (*ClientSet, error) {
		if cluster.Name != "demo" {
			t.Fatalf("cluster.Name = %q, want %q", cluster.Name, "demo")
		}
		if string(cluster.Config) != "apiVersion: v1" {
			t.Fatalf("cluster.Config = %q, want kubeconfig body", string(cluster.Config))
		}
		return &ClientSet{Version: "v1.30.0"}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/clusters/test",
		strings.NewReader(`{"name":"demo","config":"apiVersion: v1"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ClusterManager{}).TestClusterConnection(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Message string `json:"message"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Version != "v1.30.0" {
		t.Fatalf("version = %q, want %q", response.Version, "v1.30.0")
	}
}

func TestTestClusterConnectionReturnsReadableError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalTester := clusterConnectionTester
	t.Cleanup(func() {
		clusterConnectionTester = originalTester
	})

	clusterConnectionTester = func(cluster *model.Cluster) (*ClientSet, error) {
		return nil, &clusterConnectionError{
			Code:    clusterConnectionErrorTimeout,
			Message: "Connection test timed out after 12s.",
			Detail:  "context deadline exceeded",
		}
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/clusters/test",
		strings.NewReader(`{"name":"demo","config":"apiVersion: v1"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ClusterManager{}).TestClusterConnection(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	var response struct {
		Error       string `json:"error"`
		ErrorCode   string `json:"errorCode"`
		ErrorDetail string `json:"errorDetail"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.ErrorCode != clusterConnectionErrorTimeout {
		t.Fatalf("errorCode = %q, want %q", response.ErrorCode, clusterConnectionErrorTimeout)
	}
	if !strings.Contains(response.Error, "timed out") {
		t.Fatalf("error = %q, want timeout hint", response.Error)
	}
	if response.ErrorDetail == "" {
		t.Fatalf("expected errorDetail, got empty response: %+v", response)
	}
}

func TestTestClusterConnectionUsesExistingConfigWhenEditing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreDB := useClusterManagerTestDB(t)
	defer restoreDB()

	cluster := &model.Cluster{
		Name:          "demo",
		Config:        model.SecretString("apiVersion: v1\nclusters: []"),
		PrometheusURL: "http://old-prometheus.example",
		Enable:        true,
	}
	if err := model.AddCluster(cluster); err != nil {
		t.Fatalf("add cluster: %v", err)
	}

	originalTester := clusterConnectionTester
	t.Cleanup(func() {
		clusterConnectionTester = originalTester
	})

	clusterConnectionTester = func(cluster *model.Cluster) (*ClientSet, error) {
		if cluster.Name != "demo" {
			t.Fatalf("cluster.Name = %q, want %q", cluster.Name, "demo")
		}
		if string(cluster.Config) != "apiVersion: v1\nclusters: []" {
			t.Fatalf("cluster.Config = %q, want persisted config", string(cluster.Config))
		}
		if cluster.PrometheusURL != "http://new-prometheus.example" {
			t.Fatalf("cluster.PrometheusURL = %q, want request override", cluster.PrometheusURL)
		}
		return &ClientSet{Version: "v1.31.0"}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/clusters/test",
		strings.NewReader(`{"id":`+strconv.FormatUint(uint64(cluster.ID), 10)+`,"name":"demo","prometheusURL":"http://new-prometheus.example"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ClusterManager{}).TestClusterConnection(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestResolvePrometheusAuth(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		req      clusterRequest
		existing *model.Cluster
		want     prometheus.BasicAuth
		wantErr  bool
	}{
		{
			name: "no url clears credentials",
			req: clusterRequest{
				PrometheusURL:      "",
				PrometheusUsername: strPtr("admin"),
				PrometheusPassword: strPtr("s3cret"),
			},
			existing: &model.Cluster{PrometheusUsername: "old", PrometheusPassword: "old-secret"},
			want:     prometheus.BasicAuth{},
		},
		{
			name: "new credentials are stored",
			req: clusterRequest{
				PrometheusURL:      "http://prometheus.example.com",
				PrometheusUsername: strPtr("admin"),
				PrometheusPassword: strPtr("s3cret"),
			},
			want: prometheus.BasicAuth{Username: "admin", Password: "s3cret"},
		},
		{
			name: "empty password keeps the stored one",
			req: clusterRequest{
				PrometheusURL:      "http://prometheus.example.com",
				PrometheusUsername: strPtr("admin"),
				PrometheusPassword: strPtr(""),
			},
			existing: &model.Cluster{
				PrometheusUsername: "admin",
				PrometheusPassword: "stored-secret",
			},
			want: prometheus.BasicAuth{Username: "admin", Password: "stored-secret"},
		},
		{
			name: "omitted fields keep the stored credentials",
			req: clusterRequest{
				PrometheusURL: "http://prometheus.example.com",
			},
			existing: &model.Cluster{
				PrometheusUsername: "admin",
				PrometheusPassword: "stored-secret",
			},
			want: prometheus.BasicAuth{Username: "admin", Password: "stored-secret"},
		},
		{
			name: "username is trimmed",
			req: clusterRequest{
				PrometheusURL:      "http://prometheus.example.com",
				PrometheusUsername: strPtr("  admin  "),
				PrometheusPassword: strPtr("s3cret"),
			},
			want: prometheus.BasicAuth{Username: "admin", Password: "s3cret"},
		},
		{
			name: "clearing the username clears the password",
			req: clusterRequest{
				PrometheusURL:      "http://prometheus.example.com",
				PrometheusUsername: strPtr(""),
			},
			existing: &model.Cluster{
				PrometheusUsername: "admin",
				PrometheusPassword: "stored-secret",
			},
			want: prometheus.BasicAuth{},
		},
		{
			name: "username without password is rejected",
			req: clusterRequest{
				PrometheusURL:      "http://prometheus.example.com",
				PrometheusUsername: strPtr("admin"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolvePrometheusAuth(&tt.req, tt.existing)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got auth %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePrometheusAuth() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolvePrometheusAuth() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCreateAndUpdateClusterPersistPrometheusAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreDB := useClusterManagerTestDB(t)
	defer restoreDB()

	// These handlers wait for the cluster sync worker; without a running
	// ClusterManager the request would block forever.
	restoreSync := stubClusterSync(t)
	defer restoreSync()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/clusters/",
		strings.NewReader(`{"name":"demo","config":"apiVersion: v1","prometheusURL":"http://prometheus.example.com","prometheusUsername":"admin","prometheusPassword":"s3cret"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ClusterManager{}).CreateCluster(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}

	created, err := model.GetClusterByName("demo")
	if err != nil {
		t.Fatalf("get created cluster: %v", err)
	}
	if created.PrometheusUsername != "admin" {
		t.Fatalf("PrometheusUsername = %q, want %q", created.PrometheusUsername, "admin")
	}
	if string(created.PrometheusPassword) != "s3cret" {
		t.Fatalf("PrometheusPassword = %q, want %q", string(created.PrometheusPassword), "s3cret")
	}

	// Updating with an empty password must keep the stored secret.
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(created.ID), 10)}}
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/clusters/"+strconv.FormatUint(uint64(created.ID), 10),
		strings.NewReader(`{"name":"demo","prometheusURL":"http://prometheus.example.com","prometheusUsername":"admin","prometheusPassword":""}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ClusterManager{}).UpdateCluster(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	updated, err := model.GetClusterByID(created.ID)
	if err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	if updated.PrometheusUsername != "admin" {
		t.Fatalf("PrometheusUsername = %q, want %q", updated.PrometheusUsername, "admin")
	}
	if string(updated.PrometheusPassword) != "s3cret" {
		t.Fatalf("PrometheusPassword = %q, want the stored secret to survive an empty update", string(updated.PrometheusPassword))
	}
}

func TestGetClusterListNeverReturnsPrometheusPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreDB := useClusterManagerTestDB(t)
	defer restoreDB()

	cluster := &model.Cluster{
		Name:               "demo",
		PrometheusURL:      "http://prometheus.example.com",
		PrometheusUsername: "admin",
		PrometheusPassword: "s3cret",
		Enable:             true,
	}
	if err := model.AddCluster(cluster); err != nil {
		t.Fatalf("add cluster: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/clusters/", nil)

	(&ClusterManager{}).GetClusterList(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if strings.Contains(recorder.Body.String(), "s3cret") {
		t.Fatalf("response leaked the Prometheus password: %s", recorder.Body.String())
	}

	var response []map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("got %d clusters, want 1", len(response))
	}
	if got := response[0]["prometheusUsername"]; got != "admin" {
		t.Fatalf("prometheusUsername = %v, want %q", got, "admin")
	}
	if got := response[0]["prometheusPasswordConfigured"]; got != true {
		t.Fatalf("prometheusPasswordConfigured = %v, want true", got)
	}
	if _, ok := response[0]["prometheusPassword"]; ok {
		t.Fatalf("response must not contain a prometheusPassword field")
	}
}

func TestTestClusterConnectionReusesStoredPrometheusCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreDB := useClusterManagerTestDB(t)
	defer restoreDB()

	cluster := &model.Cluster{
		Name:               "demo",
		Config:             model.SecretString("apiVersion: v1\nclusters: []"),
		PrometheusURL:      "http://prometheus.example.com",
		PrometheusUsername: "admin",
		PrometheusPassword: "s3cret",
		Enable:             true,
	}
	if err := model.AddCluster(cluster); err != nil {
		t.Fatalf("add cluster: %v", err)
	}

	originalTester := clusterConnectionTester
	t.Cleanup(func() {
		clusterConnectionTester = originalTester
	})

	clusterConnectionTester = func(cluster *model.Cluster) (*ClientSet, error) {
		if cluster.PrometheusUsername != "admin" {
			t.Fatalf("PrometheusUsername = %q, want %q", cluster.PrometheusUsername, "admin")
		}
		if string(cluster.PrometheusPassword) != "s3cret" {
			t.Fatalf("PrometheusPassword = %q, want the stored secret", string(cluster.PrometheusPassword))
		}
		return &ClientSet{Version: "v1.31.0"}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/clusters/test",
		strings.NewReader(`{"id":`+strconv.FormatUint(uint64(cluster.ID), 10)+`,"name":"demo"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ClusterManager{}).TestClusterConnection(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

// stubClusterSync replaces the package sync channel with one that answers
// immediately, so handler tests that call requestClusterSync(true) do not block.
func stubClusterSync(t *testing.T) func() {
	t.Helper()

	originalSyncNow := syncNow
	syncChannel := make(chan chan error, 1)
	syncNow = syncChannel

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case done := <-syncChannel:
				if done != nil {
					done <- nil
					close(done)
				}
			case <-stop:
				return
			}
		}
	}()

	return func() {
		close(stop)
		syncNow = originalSyncNow
	}
}
