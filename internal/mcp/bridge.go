package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/ifcoid/kiropi/internal/config"
	"github.com/ifcoid/kiropi/pkg/models"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Bridge connects REST API requests to MCP server (Kiro)
type Bridge struct {
	cfg    *config.Config
	client *mcpclient.Client
	mu     sync.RWMutex
	ready  bool
}

// NewBridge creates a new MCP Bridge
func NewBridge(cfg *config.Config) *Bridge {
	return &Bridge{
		cfg: cfg,
	}
}

// Connect establishes connection to the MCP server
func (b *Bridge) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cfg.MCPServerCommand == "" {
		return fmt.Errorf("MCP server command not configured (set KIROPI_MCP_COMMAND)")
	}

	// Create stdio client to connect to MCP server
	client, err := mcpclient.NewStdioMCPClient(
		b.cfg.MCPServerCommand,
		nil, // env
		b.cfg.MCPServerArgs...,
	)
	if err != nil {
		return fmt.Errorf("failed to create MCP client: %w", err)
	}

	b.client = client

	// Initialize the MCP connection
	initReq := mcp.InitializeRequest{}
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "kiropi-bridge",
		Version: "1.0.0",
	}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	_, err = b.client.Initialize(ctx, initReq)
	if err != nil {
		return fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	b.ready = true
	return nil
}

// IsReady returns whether the bridge is connected and ready
func (b *Bridge) IsReady() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ready
}

// ListTools returns available tools from the MCP server
func (b *Bridge) ListTools(ctx context.Context) ([]models.Tool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.ready {
		return nil, fmt.Errorf("MCP bridge not connected")
	}

	toolsReq := mcp.ListToolsRequest{}
	result, err := b.client.ListTools(ctx, toolsReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	tools := make([]models.Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		tools = append(tools, models.Tool{
			Type: "function",
			Function: models.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return tools, nil
}

// ProcessChat processes a chat completion request through the MCP server
// It converts the OpenAI-format messages into MCP tool calls
func (b *Bridge) ProcessChat(ctx context.Context, req *models.ChatCompletionRequest) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.ready {
		return "", fmt.Errorf("MCP bridge not connected")
	}

	// Extract the last user message as the prompt
	prompt := extractLastUserMessage(req.Messages)
	if prompt == "" {
		return "", fmt.Errorf("no user message found in request")
	}

	// Build conversation context from message history
	conversationContext := buildConversationContext(req.Messages)

	// Call the MCP server's tool - try "ask" first, then "chat"
	response, err := b.callMCPTool(ctx, "ask", map[string]interface{}{
		"prompt":  prompt,
		"context": conversationContext,
	})
	if err != nil {
		// Fallback: try calling with just the prompt as a generic tool
		response, err = b.callMCPTool(ctx, "chat", map[string]interface{}{
			"messages": req.Messages,
		})
		if err != nil {
			return "", fmt.Errorf("failed to process through MCP: %w", err)
		}
	}

	return response, nil
}

// callMCPTool calls a specific tool on the MCP server
func (b *Bridge) callMCPTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = toolName
	callReq.Params.Arguments = args

	result, err := b.client.CallTool(ctx, callReq)
	if err != nil {
		return "", err
	}

	// Extract text content from the result
	var responseText strings.Builder
	for _, content := range result.Content {
		if textContent, ok := mcp.AsTextContent(content); ok {
			responseText.WriteString(textContent.Text)
		} else {
			// Try to marshal as JSON for other content types
			data, _ := json.Marshal(content)
			responseText.WriteString(string(data))
		}
	}

	if result.IsError {
		return "", fmt.Errorf("MCP tool error: %s", responseText.String())
	}

	return responseText.String(), nil
}

// Close closes the MCP connection
func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil {
		err := b.client.Close()
		b.ready = false
		return err
	}
	return nil
}

// extractLastUserMessage gets the last user message from the conversation
func extractLastUserMessage(messages []models.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// buildConversationContext builds a string context from message history
func buildConversationContext(messages []models.ChatMessage) string {
	var ctx strings.Builder
	for _, msg := range messages {
		ctx.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	return ctx.String()
}
