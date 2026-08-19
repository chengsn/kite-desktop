package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chengsn/kite-desktop/pkg/common"
	"github.com/chengsn/kite-desktop/pkg/middleware"
	"github.com/chengsn/kite-desktop/pkg/model"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "kite-handler-tests-*")
	if err != nil {
		panic(err)
	}

	common.DBType = "sqlite"
	common.DBDSN = filepath.Join(tempDir, "favorite-handler-test.db")
	model.InitDB()

	exitCode := m.Run()
	_ = os.RemoveAll(tempDir)
	os.Exit(exitCode)
}

func TestGetPreferenceClusterNamePrecedence(t *testing.T) {
	assertClusterNamePrecedence(t, newFavoriteContext, getPreferenceClusterName, "getPreferenceClusterName")
}

func TestFavoriteHandlersCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFavoriteHandlerTestDB(t)

	router := gin.New()
	router.GET("/favorites", GetFavoriteResources)
	router.POST("/favorites", SaveFavoriteResource)
	router.POST("/favorites/remove", RemoveFavoriteResource)

	saveResponse := performFavoriteRequest[tFavoriteResponse](t, router, http.MethodPost, "/favorites", favoriteRequestPayload{
		ResourceType: "Deployments",
		Namespace:    "default",
		ResourceName: "nginx",
	}, "cluster-a")
	if saveResponse.ResourceType != "deployments" {
		t.Fatalf("ResourceType = %q, want %q", saveResponse.ResourceType, "deployments")
	}

	favorites := performFavoriteRequest[[]tFavoriteResponse](t, router, http.MethodGet, "/favorites", nil, "cluster-a")
	if len(favorites) != 1 {
		t.Fatalf("GET /favorites len = %d, want 1", len(favorites))
	}
	if favorites[0].ResourceName != "nginx" {
		t.Fatalf("GET /favorites resource = %q, want %q", favorites[0].ResourceName, "nginx")
	}

	performFavoriteNoContentRequest(t, router, http.MethodPost, "/favorites/remove", favoriteRequestPayload{
		ResourceType: "deployments",
		Namespace:    "default",
		ResourceName: "nginx",
	}, "cluster-a")

	favorites = performFavoriteRequest[[]tFavoriteResponse](t, router, http.MethodGet, "/favorites", nil, "cluster-a")
	if len(favorites) != 0 {
		t.Fatalf("GET /favorites after remove len = %d, want 0", len(favorites))
	}
}

func TestSaveFavoriteResourceRequiresCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFavoriteHandlerTestDB(t)

	router := gin.New()
	router.POST("/favorites", SaveFavoriteResource)

	rec := httptest.NewRecorder()
	body := mustMarshalFavoritePayload(t, favoriteRequestPayload{
		ResourceType: "pods",
		Namespace:    "default",
		ResourceName: "pod-a",
	})
	req := httptest.NewRequest(http.MethodPost, "/favorites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /favorites status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

type favoriteRequestPayload struct {
	ResourceType string `json:"resourceType"`
	Namespace    string `json:"namespace,omitempty"`
	ResourceName string `json:"resourceName"`
}

type tFavoriteResponse struct {
	ID           uint   `json:"id"`
	ClusterName  string `json:"clusterName"`
	ResourceType string `json:"resourceType"`
	Namespace    string `json:"namespace"`
	ResourceName string `json:"resourceName"`
}

func setupFavoriteHandlerTestDB(t *testing.T) {
	t.Helper()

	if err := model.DB.Exec("DELETE FROM favorite_resources").Error; err != nil {
		t.Fatalf("cleanup favorite_resources failed: %v", err)
	}
	if err := model.DB.Where("username = ?", model.LocalDesktopUser.Username).Delete(&model.User{}).Error; err != nil {
		t.Fatalf("cleanup local desktop user failed: %v", err)
	}
}

func newFavoriteContext(t *testing.T) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/favorites", nil)
	req.AddCookie(&http.Cookie{Name: middleware.ClusterNameHeader, Value: "cookie-cluster"})
	ctx.Request = req
	return ctx
}

func mustMarshalFavoritePayload(t *testing.T, payload any) []byte {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return body
}

func performFavoriteRequest[T any](t *testing.T, router *gin.Engine, method, path string, payload any, clusterName string) T {
	t.Helper()

	rec := httptest.NewRecorder()
	var body []byte
	if payload != nil {
		body = mustMarshalFavoritePayload(t, payload)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if clusterName != "" {
		req.Header.Set(middleware.ClusterNameHeader, clusterName)
	}

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, http.StatusOK, rec.Body.String())
	}

	var result T
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rec.Body.String())
	}
	return result
}

func performFavoriteNoContentRequest(t *testing.T, router *gin.Engine, method, path string, payload any, clusterName string) {
	t.Helper()

	rec := httptest.NewRecorder()
	body := mustMarshalFavoritePayload(t, payload)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if clusterName != "" {
		req.Header.Set(middleware.ClusterNameHeader, clusterName)
	}

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, http.StatusNoContent, rec.Body.String())
	}
}
