package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"iamstagram_22520060/internal/cache"
	"iamstagram_22520060/internal/queue"
)

// FollowerProvider defines the interface for fetching followers.
// This abstracts the repository layer so workers don't depend on DB directly.
type FollowerProvider interface {
	// GetFollowerIDs returns all follower IDs for a given user.
	GetFollowerIDs(ctx context.Context, userID int64) ([]int64, error)
}

// RecentPostsProvider defines the interface for fetching recent posts.
// Used for backfilling feed when a user follows someone.
type RecentPostsProvider interface {
	// GetRecentPostsByUser returns recent posts by a user for backfilling.
	// Returns posts as (postID, timestamp) pairs.
	GetRecentPostsByUser(ctx context.Context, userID int64, limit int) ([]cache.PostScore, error)
}

// NotificationCreator defines the interface for creating notifications.
// This allows the worker to create notifications without depending on the service directly.
type NotificationCreator interface {
	// CreateNotification creates a notification and optionally sends push.
	CreateNotification(ctx context.Context, userID, actorID int64, notifType string, postID, commentID *int64) error
}

// Handler processes feed events from the queue.
type Handler struct {
	feedCache        cache.FeedCache
	followerProvider FollowerProvider
	postsProvider    RecentPostsProvider
	notifCreator     NotificationCreator // Can be nil if notifications not wired
}

// NewHandler creates a new event handler.
func NewHandler(
	feedCache cache.FeedCache,
	followerProvider FollowerProvider,
	postsProvider RecentPostsProvider,
) *Handler {
	return &Handler{
		feedCache:        feedCache,
		followerProvider: followerProvider,
		postsProvider:    postsProvider,
	}
}

// SetNotificationCreator sets the notification creator (optional, for notification events).
func (h *Handler) SetNotificationCreator(nc NotificationCreator) {
	h.notifCreator = nc
}

// HandleEvent routes an event to the appropriate handler based on type.
func (h *Handler) HandleEvent(ctx context.Context, event queue.FeedEvent) error {
	startTime := time.Now()
	var err error

	switch event.Type {
	case queue.EventPostCreated:
		err = h.handlePostCreated(ctx, event)
	case queue.EventPostDeleted:
		err = h.handlePostDeleted(ctx, event)
	case queue.EventUserFollowed:
		err = h.handleUserFollowed(ctx, event)
	case queue.EventUserUnfollowed:
		err = h.handleUserUnfollowed(ctx, event)
	// Notification events
	case queue.EventPostLiked:
		err = h.handlePostLiked(ctx, event)
	case queue.EventPostCommented:
		err = h.handlePostCommented(ctx, event)
	default:
		log.Printf("[Worker] Unknown event type: %s", event.Type)
		return fmt.Errorf("unknown event type: %s", event.Type)
	}

	if err != nil {
		log.Printf("[Worker] HandleEvent FAILED: type=%s duration=%v err=%v",
			event.Type, time.Since(startTime), err)
		return err
	}

	log.Printf("[Worker] HandleEvent OK: type=%s duration=%v", event.Type, time.Since(startTime))
	return nil
}

// handlePostCreated fans out a new post to all followers' feed caches.
func (h *Handler) handlePostCreated(ctx context.Context, event queue.FeedEvent) error {
	log.Printf("[Worker] PostCreated: post=%d author=%d", event.PostID, event.AuthorID)

	// Get all followers of the author
	followers, err := h.followerProvider.GetFollowerIDs(ctx, event.AuthorID)
	if err != nil {
		return fmt.Errorf("get followers: %w", err)
	}

	log.Printf("[Worker] PostCreated: fanning out to %d followers", len(followers))

	// Fan-out: add post to each follower's feed cache
	var failCount int
	for _, followerID := range followers {
		err := h.feedCache.AddPost(ctx, followerID, event.PostID, event.Timestamp)
		if err != nil {
			log.Printf("[Worker] PostCreated: failed to add to user=%d err=%v", followerID, err)
			failCount++
			// Continue with other followers - don't fail entire fan-out
		}
	}

	// Also add to author's own feed (they see their own posts)
	if err := h.feedCache.AddPost(ctx, event.AuthorID, event.PostID, event.Timestamp); err != nil {
		log.Printf("[Worker] PostCreated: failed to add to author's own feed err=%v", err)
	}

	log.Printf("[Worker] PostCreated DONE: post=%d fanout=%d failed=%d",
		event.PostID, len(followers)+1, failCount)

	return nil
}

// handlePostDeleted removes a post from all followers' feed caches.
func (h *Handler) handlePostDeleted(ctx context.Context, event queue.FeedEvent) error {
	log.Printf("[Worker] PostDeleted: post=%d author=%d", event.PostID, event.AuthorID)

	// Get all followers of the author
	followers, err := h.followerProvider.GetFollowerIDs(ctx, event.AuthorID)
	if err != nil {
		return fmt.Errorf("get followers: %w", err)
	}

	log.Printf("[Worker] PostDeleted: removing from %d followers' feeds", len(followers))

	// Remove from each follower's feed cache
	var failCount int
	for _, followerID := range followers {
		err := h.feedCache.RemovePost(ctx, followerID, event.PostID)
		if err != nil {
			log.Printf("[Worker] PostDeleted: failed to remove from user=%d err=%v", followerID, err)
			failCount++
		}
	}

	// Also remove from author's own feed
	if err := h.feedCache.RemovePost(ctx, event.AuthorID, event.PostID); err != nil {
		log.Printf("[Worker] PostDeleted: failed to remove from author's own feed err=%v", err)
	}

	log.Printf("[Worker] PostDeleted DONE: post=%d fanout=%d failed=%d",
		event.PostID, len(followers)+1, failCount)

	return nil
}

// handleUserFollowed backfills the follower's feed with followee's recent posts.
func (h *Handler) handleUserFollowed(ctx context.Context, event queue.FeedEvent) error {
	log.Printf("[Worker] UserFollowed: follower=%d followee=%d", event.FollowerID, event.FolloweeID)

	// Fetch recent posts from the followee
	const backfillLimit = 20 // How many recent posts to backfill
	posts, err := h.postsProvider.GetRecentPostsByUser(ctx, event.FolloweeID, backfillLimit)
	if err != nil {
		return fmt.Errorf("get recent posts: %w", err)
	}

	if len(posts) == 0 {
		log.Printf("[Worker] UserFollowed: followee=%d has no posts to backfill", event.FolloweeID)
		return nil
	}

	log.Printf("[Worker] UserFollowed: backfilling %d posts to follower=%d", len(posts), event.FollowerID)

	// Add each post to follower's feed
	var failCount int
	for _, p := range posts {
		err := h.feedCache.AddPost(ctx, event.FollowerID, p.PostID, p.Timestamp)
		if err != nil {
			log.Printf("[Worker] UserFollowed: failed to add post=%d err=%v", p.PostID, err)
			failCount++
		}
	}

	log.Printf("[Worker] UserFollowed DONE: follower=%d backfilled=%d failed=%d",
		event.FollowerID, len(posts), failCount)

	// Create follow notification for the followee
	if h.notifCreator != nil {
		err := h.notifCreator.CreateNotification(ctx, event.FolloweeID, event.FollowerID, "follow", nil, nil)
		if err != nil {
			log.Printf("[Worker] UserFollowed: failed to create notification: %v", err)
		} else {
			log.Printf("[Worker] UserFollowed: notification created for followee=%d", event.FolloweeID)
		}
	}

	return nil
}

// handleUserUnfollowed removes the followee's posts from the follower's feed.
func (h *Handler) handleUserUnfollowed(ctx context.Context, event queue.FeedEvent) error {
	log.Printf("[Worker] UserUnfollowed: follower=%d followee=%d", event.FollowerID, event.FolloweeID)

	// Fetch posts from the followee that might be in the follower's feed
	// We use a higher limit since we want to remove all their posts
	const removeLimit = 100
	posts, err := h.postsProvider.GetRecentPostsByUser(ctx, event.FolloweeID, removeLimit)
	if err != nil {
		return fmt.Errorf("get posts to remove: %w", err)
	}

	if len(posts) == 0 {
		log.Printf("[Worker] UserUnfollowed: followee=%d has no posts to remove", event.FolloweeID)
		return nil
	}

	log.Printf("[Worker] UserUnfollowed: removing %d posts from follower=%d", len(posts), event.FollowerID)

	// Remove each post from follower's feed
	var failCount int
	for _, p := range posts {
		err := h.feedCache.RemovePost(ctx, event.FollowerID, p.PostID)
		if err != nil {
			log.Printf("[Worker] UserUnfollowed: failed to remove post=%d err=%v", p.PostID, err)
			failCount++
		}
	}

	log.Printf("[Worker] UserUnfollowed DONE: follower=%d removed=%d failed=%d",
		event.FollowerID, len(posts), failCount)

	return nil
}

// handlePostLiked creates a notification for the post author when someone likes their post.
func (h *Handler) handlePostLiked(ctx context.Context, event queue.FeedEvent) error {
	log.Printf("[Worker] PostLiked: post=%d actor=%d recipient=%d", event.PostID, event.ActorID, event.RecipientID)

	if h.notifCreator == nil {
		log.Printf("[Worker] PostLiked: notification creator not set, skipping")
		return nil
	}

	// Don't notify if liking own post
	if event.ActorID == event.RecipientID {
		return nil
	}

	postID := event.PostID
	err := h.notifCreator.CreateNotification(ctx, event.RecipientID, event.ActorID, "like", &postID, nil)
	if err != nil {
		return fmt.Errorf("create like notification: %w", err)
	}

	log.Printf("[Worker] PostLiked DONE: notification created")
	return nil
}

// handlePostCommented creates a notification for the post author when someone comments.
func (h *Handler) handlePostCommented(ctx context.Context, event queue.FeedEvent) error {
	log.Printf("[Worker] PostCommented: post=%d actor=%d recipient=%d", event.PostID, event.ActorID, event.RecipientID)

	if h.notifCreator == nil {
		log.Printf("[Worker] PostCommented: notification creator not set, skipping")
		return nil
	}

	if event.ActorID == event.RecipientID {
		return nil
	}

	postID := event.PostID
	err := h.notifCreator.CreateNotification(ctx, event.RecipientID, event.ActorID, "comment", &postID, event.CommentID)
	if err != nil {
		return fmt.Errorf("create comment notification: %w", err)
	}

	log.Printf("[Worker] PostCommented DONE: notification created")
	return nil
}
