package repository

import (
	"context"
	"time"

	"iamstagram_22520060/internal/model"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	// Create inserts a new user into the database
	// Returns error if username already exists or on database failure
	Create(ctx context.Context, user *model.User) error

	// GetByID retrieves a user by their ID
	// Returns model.ErrUserNotFound if user doesn't exist
	GetByID(ctx context.Context, id int64) (*model.User, error)

	// GetByUsername retrieves a user by their username
	// Returns model.ErrUserNotFound if user doesn't exist
	GetByUsername(ctx context.Context, username string) (*model.User, error)

	// ExistsByUsername checks if a username is already taken
	ExistsByUsername(ctx context.Context, username string) (bool, error)
}

// RefreshTokenRepository defines the interface for refresh token data access
type RefreshTokenRepository interface {
	// Create inserts a new refresh token into the database
	Create(ctx context.Context, token *model.RefreshToken) error

	// FindByTokenHash retrieves a refresh token by its hash
	// Returns model.ErrRefreshTokenNotFound if token doesn't exist
	FindByTokenHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error)

	// Revoke marks a token as revoked and optionally links to its replacement
	Revoke(ctx context.Context, id string, replacedBy *string) error

	// RevokeAllForUser revokes all active refresh tokens for a user
	RevokeAllForUser(ctx context.Context, userID int64) error

	// DeleteExpired removes tokens that expired before the given duration
	// Returns the number of deleted tokens
	DeleteExpired(ctx context.Context, olderThan time.Duration) (int64, error)
}
