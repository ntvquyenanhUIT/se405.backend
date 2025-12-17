package model

import (
	"errors"
	"time"
)

// Post represents a user's post with its metadata.
type Post struct {
	ID           int64      `db:"id" json:"id"`
	UserID       int64      `db:"user_id" json:"user_id"`
	Caption      *string    `db:"caption" json:"caption"`
	LikeCount    int        `db:"like_count" json:"like_count"`
	CommentCount int        `db:"comment_count" json:"comment_count"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt    *time.Time `db:"deleted_at" json:"-"`

	// Joined fields (not in posts table)
	Media   []PostMedia  `json:"media,omitempty"`
	Author  *UserSummary `json:"author,omitempty"`
	IsLiked bool         `json:"is_liked"`
}

// PostMedia represents a single media item in a post (carousel support).
type PostMedia struct {
	ID        int64  `db:"id" json:"id"`
	PostID    int64  `db:"post_id" json:"-"`
	MediaURL  string `db:"media_url" json:"media_url"`
	MediaType string `db:"media_type" json:"media_type"` // "image" or "video"
	Position  int    `db:"position" json:"position"`
}

// PostThumbnail is a lightweight representation for profile grids.
type PostThumbnail struct {
	ID           int64  `db:"id" json:"id"`
	ThumbnailURL string `db:"thumbnail_url" json:"thumbnail_url"` // First media URL
	MediaCount   int    `db:"media_count" json:"media_count"`     // For carousel indicator
}

// FeedPost is an enriched post for feed display.
type FeedPost struct {
	Post
	Author UserSummary `json:"author"`
}

// FeedResponse is the paginated feed response.
type FeedResponse struct {
	Posts      []FeedPost `json:"posts"`
	NextCursor *string    `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
}

// PostListResponse is the paginated post list response (for profile).
type PostListResponse struct {
	Posts      []PostThumbnail `json:"posts"`
	NextCursor *string         `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}

// CreatePostRequest is the request body for creating a post.
type CreatePostRequest struct {
	Caption   *string  `json:"caption"`
	MediaURLs []string `json:"media_urls"` // Pre-uploaded media URLs
}

// Post media constants
const (
	MaxPostMediaCount    = 10
	MaxPostCaptionLength = 2200 // Instagram's limit
	PostMediaFolder      = "posts"
	MaxPostMediaSize     = 10 * 1024 * 1024 // 10MB per media
)

// Post errors
var (
	ErrPostNotFound    = errors.New("post not found")
	ErrNotPostOwner    = errors.New("not the owner of this post")
	ErrNoMediaProvided = errors.New("at least one media is required")
	ErrTooManyMedia    = errors.New("too many media items")
	ErrCaptionTooLong  = errors.New("caption too long")
	ErrInvalidMediaURL = errors.New("invalid media URL")
)
