package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/model"
)

// userRepository implements UserRepository using sqlx
type userRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *sqlx.DB) UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user into the database
func (r *userRepository) Create(ctx context.Context, u *model.User) error {
	query := `
		INSERT INTO users (username, password_hashed, display_name, avatar_url, avatar_key, is_new_user, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, is_new_user, follower_count, following_count, post_count, created_at, updated_at
	`

	row := r.db.QueryRowxContext(ctx, query,
		u.Username,
		u.PasswordHashed,
		u.DisplayName,
		u.AvatarURL,
		u.AvatarKey,
		u.IsNewUser,
	)

	err := row.Scan(
		&u.ID,
		&u.IsNewUser,
		&u.FollowerCount,
		&u.FollowingCount,
		&u.PostCount,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by their ID
func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	query := `
		SELECT id, username, password_hashed, display_name, avatar_url, avatar_key, bio, is_new_user,
		       follower_count, following_count, post_count, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var u model.User
	err := r.db.GetContext(ctx, &u, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return &u, nil
}

// GetByUsername retrieves a user by their username
func (r *userRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `
		SELECT id, username, password_hashed, display_name, avatar_url, avatar_key, bio, is_new_user,
		       follower_count, following_count, post_count, created_at, updated_at
		FROM users
		WHERE username = $1
	`

	var u model.User
	err := r.db.GetContext(ctx, &u, query, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &u, nil
}

// ExistsByUsername checks if a username is already taken
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, username)
	if err != nil {
		return false, fmt.Errorf("failed to check username existence: %w", err)
	}

	return exists, nil
}

func (r *userRepository) Search(ctx context.Context, query string, limit int) ([]model.UserSummary, error) {
	searchQuery := `
		SELECT id, username, display_name, avatar_url
		FROM users
		WHERE username ILIKE $1
		ORDER BY follower_count DESC
		LIMIT $2
	`

	var users []model.UserSummary
	err := r.db.SelectContext(ctx, &users, searchQuery, query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	return users, nil
}

func (r *userRepository) IncrementFollowerCount(ctx context.Context, tx *sqlx.Tx, userID int64, delta int) error {
	query := `UPDATE users SET follower_count = follower_count + $1 WHERE id = $2`
	_, err := tx.ExecContext(ctx, query, delta, userID)
	if err != nil {
		return fmt.Errorf("failed to increment follower count: %w", err)
	}
	return nil
}

func (r *userRepository) IncrementFollowingCount(ctx context.Context, tx *sqlx.Tx, userID int64, delta int) error {
	query := `UPDATE users SET following_count = following_count + $1 WHERE id = $2`
	_, err := tx.ExecContext(ctx, query, delta, userID)
	if err != nil {
		return fmt.Errorf("failed to increment following count: %w", err)
	}
	return nil
}
