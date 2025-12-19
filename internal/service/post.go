package service

import (
	"context"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"

	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/queue"
	"iamstagram_22520060/internal/repository"
)

type PostService struct {
	postRepo  repository.PostRepository
	userRepo  repository.UserRepository
	publisher queue.Publisher
	db        *sqlx.DB
}

func NewPostService(
	postRepo repository.PostRepository,
	userRepo repository.UserRepository,
	publisher queue.Publisher,
	db *sqlx.DB,
) *PostService {
	return &PostService{
		postRepo:  postRepo,
		userRepo:  userRepo,
		publisher: publisher,
		db:        db,
	}
}

// Create creates a new post and publishes an event for fan-out.
func (s *PostService) Create(ctx context.Context, userID int64, req model.CreatePostRequest) (*model.Post, error) {
	// Validate
	if len(req.MediaURLs) == 0 {
		return nil, model.ErrNoMediaProvided
	}
	if len(req.MediaURLs) > model.MaxPostMediaCount {
		return nil, model.ErrTooManyMedia
	}
	if req.Caption != nil && len(*req.Caption) > model.MaxPostCaptionLength {
		return nil, model.ErrCaptionTooLong
	}

	// Create post in DB
	post, err := s.postRepo.Create(ctx, userID, req.Caption, req.MediaURLs)
	if err != nil {
		return nil, fmt.Errorf("create post: %w", err)
	}

	// Publish event for async fan-out
	event := queue.NewPostCreatedEvent(post.ID, userID)
	msgID, err := s.publisher.Publish(ctx, queue.StreamFeed, event)
	if err != nil {
		// Log but don't fail - post is created, fan-out can be retried
		log.Printf("[PostService] Failed to publish PostCreated event: post=%d err=%v", post.ID, err)
	} else {
		log.Printf("[PostService] Published PostCreated: post=%d msgID=%s", post.ID, msgID)
	}

	// Fetch author info
	author, err := s.userRepo.GetByID(ctx, userID)
	if err == nil {
		post.Author = &model.UserSummary{
			ID:          author.ID,
			Username:    author.Username,
			DisplayName: author.DisplayName,
			AvatarURL:   author.AvatarURL,
		}
	}

	return post, nil
}

// GetByID retrieves a single post with full details.
func (s *PostService) GetByID(ctx context.Context, postID int64, viewerID *int64) (*model.Post, error) {
	post, err := s.postRepo.GetByID(ctx, postID)
	if err != nil {
		return nil, err
	}

	// Fetch author info
	author, err := s.userRepo.GetByID(ctx, post.UserID)
	if err == nil {
		post.Author = &model.UserSummary{
			ID:          author.ID,
			Username:    author.Username,
			DisplayName: author.DisplayName,
			AvatarURL:   author.AvatarURL,
		}
	}

	// Check if viewer liked this post
	if viewerID != nil {
		likeStatus, err := s.postRepo.CheckLikes(ctx, *viewerID, []int64{postID})
		if err != nil {
			log.Printf("[PostService] Failed to check like status: %v", err)
		} else {
			post.IsLiked = likeStatus[postID]
		}
	}

	return post, nil
}

// Delete soft-deletes a post and publishes an event to remove from feeds.
func (s *PostService) Delete(ctx context.Context, postID, userID int64) error {
	// Delete from DB (validates ownership)
	err := s.postRepo.Delete(ctx, postID, userID)
	if err != nil {
		return err
	}

	// Publish event for async removal from feeds
	event := queue.NewPostDeletedEvent(postID, userID)
	msgID, err := s.publisher.Publish(ctx, queue.StreamFeed, event)
	if err != nil {
		log.Printf("[PostService] Failed to publish PostDeleted event: post=%d err=%v", postID, err)
	} else {
		log.Printf("[PostService] Published PostDeleted: post=%d msgID=%s", postID, msgID)
	}

	return nil
}

// GetUserPosts retrieves post thumbnails for a user's profile.
func (s *PostService) GetUserPosts(ctx context.Context, userID int64, cursor *string, limit int) (*model.PostListResponse, error) {
	if limit <= 0 {
		limit = 12 // Default for 3x4 grid
	}
	if limit > 36 {
		limit = 36 // Max for reasonable page size
	}

	thumbnails, nextCursor, err := s.postRepo.GetUserThumbnails(ctx, userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("get user thumbnails: %w", err)
	}

	// Compute has_more: if we got exactly limit posts, there might be more
	hasMore := len(thumbnails) == limit && nextCursor != nil

	// Only include cursor when has_more is true
	var finalCursor *string
	if hasMore {
		finalCursor = nextCursor
	}

	return &model.PostListResponse{
		Posts:      thumbnails,
		NextCursor: finalCursor,
		HasMore:    hasMore,
	}, nil
}

// Like adds a like to a post. Uses transaction: insert like + increment counter.
func (s *PostService) Like(ctx context.Context, postID, userID int64) error {
	// Verify post exists first
	exists, err := s.postRepo.Exists(ctx, postID)
	if err != nil {
		return fmt.Errorf("check post exists: %w", err)
	}
	if !exists {
		return model.ErrPostNotFound
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert like (fails if already liked)
	if err := s.postRepo.Like(ctx, tx, postID, userID); err != nil {
		return err
	}

	// Increment like count
	if err := s.postRepo.IncrementLikeCount(ctx, tx, postID, 1); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	log.Printf("[PostService] User %d liked post %d", userID, postID)

	// Publish notification event (after commit, best-effort)
	if s.publisher != nil {
		authorID, err := s.postRepo.GetAuthorID(ctx, postID)
		if err == nil && authorID != userID {
			event := queue.NewPostLikedEvent(postID, userID, authorID)
			if _, err := s.publisher.Publish(ctx, queue.StreamFeed, event); err != nil {
				log.Printf("[PostService] Failed to publish PostLiked event: %v", err)
			}
		}
	}

	return nil
}

// Unlike removes a like from a post. Uses transaction: delete like + decrement counter.
func (s *PostService) Unlike(ctx context.Context, postID, userID int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete like (fails if not liked)
	if err := s.postRepo.Unlike(ctx, tx, postID, userID); err != nil {
		return err
	}

	// Decrement like count
	if err := s.postRepo.IncrementLikeCount(ctx, tx, postID, -1); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	log.Printf("[PostService] User %d unliked post %d", userID, postID)
	return nil
}

// GetPostLikers returns paginated list of users who liked a post.
func (s *PostService) GetPostLikers(ctx context.Context, postID int64, cursor *string, limit int) (*model.LikersListResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// Verify post exists
	exists, err := s.postRepo.Exists(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("check post exists: %w", err)
	}
	if !exists {
		return nil, model.ErrPostNotFound
	}

	users, nextCursor, err := s.postRepo.GetPostLikers(ctx, postID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("get post likers: %w", err)
	}

	hasMore := nextCursor != nil

	var finalCursor *string
	if hasMore {
		finalCursor = nextCursor
	}

	return &model.LikersListResponse{
		Users:      users,
		NextCursor: finalCursor,
		HasMore:    hasMore,
	}, nil
}

