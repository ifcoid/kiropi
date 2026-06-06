package api

import (
	"encoding/json"
	"fmt"
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

	// Route to streaming or non-streaming handler
	if req.Stream {
		h.chatCompletionStream(c, &req)
		return
	}

	h.chatCompletionNonStream(c, &req)
}

// chatCompletionNonStream handles non-streaming chat completions
func (h *Handler) chatCompletionNonStream(c *gin.Context, req *models.ChatCompletionRequest) {
	// Process through MCP bridge
	response, err := h.bridge.ProcessChat(c.Request.Context(), req)
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

// chatCompletionStream handles SSE streaming chat completions
func (h *Handler) chatCompletionStream(c *gin.Context, req *models.ChatCompletionRequest) {
	completionID := "chatcmpl-" + uuid.New().String()[:8]
	modelName := req.Model
	if modelName == "" {
		modelName = h.cfg.DefaultModel
	}
	created := time.Now().Unix()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Get the http.Flusher
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "Streaming not supported",
				Type:    "server_error",
				Code:    "no_flusher",
			},
		})
		return
	}

	// Send initial chunk with role
	initialChunk := models.ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   modelName,
		Choices: []models.ChunkChoice{
			{
				Index: 0,
				Delta: models.ChunkDelta{
					Role: "assistant",
				},
				FinishReason: nil,
			},
		},
	}
	h.writeSSEChunk(c, flusher, &initialChunk)

	// Process through MCP bridge with streaming
	chunks := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunks)
		errChan <- h.bridge.ProcessChatStream(c.Request.Context(), req, chunks)
	}()

	// Stream chunks to client
	for chunk := range chunks {
		select {
		case <-c.Request.Context().Done():
			return
		default:
			chunkData := models.ChatCompletionChunk{
				ID:      completionID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   modelName,
				Choices: []models.ChunkChoice{
					{
						Index: 0,
						Delta: models.ChunkDelta{
							Content: chunk,
						},
						FinishReason: nil,
					},
				},
			}
			h.writeSSEChunk(c, flusher, &chunkData)
		}
	}

	// Check for errors
	if err := <-errChan; err != nil {
		// Send error as a chunk (best effort, client may have disconnected)
		errChunk := models.ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   modelName,
			Choices: []models.ChunkChoice{
				{
					Index: 0,
					Delta: models.ChunkDelta{
						Content: fmt.Sprintf("\n\n[Error: %s]", err.Error()),
					},
					FinishReason: nil,
				},
			},
		}
		h.writeSSEChunk(c, flusher, &errChunk)
	}

	// Send final chunk with finish_reason
	stopReason := "stop"
	finalChunk := models.ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   modelName,
		Choices: []models.ChunkChoice{
			{
				Index:        0,
				Delta:        models.ChunkDelta{},
				FinishReason: &stopReason,
			},
		},
	}
	h.writeSSEChunk(c, flusher, &finalChunk)

	// Send [DONE] marker
	fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()
}

// writeSSEChunk writes a single SSE event to the response
func (h *Handler) writeSSEChunk(c *gin.Context, flusher http.Flusher, chunk *models.ChatCompletionChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	flusher.Flush()
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
