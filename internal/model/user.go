package model

import (
	"errors"
	"time"
)

// User represents a user in the system
type User struct {
	ID             int64     `db:"id" json:"id"`
	Username       string    `db:"username" json:"username"`
	PasswordHashed string    `db:"password_hashed" json:"-"` // "-" hides from JSON output
	DisplayName    *string   `db:"display_name" json:"display_name"`
	AvatarURL      *string   `db:"avatar_url" json:"avatar_url"`
	AvatarKey      *string   `db:"avatar_key" json:"-"`
	Bio            *string   `db:"bio" json:"bio"`
	IsNewUser      bool      `db:"is_new_user" json:"is_new_user"` // For onboarding flow
	FollowerCount  int       `db:"follower_count" json:"follower_count"`
	FollowingCount int       `db:"following_count" json:"following_count"`
	PostCount      int       `db:"post_count" json:"post_count"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// RegisterRequest represents the data needed to register a new user
type RegisterRequest struct {
	Username    string  `json:"username"`
	Password    string  `json:"password"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"-"`
	AvatarKey   *string `json:"-"`
}

// LoginRequest represents the data needed to log in
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var (
	// ErrUserNotFound is returned when a user cannot be found
	ErrUserNotFound = errors.New("user not found")

	// ErrUsernameExists is returned when attempting to create a user with a taken username
	ErrUsernameExists = errors.New("username already exists")

	// ErrInvalidCredentials is returned when login credentials are incorrect
	ErrInvalidCredentials = errors.New("invalid credentials")
)
