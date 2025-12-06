package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/model"
)

type refreshTokenRepository struct {
	db *sqlx.DB
}

// NewRefreshTokenRepository creates a new refresh token repository
func NewRefreshTokenRepository(db *sqlx.DB) RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

// Create inserts a new refresh token into the database
func (r *refreshTokenRepository) Create(ctx context.Context, token *model.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, device_info, ip_address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	err := r.db.QueryRowxContext(ctx, query,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.DeviceInfo,
		token.IPAddress,
	).Scan(&token.ID, &token.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create refresh token: %w", err)
	}
	return nil
}

// FindByTokenHash retrieves a refresh token by its hash
func (r *refreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by, device_info, ip_address
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var token model.RefreshToken
	err := r.db.GetContext(ctx, &token, query, tokenHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("failed to find refresh token: %w", err)
	}
	return &token, nil
}

// Revoke marks a token as revoked and optionally links to its replacement
func (r *refreshTokenRepository) Revoke(ctx context.Context, id string, replacedBy *string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW(), replaced_by = $2
		WHERE id = $1 AND revoked_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query, id, replacedBy)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}
	return nil
}

// RevokeAllForUser revokes all active refresh tokens for a user
func (r *refreshTokenRepository) RevokeAllForUser(ctx context.Context, userID int64) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke all tokens for user: %w", err)
	}
	return nil
}

// DeleteExpired removes tokens that expired before the given duration
func (r *refreshTokenRepository) DeleteExpired(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `
		DELETE FROM refresh_tokens
		WHERE expires_at < NOW() - $1::interval
	`
	result, err := r.db.ExecContext(ctx, query, olderThan.String())
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}
	return result.RowsAffected()
}
