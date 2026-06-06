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
	mcpserver "github.com/ifcoid/kiropi/internal/mcp"
	"github.com/ifcoid/kiropi/pkg/models"
)

func main() {
	fmt.Println(`
╦╔═╦╦═╗╔═╗╔═╗╦
╠╩╗║╠╦╝║ ║╠═╝║
╩ ╩╩╩╚═╚═╝╩  ╩
MCP Server + OpenAI-Compatible REST API
	`)

	// Load configuration
	cfg := config.Load()

	// Create shared prompt queue
	queue := models.NewPromptQueue(cfg.PromptTimeout)

	// Create and setup MCP server
	mcpSrv := mcpserver.NewServer(cfg, queue)
	mcpSrv.Setup()

	// Setup REST API router
	router := api.SetupRouter(queue, cfg)

	// Create HTTP server for REST API
	restServer := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.PromptTimeout + 10*time.Second, // Longer than prompt timeout
		IdleTimeout:  60 * time.Second,
	}

	// Start MCP SSE server in goroutine
	go func() {
		log.Printf("[MCP] Starting MCP SSE Server on port %s", cfg.MCPPort)
		log.Printf("[MCP] Kiro connects to: http://localhost:%s/sse", cfg.MCPPort)
		log.Printf("[MCP] Tools available: get_pending_prompt, submit_response, queue_status")
		if err := mcpSrv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[MCP] Failed to start MCP server: %v", err)
		}
	}()

	// Start REST API server in goroutine
	go func() {
		log.Printf("[API] Starting REST server on port %s", cfg.ServerPort)
		log.Printf("[API] Endpoints:")
		log.Printf("[API]   POST /v1/chat/completions  - OpenAI-compatible chat")
		log.Printf("[API]   POST /api/ask              - Alias for chat completions")
		log.Printf("[API]   GET  /v1/models            - List available models")
		log.Printf("[API]   GET  /health               - Health check")
		log.Printf("[API] Prompt timeout: %s", cfg.PromptTimeout)

		if err := restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[API] Failed to start REST server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Server] Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := restServer.Shutdown(ctx); err != nil {
		log.Printf("[API] REST server shutdown error: %v", err)
	}
	if err := mcpSrv.Shutdown(ctx); err != nil {
		log.Printf("[MCP] MCP server shutdown error: %v", err)
	}

	log.Println("[Server] Stopped.")
}
