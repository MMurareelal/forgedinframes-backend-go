package middleware

import (
	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
)

// SecurityHeaders applies security HTTP headers equivalent to Express Helmet
func SecurityHeaders(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME-sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		// Cross-Origin Resource Policy
		c.Header("Cross-Origin-Resource-Policy", "cross-origin")
		// Content Security Policy
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https: blob:; connect-src 'self' https:; object-src 'none';")

		if cfg.NodeEnv == "production" {
			// HTTP Strict Transport Security
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		c.Next()
	}
}
