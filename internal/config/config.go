package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	// REST API server port
	ServerPort string

	// MCP Server configuration
	MCPServerCommand string   // Command to start MCP server (e.g., "npx", "node", "python")
	MCPServerArgs    []string // Arguments for the MCP server command

	// MCP Transport type: "stdio" or "sse"
	MCPTransport string

	// MCP SSE URL (if using SSE transport)
	MCPSSEURL string

	// API Key for authentication (optional)
	APIKey string

	// Default model name to report
	DefaultModel string

	// Max concurrent requests
	MaxConcurrent int
}

// Load reads configuration from environment variables with sensible defaults
func Load() *Config {
	maxConcurrent, _ := strconv.Atoi(getEnv("KIROPI_MAX_CONCURRENT", "10"))

	return &Config{
		ServerPort:       getEnv("KIROPI_PORT", "8080"),
		MCPServerCommand: getEnv("KIROPI_MCP_COMMAND", ""),
		MCPServerArgs:    parseArgs(getEnv("KIROPI_MCP_ARGS", "")),
		MCPTransport:     getEnv("KIROPI_MCP_TRANSPORT", "stdio"),
		MCPSSEURL:        getEnv("KIROPI_MCP_SSE_URL", ""),
		APIKey:           getEnv("KIROPI_API_KEY", ""),
		DefaultModel:     getEnv("KIROPI_MODEL", "kiropi-1"),
		MaxConcurrent:    maxConcurrent,
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func parseArgs(args string) []string {
	if args == "" {
		return []string{}
	}
	// Simple space-separated parsing
	result := []string{}
	current := ""
	inQuote := false
	for _, ch := range args {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current != "" {
				result = append(result, current)
				current = ""
			}
		default:
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
