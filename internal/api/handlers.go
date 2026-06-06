package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ifcoid/kiropi/internal/config"
	"github.com/ifcoid/kiropi/pkg/models"
)

// Handler holds the API handler dependencies
type Handler struct {
	queue *models.PromptQueue
	cfg   *config.Config
}

// NewHandler creates a new API handler
func NewHandler(queue *models.PromptQueue, cfg *config.Config) *Handler {
	return &Handler{
		queue: queue,
		cfg:   cfg,
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

	// Route to streaming or non-streaming handler
	if req.Stream {
		h.chatCompletionStream(c, &req)
		return
	}

	h.chatCompletionNonStream(c, &req)
}

// chatCompletionNonStream handles non-streaming chat completions
func (h *Handler) chatCompletionNonStream(c *gin.Context, req *models.ChatCompletionRequest) {
	// Generate prompt ID and enqueue
	promptID := uuid.New().String()
	modelName := req.Model
	if modelName == "" {
		modelName = h.cfg.DefaultModel
	}

	prompt := h.queue.Enqueue(promptID, req.Messages, modelName)
	defer h.queue.Remove(promptID)

	// Wait for Kiro to respond (with timeout)
	select {
	case <-prompt.Done:
		// Kiro has responded
	case <-time.After(h.queue.Timeout()):
		c.JSON(http.StatusGatewayTimeout, models.ErrorResponse{
			Error: models.ErrorDetail{
				Message: "Timeout waiting for AI response. Kiro may not be connected.",
				Type:    "server_error",
				Code:    "timeout",
			},
		})
		return
	case <-c.Request.Context().Done():
		// Client disconnected
		return
	}

	// Build OpenAI-compatible response
	completionID := "chatcmpl-" + promptID[:8]

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
					Content: prompt.Response,
				},
				FinishReason: "stop",
			},
		},
		Usage: models.Usage{
			PromptTokens:     estimateTokens(req.Messages),
			CompletionTokens: estimateTokens([]models.ChatMessage{{Content: prompt.Response}}),
			TotalTokens:      estimateTokens(req.Messages) + estimateTokens([]models.ChatMessage{{Content: prompt.Response}}),
		},
	})
}

// chatCompletionStream handles SSE streaming chat completions
func (h *Handler) chatCompletionStream(c *gin.Context, req *models.ChatCompletionRequest) {
	// Generate prompt ID and enqueue
	promptID := uuid.New().String()
	modelName := req.Model
	if modelName == "" {
		modelName = h.cfg.DefaultModel
	}

	prompt := h.queue.Enqueue(promptID, req.Messages, modelName)
	defer h.queue.Remove(promptID)

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

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

	completionID := "chatcmpl-" + promptID[:8]
	created := time.Now().Unix()

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
	writeSSEChunk(c, flusher, &initialChunk)

	// Wait for Kiro to respond (with timeout)
	select {
	case <-prompt.Done:
		// Kiro has responded — stream in chunks
	case <-time.After(h.queue.Timeout()):
		// Timeout — send error chunk
		errChunk := models.ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   modelName,
			Choices: []models.ChunkChoice{
				{
					Index: 0,
					Delta: models.ChunkDelta{
						Content: "[Error: Timeout waiting for AI response. Kiro may not be connected.]",
					},
					FinishReason: nil,
				},
			},
		}
		writeSSEChunk(c, flusher, &errChunk)
		stopReason := "stop"
		finalChunk := models.ChatCompletionChunk{
			ID: completionID, Object: "chat.completion.chunk", Created: created, Model: modelName,
			Choices: []models.ChunkChoice{{Index: 0, Delta: models.ChunkDelta{}, FinishReason: &stopReason}},
		}
		writeSSEChunk(c, flusher, &finalChunk)
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		flusher.Flush()
		return
	case <-c.Request.Context().Done():
		return
	}

	// Stream the response in chunks
	response := prompt.Response
	chunkSize := 20 // characters per chunk
	runes := []rune(response)

	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		contentChunk := models.ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   modelName,
			Choices: []models.ChunkChoice{
				{
					Index: 0,
					Delta: models.ChunkDelta{
						Content: string(runes[i:end]),
					},
					FinishReason: nil,
				},
			},
		}
		writeSSEChunk(c, flusher, &contentChunk)
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
	writeSSEChunk(c, flusher, &finalChunk)

	// Send [DONE] marker
	fmt.Fprint(c.Writer, "data: [DONE]\n\n")
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

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"queue_pending": h.queue.PendingCount(),
		"queue_total":   h.queue.TotalCount(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

// writeSSEChunk writes a single SSE event to the response
func writeSSEChunk(c *gin.Context, flusher http.Flusher, chunk *models.ChatCompletionChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	flusher.Flush()
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
