package ai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chengsn/kite-desktop/pkg/cluster"
	"github.com/chengsn/kite-desktop/pkg/common"
	"github.com/chengsn/kite-desktop/pkg/model"
	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "kite-ai-tests-*")
	if err != nil {
		panic(err)
	}

	common.DBType = "sqlite"
	common.DBDSN = filepath.Join(tempDir, "ai-test.db")
	model.InitDB()

	exitCode := m.Run()
	_ = os.RemoveAll(tempDir)
	os.Exit(exitCode)
}

func TestChatHistoryHandlersCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if err := model.DB.Where("session_id LIKE ?", "handler-session-%").Delete(&model.AIChatMessage{}).Error; err != nil {
		t.Fatalf("cleanup messages error = %v", err)
	}
	if err := model.DB.Where("session_id LIKE ?", "handler-session-%").Delete(&model.AIChatSession{}).Error; err != nil {
		t.Fatalf("cleanup sessions error = %v", err)
	}

	upsertBody := map[string]interface{}{
		"title": "Check deployment rollout",
		"pageContext": map[string]interface{}{
			"page":         "deployment-detail",
			"namespace":    "default",
			"resourceName": "nginx",
			"resourceKind": "deployment",
		},
		"messages": []map[string]interface{}{
			{
				"id":      "m1",
				"role":    "user",
				"content": "why rollout stuck",
			},
			{
				"id":      "m2",
				"role":    "assistant",
				"content": "checking deployment status",
			},
			{
				"id":         "m3",
				"role":       "tool",
				"content":    "get_resource completed",
				"toolName":   "get_resource",
				"toolCallId": "call-1",
				"toolArgs": map[string]interface{}{
					"kind": "deployment",
					"name": "nginx",
				},
				"toolResult": "ok",
			},
		},
	}

	rec := performHistoryRequest(t, http.MethodPut, "/api/v1/ai/sessions/handler-session-1", upsertBody, "handler-session-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var upsertResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &upsertResp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if upsertResp["sessionId"] != "handler-session-1" {
		t.Fatalf("sessionId = %#v, want handler-session-1", upsertResp["sessionId"])
	}
	if upsertResp["messageCount"] != float64(3) {
		t.Fatalf("messageCount = %#v, want 3", upsertResp["messageCount"])
	}

	rec = performHistoryRequest(t, http.MethodGet, "/api/v1/ai/sessions?page=1&pageSize=20", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET list status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var listResp struct {
		Data []struct {
			SessionID    string `json:"sessionId"`
			Title        string `json:"title"`
			MessageCount int    `json:"messageCount"`
			ClusterName  string `json:"clusterName"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatal("list response is empty")
	}
	if listResp.Data[0].SessionID != "handler-session-1" {
		t.Fatalf("list sessionId = %q, want %q", listResp.Data[0].SessionID, "handler-session-1")
	}
	if listResp.Data[0].ClusterName != "cluster-a" {
		t.Fatalf("clusterName = %q, want %q", listResp.Data[0].ClusterName, "cluster-a")
	}

	rec = performHistoryRequest(t, http.MethodGet, "/api/v1/ai/sessions/handler-session-1", nil, "handler-session-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET detail status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var detailResp struct {
		SessionID    string `json:"sessionId"`
		MessageCount int    `json:"messageCount"`
		Messages     []struct {
			ID         string                 `json:"id"`
			Role       string                 `json:"role"`
			ToolArgs   map[string]interface{} `json:"toolArgs"`
			ToolResult string                 `json:"toolResult"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("json.Unmarshal(detail) error = %v", err)
	}
	if detailResp.SessionID != "handler-session-1" {
		t.Fatalf("detail sessionId = %q, want %q", detailResp.SessionID, "handler-session-1")
	}
	if detailResp.MessageCount != 3 || len(detailResp.Messages) != 3 {
		t.Fatalf("unexpected detail payload: %#v", detailResp)
	}
	if detailResp.Messages[2].ToolArgs["kind"] != "deployment" {
		t.Fatalf("toolArgs.kind = %#v, want deployment", detailResp.Messages[2].ToolArgs["kind"])
	}

	rec = performHistoryRequest(t, http.MethodDelete, "/api/v1/ai/sessions/handler-session-1", nil, "handler-session-1")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = performHistoryRequest(t, http.MethodGet, "/api/v1/ai/sessions/handler-session-1", nil, "handler-session-1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET detail after delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func performHistoryRequest(
	t *testing.T,
	method string,
	target string,
	body interface{},
	sessionID string,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader *bytes.Reader
	if body == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, target, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	ctx.Set("cluster", &cluster.ClientSet{Name: "cluster-a"})
	if sessionID != "" {
		ctx.Params = gin.Params{{Key: "sessionId", Value: sessionID}}
	}

	switch method {
	case http.MethodGet:
		if sessionID == "" {
			HandleListSessions(ctx)
		} else {
			HandleGetSession(ctx)
		}
	case http.MethodPut:
		HandleUpsertSession(ctx)
	case http.MethodDelete:
		HandleDeleteSession(ctx)
	default:
		t.Fatalf("unsupported method %s", method)
	}

	return rec
}
