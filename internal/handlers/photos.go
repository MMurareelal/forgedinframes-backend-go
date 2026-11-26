package handlers

import (
	"net/http"
	"strings"
	"time"

	"firebase.google.com/go/v4/db"
	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/middleware"
	"forgedinframes-backend-go/internal/models"
)

type PhotosHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewPhotosHandler(cfg *config.Config, fbClient *fb.Client) *PhotosHandler {
	return &PhotosHandler{cfg: cfg, fbClient: fbClient}
}

// GET /api/photos/categories/routes and /getcategoryRoutes
func (h *PhotosHandler) GetCategoryRoutes(c *gin.Context) {
	if h.fbClient == nil || h.fbClient.DBClient == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	var routes map[string]interface{}
	ref := h.fbClient.DBClient.NewRef("categoryRoutes")
	if err := ref.Get(c.Request.Context(), &routes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch category routes: " + err.Error()})
		return
	}

	if routes == nil {
		routes = make(map[string]interface{})
	}
	c.JSON(http.StatusOK, routes)
}

// GET /api/photos and /photocontainer
func (h *PhotosHandler) GetPhotoContainer(c *gin.Context) {
	if h.fbClient == nil || h.fbClient.DBClient == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	var photoContainer map[string]interface{}
	ref := h.fbClient.DBClient.NewRef("PhotoContainer")
	if err := ref.Get(c.Request.Context(), &photoContainer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch photo container: " + err.Error()})
		return
	}

	if photoContainer == nil {
		photoContainer = make(map[string]interface{})
	}

	filterType := c.Query("type")
	if filterType == "" {
		c.JSON(http.StatusOK, photoContainer)
		return
	}

	// Filter by type matching category
	filtered := make([]interface{}, 0)
	for _, val := range photoContainer {
		if catMap, ok := val.(map[string]interface{}); ok {
			if t, ok := catMap["type"].(string); ok && t == filterType {
				filtered = append(filtered, catMap)
			}
		}
	}
	c.JSON(http.StatusOK, filtered)
}

// GET /api/photos/photocontainer/:type
func (h *PhotosHandler) GetCategoryByType(c *gin.Context) {
	categoryType := c.Param("type")
	if !models.IsSafeKey(categoryType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	if h.fbClient == nil || h.fbClient.DBClient == nil {
		c.JSON(http.StatusOK, nil)
		return
	}

	var category interface{}
	ref := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryType)
	if err := ref.Get(c.Request.Context(), &category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch category: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, category)
}

func getCategoryKey(c *gin.Context) string {
	val := c.Param("type")
	if val == "" {
		val = c.Param("categoryName")
	}
	return val
}

// POST /api/photos/photocontainer/:categoryName (or :type)
func (h *PhotosHandler) CreateCategory(c *gin.Context) {
	categoryName := getCategoryKey(c)
	if !models.IsSafeKey(categoryName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	photoContainerRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryName)
	if err := photoContainerRef.Set(ctx, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save category: " + err.Error()})
		return
	}

	categoryRoutesRef := h.fbClient.DBClient.NewRef("categoryRoutes")
	newRoute := models.CategoryRoute{
		CategoryName:  categoryName,
		CategoryOrder: 0,
	}
	if _, err := categoryRoutesRef.Push(ctx, newRoute); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to push route: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "Success",
		"message": "Category and respective route have been added successfully.",
	})
}

// PUT /api/photos/photocontainer/updatecategory/:categoryName (or :type)
func (h *PhotosHandler) UpdateCategory(c *gin.Context) {
	categoryName := getCategoryKey(c)
	if !models.IsSafeKey(categoryName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	categoryRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryName)

	var existing interface{}
	if err := categoryRef.Get(ctx, &existing); err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	if err := categoryRef.Update(ctx, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update category: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "Success",
		"message": "Category updated successfully.",
	})
}

// PUT /api/photos/photocontainer/:categoryName/changevisibility/:visibility
func (h *PhotosHandler) ChangeCategoryVisibility(c *gin.Context) {
	categoryName := getCategoryKey(c)
	visibility := c.Param("visibility")
	if !models.IsSafeKey(categoryName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	ctx := c.Request.Context()
	categoryRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryName)

	var existing interface{}
	if err := categoryRef.Get(ctx, &existing); err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	isActive := visibility != "hide"
	if err := categoryRef.Update(ctx, map[string]interface{}{"isActive": isActive}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to change visibility: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "Success",
		"message": "Category visibility changed successfully.",
	})
}

// PATCH /api/photos/photocontainer/:type/updatephoto/:uid
func (h *PhotosHandler) UpdatePhoto(c *gin.Context) {
	categoryType := c.Param("type")
	photoUID := c.Param("uid")
	if !models.IsSafeKey(categoryType) || !models.IsSafeKey(photoUID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	imageRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryType + "/images/" + photoUID)

	var existing interface{}
	if err := imageRef.Get(ctx, &existing); err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Photo not found"})
		return
	}

	if err := imageRef.Update(ctx, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update photo: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, req)
}

// DELETE /api/photos/photocontainer/:type/deletephoto/:uid
func (h *PhotosHandler) DeletePhoto(c *gin.Context) {
	categoryType := c.Param("type")
	photoUID := c.Param("uid")
	if !models.IsSafeKey(categoryType) || !models.IsSafeKey(photoUID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	ctx := c.Request.Context()
	imageRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryType + "/images/" + photoUID)

	var existing interface{}
	if err := imageRef.Get(ctx, &existing); err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Photo not found"})
		return
	}

	if err := imageRef.Delete(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete photo: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Success", "message": "Photo deleted successfully."})
}

// POST /api/photos/photocontainer/:type/likephoto/:uid
func (h *PhotosHandler) LikePhoto(c *gin.Context) {
	categoryType := c.Param("type")
	photoUID := c.Param("uid")
	if !models.IsSafeKey(categoryType) || !models.IsSafeKey(photoUID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	user, ok := middleware.GetUserFromContext(c)
	if !ok || user.UID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "User not authenticated"})
		return
	}

	ctx := c.Request.Context()
	likesRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryType + "/images/" + photoUID + "/likes")

	var currentEvent string
	var finalLikes models.LikesData

	fn := func(node db.TransactionNode) (interface{}, error) {
		var likes models.LikesData
		_ = node.Unmarshal(&likes)

		existingIndex := -1
		for i, item := range likes.LikedBy {
			if item.UserID == user.UID {
				existingIndex = i
				break
			}
		}

		if existingIndex >= 0 {
			// User already liked -> unlike
			likes.LikedBy = append(likes.LikedBy[:existingIndex], likes.LikedBy[existingIndex+1:]...)
			if likes.TotalLikes > 0 {
				likes.TotalLikes--
			}
			currentEvent = "Disliked"
		} else {
			// New like
			likes.LikedBy = append(likes.LikedBy, models.LikedByUser{
				UserID:  user.UID,
				LikedAt: time.Now().UTC().Format(time.RFC3339),
			})
			likes.TotalLikes++
			currentEvent = "Liked"
		}

		finalLikes = likes
		return likes, nil
	}

	if err := likesRef.Transaction(ctx, fn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed: " + err.Error()})
		return
	}

	if currentEvent == "Liked" {
		photoTitle := photoUID
		var titleVal string
		if err := h.fbClient.DBClient.NewRef("PhotoContainer/"+categoryType+"/images/"+photoUID+"/title").Get(ctx, &titleVal); err == nil && titleVal != "" {
			photoTitle = titleVal
		}
		actorName := user.Email
		var dbUser models.User
		if err := h.fbClient.DBClient.NewRef("users/"+user.UID).Get(ctx, &dbUser); err == nil && dbUser.Name != "" {
			actorName = dbUser.Name
		}
		if actorName == "" {
			actorName = "A visitor"
		}
		avatar := dbUser.PhotoURL
		if avatar == "" {
			avatar = dbUser.ProfilePicture
		}
		CreateAdminNotification(ctx, h.fbClient, models.AdminNotification{
			Type:        "photo_like",
			Title:       "New like on photograph",
			Message:     actorName + " liked \"" + photoTitle + "\" in " + categoryType,
			ActorUID:    user.UID,
			ActorName:   actorName,
			ActorAvatar: avatar,
			TargetID:    photoUID,
			TargetTitle: photoTitle,
			TargetPath:  "/photos/" + strings.ToLower(categoryType),
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}

	finalLikes.CurrentEvent = currentEvent
	c.JSON(http.StatusOK, finalLikes)
}

// PATCH /api/photos/photocontainer/:type/makecategorythumbnail
func (h *PhotosHandler) MakeCategoryThumbnail(c *gin.Context) {
	categoryType := c.Param("type")
	if !models.IsSafeKey(categoryType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Parameter", "message": "Parameter contains invalid characters."})
		return
	}

	var req models.CategoryThumbnailRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ImgURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imgURL is required"})
		return
	}

	ctx := c.Request.Context()
	categoryRef := h.fbClient.DBClient.NewRef("PhotoContainer/" + categoryType)

	var category map[string]interface{}
	if err := categoryRef.Get(ctx, &category); err != nil || category == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	var thumbnails []string
	if rawThumbs, ok := category["thumbnails"].([]interface{}); ok {
		for _, t := range rawThumbs {
			if str, ok := t.(string); ok {
				thumbnails = append(thumbnails, str)
			}
		}
	}

	found := false
	for _, t := range thumbnails {
		if t == req.ImgURL {
			found = true
			break
		}
	}
	if !found {
		thumbnails = append(thumbnails, req.ImgURL)
	}

	updates := map[string]interface{}{
		"bgimg":      req.ImgURL,
		"thumbnails": thumbnails,
	}

	if err := categoryRef.Update(ctx, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update thumbnail: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"imgURL":     req.ImgURL,
		"thumbnails": thumbnails,
	})
}
