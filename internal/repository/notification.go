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

type notificationRepository struct {
	db *sqlx.DB
}

func NewNotificationRepository(db *sqlx.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

// Create inserts a new notification.
func (r *notificationRepository) Create(ctx context.Context, userID, actorID int64, notifType string, postID, commentID *int64) error {
	query := `
		INSERT INTO notifications (user_id, actor_id, type, post_id, comment_id)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, userID, actorID, notifType, postID, commentID)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// GetFollowNotifications returns non-aggregated follow notifications with actor info.
func (r *notificationRepository) GetFollowNotifications(ctx context.Context, userID int64, limit int) ([]model.Notification, error, int) {
	query := `
		SELECT n.id, n.user_id, n.actor_id, n.type, n.post_id, n.comment_id, n.is_read, n.created_at,
		       u.id as "actor.id", u.username as "actor.username", 
		       u.display_name as "actor.display_name", u.avatar_url as "actor.avatar_url"
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		WHERE n.user_id = $1 AND n.type = 'follow'
		ORDER BY n.created_at DESC
		LIMIT $2
	`

	type notifRow struct {
		ID             int64     `db:"id"`
		UserID         int64     `db:"user_id"`
		ActorID        int64     `db:"actor_id"`
		Type           string    `db:"type"`
		PostID         *int64    `db:"post_id"`
		CommentID      *int64    `db:"comment_id"`
		IsRead         bool      `db:"is_read"`
		CreatedAt      time.Time `db:"created_at"`
		ActorIDJoined  int64     `db:"actor.id"`
		ActorUsername  string    `db:"actor.username"`
		ActorDisplay   *string   `db:"actor.display_name"`
		ActorAvatarURL *string   `db:"actor.avatar_url"`
	}

	var rows []notifRow
	err := r.db.SelectContext(ctx, &rows, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get follow notifications: %w", err), 0
	}

	notifications := make([]model.Notification, len(rows))
	unreadCount := 0
	for i, row := range rows {
		if !row.IsRead {
			unreadCount += 1
		}
		notifications[i] = model.Notification{
			ID:        row.ID,
			UserID:    row.UserID,
			ActorID:   row.ActorID,
			Type:      row.Type,
			PostID:    row.PostID,
			CommentID: row.CommentID,
			IsRead:    row.IsRead,
			CreatedAt: row.CreatedAt,
			Actor: &model.UserSummary{
				ID:          row.ActorIDJoined,
				Username:    row.ActorUsername,
				DisplayName: row.ActorDisplay,
				AvatarURL:   row.ActorAvatarURL,
			},
		}
	}

	return notifications, nil, unreadCount
}

// GetAggregatedNotifications returns likes/comments grouped by post.
func (r *notificationRepository) GetAggregatedNotifications(ctx context.Context, userID int64, limit int) ([]model.AggregatedNotification, error, int) {
	// First, get aggregated data grouped by type and post
	query := `
		SELECT 
			n.type,
			n.post_id,
			array_agg(n.actor_id ORDER BY n.created_at DESC) as actor_ids,
			COUNT(*) as total_count,
			MAX(n.created_at) as latest_at,
			bool_and(n.is_read) as is_read
		FROM notifications n
		WHERE n.user_id = $1 AND n.type IN ('like', 'comment')
		GROUP BY n.type, n.post_id
		ORDER BY latest_at DESC
		LIMIT $2
	`

	type aggRow struct {
		Type       string        `db:"type"`
		PostID     *int64        `db:"post_id"`
		ActorIDs   pq.Int64Array `db:"actor_ids"`
		TotalCount int           `db:"total_count"`
		LatestAt   time.Time     `db:"latest_at"`
		IsRead     bool          `db:"is_read"`
	}

	var rows []aggRow
	err := r.db.SelectContext(ctx, &rows, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get aggregated notifications: %w", err), 0
	}

	if len(rows) == 0 {
		return []model.AggregatedNotification{}, nil, 0
	}

	// Collect unique actor IDs to fetch user summaries and compute unreadCount
	actorIDSet := make(map[int64]bool)
	unreadCount := 0
	for _, row := range rows {
		// Only need first 3 actors for display
		for i, id := range row.ActorIDs {
			if i >= 3 {
				break
			}
			actorIDSet[id] = true
		}
		if !row.IsRead {
			unreadCount += row.TotalCount
		}
	}

	// Fetch user summaries for actors
	actorIDs := make([]int64, 0, len(actorIDSet))
	for id := range actorIDSet {
		actorIDs = append(actorIDs, id)
	}

	actorMap := make(map[int64]model.UserSummary)
	if len(actorIDs) > 0 {
		userQuery := `
			SELECT id, username, display_name, avatar_url
			FROM users
			WHERE id = ANY($1)
		`
		var users []model.UserSummary
		err = r.db.SelectContext(ctx, &users, userQuery, pq.Array(actorIDs))
		if err != nil {
			return nil, fmt.Errorf("get actors: %w", err), 0
		}
		for _, u := range users {
			actorMap[u.ID] = u
		}
	}

	// Build result
	result := make([]model.AggregatedNotification, len(rows))
	for i, row := range rows {
		// Get first 3 actors
		actors := make([]model.UserSummary, 0, 3)
		for j, id := range row.ActorIDs {
			if j >= 3 {
				break
			}
			if actor, ok := actorMap[id]; ok {
				actors = append(actors, actor)
			}
		}

		result[i] = model.AggregatedNotification{
			Type:       row.Type,
			PostID:     row.PostID,
			Actors:     actors,
			TotalCount: row.TotalCount,
			LatestAt:   row.LatestAt,
			IsRead:     row.IsRead,
		}
	}

	return result, nil, unreadCount
}

// MarkAsRead marks specific notifications as read.
func (r *notificationRepository) MarkAsRead(ctx context.Context, userID int64, notificationIDs []int64) error {
	if len(notificationIDs) == 0 {
		return nil
	}

	query := `
		UPDATE notifications
		SET is_read = true
		WHERE user_id = $1 AND id = ANY($2)
	`
	_, err := r.db.ExecContext(ctx, query, userID, pq.Array(notificationIDs))
	if err != nil {
		return fmt.Errorf("mark notifications as read: %w", err)
	}
	return nil
}

// MarkAllAsRead marks all notifications for a user as read.
func (r *notificationRepository) MarkAllAsRead(ctx context.Context, userID int64) error {
	query := `
		UPDATE notifications
		SET is_read = true
		WHERE user_id = $1 AND is_read = false
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("mark all notifications as read: %w", err)
	}
	return nil
}

// GetUnreadCount returns the count of unread notifications.
func (r *notificationRepository) GetUnreadCount(ctx context.Context, userID int64) (int, error) {
	query := `
		SELECT COUNT(*) FROM notifications
		WHERE user_id = $1 AND is_read = false
	`
	var count int
	err := r.db.GetContext(ctx, &count, query, userID)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("get unread count: %w", err)
	}
	return count, nil
}
