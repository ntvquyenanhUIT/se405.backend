package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

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
}
