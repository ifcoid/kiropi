package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifcoid/kiropi/internal/api"
	"github.com/ifcoid/kiropi/internal/config"
	mcpbridge "github.com/ifcoid/kiropi/internal/mcp"
)

func main() {
	fmt.Println(`
╦╔═╦╦═╗╔═╗╔═╗╦
╠╩╗║╠╦╝║ ║╠═╝║
╩ ╩╩╩╚═╚═╝╩  ╩
MCP Bridge + OpenAI-Compatible REST API
	`)

	// Load configuration
	cfg := config.Load()

	// Create MCP bridge
	bridge := mcpbridge.NewBridge(cfg)

	// Connect to MCP server if command is configured
	if cfg.MCPServerCommand != "" {
		log.Println("[MCP] Connecting to MCP server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := bridge.Connect(ctx); err != nil {
			log.Printf("[MCP] Warning: Failed to connect to MCP server: %v", err)
			log.Println("[MCP] Server will start in degraded mode (REST API only)")
		} else {
			log.Println("[MCP] Successfully connected to MCP server")
		}
		cancel()
	} else {
		log.Println("[MCP] No MCP server command configured. Running in REST-only mode.")
		log.Println("[MCP] Set KIROPI_MCP_COMMAND to enable MCP bridge.")
	}

	// Setup REST API router
	router := api.SetupRouter(bridge, cfg)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("[API] Starting server on port %s", cfg.ServerPort)
		log.Printf("[API] Endpoints:")
		log.Printf("[API]   POST /v1/chat/completions  - OpenAI-compatible chat")
		log.Printf("[API]   POST /api/ask              - Alias for chat completions")
		log.Printf("[API]   GET  /v1/models            - List available models")
		log.Printf("[API]   GET  /v1/tools             - List MCP tools")
		log.Printf("[API]   GET  /health               - Health check")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[API] Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Server] Shutting down...")

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[API] Server shutdown error: %v", err)
	}

	// Close MCP bridge
	if err := bridge.Close(); err != nil {
		log.Printf("[MCP] Bridge close error: %v", err)
	}

	log.Println("[Server] Stopped.")
}
