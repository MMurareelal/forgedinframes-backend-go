package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
)

var startTime = time.Now()

type HealthHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewHealthHandler(cfg *config.Config, fbClient *fb.Client) *HealthHandler {
	return &HealthHandler{cfg: cfg, fbClient: fbClient}
}

func (h *HealthHandler) Health(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	credSource := "unknown"
	if h.fbClient != nil {
		credSource = h.fbClient.CredentialType
	}

	uptimeSeconds := int64(time.Since(startTime).Seconds())

	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"environment": h.cfg.NodeEnv,
		"firebase": gin.H{
			"projectId":        h.cfg.FirebaseProjectID,
			"databaseURL":      h.cfg.FirebaseDatabaseURL,
			"credentialSource": credSource,
		},
		"uptimeSeconds": uptimeSeconds,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
		"memory": gin.H{
			"allocMB":      m.Alloc / 1024 / 1024,
			"totalAllocMB": m.TotalAlloc / 1024 / 1024,
			"sysMB":        m.Sys / 1024 / 1024,
			"numGC":        m.NumGC,
		},
	})
}
