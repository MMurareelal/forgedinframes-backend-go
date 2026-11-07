package models

import (
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	SafeKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	ScriptRegex  = regexp.MustCompile(`(?i)<script\b[^>]*>`)
)

func IsSafeKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	return SafeKeyRegex.MatchString(key)
}

func ContainsScriptTag(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<script") || ScriptRegex.MatchString(s)
}

// AccessList represents the RBAC permissions of a user
type AccessList struct {
	PhotosEditAccess bool `json:"photosEditAccess"`
	BlogsEditAccess  bool `json:"blogsEditAccess"`
	EditorAccess     bool `json:"editorAccess"`
	UploadEditAccess bool `json:"uploadEditAccess"`
}

// User represents a user profile in Firebase Realtime Database
type User struct {
	UID            string     `json:"uid"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	ProfilePicture string     `json:"profile_picture"`
	AccessList     AccessList `json:"accessList"`
	CreatedAt      string     `json:"createdAt,omitempty"`
	LastLoginAt    string     `json:"lastLoginAt,omitempty"`
}

// UserSyncRequest DTO for POST /api/users/:uid
type UserSyncRequest struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	ProfilePicture string `json:"profile_picture"`
}

// AuthClaims represents JWT payload
type AuthClaims struct {
	UID         string     `json:"uid"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	PhotoURL    string     `json:"photoURL"`
	AccessList  AccessList `json:"accessList"`
	jwt.RegisteredClaims
}

// AuthUser Context representation of the authenticated user
type AuthUser struct {
	UID         string     `json:"uid"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	PhotoURL    string     `json:"photoURL"`
	AccessList  AccessList `json:"accessList"`
}

// --- Photo Models ---

type CategoryRoute struct {
	CategoryName  string `json:"categoryName"`
	CategoryOrder int    `json:"categoryOrder"`
}

type LikedByUser struct {
	UserID  string `json:"userId"`
	LikedAt string `json:"likedAt"`
}

type LikesData struct {
	TotalLikes   int           `json:"totalLikes"`
	LikedBy      []LikedByUser `json:"likedBy"`
	CurrentEvent string        `json:"currentEvent,omitempty"`
}

type CategoryCreateRequest struct {
	Type        string   `json:"type" binding:"required"`
	IsActive    *bool    `json:"isActive,omitempty"`
	BgImg       string   `json:"bgimg,omitempty"`
	Thumbnails  []string `json:"thumbnails,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
}

type CategoryThumbnailRequest struct {
	ImgURL string `json:"imgURL" binding:"required"`
}

type PhotoUpdateRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	URL         *string `json:"url,omitempty"`
	Featured    *bool   `json:"featured,omitempty"`
}

// --- Blog Models ---

type Comment struct {
	Comment                string `json:"comment,omitempty"`
	Content                string `json:"content,omitempty"`
	UserID                 string `json:"userId,omitempty"`
	UserName               string `json:"userName,omitempty"`
	PostedBy               string `json:"postedBy,omitempty"`
	PostedByUID            string `json:"postedByUID,omitempty"`
	PostedByProfilePicture string `json:"postedByProfilePicture,omitempty"`
	CreatedAt              string `json:"createdAt,omitempty"`
}

type BlogPost struct {
	ID         string                 `json:"id,omitempty"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`
	Slug       string                 `json:"slug,omitempty"`
	Excerpt    string                 `json:"excerpt,omitempty"`
	CoverImage string                 `json:"coverImage,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Author     string                 `json:"author,omitempty"`
	AuthorID   string                 `json:"authorId,omitempty"`
	CreatedAt  string                 `json:"createdAt,omitempty"`
	UpdatedAt  string                 `json:"updatedAt,omitempty"`
	Comments   map[string]interface{} `json:"comments,omitempty"`
}

type BlogPostCreateRequest struct {
	Title      string   `json:"title" binding:"required"`
	Content    string   `json:"content" binding:"required"`
	Slug       string   `json:"slug,omitempty"`
	Excerpt    string   `json:"excerpt,omitempty"`
	CoverImage string   `json:"coverImage,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Author     string   `json:"author,omitempty"`
}

type CommentCreateRequest struct {
	Comment                string `json:"comment,omitempty"`
	Content                string `json:"content,omitempty"`
	UserName               string `json:"userName,omitempty"`
	PostedBy               string `json:"postedBy,omitempty"`
	PostedByUID            string `json:"postedByUID,omitempty"`
	PostedByProfilePicture string `json:"postedByProfilePicture,omitempty"`
}

func (c *CommentCreateRequest) GetText() string {
	if strings.TrimSpace(c.Content) != "" {
		return strings.TrimSpace(c.Content)
	}
	return strings.TrimSpace(c.Comment)
}
