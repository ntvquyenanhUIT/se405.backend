package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"iamstagram_22520060/internal/model"
)

type followRepository struct {
	db *sqlx.DB
}

func NewFollowRepository(db *sqlx.DB) FollowRepository {
	return &followRepository{db: db}
}

func (r *followRepository) Create(ctx context.Context, tx *sqlx.Tx, followerID, followeeID int64) (bool, error) {
	query := `
		INSERT INTO follows (follower_id, followee_id)
		VALUES ($1, $2)
		ON CONFLICT (follower_id, followee_id) DO NOTHING
	`
	result, err := tx.ExecContext(ctx, query, followerID, followeeID)
	if err != nil {
		return false, fmt.Errorf("failed to create follow: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

func (r *followRepository) Delete(ctx context.Context, tx *sqlx.Tx, followerID, followeeID int64) error {
	query := `DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2`
	result, err := tx.ExecContext(ctx, query, followerID, followeeID)
	if err != nil {
		return fmt.Errorf("failed to delete follow: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return model.ErrNotFollowing
	}

	return nil
}

func (r *followRepository) Exists(ctx context.Context, followerID, followeeID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND followee_id = $2)`
	var exists bool
	err := r.db.GetContext(ctx, &exists, query, followerID, followeeID)
	if err != nil {
		return false, fmt.Errorf("failed to check follow existence: %w", err)
	}
	return exists, nil
}

// GetFollowers retrieves users who follow the specified user with cursor-based pagination.
//
// Cursor pagination implementation:
//   - cursor == nil: Start from beginning, fetch latest followers (ORDER BY created_at DESC)
//   - cursor != nil: Fetch followers created BEFORE the cursor timestamp
//   - Always fetch limit+1 to check if more results exist
//   - If we got more than limit: trim to limit, set nextCursor to last item's timestamp
//   - If we got exactly limit or less: no nextCursor (end of list)
//
// Why created_at as cursor instead of offset?
//
//	✅ Consistent results even when new followers are added during pagination
//	✅ O(log n) seeks with index on (followee_id, created_at DESC)
//	❌ Offset pagination breaks when data changes, has O(n) performance on large offsets
//
// Returns: users slice, nextCursor (nil if no more results), error
func (r *followRepository) GetFollowers(ctx context.Context, userID int64, cursor *time.Time, limit int) ([]model.UserSummary, *time.Time, error) {
	var query string
	var args []interface{}

	if cursor == nil {
		query = `
			SELECT u.id, u.username, u.display_name, u.avatar_url, f.created_at
			FROM follows f
			JOIN users u ON u.id = f.follower_id
			WHERE f.followee_id = $1
			ORDER BY f.created_at DESC
			LIMIT $2
		`
		args = []interface{}{userID, limit + 1}
	} else {
		query = `
			SELECT u.id, u.username, u.display_name, u.avatar_url, f.created_at
			FROM follows f
			JOIN users u ON u.id = f.follower_id
			WHERE f.followee_id = $1 AND f.created_at < $2
			ORDER BY f.created_at DESC
			LIMIT $3
		`
		args = []interface{}{userID, cursor, limit + 1}
	}

	type userWithTime struct {
		model.UserSummary
		CreatedAt time.Time `db:"created_at"`
	}

	var results []userWithTime
	err := r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get followers: %w", err)
	}

	var users []model.UserSummary
	var nextCursor *time.Time

	// Cursor computation: If we got more than requested limit, there are more results.
	// Trim to limit and use the last item's timestamp as the next cursor.
	if len(results) > limit {
		results = results[:limit]
		nextCursor = &results[len(results)-1].CreatedAt
	}

	for _, result := range results {
		users = append(users, result.UserSummary)
	}

	return users, nextCursor, nil
}

// GetFollowing retrieves users that the specified user follows with cursor-based pagination.
// See GetFollowers documentation for detailed explanation of cursor pagination approach.
func (r *followRepository) GetFollowing(ctx context.Context, userID int64, cursor *time.Time, limit int) ([]model.UserSummary, *time.Time, error) {
	var query string
	var args []interface{}

	if cursor == nil {
		query = `
			SELECT u.id, u.username, u.display_name, u.avatar_url, f.created_at
			FROM follows f
			JOIN users u ON u.id = f.followee_id
			WHERE f.follower_id = $1
			ORDER BY f.created_at DESC
			LIMIT $2
		`
		args = []interface{}{userID, limit + 1}
	} else {
		query = `
			SELECT u.id, u.username, u.display_name, u.avatar_url, f.created_at
			FROM follows f
			JOIN users u ON u.id = f.followee_id
			WHERE f.follower_id = $1 AND f.created_at < $2
			ORDER BY f.created_at DESC
			LIMIT $3
		`
		args = []interface{}{userID, cursor, limit + 1}
	}

	type userWithTime struct {
		model.UserSummary
		CreatedAt time.Time `db:"created_at"`
	}

	var results []userWithTime
	err := r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get following: %w", err)
	}

	var users []model.UserSummary
	var nextCursor *time.Time

	if len(results) > limit {
		results = results[:limit]
		nextCursor = &results[len(results)-1].CreatedAt
	}

	for _, result := range results {
		users = append(users, result.UserSummary)
	}

	return users, nextCursor, nil
}

func (r *followRepository) CheckFollows(ctx context.Context, followerID int64, followeeIDs []int64) (map[int64]bool, error) {
	if len(followeeIDs) == 0 {
		return make(map[int64]bool), nil
	}

	query := `SELECT followee_id FROM follows WHERE follower_id = $1 AND followee_id = ANY($2)`
	var followedIDs []int64
	err := r.db.SelectContext(ctx, &followedIDs, query, followerID, pq.Array(followeeIDs))
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check follows: %w", err)
	}

	result := make(map[int64]bool)
	for _, id := range followeeIDs {
		result[id] = false
	}
	for _, id := range followedIDs {
		result[id] = true
	}

	return result, nil
}
