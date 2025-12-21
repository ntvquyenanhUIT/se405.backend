package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/model"
)

type commentRepository struct {
	db *sqlx.DB
}

func NewCommentRepository(db *sqlx.DB) CommentRepository {
	return &commentRepository{db: db}
}

// Create inserts a new comment. Uses transaction for atomic counter update.
func (r *commentRepository) Create(ctx context.Context, tx *sqlx.Tx, postID, userID int64, content string, parentID *int64) (*model.Comment, error) {
	query := `
		INSERT INTO post_comments (post_id, user_id, content, parent_comment_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, post_id, user_id, content, parent_comment_id, created_at
	`
	var comment model.Comment
	err := tx.GetContext(ctx, &comment, query, postID, userID, content, parentID)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}
	return &comment, nil
}

// Update updates a comment's content. Only the owner can update.
func (r *commentRepository) Update(ctx context.Context, commentID, userID int64, content string) (*model.Comment, error) {
	query := `
		UPDATE post_comments 
		SET content = $1
		WHERE id = $2 AND user_id = $3
		RETURNING id, post_id, user_id, content, parent_comment_id, created_at
	`
	var comment model.Comment
	err := r.db.GetContext(ctx, &comment, query, content, commentID, userID)
	if err == sql.ErrNoRows {
		// Check if comment exists but belongs to different user
		var exists bool
		r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM post_comments WHERE id = $1)`, commentID)
		if exists {
			return nil, model.ErrNotCommentOwner
		}
		return nil, model.ErrCommentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}
	return &comment, nil
}

// Delete removes a comment and all its replies (via ON DELETE CASCADE).
// Returns the postID and the total count of deleted comments for counter decrement.
// Only the comment owner can delete.
func (r *commentRepository) Delete(ctx context.Context, tx *sqlx.Tx, commentID, userID int64) (postID int64, deletedCount int, err error) {
	// First check ownership and get post_id
	var comment struct {
		PostID int64 `db:"post_id"`
		UserID int64 `db:"user_id"`
	}
	err = tx.GetContext(ctx, &comment, `
		SELECT post_id, user_id FROM post_comments WHERE id = $1
	`, commentID)
	if err == sql.ErrNoRows {
		return 0, 0, model.ErrCommentNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("get comment: %w", err)
	}

	// Check ownership
	if comment.UserID != userID {
		return 0, 0, model.ErrNotCommentOwner
	}

	// Count how many comments will be deleted (this comment + all replies)
	// This must be done BEFORE the delete since ON DELETE CASCADE will remove them
	err = tx.GetContext(ctx, &deletedCount, `
		SELECT COUNT(*) FROM post_comments 
		WHERE id = $1 OR parent_comment_id = $1
	`, commentID)
	if err != nil {
		return 0, 0, fmt.Errorf("count comments to delete: %w", err)
	}

	// Delete the comment (replies will be cascade-deleted by DB)
	_, err = tx.ExecContext(ctx, `
		DELETE FROM post_comments WHERE id = $1
	`, commentID)
	if err != nil {
		return 0, 0, fmt.Errorf("delete comment: %w", err)
	}

	return comment.PostID, deletedCount, nil
}

// GetByPostID returns paginated comments for a post.
func (r *commentRepository) GetByPostID(ctx context.Context, postID int64, cursor *string, limit int) ([]model.Comment, *string, error) {
	var query string
	var args []interface{}

	if cursor == nil {
		query = `
			SELECT c.id, c.post_id, c.user_id, c.content, c.parent_comment_id, c.created_at,
			       u.id as "author.id", u.username as "author.username", 
			       u.display_name as "author.display_name", u.avatar_url as "author.avatar_url"
			FROM post_comments c
			JOIN users u ON u.id = c.user_id
			WHERE c.post_id = $1
			ORDER BY c.created_at DESC, c.id DESC
			LIMIT $2
		`
		args = []interface{}{postID, limit + 1}
	} else {
		ts, id, err := parseCommentCursor(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		query = `
			SELECT c.id, c.post_id, c.user_id, c.content, c.parent_comment_id, c.created_at,
			       u.id as "author.id", u.username as "author.username", 
			       u.display_name as "author.display_name", u.avatar_url as "author.avatar_url"
			FROM post_comments c
			JOIN users u ON u.id = c.user_id
			WHERE c.post_id = $1 AND (c.created_at, c.id) < ($2, $3)
			ORDER BY c.created_at DESC, c.id DESC
			LIMIT $4
		`
		args = []interface{}{postID, ts, id, limit + 1}
	}

	// Use a struct that can scan the joined author data
	type commentRow struct {
		ID              int64     `db:"id"`
		PostID          int64     `db:"post_id"`
		UserID          int64     `db:"user_id"`
		Content         string    `db:"content"`
		ParentCommentID *int64    `db:"parent_comment_id"`
		CreatedAt       time.Time `db:"created_at"`
		AuthorID        int64     `db:"author.id"`
		AuthorUsername  string    `db:"author.username"`
		AuthorDisplay   *string   `db:"author.display_name"`
		AuthorAvatar    *string   `db:"author.avatar_url"`
	}

	var rows []commentRow
	err := r.db.SelectContext(ctx, &rows, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("get comments: %w", err)
	}

	// Convert to Comment with Author
	comments := make([]model.Comment, len(rows))
	for i, row := range rows {
		comments[i] = model.Comment{
			ID:              row.ID,
			PostID:          row.PostID,
			UserID:          row.UserID,
			Content:         row.Content,
			ParentCommentID: row.ParentCommentID,
			CreatedAt:       row.CreatedAt,
			Author: &model.UserSummary{
				ID:          row.AuthorID,
				Username:    row.AuthorUsername,
				DisplayName: row.AuthorDisplay,
				AvatarURL:   row.AuthorAvatar,
			},
		}
	}

	// Check for more and build cursor
	var nextCursor *string
	if len(comments) > limit {
		comments = comments[:limit]
		last := comments[len(comments)-1]
		c := formatCommentCursor(last.CreatedAt, last.ID)
		nextCursor = &c
	}

	return comments, nextCursor, nil
}

// GetByID retrieves a single comment.
func (r *commentRepository) GetByID(ctx context.Context, commentID int64) (*model.Comment, error) {
	query := `
		SELECT id, post_id, user_id, content, parent_comment_id, created_at
		FROM post_comments
		WHERE id = $1
	`
	var comment model.Comment
	err := r.db.GetContext(ctx, &comment, query, commentID)
	if err == sql.ErrNoRows {
		return nil, model.ErrCommentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get comment: %w", err)
	}
	return &comment, nil
}

// Helper: parse comment cursor "id:timestamp"
func parseCommentCursor(cursor string) (time.Time, int64, error) {
	parts := strings.Split(cursor, ":")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid cursor format")
	}
	var id int64
	var ts int64
	_, err := fmt.Sscanf(parts[0], "%d", &id)
	if err != nil {
		return time.Time{}, 0, err
	}
	_, err = fmt.Sscanf(parts[1], "%d", &ts)
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Unix(ts, 0), id, nil
}

// Helper: format comment cursor "id:timestamp"
func formatCommentCursor(t time.Time, id int64) string {
	return fmt.Sprintf("%d:%d", id, t.Unix())
}
