package model

import (
	"time"
)

// DeviceToken represents a user's registered device for push notifications.
// Supports multiple devices per user.
type DeviceToken struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"-"`
	Token     string    `db:"token" json:"-"`     // FCM token, hidden from JSON
	Platform  string    `db:"platform" json:"platform"` // "ios", "android"
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// RegisterTokenRequest is the request body for registering a device token.
type RegisterTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"` // "ios" or "android"
}

// Platform constants
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)
