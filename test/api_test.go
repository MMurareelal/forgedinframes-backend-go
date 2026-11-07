package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	"forgedinframes-backend-go/internal/handlers"
	"forgedinframes-backend-go/internal/middleware"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Port:                   "3002",
		NodeEnv:                "test",
		GinMode:                "test",
		JWTSecret:              "test-jwt-secret-key-12345",
		JWTExpiresIn:           24 * time.Hour,
		AllowedOrigins:         []string{"http://localhost:3000", "https://forgedinframes.com"},
		AdminEmails:            []string{"admin@forgedinframes.com"},
		RateLimitWindowSeconds: 60,
		RateLimitMaxRequests:   100,
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.SecurityHeaders(cfg))
	router.Use(middleware.CORS(cfg))

	healthH := handlers.NewHealthHandler(cfg, nil)
	authH := handlers.NewAuthHandler(cfg, nil)
	photosH := handlers.NewPhotosHandler(cfg, nil)
	blogsH := handlers.NewBlogsHandler(cfg, nil)

	router.GET("/health", healthH.Health)
	router.GET("/api/health", healthH.Health)

	api := router.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/logout", authH.Logout)
		}

		photos := api.Group("/photos")
		{
			photos.GET("/categories/routes", photosH.GetCategoryRoutes)
			photos.GET("/", photosH.GetPhotoContainer)
		}

		blogs := api.Group("/blogs")
		{
			blogs.GET("/", blogsH.GetAllBlogs)
		}
	}

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Unknown Endpoint",
			"message": "The requested API route does not exist.",
		})
	})

	return router
}

func TestHealthCheck(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", resp["status"])
	}

	if resp["environment"] != "test" {
		t.Errorf("Expected environment 'test', got '%v'", resp["environment"])
	}
}

func TestSecurityHeaders(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("Expected X-Content-Type-Options: nosniff")
	}

	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("Expected X-Frame-Options: DENY")
	}

	if w.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Errorf("Expected Cross-Origin-Resource-Policy: cross-origin")
	}
}

func TestCORSPreflight(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/api/photos", nil)
	req.Header.Set("Origin", "https://forgedinframes.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204 for preflight, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "https://forgedinframes.com" {
		t.Errorf("CORS origin header incorrect: %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func Test404UnknownEndpoint(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/non-existent-route", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "Unknown Endpoint" {
		t.Errorf("Expected Unknown Endpoint error, got %v", resp["error"])
	}
}

func TestLogout(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/logout", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["success"] != true {
		t.Errorf("Expected success: true")
	}
}
