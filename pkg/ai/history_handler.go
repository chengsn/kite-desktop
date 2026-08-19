package ai

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/chengsn/kite-desktop/pkg/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type chatHistoryPageContext struct {
	Page         string `json:"page"`
	Namespace    string `json:"namespace"`
	ResourceName string `json:"resourceName"`
	ResourceKind string `json:"resourceKind"`
}

type chatHistoryMessage struct {
	ID            string                 `json:"id"`
	Role          string                 `json:"role"`
	Content       string                 `json:"content"`
	Thinking      string                 `json:"thinking,omitempty"`
	ToolCallID    string                 `json:"toolCallId,omitempty"`
	ToolName      string                 `json:"toolName,omitempty"`
	ToolArgs      map[string]interface{} `json:"toolArgs,omitempty"`
	ToolResult    string                 `json:"toolResult,omitempty"`
	ActionStatus  string                 `json:"actionStatus,omitempty"`
	InputRequest  map[string]interface{} `json:"inputRequest,omitempty"`
	PendingAction map[string]interface{} `json:"pendingAction,omitempty"`
}

type upsertChatSessionRequest struct {
	Title       string                 `json:"title"`
	PageContext chatHistoryPageContext `json:"pageContext"`
	Messages    []chatHistoryMessage   `json:"messages"`
}

type chatSessionSummaryResponse struct {
	SessionID     string                 `json:"sessionId"`
	Title         string                 `json:"title"`
	ClusterName   string                 `json:"clusterName"`
	PageContext   chatHistoryPageContext `json:"pageContext"`
	MessageCount  int                    `json:"messageCount"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
	LastMessageAt string                 `json:"lastMessageAt"`
}

type chatSessionDetailResponse struct {
	chatSessionSummaryResponse
	Messages []chatHistoryMessage `json:"messages"`
}

func HandleListSessions(c *gin.Context) {
	cs, ok := getClusterClientSet(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No cluster selected"})
		return
	}

	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("pageSize"), 20)

	sessions, total, err := model.ListAIChatSessions(cs.Name, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]chatSessionSummaryResponse, 0, len(sessions))
	for _, session := range sessions {
		data = append(data, buildChatSessionSummaryResponse(session))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     data,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func HandleGetSession(c *gin.Context) {
	cs, ok := getClusterClientSet(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No cluster selected"})
		return
	}

	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	session, err := model.GetAIChatSession(cs.Name, sessionID)
	if err != nil {
		handleChatSessionError(c, err)
		return
	}

	messages, err := model.ListAIChatMessages(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	messageDTOs, err := buildChatHistoryMessages(messages)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := chatSessionDetailResponse{
		chatSessionSummaryResponse: buildChatSessionSummaryResponse(*session),
		Messages:                   messageDTOs,
	}
	c.JSON(http.StatusOK, response)
}

func HandleUpsertSession(c *gin.Context) {
	cs, ok := getClusterClientSet(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No cluster selected"})
		return
	}

	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	var req upsertChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages are required"})
		return
	}

	modelMessages, err := buildModelChatMessages(sessionID, req.Messages)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New Chat"
	}

	session, err := model.UpsertAIChatSession(model.AIChatSessionSnapshot{
		SessionID:    sessionID,
		Title:        title,
		ClusterName:  cs.Name,
		Page:         strings.TrimSpace(req.PageContext.Page),
		Namespace:    strings.TrimSpace(req.PageContext.Namespace),
		ResourceName: strings.TrimSpace(req.PageContext.ResourceName),
		ResourceKind: strings.TrimSpace(req.PageContext.ResourceKind),
		Messages:     modelMessages,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, buildChatSessionSummaryResponse(*session))
}

func HandleDeleteSession(c *gin.Context) {
	cs, ok := getClusterClientSet(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No cluster selected"})
		return
	}

	sessionID := strings.TrimSpace(c.Param("sessionId"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	if err := model.DeleteAIChatSession(cs.Name, sessionID); err != nil {
		handleChatSessionError(c, err)
		return
	}

	c.AbortWithStatus(http.StatusNoContent)
}

func parsePositiveInt(raw string, fallback int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func handleChatSessionError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat session not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func buildChatSessionSummaryResponse(session model.AIChatSession) chatSessionSummaryResponse {
	return chatSessionSummaryResponse{
		SessionID:   session.SessionID,
		Title:       session.Title,
		ClusterName: session.ClusterName,
		PageContext: chatHistoryPageContext{
			Page:         session.Page,
			Namespace:    session.Namespace,
			ResourceName: session.ResourceName,
			ResourceKind: session.ResourceKind,
		},
		MessageCount:  session.MessageCount,
		CreatedAt:     session.CreatedAt.Format(timeLayoutRFC3339),
		UpdatedAt:     session.UpdatedAt.Format(timeLayoutRFC3339),
		LastMessageAt: session.LastMessageAt.Format(timeLayoutRFC3339),
	}
}

const timeLayoutRFC3339 = "2006-01-02T15:04:05Z07:00"

func buildModelChatMessages(sessionID string, messages []chatHistoryMessage) ([]model.AIChatMessage, error) {
	items := make([]model.AIChatMessage, 0, len(messages))
	for idx, message := range messages {
		item := model.AIChatMessage{
			SessionID:    sessionID,
			MessageID:    strings.TrimSpace(message.ID),
			Seq:          idx + 1,
			Role:         strings.TrimSpace(message.Role),
			Content:      message.Content,
			Thinking:     message.Thinking,
			ToolCallID:   strings.TrimSpace(message.ToolCallID),
			ToolName:     strings.TrimSpace(message.ToolName),
			ToolResult:   message.ToolResult,
			ActionStatus: strings.TrimSpace(message.ActionStatus),
		}
		if item.MessageID == "" {
			item.MessageID = strconv.Itoa(idx + 1)
		}
		if item.Role == "" {
			return nil, errors.New("message role is required")
		}
		if err := item.ToolArgs.Marshal(message.ToolArgs); err != nil {
			return nil, err
		}
		if err := item.InputRequest.Marshal(message.InputRequest); err != nil {
			return nil, err
		}
		if err := item.PendingAction.Marshal(message.PendingAction); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func buildChatHistoryMessages(messages []model.AIChatMessage) ([]chatHistoryMessage, error) {
	items := make([]chatHistoryMessage, 0, len(messages))
	for _, message := range messages {
		item := chatHistoryMessage{
			ID:           message.MessageID,
			Role:         message.Role,
			Content:      message.Content,
			Thinking:     message.Thinking,
			ToolCallID:   message.ToolCallID,
			ToolName:     message.ToolName,
			ToolResult:   message.ToolResult,
			ActionStatus: message.ActionStatus,
		}
		if data, err := unmarshalJSONFieldMap(message.ToolArgs); err != nil {
			return nil, err
		} else {
			item.ToolArgs = data
		}
		if data, err := unmarshalJSONFieldMap(message.InputRequest); err != nil {
			return nil, err
		} else {
			item.InputRequest = data
		}
		if data, err := unmarshalJSONFieldMap(message.PendingAction); err != nil {
			return nil, err
		} else {
			item.PendingAction = data
		}
		items = append(items, item)
	}
	return items, nil
}

func unmarshalJSONFieldMap(field model.JSONField) (map[string]interface{}, error) {
	if len(field) == 0 {
		return nil, nil
	}
	result := make(map[string]interface{})
	if err := field.Unmarshal(&result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
