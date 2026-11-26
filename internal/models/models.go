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
	PhotoURL       string     `json:"photoURL,omitempty"`
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

// --- Analytics Models (GA4 Reporting) ---

type AnalyticsTimeSeriesPoint struct {
	Date     string `json:"date"`
	Views    int    `json:"views"`
	Visitors int    `json:"visitors"`
}

type AnalyticsCategoryBreakdown struct {
	Name       string `json:"name"`
	Views      int    `json:"views"`
	Percentage int    `json:"percentage"`
}

type AnalyticsPageBreakdown struct {
	Path           string `json:"path"`
	Title          string `json:"title"`
	Views          int    `json:"views"`
	UniqueVisitors int    `json:"uniqueVisitors"`
	Percentage     int    `json:"percentage"`
}

type AnalyticsTopPhoto struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Category       string `json:"category"`
	Views          int    `json:"views"`
	EngagementRate int    `json:"engagementRate"`
	ThumbnailURL   string `json:"thumbnailUrl,omitempty"`
}

type AnalyticsTopBlog struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Reads       int    `json:"reads"`
	AvgReadTime string `json:"avgReadTime"`
}

type AnalyticsDeviceBreakdown struct {
	Desktop int `json:"desktop"`
	Mobile  int `json:"mobile"`
	Tablet  int `json:"tablet"`
}

type AnalyticsBrowserBreakdown struct {
	Browser    string `json:"browser"`
	Percentage int    `json:"percentage"`
}

type AnalyticsReferrerBreakdown struct {
	Source     string `json:"source"`
	Count      int    `json:"count"`
	Percentage int    `json:"percentage"`
}

type AnalyticsLocationBreakdown struct {
	Country    string `json:"country"`
	Visitors   int    `json:"visitors"`
	Percentage int    `json:"percentage"`
}

type AnalyticsEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Path      string                 `json:"path"`
	Title     string                 `json:"title,omitempty"`
	Timestamp int64                  `json:"timestamp"`
	SessionID string                 `json:"sessionId"`
	Device    string                 `json:"device"`
	Browser   string                 `json:"browser"`
	OS        string                 `json:"os"`
	Referrer  string                 `json:"referrer,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type AnalyticsSummary struct {
	TotalViews             int                          `json:"totalViews"`
	UniqueVisitors         int                          `json:"uniqueVisitors"`
	AvgDwellSeconds        int                          `json:"avgDwellSeconds"`
	TotalPhotoInteractions int                          `json:"totalPhotoInteractions"`
	TimeRange              string                       `json:"timeRange"`
	ViewsTrend             int                          `json:"viewsTrend"`
	VisitorsTrend          int                          `json:"visitorsTrend"`
	InteractionsTrend      int                          `json:"interactionsTrend"`
	TimeSeries             []AnalyticsTimeSeriesPoint   `json:"timeSeries"`
	CategoryBreakdown      []AnalyticsCategoryBreakdown `json:"categoryBreakdown"`
	PageBreakdown          []AnalyticsPageBreakdown     `json:"pageBreakdown"`
	TopPhotos              []AnalyticsTopPhoto          `json:"topPhotos"`
	TopBlogs               []AnalyticsTopBlog           `json:"topBlogs"`
	DeviceBreakdown        AnalyticsDeviceBreakdown     `json:"deviceBreakdown"`
	BrowserBreakdown       []AnalyticsBrowserBreakdown  `json:"browserBreakdown"`
	ReferrerBreakdown      []AnalyticsReferrerBreakdown `json:"referrerBreakdown"`
	LocationBreakdown      []AnalyticsLocationBreakdown `json:"locationBreakdown"`
	RecentEvents           []AnalyticsEvent             `json:"recentEvents"`
}

type GA4SummaryResponse struct {
	Connected  bool             `json:"connected"`
	Source     string           `json:"source"`
	PropertyID string           `json:"propertyId,omitempty"`
	Summary    AnalyticsSummary `json:"summary"`
	Error      string           `json:"error,omitempty"`
}

// --- Admin Notifications Models ---

type AdminNotification struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "blog_comment" | "photo_like"
	Title       string `json:"title"`
	Message     string `json:"message"`
	ActorUID    string `json:"actorUid"`
	ActorName   string `json:"actorName"`
	ActorAvatar string `json:"actorAvatar,omitempty"`
	TargetID    string `json:"targetId"`
	TargetTitle string `json:"targetTitle,omitempty"`
	TargetPath  string `json:"targetPath"`
	Read        bool   `json:"read"`
	CreatedAt   string `json:"createdAt"`
}

type NotificationsResponse struct {
	Notifications []AdminNotification `json:"notifications"`
	UnreadCount   int                 `json:"unreadCount"`
}
