package handlers

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/middleware"
	"forgedinframes-backend-go/internal/models"
)

type NotificationsHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewNotificationsHandler(cfg *config.Config, fbClient *fb.Client) *NotificationsHandler {
	return &NotificationsHandler{
		cfg:      cfg,
		fbClient: fbClient,
	}
}

// CreateAdminNotification pushes a new admin notification to Firebase RTDB (best-effort).
func CreateAdminNotification(ctx context.Context, fbClient *fb.Client, notif models.AdminNotification) {
	if fbClient == nil || fbClient.DBClient == nil {
		return
	}

	if notif.CreatedAt == "" {
		notif.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	notif.Read = false

	ref := fbClient.DBClient.NewRef("adminNotifications")
	pushRef, err := ref.Push(ctx, notif)
	if err != nil {
		log.Printf("Warning: failed to create admin notification: %v", err)
		return
	}
	if pushRef != nil && pushRef.Key != "" {
		_ = pushRef.Update(ctx, map[string]interface{}{"id": pushRef.Key})
	}
}

func (h *NotificationsHandler) isAdminUser(ctx context.Context, user models.AuthUser) bool {
	if h.cfg.IsAdminEmail(user.Email) {
		return true
	}
	if user.AccessList.EditorAccess || user.AccessList.PhotosEditAccess || user.AccessList.BlogsEditAccess {
		return true
	}
	if h.fbClient != nil && h.fbClient.DBClient != nil {
		var dbUser models.User
		if err := h.fbClient.DBClient.NewRef("users/"+user.UID).Get(ctx, &dbUser); err == nil {
			if dbUser.AccessList.EditorAccess || dbUser.AccessList.PhotosEditAccess || dbUser.AccessList.BlogsEditAccess {
				return true
			}
		}
	}
	return false
}

// ListNotifications handles GET /api/notifications
func (h *NotificationsHandler) ListNotifications(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	if !h.isAdminUser(c.Request.Context(), user) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden", "message": "Admin privileges required."})
		return
	}

	if h.fbClient == nil || h.fbClient.DBClient == nil {
		c.JSON(http.StatusOK, models.NotificationsResponse{
			Notifications: make([]models.AdminNotification, 0),
			UnreadCount:   0,
		})
		return
	}

	var rawMap map[string]models.AdminNotification
	if err := h.fbClient.DBClient.NewRef("adminNotifications").Get(c.Request.Context(), &rawMap); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications: " + err.Error()})
		return
	}

	notifications := make([]models.AdminNotification, 0, len(rawMap))
	unreadCount := 0
	for key, item := range rawMap {
		if item.ID == "" {
			item.ID = key
		}
		if !item.Read {
			unreadCount++
		}
		notifications = append(notifications, item)
	}

	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt > notifications[j].CreatedAt
	})

	if len(notifications) > 50 {
		notifications = notifications[:50]
	}

	c.JSON(http.StatusOK, models.NotificationsResponse{
		Notifications: notifications,
		UnreadCount:   unreadCount,
	})
}

// MarkAsRead handles PATCH /api/notifications/:id/read
func (h *NotificationsHandler) MarkAsRead(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}
	if !h.isAdminUser(c.Request.Context(), user) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden", "message": "Admin privileges required."})
		return
	}

	notifID := strings.TrimSpace(c.Param("id"))
	if !models.IsSafeKey(notifID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	if h.fbClient != nil && h.fbClient.DBClient != nil {
		if err := h.fbClient.DBClient.NewRef("adminNotifications/"+notifID).Update(c.Request.Context(), map[string]interface{}{
			"read": true,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notification as read: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"id": notifID, "read": true})
}

// MarkAllAsRead handles PATCH /api/notifications/read-all
func (h *NotificationsHandler) MarkAllAsRead(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}
	if !h.isAdminUser(c.Request.Context(), user) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden", "message": "Admin privileges required."})
		return
	}

	if h.fbClient != nil && h.fbClient.DBClient != nil {
		ctx := c.Request.Context()
		var rawMap map[string]models.AdminNotification
		if err := h.fbClient.DBClient.NewRef("adminNotifications").Get(ctx, &rawMap); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications: " + err.Error()})
			return
		}

		updates := make(map[string]interface{})
		for key, item := range rawMap {
			if !item.Read {
				updates[key+"/read"] = true
			}
		}
		if len(updates) > 0 {
			if err := h.fbClient.DBClient.NewRef("adminNotifications").Update(ctx, updates); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notifications as read: " + err.Error()})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteNotification handles DELETE /api/notifications/:id
func (h *NotificationsHandler) DeleteNotification(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}
	if !h.isAdminUser(c.Request.Context(), user) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden", "message": "Admin privileges required."})
		return
	}

	notifID := strings.TrimSpace(c.Param("id"))
	if !models.IsSafeKey(notifID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	if h.fbClient != nil && h.fbClient.DBClient != nil {
		if err := h.fbClient.DBClient.NewRef("adminNotifications/"+notifID).Delete(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notification: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"id": notifID, "deleted": true})
}
