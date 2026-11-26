package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/handlers"
	"forgedinframes-backend-go/internal/middleware"
)

func main() {
	cfg := config.LoadConfig()

	gin.SetMode(cfg.GinMode)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fbClient, err := fb.InitFirebase(ctx, cfg)
	if err != nil {
		log.Printf("Warning: Firebase initialization returned error: %v", err)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.SecurityHeaders(cfg))
	router.Use(middleware.CORS(cfg))

	if cfg.TrustProxy {
		_ = router.SetTrustedProxies(nil)
	}

	// Rate Limiters
	rateWindow := time.Duration(cfg.RateLimitWindowSeconds) * time.Second
	globalLimiter := middleware.NewIPRateLimiter(cfg.RateLimitMaxRequests, rateWindow)
	mutationLimiter := middleware.NewIPRateLimiter(cfg.MutationLimitMaxRequests, rateWindow)
	uploadLimiter := middleware.NewIPRateLimiter(cfg.UploadLimitMaxRequests, rateWindow)
	authLimiter := middleware.NewIPRateLimiter(30, rateWindow)

	// Middlewares
	authMW := middleware.Authenticate(fbClient, cfg)
	optionalAuthMW := middleware.OptionalAuth(fbClient, cfg)
	requirePhotosMW := middleware.RequireResourceAccess(fbClient, cfg, "photos")
	requireBlogsMW := middleware.RequireResourceAccess(fbClient, cfg, "blogs")

	globalRateMW := middleware.RateLimitMiddleware(globalLimiter, "Too many requests from this IP, please try again later.")
	mutationRateMW := middleware.RateLimitMiddleware(mutationLimiter, "Too many modification requests, please slow down.")
	uploadRateMW := middleware.RateLimitMiddleware(uploadLimiter, "Upload rate limit exceeded, please wait before uploading more files.")
	authRateMW := middleware.RateLimitMiddleware(authLimiter, "Too many authentication requests, please try again later.")

	// Handlers
	healthH := handlers.NewHealthHandler(cfg, fbClient)
	authH := handlers.NewAuthHandler(cfg, fbClient)
	photosH := handlers.NewPhotosHandler(cfg, fbClient)
	blogsH := handlers.NewBlogsHandler(cfg, fbClient)
	storageH := handlers.NewStorageHandler(cfg, fbClient)
	usersH := handlers.NewUsersHandler(cfg, fbClient)
	analyticsH := handlers.NewAnalyticsHandler(cfg, fbClient)
	notifH := handlers.NewNotificationsHandler(cfg, fbClient)

	// Health Check
	router.GET("/health", healthH.Health)
	router.GET("/api/health", healthH.Health)

	// API Group with Global Rate Limiting
	api := router.Group("/api", globalRateMW)
	{
		// Auth Routes
		auth := api.Group("/auth")
		{
			auth.GET("/google", authH.GoogleAuth)
			auth.GET("/google/callback", authH.GoogleCallback)
			auth.GET("/me", authRateMW, authMW, authH.Me)
			auth.POST("/logout", authH.Logout)
		}

		// Photos Routes
		photos := api.Group("/photos")
		{
			photos.GET("/categories/routes", photosH.GetCategoryRoutes)
			photos.GET("/getcategoryRoutes", photosH.GetCategoryRoutes)

			photos.GET("/", photosH.GetPhotoContainer)
			photos.GET("/photocontainer", photosH.GetPhotoContainer)
			photos.GET("/photocontainer/:type", photosH.GetCategoryByType)

			photos.POST("/photocontainer/:type", mutationRateMW, authMW, requirePhotosMW, photosH.CreateCategory)
			photos.PUT("/photocontainer/updatecategory/:type", mutationRateMW, authMW, requirePhotosMW, photosH.UpdateCategory)
			photos.PUT("/photocontainer/:type/changevisibility/:visibility", mutationRateMW, authMW, requirePhotosMW, photosH.ChangeCategoryVisibility)

			photos.PATCH("/photocontainer/:type/updatephoto/:uid", mutationRateMW, authMW, requirePhotosMW, photosH.UpdatePhoto)
			photos.DELETE("/photocontainer/:type/deletephoto/:uid", mutationRateMW, authMW, requirePhotosMW, photosH.DeletePhoto)

			photos.POST("/photocontainer/:type/likephoto/:uid", mutationRateMW, authMW, photosH.LikePhoto)
			photos.PATCH("/photocontainer/:type/makecategorythumbnail", mutationRateMW, authMW, requirePhotosMW, photosH.MakeCategoryThumbnail)
		}

		// Blogs Routes
		blogs := api.Group("/blogs")
		{
			blogs.GET("/", blogsH.GetAllBlogs)
			blogs.GET("/getblogs", blogsH.GetAllBlogs)
			blogs.GET("/:blogpost", blogsH.GetBlogByID)

			blogs.POST("/", mutationRateMW, authMW, requireBlogsMW, blogsH.AddBlog)
			blogs.POST("/addblog", mutationRateMW, authMW, requireBlogsMW, blogsH.AddBlog)

			blogs.PUT("/:blogpost", mutationRateMW, authMW, requireBlogsMW, blogsH.UpdateBlog)
			blogs.PUT("/updateblog/:blogpost", mutationRateMW, authMW, requireBlogsMW, blogsH.UpdateBlog)

			blogs.DELETE("/:blogpost", mutationRateMW, authMW, requireBlogsMW, blogsH.DeleteBlog)
			blogs.DELETE("/deleteblog/:blogpost", mutationRateMW, authMW, requireBlogsMW, blogsH.DeleteBlog)

			blogs.GET("/:blogpost/comments", blogsH.GetComments)
			blogs.GET("/getcomments/:blogpost", blogsH.GetComments)

			blogs.POST("/:blogpost/comments", mutationRateMW, authMW, blogsH.AddComment)
			blogs.POST("/:blogpost/addcomment", mutationRateMW, authMW, blogsH.AddComment)

			blogs.DELETE("/:blogpost/comments/:key", mutationRateMW, authMW, blogsH.DeleteComment)
			blogs.DELETE("/:blogpost/deletecomment/:key", mutationRateMW, authMW, blogsH.DeleteComment)
		}

		// Storage & Upload Routes
		storageGroup := api.Group("/storage", uploadRateMW)
		{
			storageGroup.POST("/", authMW, storageH.VerifyStorageAccess("upload"), storageH.UploadPhoto)
			storageGroup.POST("/uploadphoto", authMW, storageH.VerifyStorageAccess("upload"), storageH.UploadPhoto)
			storageGroup.DELETE("/files/:fileName", authMW, storageH.VerifyStorageAccess("delete"), storageH.DeletePhoto)
			storageGroup.DELETE("/deletephoto", authMW, storageH.VerifyStorageAccess("delete"), storageH.DeletePhoto)
		}

		uploadGroup := api.Group("/upload", uploadRateMW)
		{
			uploadGroup.POST("/", authMW, storageH.VerifyStorageAccess("upload"), storageH.UploadPhoto)
			uploadGroup.POST("/uploadphoto", authMW, storageH.VerifyStorageAccess("upload"), storageH.UploadPhoto)
			uploadGroup.DELETE("/files/:fileName", authMW, storageH.VerifyStorageAccess("delete"), storageH.DeletePhoto)
			uploadGroup.DELETE("/deletephoto", authMW, storageH.VerifyStorageAccess("delete"), storageH.DeletePhoto)
		}

		// Users Routes
		users := api.Group("/users")
		{
			users.GET("", authMW, usersH.ListUsers)
			users.POST("/:uid", authRateMW, authMW, usersH.SyncUser)
			users.GET("/:uid", authMW, usersH.GetUser)
			users.PATCH("/:uid/access", authMW, usersH.UpdateUserAccess)
		}

		// Analytics Routes (GA4)
		analytics := api.Group("/analytics")
		{
			analytics.GET("/ga4/summary", optionalAuthMW, analyticsH.GetGA4Summary)
		}

		// Admin Notifications Routes
		notifications := api.Group("/notifications", authMW)
		{
			notifications.GET("", notifH.ListNotifications)
			notifications.PATCH("/read-all", notifH.MarkAllAsRead)
			notifications.PATCH("/:id/read", notifH.MarkAsRead)
			notifications.DELETE("/:id", notifH.DeleteNotification)
		}
	}

	// 404 Route Handler
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Unknown Endpoint",
			"message": "The requested API route does not exist.",
		})
	})

	serverAddr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	go func() {
		log.Printf("Forged In Frames API (Go) listening on port %s in %s mode", cfg.Port, cfg.NodeEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down API server gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly.")
}
