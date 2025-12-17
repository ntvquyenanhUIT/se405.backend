package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"iamstagram_22520060/internal/cache"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/repository"
)

const (
	// FeedDefaultLimit is the default number of posts per page
	FeedDefaultLimit = 10

	// FeedMaxLimit is the maximum number of posts per page
	FeedMaxLimit = 50

	// CacheWarmLimit is max posts to fetch when warming cache
	CacheWarmLimit = 500
)

type FeedService struct {
	feedCache  cache.FeedCache
	postRepo   repository.PostRepository
	followRepo repository.FollowRepository
	userRepo   repository.UserRepository
}

func NewFeedService(
	feedCache cache.FeedCache,
	postRepo repository.PostRepository,
	followRepo repository.FollowRepository,
	userRepo repository.UserRepository,
) *FeedService {
	return &FeedService{
		feedCache:  feedCache,
		postRepo:   postRepo,
		followRepo: followRepo,
		userRepo:   userRepo,
	}
}

// GetFeed retrieves the user's feed with cursor-based pagination.
//
// Flow:
// 1. Check if cache exists for user
// 2. If no cache -> warm it (fetch all posts from followees, up to 500)
// 3. Get post IDs from cache (using cursor if provided)
// 4. Hydrate: fetch full post details from DB
// 5. Build next cursor from last post
func (s *FeedService) GetFeed(ctx context.Context, userID int64, cursor *string, limit int) (*model.FeedResponse, error) {
	startTime := time.Now()

	// Validate/default limit
	if limit <= 0 {
		limit = FeedDefaultLimit
	}
	if limit > FeedMaxLimit {
		limit = FeedMaxLimit
	}

	// Step 1: Check cache existence
	exists, err := s.feedCache.Exists(ctx, userID)
	if err != nil {
		log.Printf("[FeedService] Cache check failed for user=%d: %v", userID, err)
		// Continue without cache - fall back to DB
	}

	// Step 2: Warm cache if needed
	if !exists {
		log.Printf("[FeedService] Cache miss for user=%d, warming...", userID)
		if err := s.warmCache(ctx, userID); err != nil {
			log.Printf("[FeedService] Cache warm failed for user=%d: %v", userID, err)
			// Continue - we'll fetch directly from DB
		}
	}

	// Step 3: Get post IDs from cache
	var cursorScore *float64
	if cursor != nil {
		score, _, err := parseFeedCursor(*cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		cursorScore = &score
	}

	postIDs, scores, err := s.feedCache.GetFeed(ctx, userID, cursorScore, limit)
	if err != nil {
		log.Printf("[FeedService] GetFeed cache error: %v", err)
		return nil, fmt.Errorf("get feed from cache: %w", err)
	}

	// If cache is empty/missing, try direct DB fetch
	if len(postIDs) == 0 {
		log.Printf("[FeedService] Empty feed for user=%d", userID)
		return &model.FeedResponse{Posts: []model.FeedPost{}}, nil
	}

	// Step 4: Hydrate posts from DB
	posts, err := s.hydratePosts(ctx, userID, postIDs)
	if err != nil {
		return nil, fmt.Errorf("hydrate posts: %w", err)
	}

	// Step 5: Build next cursor and check if there are more posts
	var nextCursor *string
	hasMore := len(posts) == limit // If we got exactly limit posts, there might be more
	if hasMore && len(scores) > 0 {
		lastPost := posts[len(posts)-1]
		lastScore := scores[len(scores)-1]
		c := formatFeedCursor(lastScore, lastPost.ID)
		nextCursor = &c
	}

	log.Printf("[FeedService] GetFeed OK: user=%d posts=%d hasMore=%v duration=%v",
		userID, len(posts), hasMore, time.Since(startTime))

	return &model.FeedResponse{
		Posts:      posts,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// warmCache populates the user's feed cache from DB.
func (s *FeedService) warmCache(ctx context.Context, userID int64) error {
	startTime := time.Now()

	// Get all followee IDs
	followeeIDs, err := s.followRepo.GetFolloweeIDs(ctx, userID)
	if err != nil {
		return fmt.Errorf("get followee ids: %w", err)
	}

	// Include user's own posts in their feed
	followeeIDs = append(followeeIDs, userID)

	if len(followeeIDs) == 0 {
		log.Printf("[FeedService] User %d follows no one, empty feed", userID)
		return nil
	}

	// Fetch all post IDs from followees (up to cache cap)
	posts, err := s.postRepo.GetFeedPostIDs(ctx, followeeIDs, CacheWarmLimit)
	if err != nil {
		return fmt.Errorf("get feed post ids: %w", err)
	}

	if len(posts) == 0 {
		log.Printf("[FeedService] No posts to warm for user=%d", userID)
		return nil
	}

	// Warm the cache
	if err := s.feedCache.WarmCache(ctx, userID, posts); err != nil {
		return fmt.Errorf("warm cache: %w", err)
	}

	log.Printf("[FeedService] Cache warmed: user=%d posts=%d duration=%v",
		userID, len(posts), time.Since(startTime))

	return nil
}

// hydratePosts fetches full post details and enriches with author info.
func (s *FeedService) hydratePosts(ctx context.Context, viewerID int64, postIDs []int64) ([]model.FeedPost, error) {
	// Fetch posts from DB
	posts, err := s.postRepo.GetByIDs(ctx, postIDs)
	if err != nil {
		return nil, fmt.Errorf("get posts by ids: %w", err)
	}

	// Collect unique author IDs
	authorIDSet := make(map[int64]struct{})
	for _, p := range posts {
		authorIDSet[p.UserID] = struct{}{}
	}
	authorIDs := make([]int64, 0, len(authorIDSet))
	for id := range authorIDSet {
		authorIDs = append(authorIDs, id)
	}

	// Fetch author details
	authors := make(map[int64]model.UserSummary)
	for _, authorID := range authorIDs {
		user, err := s.userRepo.GetByID(ctx, authorID)
		if err != nil {
			log.Printf("[FeedService] Failed to get author %d: %v", authorID, err)
			continue
		}
		authors[authorID] = model.UserSummary{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			AvatarURL:   user.AvatarURL,
		}
	}

	// Check if viewer follows these authors (for "following" indicator)
	followStatus, err := s.followRepo.CheckFollows(ctx, viewerID, authorIDs)
	if err != nil {
		log.Printf("[FeedService] Failed to check follows: %v", err)
	}

	// Check which posts the viewer has liked
	likeStatus, err := s.postRepo.CheckLikes(ctx, viewerID, postIDs)
	if err != nil {
		log.Printf("[FeedService] Failed to check likes: %v", err)
	}

	// Build feed posts
	feedPosts := make([]model.FeedPost, len(posts))
	for i, p := range posts {
		author := authors[p.UserID]
		if followStatus != nil {
			author.IsFollowing = followStatus[p.UserID]
		}
		if likeStatus != nil {
			p.IsLiked = likeStatus[p.ID]
		}
		feedPosts[i] = model.FeedPost{
			Post:   p,
			Author: author,
		}
	}

	return feedPosts, nil
}

// parseFeedCursor parses "id:timestamp" format cursor.
// Returns the timestamp (as score) and post ID.
func parseFeedCursor(cursor string) (float64, int64, error) {
	parts := strings.Split(cursor, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid cursor format, expected id:timestamp")
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid post id in cursor: %w", err)
	}

	score, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid timestamp in cursor: %w", err)
	}

	return score, id, nil
}

// formatFeedCursor creates "id:timestamp" format cursor.
func formatFeedCursor(score float64, id int64) string {
	return fmt.Sprintf("%d:%.0f", id, score)
}
