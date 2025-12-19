package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/model"
)

type deviceTokenRepository struct {
	db *sqlx.DB
}

func NewDeviceTokenRepository(db *sqlx.DB) DeviceTokenRepository {
	return &deviceTokenRepository{db: db}
}

// Upsert creates or updates a device token for a user.
// If the token already exists, updates the user_id and platform.
func (r *deviceTokenRepository) Upsert(ctx context.Context, userID int64, token, platform string) error {
	query := `
		INSERT INTO device_tokens (user_id, token, platform, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (token) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			platform = EXCLUDED.platform,
			updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, query, userID, token, platform)
	if err != nil {
		return fmt.Errorf("upsert device token: %w", err)
	}
	return nil
}

// GetByUserID returns all device tokens for a user.
func (r *deviceTokenRepository) GetByUserID(ctx context.Context, userID int64) ([]model.DeviceToken, error) {
	query := `
		SELECT id, user_id, token, platform, created_at, updated_at
		FROM device_tokens
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`
	var tokens []model.DeviceToken
	err := r.db.SelectContext(ctx, &tokens, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get device tokens: %w", err)
	}
	return tokens, nil
}

// Delete removes a device token.
func (r *deviceTokenRepository) Delete(ctx context.Context, token string) error {
	query := `DELETE FROM device_tokens WHERE token = $1`
	_, err := r.db.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("delete device token: %w", err)
	}
	return nil
}
