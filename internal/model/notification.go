package model

import (
	"time"
)

// Notification types
const (
	NotificationTypeFollow  = "follow"
	NotificationTypeLike    = "like"
	NotificationTypeComment = "comment"
)

// Notification represents a single notification record in the database.
type Notification struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"-"`          // Recipient
	ActorID   int64     `db:"actor_id" json:"actor_id"`  // Who triggered it
	Type      string    `db:"type" json:"type"`          // follow, like, comment
	PostID    *int64    `db:"post_id" json:"post_id,omitempty"`
	CommentID *int64    `db:"comment_id" json:"comment_id,omitempty"`
	IsRead    bool      `db:"is_read" json:"is_read"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// Joined field for display
	Actor *UserSummary `json:"actor,omitempty"`
}

// AggregatedNotification groups likes/comments on the same post.
// Used for "user1 and 5 others liked your post" display.
type AggregatedNotification struct {
	Type       string        `json:"type"`                    // like, comment
	PostID     *int64        `json:"post_id,omitempty"`       // For navigation to post
	ActorID    *int64        `json:"actor_id,omitempty"`      // For follow navigation
	Actors     []UserSummary `json:"actors"`                  // First 2-3 actors
	TotalCount int           `json:"total_count"`             // Total actors (for "and X others")
	LatestAt   time.Time     `json:"latest_at"`               // Most recent activity
	IsRead     bool          `json:"is_read"`                 // True if ALL in group are read
}

// NotificationListResponse is the paginated notification list response.
type NotificationListResponse struct {
	// Follows are not aggregated - shown individually
	Follows []Notification `json:"follows"`
	// Likes and comments are aggregated by post
	Aggregated []AggregatedNotification `json:"aggregated"`
	// Unread count for badge
	UnreadCount int `json:"unread_count"`
}

// MarkReadRequest is the request body for marking notifications as read.
type MarkReadRequest struct {
	NotificationIDs []int64 `json:"notification_ids"`
}
