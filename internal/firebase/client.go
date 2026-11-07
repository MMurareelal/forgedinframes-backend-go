package firebase

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/db"
	firebaseStorage "firebase.google.com/go/v4/storage"
	"google.golang.org/api/option"

	"forgedinframes-backend-go/config"
)

type Client struct {
	App             *firebase.App
	AuthClient      *auth.Client
	DBClient        *db.Client
	StorageClient   *firebaseStorage.Client
	BucketHandle    *storage.BucketHandle
	CredentialType  string
	TargetProjectID string
}

func parseServiceAccountBytes(raw string) ([]byte, error) {
	str := strings.TrimSpace(raw)
	if (strings.HasPrefix(str, "'") && strings.HasSuffix(str, "'")) ||
		(strings.HasPrefix(str, "\"") && strings.HasSuffix(str, "\"")) {
		str = strings.TrimSpace(str[1 : len(str)-1])
	}

	if !strings.HasPrefix(str, "{") {
		decoded, err := base64.StdEncoding.DecodeString(str)
		if err == nil && strings.HasPrefix(strings.TrimSpace(string(decoded)), "{") {
			return decoded, nil
		}
	}
	return []byte(str), nil
}

// InitFirebase initializes Firebase App, Auth, Database, and Storage clients
func InitFirebase(ctx context.Context, cfg *config.Config) (*Client, error) {
	var opts []option.ClientOption
	credSource := "uninitialized"

	// 1. Check environment variable string
	if cfg.FirebaseServiceAccountKey != "" {
		bytes, err := parseServiceAccountBytes(cfg.FirebaseServiceAccountKey)
		if err == nil {
			opts = append(opts, option.WithCredentialsJSON(bytes))
			credSource = "FIREBASE_SERVICE_ACCOUNT_KEY (env)"
		} else {
			log.Printf("Warning: failed to parse FIREBASE_SERVICE_ACCOUNT_KEY: %v", err)
		}
	}

	// 2. Check path if not loaded from env string
	if len(opts) == 0 && cfg.FirebaseServiceAccountPath != "" {
		if _, err := os.Stat(cfg.FirebaseServiceAccountPath); err == nil {
			opts = append(opts, option.WithCredentialsFile(cfg.FirebaseServiceAccountPath))
			credSource = fmt.Sprintf("File (%s)", cfg.FirebaseServiceAccountPath)
		}
	}

	// 3. Auto-discover service account JSON file in working directory
	if len(opts) == 0 {
		files, _ := filepath.Glob("*-firebase-adminsdk-*.json")
		if len(files) == 0 {
			files, _ = filepath.Glob("*service-account*.json")
		}
		if len(files) > 0 {
			opts = append(opts, option.WithCredentialsFile(files[0]))
			credSource = fmt.Sprintf("Auto-discovered (%s)", files[0])
		}
	}

	fbConfig := &firebase.Config{
		DatabaseURL:   cfg.FirebaseDatabaseURL,
		StorageBucket: cfg.FirebaseStorageBucket,
		ProjectID:     cfg.FirebaseProjectID,
	}

	app, err := firebase.NewApp(ctx, fbConfig, opts...)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase auth: %w", err)
	}

	dbClient, err := app.Database(ctx)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase database: %w", err)
	}

	storageClient, err := app.Storage(ctx)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase storage: %w", err)
	}

	bucketHandle, err := storageClient.Bucket(cfg.FirebaseStorageBucket)
	if err != nil {
		log.Printf("Warning: could not get bucket handle for bucket '%s': %v", cfg.FirebaseStorageBucket, err)
	}

	log.Printf("Firebase initialized successfully using %s", credSource)

	return &Client{
		App:             app,
		AuthClient:      authClient,
		DBClient:        dbClient,
		StorageClient:   storageClient,
		BucketHandle:    bucketHandle,
		CredentialType:  credSource,
		TargetProjectID: cfg.FirebaseProjectID,
	}, nil
}
