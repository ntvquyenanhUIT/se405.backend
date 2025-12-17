package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/queue"
	"iamstagram_22520060/internal/repository"
)

type FollowService struct {
	followRepo repository.FollowRepository
	userRepo   repository.UserRepository
	db         *sqlx.DB
	publisher  queue.Publisher
}

func NewFollowService(
	followRepo repository.FollowRepository,
	userRepo repository.UserRepository,
	db *sqlx.DB,
	publisher queue.Publisher,
) *FollowService {
	return &FollowService{
		followRepo: followRepo,
		userRepo:   userRepo,
		db:         db,
		publisher:  publisher,
	}
}

func (s *FollowService) Follow(ctx context.Context, followerID, followeeID int64) error {
	if followerID == followeeID {
		return model.ErrCannotFollowSelf
	}

	_, err := s.userRepo.GetByID(ctx, followeeID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	inserted, err := s.followRepo.Create(ctx, tx, followerID, followeeID)
	if err != nil {
		return err
	}

	if !inserted {
		return model.ErrAlreadyFollowing
	}

	if err := s.userRepo.IncrementFollowerCount(ctx, tx, followeeID, 1); err != nil {
		return err
	}

	if err := s.userRepo.IncrementFollowingCount(ctx, tx, followerID, 1); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Publish event for async backfill (after commit!)
	if s.publisher != nil {
		event := queue.NewUserFollowedEvent(followerID, followeeID)
		msgID, err := s.publisher.Publish(ctx, queue.StreamFeed, event)
		if err != nil {
			log.Printf("[FollowService] Failed to publish UserFollowed event: follower=%d followee=%d err=%v",
				followerID, followeeID, err)
		} else {
			log.Printf("[FollowService] Published UserFollowed: follower=%d followee=%d msgID=%s",
				followerID, followeeID, msgID)
		}
	}

	return nil
}

func (s *FollowService) Unfollow(ctx context.Context, followerID, followeeID int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := s.followRepo.Delete(ctx, tx, followerID, followeeID); err != nil {
		return err
	}

	if err := s.userRepo.IncrementFollowerCount(ctx, tx, followeeID, -1); err != nil {
		return err
	}

	if err := s.userRepo.IncrementFollowingCount(ctx, tx, followerID, -1); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Publish event for async removal (after commit!)
	if s.publisher != nil {
		event := queue.NewUserUnfollowedEvent(followerID, followeeID)
		msgID, err := s.publisher.Publish(ctx, queue.StreamFeed, event)
		if err != nil {
			log.Printf("[FollowService] Failed to publish UserUnfollowed event: follower=%d followee=%d err=%v",
				followerID, followeeID, err)
		} else {
			log.Printf("[FollowService] Published UserUnfollowed: follower=%d followee=%d msgID=%s",
				followerID, followeeID, msgID)
		}
	}

	return nil
}

// GetFollowers retrieves users who follow the specified user with cursor-based pagination.
//
// Cursor pagination explanation:
// - When cursor is nil: Fetch from the beginning (latest followers first)
// - When cursor is provided: Fetch followers created BEFORE that timestamp
// - We fetch limit+1 items to determine if there are more results
// - If we got more than limit, the last item's timestamp becomes the next cursor
//
// Design decision - Two-query approach (fetch users + enrich follow status):
// We first fetch the follower list, then batch-check if viewer follows them.
// Alternative would be a complex LEFT JOIN in SQL, but current approach:
//
//	✅ Simpler SQL queries (easier to maintain/debug)
//	✅ Graceful degradation (if CheckFollows fails, we still return users)
//	✅ Single batch query for follow status (not N+1, uses ANY($1) in WHERE clause)
//	⚠️ Two DB roundtrips vs one
//
// TODO: Profile with real-world data. If performance becomes an issue, consider
// rewriting with LEFT JOIN to reduce to single query.
func (s *FollowService) GetFollowers(ctx context.Context, userID int64, cursor *time.Time, limit int, viewerID *int64) (*model.FollowListResponse, error) {
	users, nextCursor, err := s.followRepo.GetFollowers(ctx, userID, cursor, limit)
	if err != nil {
		return nil, err
	}

	if viewerID != nil {
		users = s.enrichWithFollowStatus(ctx, *viewerID, users)
	}

	var nextCursorStr *string
	if nextCursor != nil {
		str := nextCursor.Format(time.RFC3339)
		nextCursorStr = &str
	}

	return &model.FollowListResponse{
		Users:      users,
		NextCursor: nextCursorStr,
		HasMore:    nextCursor != nil,
	}, nil
}

// GetFollowing retrieves users that the specified user follows with cursor-based pagination.
// See GetFollowers documentation for cursor pagination and design decision explanations.
func (s *FollowService) GetFollowing(ctx context.Context, userID int64, cursor *time.Time, limit int, viewerID *int64) (*model.FollowListResponse, error) {
	users, nextCursor, err := s.followRepo.GetFollowing(ctx, userID, cursor, limit)
	if err != nil {
		return nil, err
	}

	if viewerID != nil {
		users = s.enrichWithFollowStatus(ctx, *viewerID, users)
	}

	var nextCursorStr *string
	if nextCursor != nil {
		str := nextCursor.Format(time.RFC3339)
		nextCursorStr = &str
	}

	return &model.FollowListResponse{
		Users:      users,
		NextCursor: nextCursorStr,
		HasMore:    nextCursor != nil,
	}, nil
}

// enrichWithFollowStatus performs a BATCH check (not N+1!) to determine if the viewer
// follows each user in the list. It collects all user IDs and makes ONE database query
// using WHERE followee_id = ANY($1), then maps the results back to the user list.
//
// This prevents N+1 queries while keeping the code modular. If the batch check fails,
// we gracefully return users with is_following=false rather than failing the entire request.
func (s *FollowService) enrichWithFollowStatus(ctx context.Context, viewerID int64, users []model.UserSummary) []model.UserSummary {
	if len(users) == 0 {
		return users
	}

	userIDs := make([]int64, len(users))
	for i, user := range users {
		userIDs[i] = user.ID
	}

	followMap, err := s.followRepo.CheckFollows(ctx, viewerID, userIDs)
	if err != nil {
		return users
	}

	for i := range users {
		users[i].IsFollowing = followMap[users[i].ID]
	}

	return users
}
