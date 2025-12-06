package model

import (
	"errors"
	"time"
)

// RefreshToken represents a refresh token stored in the database
type RefreshToken struct {
	ID         string     `db:"id" json:"id"`
	UserID     int64      `db:"user_id" json:"user_id"`
	TokenHash  string     `db:"token_hash" json:"-"` // Never expose hash
	ExpiresAt  time.Time  `db:"expires_at" json:"expires_at"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	RevokedAt  *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	ReplacedBy *string    `db:"replaced_by" json:"replaced_by,omitempty"`
	DeviceInfo *string    `db:"device_info" json:"device_info,omitempty"`
	IPAddress  *string    `db:"ip_address" json:"ip_address,omitempty"`
}

// IsValid returns true if the token is not expired and not revoked
func (t *RefreshToken) IsValid() bool {
	return t.RevokedAt == nil && time.Now().Before(t.ExpiresAt)
}

// IsRevoked returns true if the token has been revoked
func (t *RefreshToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

// IsExpired returns true if the token has expired
func (t *RefreshToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// Refresh token errors
var (
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	ErrRefreshTokenExpired  = errors.New("refresh token expired")
	ErrRefreshTokenRevoked  = errors.New("refresh token revoked")
	ErrRefreshTokenReused   = errors.New("refresh token reuse detected")
)

// Token API error codes (used in HTTP responses)
const (
	CodeTokenExpired = "TOKEN_EXPIRED"
	CodeTokenInvalid = "TOKEN_INVALID"
	CodeTokenReused  = "TOKEN_REUSED"
)

// TokenPair represents both tokens returned after login/refresh
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // Seconds until access token expires
}

// LoginResponse is returned after successful login
type LoginResponse struct {
	User         *User  `json:"user"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// RefreshRequest is the request body for POST /auth/refresh
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest is the request body for POST /auth/logout
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}
