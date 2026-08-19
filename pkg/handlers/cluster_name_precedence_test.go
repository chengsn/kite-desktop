package handlers

import (
	"testing"

	"github.com/chengsn/kite-desktop/pkg/middleware"
	"github.com/gin-gonic/gin"
)

func assertClusterNamePrecedence(
	t *testing.T,
	newContext func(*testing.T) *gin.Context,
	getClusterName func(*gin.Context) string,
	getterName string,
) {
	t.Helper()

	t.Run("context beats header query and cookie", func(t *testing.T) {
		ctx := newContext(t)
		ctx.Set(middleware.ClusterNameKey, "context-cluster")
		ctx.Request.Header.Set(middleware.ClusterNameHeader, "header-cluster")
		ctx.Request.URL.RawQuery = middleware.ClusterNameHeader + "=query-cluster"

		if got := getClusterName(ctx); got != "context-cluster" {
			t.Fatalf("%s() = %q, want %q", getterName, got, "context-cluster")
		}
	})

	t.Run("header beats query and cookie", func(t *testing.T) {
		ctx := newContext(t)
		ctx.Request.Header.Set(middleware.ClusterNameHeader, "header-cluster")
		ctx.Request.URL.RawQuery = middleware.ClusterNameHeader + "=query-cluster"

		if got := getClusterName(ctx); got != "header-cluster" {
			t.Fatalf("%s() = %q, want %q", getterName, got, "header-cluster")
		}
	})

	t.Run("query beats cookie", func(t *testing.T) {
		ctx := newContext(t)
		ctx.Request.URL.RawQuery = middleware.ClusterNameHeader + "=query-cluster"

		if got := getClusterName(ctx); got != "query-cluster" {
			t.Fatalf("%s() = %q, want %q", getterName, got, "query-cluster")
		}
	})

	t.Run("cookie fallback", func(t *testing.T) {
		ctx := newContext(t)

		if got := getClusterName(ctx); got != "cookie-cluster" {
			t.Fatalf("%s() = %q, want %q", getterName, got, "cookie-cluster")
		}
	})
}
