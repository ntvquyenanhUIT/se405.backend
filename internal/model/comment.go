package model

import (
	"errors"
	"time"
)

// Comment represents a comment on a post.
type Comment struct {
	ID              int64        `db:"id" json:"id"`
	PostID          int64        `db:"post_id" json:"post_id"`
	UserID          int64        `db:"user_id" json:"-"`
	Content         string       `db:"content" json:"content"`
	ParentCommentID *int64       `db:"parent_comment_id" json:"parent_comment_id,omitempty"`
	CreatedAt       time.Time    `db:"created_at" json:"created_at"`
	Author          *UserSummary `json:"author,omitempty"` // Joined field
}

// CreateCommentRequest is the request body for creating a comment.
type CreateCommentRequest struct {
	Content         string `json:"content"`
	ParentCommentID *int64 `json:"parent_comment_id,omitempty"`
}

// UpdateCommentRequest is the request body for updating a comment.
type UpdateCommentRequest struct {
	Content string `json:"content"`
}

// CommentListResponse is the paginated comment list response.
type CommentListResponse struct {
	Comments   []Comment `json:"comments"`
	NextCursor *string   `json:"next_cursor,omitempty"`
	HasMore    bool      `json:"has_more"`
}

// LikersListResponse is the paginated likers list response.
type LikersListResponse struct {
	Users      []UserSummary `json:"users"`
	NextCursor *string       `json:"next_cursor,omitempty"`
	HasMore    bool          `json:"has_more"`
}

// Comment constraints
const (
	MaxCommentLength = 2200 // Same as Instagram caption limit
)

// Comment errors
var (
	ErrCommentNotFound = errors.New("comment not found")
	ErrNotCommentOwner = errors.New("not the owner of this comment")
	ErrContentRequired = errors.New("comment content is required")
	ErrContentTooLong  = errors.New("comment content too long")
)
