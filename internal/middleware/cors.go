package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
)

// CORS handles Cross-Origin Resource Sharing matching Node Express settings
func CORS(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		isAllowed := false
		if origin == "" || cfg.NodeEnv != "production" {
			isAllowed = true
		} else {
			for _, allowed := range cfg.AllowedOrigins {
				if strings.EqualFold(allowed, origin) {
					isAllowed = true
					break
				}
			}
		}

		if isAllowed {
			if origin != "" {
				c.Header("Access-Control-Allow-Origin", origin)
			} else {
				c.Header("Access-Control-Allow-Origin", "*")
			}
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, UserId, X-Requested-With, Accept")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == http.MethodOptions {
			if isAllowed {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Not allowed by CORS"})
			}
			return
		}

		if !isAllowed && origin != "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Not allowed by CORS"})
			return
		}

		c.Next()
	}
}
