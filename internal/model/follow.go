package model

import (
	"errors"
	"time"
)

type Follow struct {
	FollowerID int64     `db:"follower_id" json:"follower_id"`
	FolloweeID int64     `db:"followee_id" json:"followee_id"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

type UserSummary struct {
	ID          int64   `db:"id" json:"id"`
	Username    string  `db:"username" json:"username"`
	DisplayName *string `db:"display_name" json:"display_name"`
	AvatarURL   *string `db:"avatar_url" json:"avatar_url"`
	IsFollowing bool    `json:"is_following"`
}

type FollowListResponse struct {
	Users      []UserSummary `json:"users"`
	NextCursor *string       `json:"next_cursor,omitempty"`
	HasMore    bool          `json:"has_more"`
}

var (
	ErrAlreadyFollowing = errors.New("already following this user")
	ErrNotFollowing     = errors.New("not following this user")
	ErrCannotFollowSelf = errors.New("cannot follow yourself")
)
