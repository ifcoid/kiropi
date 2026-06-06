package api

import (
	"github.com/gin-gonic/gin"
	"github.com/ifcoid/kiropi/internal/config"
	mcpbridge "github.com/ifcoid/kiropi/internal/mcp"
)

// SetupRouter configures and returns the Gin router
func SetupRouter(bridge *mcpbridge.Bridge, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(CORSMiddleware())
	router.Use(AuthMiddleware(cfg))

	// Handler
	handler := NewHandler(bridge, cfg)

	// Health check (no auth required - handled in middleware)
	router.GET("/health", handler.HealthCheck)

	// OpenAI-compatible endpoints
	v1 := router.Group("/v1")
	{
		v1.POST("/chat/completions", handler.ChatCompletion)
		v1.GET("/models", handler.ListModels)
		v1.GET("/tools", handler.ListTools)
	}

	// Convenience alias
	router.POST("/api/ask", handler.ChatCompletion)

	return router
}
