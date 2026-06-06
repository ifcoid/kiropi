package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifcoid/kiropi/internal/config"
	"github.com/ifcoid/kiropi/pkg/models"
)

// AuthMiddleware validates the API key if configured
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth if no API key is configured
		if cfg.APIKey == "" {
			c.Next()
			return
		}

		// Skip auth for health check
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		// Extract Bearer token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Message: "Missing Authorization header",
					Type:    "authentication_error",
					Code:    "missing_auth",
				},
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Message: "Invalid Authorization header format. Expected: Bearer <token>",
					Type:    "authentication_error",
					Code:    "invalid_auth_format",
				},
			})
			return
		}

		if token != cfg.APIKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Message: "Invalid API key",
					Type:    "authentication_error",
					Code:    "invalid_api_key",
				},
			})
			return
		}

		c.Next()
	}
}

// CORSMiddleware adds CORS headers
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
