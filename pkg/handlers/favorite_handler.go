package handlers

import (
	"net/http"
	"strings"

	"github.com/chengsn/kite-desktop/pkg/middleware"
	"github.com/chengsn/kite-desktop/pkg/model"
	"github.com/gin-gonic/gin"
)

type favoriteResourceRequest struct {
	ResourceType string `json:"resourceType" binding:"required"`
	Namespace    string `json:"namespace,omitempty"`
	ResourceName string `json:"resourceName" binding:"required"`
}

func GetFavoriteResources(c *gin.Context) {
	clusterName := getPreferenceClusterName(c)
	if clusterName == "" {
		c.JSON(http.StatusOK, []model.FavoriteResource{})
		return
	}

	favorites, err := model.ListDesktopFavoriteResources(clusterName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, favorites)
}

func SaveFavoriteResource(c *gin.Context) {
	clusterName := getPreferenceClusterName(c)
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster name is required"})
		return
	}

	var req favoriteResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	favorite, err := model.SaveDesktopFavoriteResource(
		clusterName,
		req.ResourceType,
		req.Namespace,
		req.ResourceName,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, favorite)
}

func RemoveFavoriteResource(c *gin.Context) {
	clusterName := getPreferenceClusterName(c)
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster name is required"})
		return
	}

	var req favoriteResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := model.DeleteDesktopFavoriteResource(
		clusterName,
		req.ResourceType,
		req.Namespace,
		req.ResourceName,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func getPreferenceClusterName(c *gin.Context) string {
	if clusterName := strings.TrimSpace(c.GetString(middleware.ClusterNameKey)); clusterName != "" {
		return clusterName
	}
	if clusterName := strings.TrimSpace(c.GetHeader(middleware.ClusterNameHeader)); clusterName != "" {
		return clusterName
	}
	if clusterName, ok := c.GetQuery(middleware.ClusterNameHeader); ok {
		if trimmed := strings.TrimSpace(clusterName); trimmed != "" {
			return trimmed
		}
	}
	return middleware.ReadClusterNameCookie(c)
}
