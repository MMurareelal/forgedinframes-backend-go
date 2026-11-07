package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/middleware"
	"forgedinframes-backend-go/internal/models"
)

var allowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9._-]`)
var dirAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

type StorageHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewStorageHandler(cfg *config.Config, fbClient *fb.Client) *StorageHandler {
	return &StorageHandler{cfg: cfg, fbClient: fbClient}
}

// sanitizeStoragePath prevents path traversal attacks
func sanitizeStoragePath(userPath, fileName string) string {
	if fileName == "" {
		return ""
	}
	baseFile := filepath.Base(fileName)
	safeFileName := nonAlphanumericRegex.ReplaceAllString(baseFile, "_")
	if userPath == "" {
		return safeFileName
	}

	segments := strings.Split(userPath, "/")
	var safeSegments []string
	for _, seg := range segments {
		baseSeg := filepath.Base(seg)
		cleanSeg := dirAlphanumericRegex.ReplaceAllString(baseSeg, "_")
		if cleanSeg != "" && cleanSeg != "." && cleanSeg != ".." {
			safeSegments = append(safeSegments, cleanSeg)
		}
	}

	if len(safeSegments) == 0 {
		return safeFileName
	}
	return strings.Join(safeSegments, "/") + "/" + safeFileName
}

// VerifyStorageAccess checks whether the user has rights to upload/delete photography or blog assets
func (h *StorageHandler) VerifyStorageAccess(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.GetUserFromContext(c)
		if !ok || user.UID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized. Authentication required."})
			return
		}

		if h.cfg.IsAdminEmail(user.Email) {
			c.Next()
			return
		}

		refPath := c.PostForm("refPath")
		if refPath == "" {
			refPath = c.Query("refPath")
		}
		fileName := c.Param("fileName")
		if fileName == "" {
			fileName = c.PostForm("fileName")
		}

		target := strings.ToLower(fmt.Sprintf("%s %s %s", refPath, fileName, c.Request.URL.Path))
		isBlog := strings.Contains(target, "blog")

		accessList := user.AccessList
		if h.fbClient != nil && h.fbClient.DBClient != nil {
			var dbUser models.User
			if err := h.fbClient.DBClient.NewRef("users/"+user.UID).Get(c.Request.Context(), &dbUser); err == nil && dbUser.UID != "" {
				accessList = dbUser.AccessList
			}
		}

		hasPermission := false
		if isBlog {
			hasPermission = accessList.EditorAccess || accessList.BlogsEditAccess || accessList.PhotosEditAccess || accessList.UploadEditAccess
		} else {
			hasPermission = accessList.PhotosEditAccess || accessList.UploadEditAccess || accessList.EditorAccess
		}

		if hasPermission {
			c.Next()
			return
		}

		assetType := "photography"
		if isBlog {
			assetType = "blog"
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": fmt.Sprintf("You do not have permission to %s %s assets.", action, assetType),
		})
	}
}

// UploadPhoto handles POST /api/storage, /api/upload, and /uploadphoto
func (h *StorageHandler) UploadPhoto(c *gin.Context) {
	if h.fbClient == nil || h.fbClient.BucketHandle == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage bucket handle not available."})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file uploaded!"})
		return
	}

	// Validate file size
	if fileHeader.Size > h.cfg.MaxFileSizeBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("File size exceeds limit of %d bytes", h.cfg.MaxFileSizeBytes)})
		return
	}

	// Validate MIME type
	contentType := fileHeader.Header.Get("Content-Type")
	if !allowedMIMETypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Only JPEG, PNG, and WebP images are allowed."})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file: " + err.Error()})
		return
	}
	defer file.Close()

	fileName := c.PostForm("fileName")
	if fileName == "" {
		fileName = fmt.Sprintf("%d_%s", time.Now().UnixMilli(), fileHeader.Filename)
	}

	refPath := sanitizeStoragePath(c.PostForm("refPath"), fileName)
	ctx := c.Request.Context()

	obj := h.fbClient.BucketHandle.Object(refPath)
	writer := obj.NewWriter(ctx)
	writer.ContentType = contentType

	if _, err := io.Copy(writer, file); err != nil {
		_ = writer.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file to storage: " + err.Error()})
		return
	}

	if err := writer.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize upload: " + err.Error()})
		return
	}

	// Public Firebase Storage URL format
	encodedPath := url.PathEscape(refPath)
	publicURL := fmt.Sprintf("https://firebasestorage.googleapis.com/v0/b/%s/o/%s?alt=media",
		h.cfg.FirebaseStorageBucket, encodedPath)

	c.JSON(http.StatusCreated, gin.H{
		"success":   true,
		"fileName":  fileName,
		"path":      refPath,
		"publicUrl": publicURL,
	})
}

// DeletePhoto handles DELETE /api/storage/files/:fileName and /deletephoto
func (h *StorageHandler) DeletePhoto(c *gin.Context) {
	if h.fbClient == nil || h.fbClient.BucketHandle == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage bucket handle not available."})
		return
	}

	fileName := c.Param("fileName")
	if fileName == "" {
		fileName = c.PostForm("fileName")
	}
	if fileName == "" {
		var body struct {
			FileName string `json:"fileName"`
			RefPath  string `json:"refPath"`
		}
		_ = c.ShouldBindJSON(&body)
		fileName = body.FileName
	}

	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileName is required."})
		return
	}

	refPath := c.PostForm("refPath")
	if refPath == "" {
		refPath = c.Query("refPath")
	}

	safePath := sanitizeStoragePath(refPath, fileName)
	ctx := c.Request.Context()
	obj := h.fbClient.BucketHandle.Object(safePath)

	// Check if object exists
	_, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"status":  "Success",
			"message": "File already deleted or does not exist.",
		})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check file existence: " + err.Error()})
		return
	}

	if err := obj.Delete(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  "Success",
		"message": "File deleted successfully.",
	})
}
