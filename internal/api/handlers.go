package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ifcoid/kiropi/internal/config"
	mcpbridge "github.com/ifcoid/kiropi/internal/mcp"
	"github.com/ifcoid/kiropi/pkg/models"
)

// Handler holds the API handler dependencies
type Handler struct {
	bridge *mcpbridge.Bridge
	cfg    *config.Config
}

// NewHandler creates a new API handler
func NewHandler(bridge *mcpbridge.Bridge, cfg *config.Config) *Handler {
	return &Handler{
		bridge: bridge,
		cfg:    cfg,
	}
}

// ChatCompletion handles POST /v1/chat/completions (OpenAI-compatible)
func (h *Handler) ChatCompletion(c *gin.Context) {
	var req models.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
				Code:    "invalid_body",
			},
		})
		return
	}

	// Validate messages
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "Messages array is required and must not be empty",
				Type:    "invalid_request_error",
				Code:    "missing_messages",
			},
		})
		return
	}

	// Check if bridge is ready
	if !h.bridge.IsReady() {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "MCP bridge is not connected. Service unavailable.",
				Type:    "server_error",
				Code:    "bridge_not_ready",
			},
		})
		return
	}

	// Process through MCP bridge
	response, err := h.bridge.ProcessChat(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "Failed to process request: " + err.Error(),
				Type:    "server_error",
				Code:    "mcp_error",
			},
		})
		return
	}

	// Build OpenAI-compatible response
	completionID := "chatcmpl-" + uuid.New().String()[:8]
	modelName := req.Model
	if modelName == "" {
		modelName = h.cfg.DefaultModel
	}

	c.JSON(http.StatusOK, models.ChatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []models.Choice{
			{
				Index: 0,
				Message: models.ChatMessage{
					Role:    "assistant",
					Content: response,
				},
				FinishReason: "stop",
			},
		},
		Usage: models.Usage{
			PromptTokens:     estimateTokens(req.Messages),
			CompletionTokens: estimateTokens([]models.ChatMessage{{Content: response}}),
			TotalTokens:      estimateTokens(req.Messages) + estimateTokens([]models.ChatMessage{{Content: response}}),
		},
	})
}

// ListModels handles GET /v1/models (OpenAI-compatible)
func (h *Handler) ListModels(c *gin.Context) {
	c.JSON(http.StatusOK, models.ModelList{
		Object: "list",
		Data: []models.ModelInfo{
			{
				ID:      h.cfg.DefaultModel,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "kiropi",
			},
		},
	})
}

// ListTools handles GET /v1/tools - returns available MCP tools
func (h *Handler) ListTools(c *gin.Context) {
	if !h.bridge.IsReady() {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "MCP bridge is not connected",
				Type:    "server_error",
				Code:    "bridge_not_ready",
			},
		})
		return
	}

	tools, err := h.bridge.ListTools(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "Failed to list tools: " + err.Error(),
				Type:    "server_error",
				Code:    "mcp_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": tools,
	})
}

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(c *gin.Context) {
	status := "ok"
	mcpStatus := "connected"

	if !h.bridge.IsReady() {
		status = "degraded"
		mcpStatus = "disconnected"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     status,
		"mcp_bridge": mcpStatus,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// estimateTokens provides a rough token count estimation (~4 chars per token)
func estimateTokens(messages []models.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4
	}
	if total == 0 {
		total = 1
	}
	return total
}
