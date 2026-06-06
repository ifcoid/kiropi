package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	// REST API server port (for NSA/apps to call)
	ServerPort string

	// MCP Server port (for Kiro to connect via SSE)
	MCPPort string

	// API Key for REST endpoint authentication (optional)
	APIKey string

	// Default model name to report
	DefaultModel string

	// Max concurrent requests
	MaxConcurrent int

	// Prompt timeout - how long to wait for Kiro to respond
	PromptTimeout time.Duration
}

// Load reads configuration from environment variables with sensible defaults
func Load() *Config {
	maxConcurrent, _ := strconv.Atoi(getEnv("KIROPI_MAX_CONCURRENT", "10"))
	timeoutSec, _ := strconv.Atoi(getEnv("KIROPI_PROMPT_TIMEOUT", "120"))

	return &Config{
		ServerPort:    getEnv("KIROPI_PORT", "50403"),
		MCPPort:       getEnv("KIROPI_MCP_PORT", "50404"),
		APIKey:        getEnv("KIROPI_API_KEY", ""),
		DefaultModel:  getEnv("KIROPI_MODEL", "kiropi-1"),
		MaxConcurrent: maxConcurrent,
		PromptTimeout: time.Duration(timeoutSec) * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
