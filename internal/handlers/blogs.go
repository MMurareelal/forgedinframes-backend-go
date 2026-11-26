package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/middleware"
	"forgedinframes-backend-go/internal/models"
)

type BlogsHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewBlogsHandler(cfg *config.Config, fbClient *fb.Client) *BlogsHandler {
	return &BlogsHandler{cfg: cfg, fbClient: fbClient}
}

// GET /api/blogs and /getblogs
func (h *BlogsHandler) GetAllBlogs(c *gin.Context) {
	if h.fbClient == nil || h.fbClient.DBClient == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	var blogs map[string]interface{}
	ref := h.fbClient.DBClient.NewRef("blogPosts")
	if err := ref.Get(c.Request.Context(), &blogs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch blogs: " + err.Error()})
		return
	}

	if blogs == nil {
		blogs = make(map[string]interface{})
	}
	c.JSON(http.StatusOK, blogs)
}

// GET /api/blogs/:blogpost
func (h *BlogsHandler) GetBlogByID(c *gin.Context) {
	blogID := c.Param("blogpost")
	if !models.IsSafeKey(blogID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	if h.fbClient == nil || h.fbClient.DBClient == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Blog post not found"})
		return
	}

	var blog map[string]interface{}
	ref := h.fbClient.DBClient.NewRef("blogPosts/" + blogID)
	if err := ref.Get(c.Request.Context(), &blog); err != nil || blog == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Blog post not found"})
		return
	}

	c.JSON(http.StatusOK, blog)
}

// POST /api/blogs and /addblog
func (h *BlogsHandler) AddBlog(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil || len(req) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Blog content is required"})
		return
	}

	// Validate title and content
	title, _ := req["title"].(string)
	content, _ := req["content"].(string)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title and content are required"})
		return
	}

	if models.ContainsScriptTag(title) || models.ContainsScriptTag(content) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content contains unsafe script tags"})
		return
	}

	ctx := c.Request.Context()
	user, hasUser := middleware.GetUserFromContext(c)
	if hasUser {
		req["authorId"] = user.UID
	}

	if _, exists := req["createdAt"]; !exists {
		req["createdAt"] = time.Now().UTC().Format(time.RFC3339)
	}

	blogRef := h.fbClient.DBClient.NewRef("blogPosts")
	newRef, err := blogRef.Push(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create blog: " + err.Error()})
		return
	}

	req["id"] = newRef.Key
	c.JSON(http.StatusCreated, req)
}

// PUT /api/blogs/:blogpost and /updateblog/:blogpost
func (h *BlogsHandler) UpdateBlog(c *gin.Context) {
	blogID := c.Param("blogpost")
	if !models.IsSafeKey(blogID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	blogRef := h.fbClient.DBClient.NewRef("blogPosts/" + blogID)

	var existing interface{}
	if err := blogRef.Get(ctx, &existing); err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Blog post not found"})
		return
	}

	req["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	if err := blogRef.Update(ctx, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update blog: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Success", "message": "Blog updated successfully"})
}

// DELETE /api/blogs/:blogpost and /deleteblog/:blogpost
func (h *BlogsHandler) DeleteBlog(c *gin.Context) {
	blogID := c.Param("blogpost")
	if !models.IsSafeKey(blogID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	ctx := c.Request.Context()
	blogRef := h.fbClient.DBClient.NewRef("blogPosts/" + blogID)

	var existing interface{}
	if err := blogRef.Get(ctx, &existing); err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Blog post not found"})
		return
	}

	if err := blogRef.Delete(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete blog: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Success", "message": "Blog post deleted successfully"})
}

// GET /api/blogs/:blogpost/comments and /getcomments/:blogpost
func (h *BlogsHandler) GetComments(c *gin.Context) {
	blogID := c.Param("blogpost")
	if !models.IsSafeKey(blogID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	var comments map[string]interface{}
	ref := h.fbClient.DBClient.NewRef(fmt.Sprintf("blogPosts/%s/comments", blogID))
	if err := ref.Get(c.Request.Context(), &comments); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments: " + err.Error()})
		return
	}

	if comments == nil {
		comments = make(map[string]interface{})
	}
	c.JSON(http.StatusOK, comments)
}

// POST /api/blogs/:blogpost/comments and /addcomment
func (h *BlogsHandler) AddComment(c *gin.Context) {
	blogID := c.Param("blogpost")
	if !models.IsSafeKey(blogID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	var req models.CommentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment request: " + err.Error()})
		return
	}

	content := req.GetText()
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comment text or content is required"})
		return
	}
	if models.ContainsScriptTag(content) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comment contains unsafe script tags"})
		return
	}

	newComment := models.Comment{
		Content:                content,
		Comment:                content,
		UserID:                 user.UID,
		PostedByUID:            user.UID,
		UserName:               req.UserName,
		PostedBy:               req.PostedBy,
		PostedByProfilePicture: req.PostedByProfilePicture,
		CreatedAt:              time.Now().UTC().Format(time.RFC3339),
	}

	ctx := c.Request.Context()
	commentsRef := h.fbClient.DBClient.NewRef(fmt.Sprintf("blogPosts/%s/comments", blogID))
	pushRef, err := commentsRef.Push(ctx, newComment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add comment: " + err.Error()})
		return
	}

	// Notify admins about new journal comment
	blogTitle := blogID
	var titleVal string
	if err := h.fbClient.DBClient.NewRef(fmt.Sprintf("blogPosts/%s/title", blogID)).Get(ctx, &titleVal); err == nil && titleVal != "" {
		blogTitle = titleVal
	}
	actorName := req.PostedBy
	if actorName == "" {
		actorName = req.UserName
	}
	if actorName == "" {
		actorName = user.Email
	}
	if actorName == "" {
		actorName = "A visitor"
	}
	snippet := content
	if len(snippet) > 120 {
		snippet = snippet[:117] + "..."
	}
	CreateAdminNotification(ctx, h.fbClient, models.AdminNotification{
		Type:        "blog_comment",
		Title:       "New comment on journal entry",
		Message:     snippet,
		ActorUID:    user.UID,
		ActorName:   actorName,
		ActorAvatar: req.PostedByProfilePicture,
		TargetID:    blogID,
		TargetTitle: blogTitle,
		TargetPath:  "/blogs/" + blogID,
		CreatedAt:   newComment.CreatedAt,
	})

	c.JSON(http.StatusCreated, gin.H{
		"id":                     pushRef.Key,
		"content":                newComment.Content,
		"comment":                newComment.Comment,
		"userId":                 newComment.UserID,
		"postedByUID":            newComment.PostedByUID,
		"userName":               newComment.UserName,
		"postedBy":               newComment.PostedBy,
		"postedByProfilePicture": newComment.PostedByProfilePicture,
		"createdAt":              newComment.CreatedAt,
	})
}

// DELETE /api/blogs/:blogpost/comments/:key and /deletecomment/:key
func (h *BlogsHandler) DeleteComment(c *gin.Context) {
	blogID := c.Param("blogpost")
	commentKey := c.Param("key")
	if !models.IsSafeKey(blogID) || !models.IsSafeKey(commentKey) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "Authentication required."})
		return
	}

	ctx := c.Request.Context()
	commentRef := h.fbClient.DBClient.NewRef(fmt.Sprintf("blogPosts/%s/comments/%s", blogID, commentKey))

	var comment models.Comment
	if err := commentRef.Get(ctx, &comment); err != nil || (comment.UserID == "" && comment.PostedByUID == "") {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}

	isOwner := h.cfg.IsAdminEmail(user.Email)
	isAuthor := comment.UserID == user.UID || comment.PostedByUID == user.UID

	if !isAuthor && !isOwner {
		var dbUser models.User
		if err := h.fbClient.DBClient.NewRef("users/"+user.UID).Get(ctx, &dbUser); err == nil {
			isOwner = dbUser.AccessList.BlogsEditAccess || dbUser.AccessList.EditorAccess
		}
	}

	if !isAuthor && !isOwner {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "You can only delete your own comments.",
		})
		return
	}

	if err := commentRef.Delete(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Success", "message": "Comment deleted successfully"})
}
