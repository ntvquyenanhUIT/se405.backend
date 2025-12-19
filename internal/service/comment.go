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

type CommentService struct {
	commentRepo repository.CommentRepository
	postRepo    repository.PostRepository
	userRepo    repository.UserRepository
	db          *sqlx.DB
	publisher   queue.Publisher
}

func NewCommentService(
	commentRepo repository.CommentRepository,
	postRepo repository.PostRepository,
	userRepo repository.UserRepository,
	db *sqlx.DB,
	publisher queue.Publisher,
) *CommentService {
	return &CommentService{
		commentRepo: commentRepo,
		postRepo:    postRepo,
		userRepo:    userRepo,
		db:          db,
		publisher:   publisher,
	}
}

// Create adds a comment to a post. Uses transaction: insert comment + increment counter.
func (s *CommentService) Create(ctx context.Context, postID, userID int64, req model.CreateCommentRequest) (*model.Comment, error) {
	// Validate content
	if len(req.Content) == 0 {
		return nil, model.ErrContentRequired
	}
	if len(req.Content) > model.MaxCommentLength {
		return nil, model.ErrContentTooLong
	}

	// Verify post exists
	exists, err := s.postRepo.Exists(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("check post exists: %w", err)
	}
	if !exists {
		return nil, model.ErrPostNotFound
	}

	// If parent comment provided, verify it exists and belongs to same post
	// Facebook-style: if replying to a reply, flatten to top-level and prepend @mention
	var actualParentID *int64 = req.ParentCommentID
	if req.ParentCommentID != nil {
		parent, err := s.commentRepo.GetByID(ctx, *req.ParentCommentID)
		if err != nil {
			return nil, err // ErrCommentNotFound or wrapped error
		}
		if parent.PostID != postID {
			return nil, fmt.Errorf("parent comment does not belong to this post")
		}

		// If parent is already a reply, flatten: reply to the top-level comment instead
		if parent.ParentCommentID != nil {
			actualParentID = parent.ParentCommentID

			// Prepend @username mention to the content
			parentAuthor, err := s.userRepo.GetByID(ctx, parent.UserID)
			if err == nil {
				req.Content = "@" + parentAuthor.Username + " " + req.Content
			}
		}
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert comment (use actualParentID which may be flattened)
	comment, err := s.commentRepo.Create(ctx, tx, postID, userID, req.Content, actualParentID)
	if err != nil {
		return nil, err
	}

	// Increment comment count
	if err := s.postRepo.IncrementCommentCount(ctx, tx, postID, 1); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Fetch author info
	author, err := s.userRepo.GetByID(ctx, userID)
	if err == nil {
		comment.Author = &model.UserSummary{
			ID:          author.ID,
			Username:    author.Username,
			DisplayName: author.DisplayName,
			AvatarURL:   author.AvatarURL,
		}
	}

	log.Printf("[CommentService] User %d commented on post %d", userID, postID)

	// Publish notification event (after commit, best-effort)
	if s.publisher != nil {
		authorID, err := s.postRepo.GetAuthorID(ctx, postID)
		if err == nil && authorID != userID {
			event := queue.NewPostCommentedEvent(postID, comment.ID, userID, authorID)
			if _, err := s.publisher.Publish(ctx, queue.StreamFeed, event); err != nil {
				log.Printf("[CommentService] Failed to publish PostCommented event: %v", err)
			}
		}
	}

	return comment, nil
}

// Delete removes a comment. Uses transaction: delete comment + decrement counter.
func (s *CommentService) Delete(ctx context.Context, commentID, userID int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete comment (returns postID for counter update)
	postID, err := s.commentRepo.Delete(ctx, tx, commentID, userID)
	if err != nil {
		return err
	}

	// Decrement comment count
	if err := s.postRepo.IncrementCommentCount(ctx, tx, postID, -1); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	log.Printf("[CommentService] User %d deleted comment %d from post %d", userID, commentID, postID)
	return nil
}

// Update updates a comment's content.
func (s *CommentService) Update(ctx context.Context, commentID, userID int64, req model.UpdateCommentRequest) (*model.Comment, error) {
	// Validate content
	if len(req.Content) == 0 {
		return nil, model.ErrContentRequired
	}
	if len(req.Content) > model.MaxCommentLength {
		return nil, model.ErrContentTooLong
	}

	// Update comment (repository handles ownership check)
	comment, err := s.commentRepo.Update(ctx, commentID, userID, req.Content)
	if err != nil {
		return nil, err
	}

	// Fetch author info
	author, err := s.userRepo.GetByID(ctx, userID)
	if err == nil {
		comment.Author = &model.UserSummary{
			ID:          author.ID,
			Username:    author.Username,
			DisplayName: author.DisplayName,
			AvatarURL:   author.AvatarURL,
		}
	}

	log.Printf("[CommentService] User %d updated comment %d", userID, commentID)
	return comment, nil
}

// GetByPostID returns paginated comments for a post.
func (s *CommentService) GetByPostID(ctx context.Context, postID int64, cursor *string, limit int) (*model.CommentListResponse, error) {
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

	comments, nextCursor, err := s.commentRepo.GetByPostID(ctx, postID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("get comments: %w", err)
	}

	hasMore := nextCursor != nil

	var finalCursor *string
	if hasMore {
		finalCursor = nextCursor
	}

	return &model.CommentListResponse{
		Comments:   comments,
		NextCursor: finalCursor,
		HasMore:    hasMore,
	}, nil
}
