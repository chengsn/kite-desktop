package middleware

import (
	"net/url"
	"strings"

	"github.com/chengsn/kite-desktop/pkg/cluster"
	"github.com/gin-gonic/gin"
)

const (
	ClusterNameHeader = "x-cluster-name"
	ClusterNameKey    = "cluster-name"
	K8sClientKey      = "k8s-client"
	PromClientKey     = "prom-client"
)

// ClusterMiddleware extracts cluster name from header and injects clients into context
func ClusterMiddleware(cm *cluster.ClusterManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.GetHeader(ClusterNameHeader)
		if clusterName == "" {
			if v, ok := c.GetQuery(ClusterNameHeader); ok {
				clusterName = v
			}
			if clusterName == "" {
				clusterName = ReadClusterNameCookie(c)
			}
		}
		cluster, err := cm.GetClientSet(clusterName)
		if err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		c.Set("cluster", cluster)
		c.Set(ClusterNameKey, cluster.Name)
		c.Next()
	}
}

func ReadClusterNameCookie(c *gin.Context) string {
	rawValue, err := c.Cookie(ClusterNameHeader)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(rawValue)
	if trimmed == "" {
		return ""
	}
	decoded, err := url.QueryUnescape(trimmed)
	if err != nil {
		return trimmed
	}
	return strings.TrimSpace(decoded)
}
