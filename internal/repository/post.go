package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"iamstagram_22520060/internal/cache"
	"iamstagram_22520060/internal/model"
)

type postRepository struct {
	db *sqlx.DB
}

func NewPostRepository(db *sqlx.DB) PostRepository {
	return &postRepository{db: db}
}

// Create inserts a new post and its media in a transaction.
func (r *postRepository) Create(ctx context.Context, userID int64, caption *string, mediaURLs []string) (*model.Post, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert post
	var post model.Post
	query := `
		INSERT INTO posts (user_id, caption)
		VALUES ($1, $2)
		RETURNING id, user_id, caption, like_count, comment_count, created_at, updated_at
	`
	err = tx.GetContext(ctx, &post, query, userID, caption)
	if err != nil {
		return nil, fmt.Errorf("insert post: %w", err)
	}

	// Insert media items
	if len(mediaURLs) > 0 {
		mediaQuery := `
			INSERT INTO post_details (post_id, media_url, media_type, position)
			VALUES ($1, $2, $3, $4)
			RETURNING id, post_id, media_url, media_type, position
		`
		post.Media = make([]model.PostMedia, len(mediaURLs))
		for i, url := range mediaURLs {
			var media model.PostMedia
			mediaType := "image" // Default; could detect from URL or content-type
			err = tx.GetContext(ctx, &media, mediaQuery, post.ID, url, mediaType, i)
			if err != nil {
				return nil, fmt.Errorf("insert media %d: %w", i, err)
			}
			post.Media[i] = media
		}
	}

	// Increment user's post count
	_, err = tx.ExecContext(ctx, `UPDATE users SET post_count = post_count + 1 WHERE id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("increment post count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &post, nil
}

// GetByID retrieves a single post with its media.
func (r *postRepository) GetByID(ctx context.Context, postID int64) (*model.Post, error) {
	query := `
		SELECT id, user_id, caption, like_count, comment_count, created_at, updated_at
		FROM posts
		WHERE id = $1 AND deleted_at IS NULL
	`
	var post model.Post
	err := r.db.GetContext(ctx, &post, query, postID)
	if err == sql.ErrNoRows {
		return nil, model.ErrPostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}

	// Fetch media
	media, err := r.getPostMedia(ctx, []int64{postID})
	if err != nil {
		return nil, err
	}
	post.Media = media[postID]

	return &post, nil
}

// Delete performs a soft delete on a post.
func (r *postRepository) Delete(ctx context.Context, postID, userID int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify ownership and soft delete
	result, err := tx.ExecContext(ctx, `
		UPDATE posts SET deleted_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, postID, userID)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		// Check if post exists but belongs to different user
		var exists bool
		r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM posts WHERE id = $1 AND deleted_at IS NULL)`, postID)
		if exists {
			return model.ErrNotPostOwner
		}
		return model.ErrPostNotFound
	}

	// Decrement user's post count
	_, err = tx.ExecContext(ctx, `UPDATE users SET post_count = post_count - 1 WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("decrement post count: %w", err)
	}

	return tx.Commit()
}

// GetByIDs retrieves multiple posts by their IDs with media.
// Used for hydrating feed from cache.
func (r *postRepository) GetByIDs(ctx context.Context, postIDs []int64) ([]model.Post, error) {
	if len(postIDs) == 0 {
		return []model.Post{}, nil
	}

	query := `
		SELECT id, user_id, caption, like_count, comment_count, created_at, updated_at
		FROM posts
		WHERE id = ANY($1) AND deleted_at IS NULL
	`
	var posts []model.Post
	err := r.db.SelectContext(ctx, &posts, query, pq.Array(postIDs))
	if err != nil {
		return nil, fmt.Errorf("get posts by ids: %w", err)
	}

	// Fetch media for all posts
	mediaMap, err := r.getPostMedia(ctx, postIDs)
	if err != nil {
		return nil, err
	}
	for i := range posts {
		posts[i].Media = mediaMap[posts[i].ID]
	}

	// Re-order posts to match input order (important for feed ordering)
	postsMap := make(map[int64]model.Post, len(posts))
	for _, p := range posts {
		postsMap[p.ID] = p
	}
	ordered := make([]model.Post, 0, len(postIDs))
	for _, id := range postIDs {
		if p, ok := postsMap[id]; ok {
			ordered = append(ordered, p)
		}
	}

	return ordered, nil
}

// GetUserThumbnails retrieves post thumbnails for a user's profile grid.
func (r *postRepository) GetUserThumbnails(ctx context.Context, userID int64, cursor *string, limit int) ([]model.PostThumbnail, *string, error) {
	var query string
	var args []interface{}

	// Parse compound cursor: "timestamp_id"
	if cursor == nil {
		query = `
			SELECT p.id, 
				   (SELECT media_url FROM post_details WHERE post_id = p.id ORDER BY position LIMIT 1) as thumbnail_url,
				   (SELECT COUNT(*) FROM post_details WHERE post_id = p.id) as media_count
			FROM posts p
			WHERE p.user_id = $1 AND p.deleted_at IS NULL
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $2
		`
		args = []interface{}{userID, limit + 1}
	} else {
		// Parse cursor "timestamp_id"
		ts, id, err := parseCursor(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		query = `
			SELECT p.id,
				   (SELECT media_url FROM post_details WHERE post_id = p.id ORDER BY position LIMIT 1) as thumbnail_url,
				   (SELECT COUNT(*) FROM post_details WHERE post_id = p.id) as media_count
			FROM posts p
			WHERE p.user_id = $1 AND p.deleted_at IS NULL
			  AND (p.created_at, p.id) < ($2, $3)
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $4
		`
		args = []interface{}{userID, ts, id, limit + 1}
	}

	var thumbnails []model.PostThumbnail
	err := r.db.SelectContext(ctx, &thumbnails, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("get thumbnails: %w", err)
	}

	// Check if there's more
	var nextCursor *string
	if len(thumbnails) > limit {
		thumbnails = thumbnails[:limit]
		last := thumbnails[len(thumbnails)-1]
		// Need to get created_at for cursor - fetch it
		var createdAt time.Time
		r.db.GetContext(ctx, &createdAt, `SELECT created_at FROM posts WHERE id = $1`, last.ID)
		c := formatCursor(createdAt, last.ID)
		nextCursor = &c
	}

	return thumbnails, nextCursor, nil
}

// GetRecentPostsByUser returns recent posts by a user (for follow backfill).
// Returns PostScore slice for cache warming.
func (r *postRepository) GetRecentPostsByUser(ctx context.Context, userID int64, limit int) ([]cache.PostScore, error) {
	query := `
		SELECT id, EXTRACT(EPOCH FROM created_at)::bigint as timestamp
		FROM posts
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`
	type row struct {
		ID        int64 `db:"id"`
		Timestamp int64 `db:"timestamp"`
	}
	var rows []row
	err := r.db.SelectContext(ctx, &rows, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent posts: %w", err)
	}

	posts := make([]cache.PostScore, len(rows))
	for i, r := range rows {
		posts[i] = cache.PostScore{PostID: r.ID, Timestamp: r.Timestamp}
	}
	return posts, nil
}

// GetFeedPostIDs returns post IDs from all followees for cache warming.
// Fetches up to `limit` posts ordered by created_at DESC.
func (r *postRepository) GetFeedPostIDs(ctx context.Context, followeeIDs []int64, limit int) ([]cache.PostScore, error) {
	if len(followeeIDs) == 0 {
		return []cache.PostScore{}, nil
	}

	query := `
		SELECT id, EXTRACT(EPOCH FROM created_at)::bigint as timestamp
		FROM posts
		WHERE user_id = ANY($1) AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`
	type row struct {
		ID        int64 `db:"id"`
		Timestamp int64 `db:"timestamp"`
	}
	var rows []row
	err := r.db.SelectContext(ctx, &rows, query, pq.Array(followeeIDs), limit)
	if err != nil {
		return nil, fmt.Errorf("get feed post ids: %w", err)
	}

	posts := make([]cache.PostScore, len(rows))
	for i, r := range rows {
		posts[i] = cache.PostScore{PostID: r.ID, Timestamp: r.Timestamp}
	}
	return posts, nil
}

// GetAuthorID returns the author of a post (for event publishing).
func (r *postRepository) GetAuthorID(ctx context.Context, postID int64) (int64, error) {
	var authorID int64
	err := r.db.GetContext(ctx, &authorID, `SELECT user_id FROM posts WHERE id = $1`, postID)
	if err == sql.ErrNoRows {
		return 0, model.ErrPostNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get author id: %w", err)
	}
	return authorID, nil
}

// CheckLikes checks which posts the user has liked.
// Returns a map of post_id -> liked (true/false).
func (r *postRepository) CheckLikes(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error) {
	if len(postIDs) == 0 {
		return make(map[int64]bool), nil
	}

	query := `SELECT post_id FROM post_likes WHERE user_id = $1 AND post_id = ANY($2)`
	var likedIDs []int64
	err := r.db.SelectContext(ctx, &likedIDs, query, userID, pq.Array(postIDs))
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("check likes: %w", err)
	}

	result := make(map[int64]bool)
	for _, id := range postIDs {
		result[id] = false
	}
	for _, id := range likedIDs {
		result[id] = true
	}

	return result, nil
}

// Like inserts a like record. Returns ErrAlreadyLiked if duplicate.
func (r *postRepository) Like(ctx context.Context, tx *sqlx.Tx, postID, userID int64) error {
	query := `INSERT INTO post_likes (post_id, user_id) VALUES ($1, $2)`
	_, err := tx.ExecContext(ctx, query, postID, userID)
	if err != nil {
		// Check for unique constraint violation
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return model.ErrAlreadyLiked
		}
		return fmt.Errorf("insert like: %w", err)
	}
	return nil
}

// Unlike deletes a like record. Returns ErrNotLiked if not found.
func (r *postRepository) Unlike(ctx context.Context, tx *sqlx.Tx, postID, userID int64) error {
	query := `DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2`
	result, err := tx.ExecContext(ctx, query, postID, userID)
	if err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return model.ErrNotLiked
	}
	return nil
}

// GetPostLikers returns paginated users who liked a post.
func (r *postRepository) GetPostLikers(ctx context.Context, postID int64, cursor *string, limit int) ([]model.UserSummary, *string, error) {
	var query string
	var args []interface{}

	if cursor == nil {
		query = `
			SELECT u.id, u.username, u.display_name, u.avatar_url
			FROM post_likes pl
			JOIN users u ON u.id = pl.user_id
			WHERE pl.post_id = $1
			ORDER BY pl.created_at DESC, pl.id DESC
			LIMIT $2
		`
		args = []interface{}{postID, limit + 1}
	} else {
		ts, id, err := parseCursor(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		query = `
			SELECT u.id, u.username, u.display_name, u.avatar_url
			FROM post_likes pl
			JOIN users u ON u.id = pl.user_id
			WHERE pl.post_id = $1 AND (pl.created_at, pl.id) < ($2, $3)
			ORDER BY pl.created_at DESC, pl.id DESC
			LIMIT $4
		`
		args = []interface{}{postID, ts, id, limit + 1}
	}

	var users []model.UserSummary
	err := r.db.SelectContext(ctx, &users, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("get post likers: %w", err)
	}

	// Check for more and build cursor
	var nextCursor *string
	if len(users) > limit {
		users = users[:limit]
		// Need to get the like's created_at and id for cursor
		var likeInfo struct {
			ID        int64     `db:"id"`
			CreatedAt time.Time `db:"created_at"`
		}
		err := r.db.GetContext(ctx, &likeInfo, `
			SELECT id, created_at FROM post_likes 
			WHERE post_id = $1 AND user_id = $2
		`, postID, users[len(users)-1].ID)
		if err == nil {
			c := formatCursor(likeInfo.CreatedAt, likeInfo.ID)
			nextCursor = &c
		}
	}

	return users, nextCursor, nil
}

// IncrementLikeCount atomically updates the like_count on a post.
func (r *postRepository) IncrementLikeCount(ctx context.Context, tx *sqlx.Tx, postID int64, delta int) error {
	query := `UPDATE posts SET like_count = like_count + $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	result, err := tx.ExecContext(ctx, query, delta, postID)
	if err != nil {
		return fmt.Errorf("update like count: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return model.ErrPostNotFound
	}
	return nil
}

// IncrementCommentCount atomically updates the comment_count on a post.
func (r *postRepository) IncrementCommentCount(ctx context.Context, tx *sqlx.Tx, postID int64, delta int) error {
	query := `UPDATE posts SET comment_count = comment_count + $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	result, err := tx.ExecContext(ctx, query, delta, postID)
	if err != nil {
		return fmt.Errorf("update comment count: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return model.ErrPostNotFound
	}
	return nil
}

// Exists checks if a post exists and is not deleted.
func (r *postRepository) Exists(ctx context.Context, postID int64) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM posts WHERE id = $1 AND deleted_at IS NULL)`, postID)
	if err != nil {
		return false, fmt.Errorf("check post exists: %w", err)
	}
	return exists, nil
}

// Helper: fetch media for multiple posts in one query
func (r *postRepository) getPostMedia(ctx context.Context, postIDs []int64) (map[int64][]model.PostMedia, error) {
	if len(postIDs) == 0 {
		return map[int64][]model.PostMedia{}, nil
	}

	query := `
		SELECT id, post_id, media_url, media_type, position
		FROM post_details
		WHERE post_id = ANY($1)
		ORDER BY post_id, position
	`
	var media []model.PostMedia
	err := r.db.SelectContext(ctx, &media, query, pq.Array(postIDs))
	if err != nil {
		return nil, fmt.Errorf("get post media: %w", err)
	}

	// Group by post_id
	result := make(map[int64][]model.PostMedia)
	for _, m := range media {
		result[m.PostID] = append(result[m.PostID], m)
	}
	return result, nil
}

// Helper: parse compound cursor "id:timestamp" (unified format)
func parseCursor(cursor string) (time.Time, int64, error) {
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

// Helper: format compound cursor "id:timestamp" (unified format)
func formatCursor(t time.Time, id int64) string {
	return fmt.Sprintf("%d:%d", id, t.Unix())
}

