package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ifcoid/kiropi/internal/config"
	"github.com/ifcoid/kiropi/pkg/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server that Kiro connects to
type Server struct {
	cfg       *config.Config
	mcpServer *server.MCPServer
	sseServer *server.SSEServer
	queue     *models.PromptQueue
}

// NewServer creates a new MCP Server instance
func NewServer(cfg *config.Config, queue *models.PromptQueue) *Server {
	return &Server{
		cfg:   cfg,
		queue: queue,
	}
}

// Setup initializes the MCP server with tools
func (s *Server) Setup() {
	// Create MCP server
	s.mcpServer = server.NewMCPServer(
		"kiropi",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithInstructions("Kiropi MCP Server - Bridge between REST API and Kiro AI. Use get_pending_prompt to fetch prompts from the queue, then submit_response to send back your answer."),
	)

	// Register tools
	s.registerTools()

	// Create SSE server for remote connections (Kiro via cloudflared)
	s.sseServer = server.NewSSEServer(s.mcpServer,
		server.WithBaseURL(fmt.Sprintf("http://localhost:%s", s.cfg.MCPPort)),
	)
}

// registerTools registers all MCP tools that Kiro can call
func (s *Server) registerTools() {
	// Tool: get_pending_prompt
	// Kiro calls this to pick up the next prompt from the queue
	getPendingTool := mcp.NewTool("get_pending_prompt",
		mcp.WithDescription("Get the next pending prompt from the queue. Returns the prompt messages that need a response. Returns empty if no prompts are waiting."),
	)

	s.mcpServer.AddTool(getPendingTool, s.handleGetPendingPrompt)

	// Tool: submit_response
	// Kiro calls this to submit its response for a prompt
	submitResponseTool := mcp.NewTool("submit_response",
		mcp.WithDescription("Submit a response for a pending prompt. The prompt_id must match a prompt retrieved via get_pending_prompt."),
		mcp.WithString("prompt_id",
			mcp.Required(),
			mcp.Description("The ID of the prompt to respond to"),
		),
		mcp.WithString("response",
			mcp.Required(),
			mcp.Description("The AI response text to send back to the caller"),
		),
	)

	s.mcpServer.AddTool(submitResponseTool, s.handleSubmitResponse)

	// Tool: queue_status
	// Kiro can check how many prompts are waiting
	queueStatusTool := mcp.NewTool("queue_status",
		mcp.WithDescription("Check the current status of the prompt queue. Returns pending count and total count."),
	)

	s.mcpServer.AddTool(queueStatusTool, s.handleQueueStatus)
}

// handleGetPendingPrompt handles the get_pending_prompt tool call from Kiro
func (s *Server) handleGetPendingPrompt(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := s.queue.Dequeue()
	if prompt == nil {
		return mcp.NewToolResultText(`{"status": "empty", "message": "No pending prompts in queue"}`), nil
	}

	// Build a JSON response with prompt details
	type promptResponse struct {
		Status   string               `json:"status"`
		PromptID string               `json:"prompt_id"`
		Model    string               `json:"model,omitempty"`
		Messages []models.ChatMessage `json:"messages"`
	}

	resp := promptResponse{
		Status:   "pending",
		PromptID: prompt.ID,
		Model:    prompt.Model,
		Messages: prompt.Messages,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal prompt: %v", err)), nil
	}

	log.Printf("[MCP] Prompt %s picked up by Kiro", prompt.ID)
	return mcp.NewToolResultText(string(data)), nil
}

// handleSubmitResponse handles the submit_response tool call from Kiro
func (s *Server) handleSubmitResponse(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	promptID, ok := request.Params.Arguments["prompt_id"].(string)
	if !ok || promptID == "" {
		return mcp.NewToolResultError("prompt_id is required"), nil
	}

	response, ok := request.Params.Arguments["response"].(string)
	if !ok || response == "" {
		return mcp.NewToolResultError("response is required"), nil
	}

	success := s.queue.SubmitResponse(promptID, response)
	if !success {
		return mcp.NewToolResultError(fmt.Sprintf("prompt %s not found or already completed", promptID)), nil
	}

	log.Printf("[MCP] Response submitted for prompt %s (%d chars)", promptID, len(response))
	return mcp.NewToolResultText(fmt.Sprintf(`{"status": "submitted", "prompt_id": "%s"}`, promptID)), nil
}

// handleQueueStatus handles the queue_status tool call
func (s *Server) handleQueueStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	status := fmt.Sprintf(`{"pending": %d, "total": %d}`, s.queue.PendingCount(), s.queue.TotalCount())
	return mcp.NewToolResultText(status), nil
}

// Start starts the MCP SSE server
func (s *Server) Start() error {
	addr := ":" + s.cfg.MCPPort
	log.Printf("[MCP] SSE Server starting on %s", addr)
	log.Printf("[MCP] SSE endpoint: /sse")
	log.Printf("[MCP] Message endpoint: /message")
	return s.sseServer.Start(addr)
}

// Shutdown gracefully stops the MCP server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.sseServer != nil {
		return s.sseServer.Shutdown(ctx)
	}
	return nil
}
