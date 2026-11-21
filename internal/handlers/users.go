package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"firebase.google.com/go/v4/db"
	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/middleware"
	"forgedinframes-backend-go/internal/models"
)

type UsersHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewUsersHandler(cfg *config.Config, fbClient *fb.Client) *UsersHandler {
	return &UsersHandler{cfg: cfg, fbClient: fbClient}
}

// SyncUser handles POST /api/users/:uid (Get or create/sync user info)
func (h *UsersHandler) SyncUser(c *gin.Context) {
	uid := c.Param("uid")
	if !models.IsSafeKey(uid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	isOwner := h.cfg.IsAdminEmail(user.Email)
	if user.UID != uid && !isOwner {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "You can only access your own user profile.",
		})
		return
	}

	var req models.UserSyncRequest
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	userRef := h.fbClient.DBClient.NewRef("users/" + uid)

	var finalUser models.User
	fn := func(node db.TransactionNode) (interface{}, error) {
		var current models.User
		_ = node.Unmarshal(&current)

		email := strings.ToLower(strings.TrimSpace(user.Email))
		if email == "" && req.Email != "" {
			email = strings.ToLower(strings.TrimSpace(req.Email))
		}
		if email == "" && current.Email != "" {
			email = strings.ToLower(strings.TrimSpace(current.Email))
		}

		userIsOwner := h.cfg.IsAdminEmail(email)

		name := req.Name
		if name == "" {
			name = user.DisplayName
		}
		if name == "" {
			name = current.Name
		}

		profilePic := req.ProfilePicture
		if profilePic == "" {
			profilePic = user.PhotoURL
		}
		if profilePic == "" {
			profilePic = current.ProfilePicture
		}

		if current.UID == "" {
			// Brand new user
			current = models.User{
				UID:            uid,
				Name:           name,
				Email:          email,
				ProfilePicture: profilePic,
				AccessList: models.AccessList{
					PhotosEditAccess: userIsOwner,
					BlogsEditAccess:  userIsOwner,
					EditorAccess:     userIsOwner,
					UploadEditAccess: userIsOwner,
				},
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
				LastLoginAt: time.Now().UTC().Format(time.RFC3339),
			}
		} else {
			// Existing user
			current.Name = name
			current.Email = email
			current.ProfilePicture = profilePic
			current.LastLoginAt = time.Now().UTC().Format(time.RFC3339)

			if userIsOwner {
				current.AccessList.PhotosEditAccess = true
				current.AccessList.BlogsEditAccess = true
				current.AccessList.EditorAccess = true
				current.AccessList.UploadEditAccess = true
			}
		}

		finalUser = current
		return current, nil
	}

	if err := userRef.Transaction(ctx, fn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync user: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, finalUser)
}

// GetUser handles GET /api/users/:uid
func (h *UsersHandler) GetUser(c *gin.Context) {
	uid := c.Param("uid")
	if !models.IsSafeKey(uid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	isOwner := h.cfg.IsAdminEmail(user.Email)
	if user.UID != uid && !isOwner {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "You can only view your own user profile.",
		})
		return
	}

	var dbUser models.User
	ctx := c.Request.Context()
	userRef := h.fbClient.DBClient.NewRef("users/" + uid)
	if err := userRef.Get(ctx, &dbUser); err != nil || dbUser.UID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "User profile not found."})
		return
	}

	c.JSON(http.StatusOK, dbUser)
}

// ListUsers handles GET /api/users (Admin-only: list all registered users)
func (h *UsersHandler) ListUsers(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	if !h.cfg.IsAdminEmail(user.Email) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "Only curators with administrative privileges can view the user directory.",
		})
		return
	}

	ctx := c.Request.Context()
	usersRef := h.fbClient.DBClient.NewRef("users")
	var usersMap map[string]models.User
	if err := usersRef.Get(ctx, &usersMap); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users: " + err.Error()})
		return
	}

	usersList := make([]models.User, 0, len(usersMap))
	for _, u := range usersMap {
		if u.UID != "" {
			if u.ProfilePicture != "" && u.PhotoURL == "" {
				u.PhotoURL = u.ProfilePicture
			} else if u.PhotoURL != "" && u.ProfilePicture == "" {
				u.ProfilePicture = u.PhotoURL
			}
			usersList = append(usersList, u)
		}
	}

	// Sort newest first by LastLoginAt / CreatedAt
	sort.Slice(usersList, func(i, j int) bool {
		if usersList[i].LastLoginAt != usersList[j].LastLoginAt {
			return usersList[i].LastLoginAt > usersList[j].LastLoginAt
		}
		return usersList[i].CreatedAt > usersList[j].CreatedAt
	})

	c.JSON(http.StatusOK, usersList)
}

// UpdateUserAccess handles PATCH /api/users/:uid/access (Admin-only: update user permissions)
func (h *UsersHandler) UpdateUserAccess(c *gin.Context) {
	uid := c.Param("uid")
	if !models.IsSafeKey(uid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	adminUser, ok := middleware.GetUserFromContext(c)
	if !ok || adminUser.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	if !h.cfg.IsAdminEmail(adminUser.Email) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "Only curators with administrative privileges can modify user permissions.",
		})
		return
	}

	var req models.AccessList
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Body", "message": "Valid AccessList payload required."})
		return
	}

	ctx := c.Request.Context()
	userRef := h.fbClient.DBClient.NewRef("users/" + uid)

	var updatedUser models.User
	fn := func(node db.TransactionNode) (interface{}, error) {
		var current models.User
		if err := node.Unmarshal(&current); err != nil || current.UID == "" {
			return nil, fmt.Errorf("user not found")
		}

		// Prevent revoking permissions of primary owner
		if h.cfg.IsAdminEmail(current.Email) {
			req.PhotosEditAccess = true
			req.BlogsEditAccess = true
			req.EditorAccess = true
			req.UploadEditAccess = true
		}

		current.AccessList = req
		updatedUser = current
		return current, nil
	}

	if err := userRef.Transaction(ctx, fn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user access: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedUser)
}
