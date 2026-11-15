package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                     string
	NodeEnv                  string
	GinMode                  string
	BackendURL               string
	FrontendURL              string
	FirebaseDatabaseURL      string
	FirebaseStorageBucket    string
	FirebaseProjectID        string
	FirebaseServiceAccountKey string
	FirebaseServiceAccountPath string
	GoogleClientID           string
	GoogleClientSecret       string
	JWTSecret                string
	JWTExpiresIn             time.Duration
	AllowedOrigins           []string
	AdminEmails              []string
	RateLimitWindowSeconds   int
	RateLimitMaxRequests     int
	UploadLimitMaxRequests   int
	MutationLimitMaxRequests int
	MaxFileSizeBytes         int64
	TrustProxy               bool
	GA4PropertyID            string
}

// LoadConfig loads environment variables from .env files and environment
func LoadConfig() *Config {
	env := os.Getenv("NODE_ENV")
	if env == "" {
		env = os.Getenv("ENV")
	}
	if env == "" {
		env = "development"
	}

	// Try loading specific environment file, then fallback to .env
	cwd, _ := os.Getwd()
	if env == "development" {
		_ = godotenv.Load(filepath.Join(cwd, ".env.dev"))
	} else if env == "production" {
		_ = godotenv.Load(filepath.Join(cwd, ".env.prod"))
	}
	_ = godotenv.Load(filepath.Join(cwd, ".env"))

	port := getEnv("PORT", "3002")
	ginMode := getEnv("GIN_MODE", "")
	if ginMode == "" {
		if env == "production" {
			ginMode = "release"
		} else {
			ginMode = "debug"
		}
	}

	backendURL := getEnv("BACKEND_URL", "http://localhost:3002")
	frontendURL := getEnv("FRONTEND_URL", "http://localhost:5173")
	if env == "production" {
		if os.Getenv("BACKEND_URL") == "" {
			backendURL = "https://forgedinframes-backend-1.onrender.com"
		}
		if os.Getenv("FRONTEND_URL") == "" {
			frontendURL = "https://forgedinframes.netlify.app"
		}
	}

	jwtExpiresInStr := getEnv("JWT_EXPIRES_IN", "168h")
	jwtDuration, err := time.ParseDuration(jwtExpiresInStr)
	if err != nil {
		jwtDuration = 7 * 24 * time.Hour
	}

	allowedOriginsStr := getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173,http://localhost:4200,https://forgedinframes.netlify.app,https://forgedinframes.com")
	allowedOrigins := splitAndTrim(allowedOriginsStr, ",")

	adminEmailsStr := getEnv("ADMIN_EMAILS", "murareelal.meghwal@gmail.com,murareelalm@gmail.com")
	adminEmails := splitAndTrim(strings.ToLower(adminEmailsStr), ",")

	maxFileSize, _ := strconv.ParseInt(getEnv("MAX_FILE_SIZE_BYTES", "5242880"), 10, 64)
	if maxFileSize <= 0 {
		maxFileSize = 5 * 1024 * 1024
	}

	rateLimitWindow, _ := strconv.Atoi(getEnv("RATE_LIMIT_WINDOW_SECONDS", "900"))
	rateLimitMax, _ := strconv.Atoi(getEnv("RATE_LIMIT_MAX_REQUESTS", "150"))
	uploadLimitMax, _ := strconv.Atoi(getEnv("UPLOAD_LIMIT_MAX_REQUESTS", "25"))
	mutationLimitMax, _ := strconv.Atoi(getEnv("MUTATION_LIMIT_MAX_REQUESTS", "60"))

	trustProxyStr := strings.ToLower(getEnv("TRUST_PROXY", "true"))
	trustProxy := trustProxyStr == "true" || trustProxyStr == "1"

	return &Config{
		Port:                     port,
		NodeEnv:                  env,
		GinMode:                  ginMode,
		BackendURL:               backendURL,
		FrontendURL:              frontendURL,
		FirebaseDatabaseURL:      getEnv("FIREBASE_DATABASE_URL", getEnv("DEVELOPMENT_FIREBASE_DATABASE_URL", "")),
		FirebaseStorageBucket:    getEnv("FIREBASE_STORAGE_BUCKET", getEnv("DEVELOPMENT_FIREBASE_STORAGE_BUCKET", "")),
		FirebaseProjectID:        getEnv("FIREBASE_PROJECT_ID", getEnv("DEVELOPMENT_FIREBASE_PROJECT_ID", "")),
		FirebaseServiceAccountKey: os.Getenv("FIREBASE_SERVICE_ACCOUNT_KEY"),
		FirebaseServiceAccountPath: os.Getenv("FIREBASE_SERVICE_ACCOUNT_PATH"),
		GoogleClientID:           os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:       os.Getenv("GOOGLE_CLIENT_SECRET"),
		JWTSecret:                getEnv("JWT_SECRET", "forgedinframes-jwt-secret-key-change-in-production"),
		JWTExpiresIn:             jwtDuration,
		AllowedOrigins:           allowedOrigins,
		AdminEmails:              adminEmails,
		RateLimitWindowSeconds:   rateLimitWindow,
		RateLimitMaxRequests:     rateLimitMax,
		UploadLimitMaxRequests:   uploadLimitMax,
		MutationLimitMaxRequests: mutationLimitMax,
		MaxFileSizeBytes:         maxFileSize,
		TrustProxy:               trustProxy,
		GA4PropertyID:            getEnv("GA4_PROPERTY_ID", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func (c *Config) IsAdminEmail(email string) bool {
	clean := strings.ToLower(strings.TrimSpace(email))
	for _, admin := range c.AdminEmails {
		if clean == admin {
			return true
		}
	}
	return false
}
