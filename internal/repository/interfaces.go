package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/cache"
	"iamstagram_22520060/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	Search(ctx context.Context, query string, limit int) ([]model.UserSummary, error)
	IncrementFollowerCount(ctx context.Context, tx *sqlx.Tx, userID int64, delta int) error
	IncrementFollowingCount(ctx context.Context, tx *sqlx.Tx, userID int64, delta int) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *model.RefreshToken) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
	Revoke(ctx context.Context, id string, replacedBy *string) error
	RevokeAllForUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context, olderThan time.Duration) (int64, error)
}

type FollowRepository interface {
	Create(ctx context.Context, tx *sqlx.Tx, followerID, followeeID int64) (bool, error)
	Delete(ctx context.Context, tx *sqlx.Tx, followerID, followeeID int64) error
	Exists(ctx context.Context, followerID, followeeID int64) (bool, error)
	GetFollowers(ctx context.Context, userID int64, cursor *time.Time, limit int) ([]model.UserSummary, *time.Time, error)
	GetFollowing(ctx context.Context, userID int64, cursor *time.Time, limit int) ([]model.UserSummary, *time.Time, error)
	CheckFollows(ctx context.Context, followerID int64, followeeIDs []int64) (map[int64]bool, error)
	// New methods for feed system
	GetFollowerIDs(ctx context.Context, userID int64) ([]int64, error)
	GetFolloweeIDs(ctx context.Context, userID int64) ([]int64, error)
}

type PostRepository interface {
	Create(ctx context.Context, userID int64, caption *string, mediaURLs []string) (*model.Post, error)
	GetByID(ctx context.Context, postID int64) (*model.Post, error)
	GetByIDs(ctx context.Context, postIDs []int64) ([]model.Post, error)
	Delete(ctx context.Context, postID, userID int64) error
	GetUserThumbnails(ctx context.Context, userID int64, cursor *string, limit int) ([]model.PostThumbnail, *string, error)
	GetRecentPostsByUser(ctx context.Context, userID int64, limit int) ([]cache.PostScore, error)
	GetFeedPostIDs(ctx context.Context, followeeIDs []int64, limit int) ([]cache.PostScore, error)
	GetAuthorID(ctx context.Context, postID int64) (int64, error)
	// CheckLikes checks which posts the user has liked
	CheckLikes(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error)
	// Like methods
	Like(ctx context.Context, tx *sqlx.Tx, postID, userID int64) error
	Unlike(ctx context.Context, tx *sqlx.Tx, postID, userID int64) error
	GetPostLikers(ctx context.Context, postID int64, cursor *string, limit int) ([]model.UserSummary, *string, error)
	IncrementLikeCount(ctx context.Context, tx *sqlx.Tx, postID int64, delta int) error
	IncrementCommentCount(ctx context.Context, tx *sqlx.Tx, postID int64, delta int) error
	// Exists checks if a post exists (not deleted)
	Exists(ctx context.Context, postID int64) (bool, error)
}

type CommentRepository interface {
	Create(ctx context.Context, tx *sqlx.Tx, postID, userID int64, content string, parentID *int64) (*model.Comment, error)
	Update(ctx context.Context, commentID, userID int64, content string) (*model.Comment, error)
	Delete(ctx context.Context, tx *sqlx.Tx, commentID, userID int64) (postID int64, err error)
	GetByPostID(ctx context.Context, postID int64, cursor *string, limit int) ([]model.Comment, *string, error)
	GetByID(ctx context.Context, commentID int64) (*model.Comment, error)
}

type NotificationRepository interface {
	// Create inserts a new notification
	Create(ctx context.Context, userID, actorID int64, notifType string, postID, commentID *int64) error
	// GetFollowNotifications returns non-aggregated follow notifications + unread count
	GetFollowNotifications(ctx context.Context, userID int64, limit int) ([]model.Notification, error, int)
	// GetAggregatedNotifications returns likes/comments grouped by post + unread count
	GetAggregatedNotifications(ctx context.Context, userID int64, limit int) ([]model.AggregatedNotification, error, int)
	// MarkAsRead marks specific notifications as read
	MarkAsRead(ctx context.Context, userID int64, notificationIDs []int64) error
	// MarkAllAsRead marks all notifications for a user as read
	MarkAllAsRead(ctx context.Context, userID int64) error
	// GetUnreadCount returns the count of unread notifications (kept for standalone use if needed)
	GetUnreadCount(ctx context.Context, userID int64) (int, error)
}

type DeviceTokenRepository interface {
	// Upsert creates or updates a device token for a user
	Upsert(ctx context.Context, userID int64, token, platform string) error
	// GetByUserID returns all device tokens for a user
	GetByUserID(ctx context.Context, userID int64) ([]model.DeviceToken, error)
	// Delete removes a device token
	Delete(ctx context.Context, token string) error
}
