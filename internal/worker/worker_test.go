package worker_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"iamstagram_22520060/internal/cache"
	"iamstagram_22520060/internal/queue"
	"iamstagram_22520060/internal/worker"
)

// =============================================================================
// Mock Implementations
// =============================================================================

// MockFollowerProvider simulates the follower repository.
type MockFollowerProvider struct {
	// followers maps userID -> list of follower IDs
	followers map[int64][]int64
}

func NewMockFollowerProvider() *MockFollowerProvider {
	return &MockFollowerProvider{
		followers: make(map[int64][]int64),
	}
}

func (m *MockFollowerProvider) AddFollower(userID, followerID int64) {
	m.followers[userID] = append(m.followers[userID], followerID)
}

func (m *MockFollowerProvider) RemoveFollower(userID, followerID int64) {
	followers := m.followers[userID]
	for i, id := range followers {
		if id == followerID {
			m.followers[userID] = append(followers[:i], followers[i+1:]...)
			return
		}
	}
}

func (m *MockFollowerProvider) GetFollowerIDs(ctx context.Context, userID int64) ([]int64, error) {
	return m.followers[userID], nil
}

// MockPostsProvider simulates the posts repository.
type MockPostsProvider struct {
	// posts maps authorID -> list of (postID, timestamp)
	posts map[int64][]cache.PostScore
}

func NewMockPostsProvider() *MockPostsProvider {
	return &MockPostsProvider{
		posts: make(map[int64][]cache.PostScore),
	}
}

func (m *MockPostsProvider) AddPost(authorID, postID int64, timestamp int64) {
	m.posts[authorID] = append(m.posts[authorID], cache.PostScore{
		PostID:    postID,
		Timestamp: timestamp,
	})
}

func (m *MockPostsProvider) GetRecentPostsByUser(ctx context.Context, userID int64, limit int) ([]cache.PostScore, error) {
	posts := m.posts[userID]
	if len(posts) > limit {
		return posts[:limit], nil
	}
	return posts, nil
}

// =============================================================================
// Test Helpers
// =============================================================================

func setupTestRedis(t *testing.T) *redis.Client {
	// Connect to local Redis (adjust URL if needed)
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("Failed to parse Redis URL: %v", err)
	}

	// Use DB 1 for testing to avoid conflicts with dev data
	opts.DB = 1

	client := redis.NewClient(opts)

	// Verify connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
	}

	// Clean up test database
	client.FlushDB(ctx)

	return client
}

func cleanupTestRedis(client *redis.Client) {
	ctx := context.Background()
	client.FlushDB(ctx)
	client.Close()
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestPostCreatedFanout tests that when a user creates a post,
// it gets added to all followers' feeds.
func TestPostCreatedFanout(t *testing.T) {
	// Setup
	client := setupTestRedis(t)
	defer cleanupTestRedis(client)

	ctx := context.Background()
	feedCache := cache.NewFeedCache(client)
	mockFollowers := NewMockFollowerProvider()
	mockPosts := NewMockPostsProvider()
	handler := worker.NewHandler(feedCache, mockFollowers, mockPosts)

	// Scenario: User 1 (author) has 3 followers: User 2, 3, 4
	authorID := int64(1)
	follower2 := int64(2)
	follower3 := int64(3)
	follower4 := int64(4)

	mockFollowers.AddFollower(authorID, follower2)
	mockFollowers.AddFollower(authorID, follower3)
	mockFollowers.AddFollower(authorID, follower4)

	// User 1 creates a new post
	postID := int64(100)
	timestamp := time.Now().Unix()
	event := queue.FeedEvent{
		Type:      queue.EventPostCreated,
		PostID:    postID,
		AuthorID:  authorID,
		Timestamp: timestamp,
	}

	// Handle the event
	err := handler.HandleEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Verify: post should be in all followers' feeds AND author's own feed
	for _, userID := range []int64{authorID, follower2, follower3, follower4} {
		score, found, err := feedCache.GetScore(ctx, userID, postID)
		if err != nil {
			t.Fatalf("GetScore failed for user %d: %v", userID, err)
		}
		if !found {
			t.Errorf("Post %d not found in user %d's feed", postID, userID)
		}
		if score != timestamp {
			t.Errorf("Wrong timestamp for post %d in user %d's feed: got %d, want %d",
				postID, userID, score, timestamp)
		}
	}

	// Verify feed sizes
	for _, userID := range []int64{authorID, follower2, follower3, follower4} {
		size, _ := feedCache.Size(ctx, userID)
		if size != 1 {
			t.Errorf("User %d's feed size: got %d, want 1", userID, size)
		}
	}

	t.Log("✓ Post created fan-out works correctly")
}

// TestPostDeletedRemoval tests that when a user deletes a post,
// it gets removed from all followers' feeds.
func TestPostDeletedRemoval(t *testing.T) {
	// Setup
	client := setupTestRedis(t)
	defer cleanupTestRedis(client)

	ctx := context.Background()
	feedCache := cache.NewFeedCache(client)
	mockFollowers := NewMockFollowerProvider()
	mockPosts := NewMockPostsProvider()
	handler := worker.NewHandler(feedCache, mockFollowers, mockPosts)

	// Scenario: User 1 (author) has 2 followers
	authorID := int64(1)
	follower2 := int64(2)
	follower3 := int64(3)

	mockFollowers.AddFollower(authorID, follower2)
	mockFollowers.AddFollower(authorID, follower3)

	// Pre-populate: add a post to everyone's feed
	postID := int64(100)
	timestamp := time.Now().Unix()
	for _, userID := range []int64{authorID, follower2, follower3} {
		feedCache.AddPost(ctx, userID, postID, timestamp)
	}

	// Verify posts are there
	for _, userID := range []int64{authorID, follower2, follower3} {
		_, found, _ := feedCache.GetScore(ctx, userID, postID)
		if !found {
			t.Fatalf("Setup failed: post not in user %d's feed", userID)
		}
	}

	// User 1 deletes the post
	event := queue.FeedEvent{
		Type:      queue.EventPostDeleted,
		PostID:    postID,
		AuthorID:  authorID,
		Timestamp: time.Now().Unix(),
	}

	// Handle the event
	err := handler.HandleEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Verify: post should be removed from all feeds
	for _, userID := range []int64{authorID, follower2, follower3} {
		_, found, err := feedCache.GetScore(ctx, userID, postID)
		if err != nil {
			t.Fatalf("GetScore failed for user %d: %v", userID, err)
		}
		if found {
			t.Errorf("Post %d should have been removed from user %d's feed", postID, userID)
		}
	}

	t.Log("✓ Post deleted removal works correctly")
}

// TestUserFollowedBackfill tests that when a user follows someone,
// the followee's recent posts are backfilled into the follower's feed.
func TestUserFollowedBackfill(t *testing.T) {
	// Setup
	client := setupTestRedis(t)
	defer cleanupTestRedis(client)

	ctx := context.Background()
	feedCache := cache.NewFeedCache(client)
	mockFollowers := NewMockFollowerProvider()
	mockPosts := NewMockPostsProvider()
	handler := worker.NewHandler(feedCache, mockFollowers, mockPosts)

	// Scenario: User 2 follows User 1
	// User 1 has 3 existing posts
	followerID := int64(2)
	followeeID := int64(1)

	now := time.Now().Unix()
	mockPosts.AddPost(followeeID, 101, now-3600) // 1 hour ago
	mockPosts.AddPost(followeeID, 102, now-1800) // 30 min ago
	mockPosts.AddPost(followeeID, 103, now-600)  // 10 min ago

	// User 2's feed should be empty initially
	exists, _ := feedCache.Exists(ctx, followerID)
	if exists {
		t.Fatal("Setup failed: follower's feed should be empty initially")
	}

	// User 2 follows User 1
	event := queue.FeedEvent{
		Type:       queue.EventUserFollowed,
		FollowerID: followerID,
		FolloweeID: followeeID,
		Timestamp:  now,
	}

	// Handle the event
	err := handler.HandleEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Verify: all 3 posts should be in follower's feed
	size, _ := feedCache.Size(ctx, followerID)
	if size != 3 {
		t.Errorf("Follower's feed size: got %d, want 3", size)
	}

	for _, postID := range []int64{101, 102, 103} {
		_, found, err := feedCache.GetScore(ctx, followerID, postID)
		if err != nil {
			t.Fatalf("GetScore failed: %v", err)
		}
		if !found {
			t.Errorf("Post %d not found in follower's feed after follow", postID)
		}
	}

	t.Log("✓ User followed backfill works correctly")
}

// TestUserUnfollowedRemoval tests that when a user unfollows someone,
// the followee's posts are removed from the follower's feed.
func TestUserUnfollowedRemoval(t *testing.T) {
	// Setup
	client := setupTestRedis(t)
	defer cleanupTestRedis(client)

	ctx := context.Background()
	feedCache := cache.NewFeedCache(client)
	mockFollowers := NewMockFollowerProvider()
	mockPosts := NewMockPostsProvider()
	handler := worker.NewHandler(feedCache, mockFollowers, mockPosts)

	// Scenario: User 2 unfollows User 1
	// User 2's feed contains posts from User 1 and User 3
	followerID := int64(2)
	unfollowedID := int64(1)
	otherUserID := int64(3)

	now := time.Now().Unix()

	// User 1's posts (to be removed)
	mockPosts.AddPost(unfollowedID, 101, now-3600)
	mockPosts.AddPost(unfollowedID, 102, now-1800)

	// User 3's posts (should remain)
	post301 := int64(301)
	post302 := int64(302)
	mockPosts.AddPost(otherUserID, post301, now-2400)
	mockPosts.AddPost(otherUserID, post302, now-1200)

	// Pre-populate follower's feed with all posts
	feedCache.AddPost(ctx, followerID, 101, now-3600)
	feedCache.AddPost(ctx, followerID, 102, now-1800)
	feedCache.AddPost(ctx, followerID, post301, now-2400)
	feedCache.AddPost(ctx, followerID, post302, now-1200)

	// Verify setup: feed has 4 posts
	size, _ := feedCache.Size(ctx, followerID)
	if size != 4 {
		t.Fatalf("Setup failed: feed should have 4 posts, got %d", size)
	}

	// User 2 unfollows User 1
	event := queue.FeedEvent{
		Type:       queue.EventUserUnfollowed,
		FollowerID: followerID,
		FolloweeID: unfollowedID,
		Timestamp:  now,
	}

	// Handle the event
	err := handler.HandleEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Verify: User 1's posts should be removed
	for _, postID := range []int64{101, 102} {
		_, found, _ := feedCache.GetScore(ctx, followerID, postID)
		if found {
			t.Errorf("Post %d should have been removed from feed", postID)
		}
	}

	// Verify: User 3's posts should remain
	for _, postID := range []int64{post301, post302} {
		_, found, _ := feedCache.GetScore(ctx, followerID, postID)
		if !found {
			t.Errorf("Post %d should still be in feed", postID)
		}
	}

	// Verify: feed size is now 2
	size, _ = feedCache.Size(ctx, followerID)
	if size != 2 {
		t.Errorf("Feed size after unfollow: got %d, want 2", size)
	}

	t.Log("✓ User unfollowed removal works correctly")
}

// TestFullWorkflow tests a complete user journey through the feed system.
func TestFullWorkflow(t *testing.T) {
	// Setup
	client := setupTestRedis(t)
	defer cleanupTestRedis(client)

	ctx := context.Background()
	feedCache := cache.NewFeedCache(client)
	mockFollowers := NewMockFollowerProvider()
	mockPosts := NewMockPostsProvider()
	handler := worker.NewHandler(feedCache, mockFollowers, mockPosts)

	// ==========================================================================
	// Scenario: Instagram-like user journey
	// ==========================================================================
	// Users: Alice (1), Bob (2), Charlie (3)
	// 1. Bob follows Alice
	// 2. Alice creates 2 posts
	// 3. Charlie follows Alice
	// 4. Alice creates 1 more post
	// 5. Bob unfollows Alice
	// 6. Alice deletes her first post
	// ==========================================================================

	alice := int64(1)
	bob := int64(2)
	charlie := int64(3)
	now := time.Now().Unix()

	fmt.Println("\n========== FULL WORKFLOW TEST ==========")

	// Step 1: Bob follows Alice
	fmt.Println("\n--- Step 1: Bob follows Alice ---")
	mockFollowers.AddFollower(alice, bob)
	// Alice has no posts yet, so nothing to backfill
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:       queue.EventUserFollowed,
		FollowerID: bob,
		FolloweeID: alice,
		Timestamp:  now,
	})
	bobSize, _ := feedCache.Size(ctx, bob)
	fmt.Printf("Bob's feed size: %d (expected: 0)\n", bobSize)

	// Step 2: Alice creates 2 posts
	fmt.Println("\n--- Step 2: Alice creates 2 posts ---")
	post1 := int64(1001)
	post2 := int64(1002)
	ts1 := now + 100
	ts2 := now + 200

	mockPosts.AddPost(alice, post1, ts1)
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:      queue.EventPostCreated,
		PostID:    post1,
		AuthorID:  alice,
		Timestamp: ts1,
	})

	mockPosts.AddPost(alice, post2, ts2)
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:      queue.EventPostCreated,
		PostID:    post2,
		AuthorID:  alice,
		Timestamp: ts2,
	})

	aliceSize, _ := feedCache.Size(ctx, alice)
	bobSize, _ = feedCache.Size(ctx, bob)
	fmt.Printf("Alice's feed size: %d (expected: 2 - sees own posts)\n", aliceSize)
	fmt.Printf("Bob's feed size: %d (expected: 2)\n", bobSize)

	// Step 3: Charlie follows Alice
	fmt.Println("\n--- Step 3: Charlie follows Alice ---")
	mockFollowers.AddFollower(alice, charlie)
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:       queue.EventUserFollowed,
		FollowerID: charlie,
		FolloweeID: alice,
		Timestamp:  now + 300,
	})

	charlieSize, _ := feedCache.Size(ctx, charlie)
	fmt.Printf("Charlie's feed size: %d (expected: 2 - backfilled)\n", charlieSize)

	// Step 4: Alice creates 1 more post
	fmt.Println("\n--- Step 4: Alice creates another post ---")
	post3 := int64(1003)
	ts3 := now + 400

	mockPosts.AddPost(alice, post3, ts3)
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:      queue.EventPostCreated,
		PostID:    post3,
		AuthorID:  alice,
		Timestamp: ts3,
	})

	aliceSize, _ = feedCache.Size(ctx, alice)
	bobSize, _ = feedCache.Size(ctx, bob)
	charlieSize, _ = feedCache.Size(ctx, charlie)
	fmt.Printf("Alice's feed: %d, Bob's feed: %d, Charlie's feed: %d (all expected: 3)\n",
		aliceSize, bobSize, charlieSize)

	// Step 5: Bob unfollows Alice
	fmt.Println("\n--- Step 5: Bob unfollows Alice ---")
	mockFollowers.RemoveFollower(alice, bob)
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:       queue.EventUserUnfollowed,
		FollowerID: bob,
		FolloweeID: alice,
		Timestamp:  now + 500,
	})

	bobSize, _ = feedCache.Size(ctx, bob)
	fmt.Printf("Bob's feed size: %d (expected: 0 - all Alice's posts removed)\n", bobSize)

	// Step 6: Alice deletes her first post
	fmt.Println("\n--- Step 6: Alice deletes first post ---")
	handler.HandleEvent(ctx, queue.FeedEvent{
		Type:      queue.EventPostDeleted,
		PostID:    post1,
		AuthorID:  alice,
		Timestamp: now + 600,
	})

	aliceSize, _ = feedCache.Size(ctx, alice)
	charlieSize, _ = feedCache.Size(ctx, charlie)
	fmt.Printf("Alice's feed: %d, Charlie's feed: %d (both expected: 2)\n", aliceSize, charlieSize)

	// Final verification
	fmt.Println("\n--- Final State ---")
	_, post1InAlice, _ := feedCache.GetScore(ctx, alice, post1)
	_, post2InAlice, _ := feedCache.GetScore(ctx, alice, post2)
	_, post3InAlice, _ := feedCache.GetScore(ctx, alice, post3)
	fmt.Printf("Alice's feed: post1=%v, post2=%v, post3=%v\n", post1InAlice, post2InAlice, post3InAlice)

	_, post1InCharlie, _ := feedCache.GetScore(ctx, charlie, post1)
	_, post2InCharlie, _ := feedCache.GetScore(ctx, charlie, post2)
	_, post3InCharlie, _ := feedCache.GetScore(ctx, charlie, post3)
	fmt.Printf("Charlie's feed: post1=%v, post2=%v, post3=%v\n", post1InCharlie, post2InCharlie, post3InCharlie)

	bobExists, _ := feedCache.Exists(ctx, bob)
	fmt.Printf("Bob's feed exists: %v (expected: false)\n", bobExists)

	fmt.Println("\n========== END WORKFLOW TEST ==========")

	// Assertions
	if aliceSize != 2 {
		t.Errorf("Alice's final feed size: got %d, want 2", aliceSize)
	}
	if charlieSize != 2 {
		t.Errorf("Charlie's final feed size: got %d, want 2", charlieSize)
	}
	if bobExists {
		t.Error("Bob's feed should not exist after unfollowing")
	}
	if post1InAlice || post1InCharlie {
		t.Error("Deleted post1 should not be in any feed")
	}

	t.Log("✓ Full workflow test passed")
}

// =============================================================================
// Stream + Worker Integration Test
// =============================================================================

// TestStreamToWorkerIntegration tests the complete flow:
// Publisher -> Stream -> Consumer -> Handler -> Cache
func TestStreamToWorkerIntegration(t *testing.T) {
	// Setup
	client := setupTestRedis(t)
	defer cleanupTestRedis(client)

	ctx := context.Background()

	// Create all components
	feedCache := cache.NewFeedCache(client)
	publisher := queue.NewPublisher(client)
	consumer := queue.NewConsumer(client)
	mockFollowers := NewMockFollowerProvider()
	mockPosts := NewMockPostsProvider()
	handler := worker.NewHandler(feedCache, mockFollowers, mockPosts)

	// Setup scenario: User 1 has followers 2 and 3
	authorID := int64(1)
	mockFollowers.AddFollower(authorID, 2)
	mockFollowers.AddFollower(authorID, 3)

	// Ensure consumer group exists
	err := consumer.EnsureGroup(ctx, queue.StreamFeed, queue.ConsumerGroupFeed)
	if err != nil {
		t.Fatalf("EnsureGroup failed: %v", err)
	}

	// Publish a post created event
	postID := int64(100)
	event := queue.NewPostCreatedEvent(postID, authorID)
	msgID, err := publisher.Publish(ctx, queue.StreamFeed, event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	log.Printf("Published message: %s", msgID)

	// Consume the message
	messages, err := consumer.Read(ctx, queue.StreamFeed, queue.ConsumerGroupFeed, "test-worker", 10, time.Second)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Process the message
	msg := messages[0]
	err = handler.HandleEvent(ctx, msg.Event)
	if err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Acknowledge
	err = consumer.Ack(ctx, queue.StreamFeed, queue.ConsumerGroupFeed, msg.ID)
	if err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Verify: post should be in all feeds
	for _, userID := range []int64{1, 2, 3} {
		_, found, _ := feedCache.GetScore(ctx, userID, postID)
		if !found {
			t.Errorf("Post not found in user %d's feed", userID)
		}
	}

	// Verify: no pending messages
	pending, _ := consumer.Pending(ctx, queue.StreamFeed, queue.ConsumerGroupFeed)
	if pending != 0 {
		t.Errorf("Expected 0 pending messages, got %d", pending)
	}

	t.Log("✓ Stream to worker integration test passed")
}
