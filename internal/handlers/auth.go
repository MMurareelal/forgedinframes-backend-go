package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/middleware"
	"forgedinframes-backend-go/internal/models"
)

type AuthHandler struct {
	cfg        *config.Config
	fbClient   *fb.Client
	oauthConf  *oauth2.Config
}

type oauthState struct {
	RedirectURI string `json:"redirect_uri"`
}

type googleUserInfo struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func NewAuthHandler(cfg *config.Config, fbClient *fb.Client) *AuthHandler {
	callbackURL := fmt.Sprintf("%s/api/auth/google/callback", cfg.BackendURL)
	oauthConf := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  callbackURL,
		Scopes: []string{
			"openid",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	return &AuthHandler{
		cfg:       cfg,
		fbClient:  fbClient,
		oauthConf: oauthConf,
	}
}

// GoogleAuth initiates the Google OAuth 2.0 consent flow
// GET /api/auth/google?redirect_uri=<frontend_url>
func (h *AuthHandler) GoogleAuth(c *gin.Context) {
	if h.cfg.GoogleClientID == "" || h.cfg.GoogleClientSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Configuration Error",
			"message": "Google OAuth client credentials are not configured.",
		})
		return
	}

	frontendRedirectURI := c.Query("redirect_uri")
	if frontendRedirectURI == "" {
		frontendRedirectURI = fmt.Sprintf("%s/auth/callback", h.cfg.FrontendURL)
	}

	stateBytes, _ := json.Marshal(oauthState{RedirectURI: frontendRedirectURI})
	authURL := h.oauthConf.AuthCodeURL(
		string(stateBytes),
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// GoogleCallback handles Google OAuth 2.0 authorization code callback
// GET /api/auth/google/callback
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	frontendRedirectURI := fmt.Sprintf("%s/auth/callback", h.cfg.FrontendURL)

	stateParam := c.Query("state")
	if stateParam != "" {
		var state oauthState
		if err := json.Unmarshal([]byte(stateParam), &state); err == nil && state.RedirectURI != "" {
			frontendRedirectURI = state.RedirectURI
		}
	}

	if errParam := c.Query("error"); errParam != "" {
		errDesc := c.Query("error_description")
		if errDesc == "" {
			errDesc = errParam
		}
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?error=%s", frontendRedirectURI, url.QueryEscape(errDesc)))
		return
	}

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?error=%s", frontendRedirectURI, url.QueryEscape("Missing authorization code")))
		return
	}

	ctx := context.Background()
	token, err := h.oauthConf.Exchange(ctx, code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?error=%s", frontendRedirectURI, url.QueryEscape("Failed to exchange authorization code")))
		return
	}

	// Fetch user info from Google
	client := h.oauthConf.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?error=%s", frontendRedirectURI, url.QueryEscape("Failed to fetch Google user profile")))
		return
	}
	defer resp.Body.Close()

	var userInfo googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil || userInfo.Email == "" {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?error=%s", frontendRedirectURI, url.QueryEscape("Invalid user profile received from Google")))
		return
	}

	email := strings.ToLower(strings.TrimSpace(userInfo.Email))
	uid := userInfo.Sub
	name := userInfo.Name
	if name == "" {
		parts := strings.Split(email, "@")
		name = parts[0]
	}
	picture := userInfo.Picture
	isOwner := h.cfg.IsAdminEmail(email)

	accessList := models.AccessList{
		PhotosEditAccess: isOwner,
		BlogsEditAccess:  isOwner,
		EditorAccess:     isOwner,
		UploadEditAccess: isOwner,
	}

	// Sync or upsert user profile in Firebase Realtime Database
	if h.fbClient != nil && h.fbClient.DBClient != nil {
		userRef := h.fbClient.DBClient.NewRef(fmt.Sprintf("users/%s", uid))
		var existingUser models.User
		if err := userRef.Get(ctx, &existingUser); err == nil && existingUser.UID != "" {
			accessList.EditorAccess = isOwner || existingUser.AccessList.EditorAccess
			accessList.BlogsEditAccess = isOwner || existingUser.AccessList.BlogsEditAccess
			accessList.PhotosEditAccess = isOwner || existingUser.AccessList.PhotosEditAccess
			accessList.UploadEditAccess = isOwner || existingUser.AccessList.UploadEditAccess

			_ = userRef.Update(ctx, map[string]interface{}{
				"name":            name,
				"email":           email,
				"profile_picture": picture,
				"accessList":      accessList,
				"lastLoginAt":     time.Now().UTC().Format(time.RFC3339),
			})
		} else {
			_ = userRef.Set(ctx, models.User{
				UID:            uid,
				Name:           name,
				Email:          email,
				ProfilePicture: picture,
				AccessList:     accessList,
				CreatedAt:      time.Now().UTC().Format(time.RFC3339),
				LastLoginAt:    time.Now().UTC().Format(time.RFC3339),
			})
		}
	}

	// Sign backend application JWT
	now := time.Now()
	claims := models.AuthClaims{
		UID:         uid,
		Email:       email,
		DisplayName: name,
		PhotoURL:    picture,
		AccessList:  accessList,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(h.cfg.JWTExpiresIn)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := jwtToken.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?error=%s", frontendRedirectURI, url.QueryEscape("Failed to generate authentication token")))
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?token=%s", frontendRedirectURI, url.QueryEscape(tokenStr)))
}

// Me retrieves current authenticated user profile and accessList
// GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "Unauthorized",
			"message": "Authorization token missing or malformed.",
		})
		return
	}

	isOwner := h.cfg.IsAdminEmail(user.Email)
	profile := models.User{
		UID:            user.UID,
		Email:          user.Email,
		Name:           user.DisplayName,
		ProfilePicture: user.PhotoURL,
		AccessList: models.AccessList{
			EditorAccess:     isOwner || user.AccessList.EditorAccess,
			BlogsEditAccess:  isOwner || user.AccessList.BlogsEditAccess,
			PhotosEditAccess: isOwner || user.AccessList.PhotosEditAccess,
			UploadEditAccess: isOwner || user.AccessList.UploadEditAccess,
		},
	}

	// Fetch latest permissions and name from DB
	if h.fbClient != nil && h.fbClient.DBClient != nil {
		var dbUser models.User
		if err := h.fbClient.DBClient.NewRef(fmt.Sprintf("users/%s", user.UID)).Get(c.Request.Context(), &dbUser); err == nil && dbUser.UID != "" {
			if dbUser.Name != "" {
				profile.Name = dbUser.Name
			}
			if dbUser.ProfilePicture != "" {
				profile.ProfilePicture = dbUser.ProfilePicture
			}
			profile.AccessList = models.AccessList{
				EditorAccess:     isOwner || dbUser.AccessList.EditorAccess,
				BlogsEditAccess:  isOwner || dbUser.AccessList.BlogsEditAccess,
				PhotosEditAccess: isOwner || dbUser.AccessList.PhotosEditAccess,
				UploadEditAccess: isOwner || dbUser.AccessList.UploadEditAccess,
			}
		}
	}

	c.JSON(http.StatusOK, profile)
}

// Logout terminates user session
// POST /api/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}
