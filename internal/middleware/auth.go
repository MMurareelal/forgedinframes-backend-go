package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/models"
)

const AuthUserKey = "authenticated_user"

// Authenticate verifies backend JWT or falls back to Firebase ID token
func Authenticate(fbClient *fb.Client, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authorization token missing or malformed.",
			})
			return
		}

		tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authorization token cannot be empty.",
			})
			return
		}

		// 1. Attempt verification with backend JWT secret
		claims := &models.AuthClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})

		if err == nil && token.Valid {
			authUser := models.AuthUser{
				UID:         claims.UID,
				Email:       claims.Email,
				DisplayName: claims.DisplayName,
				PhotoURL:    claims.PhotoURL,
				AccessList:  claims.AccessList,
			}
			c.Set(AuthUserKey, authUser)
			c.Next()
			return
		}

		// 2. Fallback to Firebase ID Token
		if fbClient != nil && fbClient.AuthClient != nil {
			fbToken, fbErr := fbClient.AuthClient.VerifyIDToken(context.Background(), tokenStr)
			if fbErr == nil {
				email, _ := fbToken.Claims["email"].(string)
				name, _ := fbToken.Claims["name"].(string)
				picture, _ := fbToken.Claims["picture"].(string)

				isOwner := cfg.IsAdminEmail(email)
				authUser := models.AuthUser{
					UID:         fbToken.UID,
					Email:       email,
					DisplayName: name,
					PhotoURL:    picture,
					AccessList: models.AccessList{
						PhotosEditAccess: isOwner,
						BlogsEditAccess:  isOwner,
						EditorAccess:     isOwner,
						UploadEditAccess: isOwner,
					},
				}
				c.Set(AuthUserKey, authUser)
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "Unauthorized",
			"message": "Invalid or expired authorization token.",
		})
	}
}

// OptionalAuth parses the token if provided, but does not abort if missing
func OptionalAuth(fbClient *fb.Client, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			claims := &models.AuthClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return []byte(cfg.JWTSecret), nil
			})
			if err == nil && token.Valid {
				c.Set(AuthUserKey, models.AuthUser{
					UID:         claims.UID,
					Email:       claims.Email,
					DisplayName: claims.DisplayName,
					PhotoURL:    claims.PhotoURL,
					AccessList:  claims.AccessList,
				})
			} else if fbClient != nil && fbClient.AuthClient != nil {
				if fbToken, fbErr := fbClient.AuthClient.VerifyIDToken(context.Background(), tokenStr); fbErr == nil {
					email, _ := fbToken.Claims["email"].(string)
					name, _ := fbToken.Claims["name"].(string)
					picture, _ := fbToken.Claims["picture"].(string)
					isOwner := cfg.IsAdminEmail(email)
					c.Set(AuthUserKey, models.AuthUser{
						UID:         fbToken.UID,
						Email:       email,
						DisplayName: name,
						PhotoURL:    picture,
						AccessList: models.AccessList{
							PhotosEditAccess: isOwner,
							BlogsEditAccess:  isOwner,
							EditorAccess:     isOwner,
							UploadEditAccess: isOwner,
						},
					})
				}
			}
		}
		c.Next()
	}
}

// RequireResourceAccess ensures user has permission to modify the given resource
func RequireResourceAccess(fbClient *fb.Client, cfg *config.Config, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get(AuthUserKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized. Authentication required.",
			})
			return
		}

		user := val.(models.AuthUser)
		if cfg.IsAdminEmail(user.Email) {
			c.Next()
			return
		}

		// Check embedded access list in JWT
		hasAccess := false
		switch resource {
		case "photos":
			hasAccess = user.AccessList.PhotosEditAccess || user.AccessList.EditorAccess
		case "blogs":
			hasAccess = user.AccessList.BlogsEditAccess || user.AccessList.EditorAccess
		case "upload", "storage":
			hasAccess = user.AccessList.UploadEditAccess || user.AccessList.PhotosEditAccess || user.AccessList.EditorAccess
		}

		if hasAccess {
			c.Next()
			return
		}

		// Fallback check against Firebase Realtime Database
		if fbClient != nil && fbClient.DBClient != nil {
			var dbUser models.User
			err := fbClient.DBClient.NewRef("users/" + user.UID).Get(context.Background(), &dbUser)
			if err == nil {
				switch resource {
				case "photos":
					hasAccess = dbUser.AccessList.PhotosEditAccess || dbUser.AccessList.EditorAccess
				case "blogs":
					hasAccess = dbUser.AccessList.BlogsEditAccess || dbUser.AccessList.EditorAccess
				case "upload", "storage":
					hasAccess = dbUser.AccessList.UploadEditAccess || dbUser.AccessList.PhotosEditAccess || dbUser.AccessList.EditorAccess
				}
			}
		}

		if hasAccess {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "You do not have permission to modify " + resource + ".",
		})
	}
}

// GetUserFromContext retrieves authenticated user from gin.Context
func GetUserFromContext(c *gin.Context) (models.AuthUser, bool) {
	val, exists := c.Get(AuthUserKey)
	if !exists {
		return models.AuthUser{}, false
	}
	u, ok := val.(models.AuthUser)
	return u, ok
}
