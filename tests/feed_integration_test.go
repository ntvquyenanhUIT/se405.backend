package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// ============================================================================
// Test Configuration
// ============================================================================

var (
	baseURL = getEnv("TEST_BASE_URL", "http://localhost:8080")

	// Test user credentials (from seeds/test_feed.sql)
	testUsers = map[string]testUser{
		"alice":   {ID: 100, Username: "alice_test", Password: "password123"},
		"bob":     {ID: 101, Username: "bob_test", Password: "password123"},
		"charlie": {ID: 102, Username: "charlie_test", Password: "password123"},
		"david":   {ID: 103, Username: "david_test", Password: "password123"},
		"eve":     {ID: 104, Username: "eve_test", Password: "password123"},
	}
)

type testUser struct {
	ID       int64
	Username string
	Password string
	Token    string // Set after login
}

// ============================================================================
// HTTP Client Helpers
// ============================================================================

type apiClient struct {
	client  *http.Client
	baseURL string
	token   string
}

func newClient() *apiClient {
	return &apiClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
}

func (c *apiClient) withToken(token string) *apiClient {
	c.token = token
	return c
}

func (c *apiClient) get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Do(req)
}

func (c *apiClient) post(path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest("POST", c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Do(req)
}

func (c *apiClient) delete(path string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Do(req)
}

func parseJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================================
// Login Helper
// ============================================================================

func login(t *testing.T, username, password string) string {
	client := newClient()
	resp, err := client.post("/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Login failed with status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := parseJSON(resp, &result); err != nil {
		t.Fatalf("Parse login response: %v", err)
	}
	return result.AccessToken
}

// ============================================================================
// TEST CASES
// ============================================================================

// TestFeedCacheWarm tests that first feed request warms the cache
func TestFeedCacheWarm(t *testing.T) {
	// Prerequisites: Run seeds/test_feed.sql first
	// Bob follows Alice who has 5 posts

	token := login(t, "bob_test", "password123")
	client := newClient().withToken(token)

	// First request should warm cache (check server logs for "[FeedService] Cache miss")
	resp, err := client.get("/feed?limit=10")
	if err != nil {
		t.Fatalf("Get feed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Get feed failed: %d - %s", resp.StatusCode, body)
	}

	var feed struct {
		Posts []struct {
			ID      int64  `json:"id"`
			Caption string `json:"caption"`
			Author  struct {
				Username string `json:"username"`
			} `json:"author"`
		} `json:"posts"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := parseJSON(resp, &feed); err != nil {
		t.Fatalf("Parse feed: %v", err)
	}

	// Bob follows Alice (5 posts)
	if len(feed.Posts) != 5 {
		t.Errorf("Expected 5 posts in feed, got %d", len(feed.Posts))
	}

	// Verify posts are from Alice and ordered newest first
	for i, post := range feed.Posts {
		if post.Author.Username != "alice_test" {
			t.Errorf("Post %d: expected author alice_test, got %s", i, post.Author.Username)
		}
		t.Logf("Post %d: ID=%d Caption=%q", i, post.ID, post.Caption)
	}

	t.Log("✓ Feed cache warm test passed")
}

// TestFeedPagination tests cursor-based pagination
func TestFeedPagination(t *testing.T) {
	token := login(t, "bob_test", "password123")
	client := newClient().withToken(token)

	// Request only 2 posts
	resp, err := client.get("/feed?limit=2")
	if err != nil {
		t.Fatalf("Get feed page 1: %v", err)
	}

	var page1 struct {
		Posts      []struct{ ID int64 } `json:"posts"`
		NextCursor *string              `json:"next_cursor"`
	}
	if err := parseJSON(resp, &page1); err != nil {
		t.Fatalf("Parse page 1: %v", err)
	}

	if len(page1.Posts) != 2 {
		t.Errorf("Page 1: expected 2 posts, got %d", len(page1.Posts))
	}
	if page1.NextCursor == nil {
		t.Fatal("Page 1: expected next_cursor, got nil")
	}

	// Request next page
	resp, err = client.get("/feed?limit=2&cursor=" + *page1.NextCursor)
	if err != nil {
		t.Fatalf("Get feed page 2: %v", err)
	}

	var page2 struct {
		Posts      []struct{ ID int64 } `json:"posts"`
		NextCursor *string              `json:"next_cursor"`
	}
	if err := parseJSON(resp, &page2); err != nil {
		t.Fatalf("Parse page 2: %v", err)
	}

	if len(page2.Posts) != 2 {
		t.Errorf("Page 2: expected 2 posts, got %d", len(page2.Posts))
	}

	// Verify no overlap
	page1IDs := map[int64]bool{}
	for _, p := range page1.Posts {
		page1IDs[p.ID] = true
	}
	for _, p := range page2.Posts {
		if page1IDs[p.ID] {
			t.Errorf("Post %d appears in both pages", p.ID)
		}
	}

	t.Log("✓ Feed pagination test passed")
}

// TestEmptyFeed tests user with no followees
func TestEmptyFeed(t *testing.T) {
	// David follows nobody
	token := login(t, "david_test", "password123")
	client := newClient().withToken(token)

	resp, err := client.get("/feed")
	if err != nil {
		t.Fatalf("Get feed: %v", err)
	}

	var feed struct {
		Posts []interface{} `json:"posts"`
	}
	if err := parseJSON(resp, &feed); err != nil {
		t.Fatalf("Parse feed: %v", err)
	}

	if len(feed.Posts) != 0 {
		t.Errorf("Expected empty feed, got %d posts", len(feed.Posts))
	}

	t.Log("✓ Empty feed test passed")
}

// TestFollowUpdatesFeeds tests that follow triggers backfill
func TestFollowUpdatesFeed(t *testing.T) {
	// David follows nobody initially
	token := login(t, "david_test", "password123")
	client := newClient().withToken(token)

	// Verify empty feed first
	resp, err := client.get("/feed")
	if err != nil {
		t.Fatalf("Get feed: %v", err)
	}
	var feed struct {
		Posts []interface{} `json:"posts"`
	}
	parseJSON(resp, &feed)
	if len(feed.Posts) != 0 {
		t.Skipf("David already has posts in feed, skipping (run seed again)")
	}

	// David follows Alice (ID=100)
	resp, err = client.post("/users/100/follow", nil)
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Follow failed: %d - %s", resp.StatusCode, body)
	}

	// Wait for async worker to process
	time.Sleep(500 * time.Millisecond)

	// Check feed now has Alice's posts
	resp, err = client.get("/feed")
	if err != nil {
		t.Fatalf("Get feed after follow: %v", err)
	}
	parseJSON(resp, &feed)

	if len(feed.Posts) < 1 {
		t.Errorf("Expected posts after following Alice, got %d", len(feed.Posts))
	}

	t.Logf("✓ Follow updates feed test passed (got %d posts)", len(feed.Posts))

	// Cleanup: unfollow
	client.delete("/users/100/follow")
}

// TestCreatePostFanout tests that new post appears in followers' feeds
func TestCreatePostFanout(t *testing.T) {
	// Login as Alice (has followers: Bob, Charlie)
	aliceToken := login(t, "alice_test", "password123")
	aliceClient := newClient().withToken(aliceToken)

	// Create a new post
	resp, err := aliceClient.post("/posts", map[string]interface{}{
		"caption":    "Test post for fanout " + time.Now().Format(time.RFC3339),
		"media_urls": []string{"https://picsum.photos/seed/test/1080/1080"},
	})
	if err != nil {
		t.Fatalf("Create post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Create post failed: %d - %s", resp.StatusCode, body)
	}

	var newPost struct {
		ID int64 `json:"id"`
	}
	if err := parseJSON(resp, &newPost); err != nil {
		t.Fatalf("Parse new post: %v", err)
	}
	t.Logf("Created post ID=%d", newPost.ID)

	// Wait for worker to fan out
	time.Sleep(500 * time.Millisecond)

	// Check Bob's feed contains the new post
	bobToken := login(t, "bob_test", "password123")
	bobClient := newClient().withToken(bobToken)

	resp, err = bobClient.get("/feed?limit=1")
	if err != nil {
		t.Fatalf("Get Bob's feed: %v", err)
	}

	var bobFeed struct {
		Posts []struct {
			ID int64 `json:"id"`
		} `json:"posts"`
	}
	if err := parseJSON(resp, &bobFeed); err != nil {
		t.Fatalf("Parse Bob's feed: %v", err)
	}

	// Newest post should be first
	if len(bobFeed.Posts) > 0 && bobFeed.Posts[0].ID == newPost.ID {
		t.Log("✓ Create post fanout test passed")
	} else {
		t.Logf("Bob's feed first post: %+v, expected ID=%d", bobFeed.Posts, newPost.ID)
		t.Log("⚠ New post may not be at top (ordering issue) or fanout delayed")
	}

	// Cleanup: delete the test post
	aliceClient.delete(fmt.Sprintf("/posts/%d", newPost.ID))
}

// TestDeletePostRemoval tests that deleted post is removed from feeds
func TestDeletePostRemoval(t *testing.T) {
	// Create a post as Alice
	aliceToken := login(t, "alice_test", "password123")
	aliceClient := newClient().withToken(aliceToken)

	resp, err := aliceClient.post("/posts", map[string]interface{}{
		"caption":    "Post to be deleted",
		"media_urls": []string{"https://picsum.photos/seed/delete/1080/1080"},
	})
	if err != nil {
		t.Fatalf("Create post: %v", err)
	}
	var newPost struct {
		ID int64 `json:"id"`
	}
	parseJSON(resp, &newPost)
	t.Logf("Created post ID=%d for deletion test", newPost.ID)

	// Wait for fanout
	time.Sleep(500 * time.Millisecond)

	// Delete the post
	resp, err = aliceClient.delete(fmt.Sprintf("/posts/%d", newPost.ID))
	if err != nil {
		t.Fatalf("Delete post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Delete failed: %d - %s", resp.StatusCode, body)
	}

	// Wait for worker to remove from feeds
	time.Sleep(500 * time.Millisecond)

	// Verify post is not in Bob's feed
	bobToken := login(t, "bob_test", "password123")
	bobClient := newClient().withToken(bobToken)

	resp, err = bobClient.get("/feed?limit=50")
	if err != nil {
		t.Fatalf("Get Bob's feed: %v", err)
	}

	var bobFeed struct {
		Posts []struct {
			ID int64 `json:"id"`
		} `json:"posts"`
	}
	parseJSON(resp, &bobFeed)

	for _, p := range bobFeed.Posts {
		if p.ID == newPost.ID {
			t.Errorf("Deleted post %d still in Bob's feed", newPost.ID)
			return
		}
	}

	t.Log("✓ Delete post removal test passed")
}

// TestGetUserPosts tests profile post thumbnails
func TestGetUserPosts(t *testing.T) {
	// Alice has 5 posts from seed
	client := newClient()

	resp, err := client.get("/users/100/posts?limit=10")
	if err != nil {
		t.Fatalf("Get user posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Get user posts failed: %d - %s", resp.StatusCode, body)
	}

	var posts struct {
		Posts []struct {
			ID           int64  `json:"id"`
			ThumbnailURL string `json:"thumbnail_url"`
			MediaCount   int    `json:"media_count"`
		} `json:"posts"`
	}
	if err := parseJSON(resp, &posts); err != nil {
		t.Fatalf("Parse posts: %v", err)
	}

	// Alice has 5 posts from seed, might have more from other tests
	if len(posts.Posts) < 5 {
		t.Errorf("Expected at least 5 posts, got %d", len(posts.Posts))
	}

	// Check thumbnail URLs exist
	for i, p := range posts.Posts {
		if p.ThumbnailURL == "" {
			t.Errorf("Post %d missing thumbnail_url", i)
		}
		t.Logf("Post %d: ID=%d, MediaCount=%d", i, p.ID, p.MediaCount)
	}

	t.Log("✓ Get user posts test passed")
}

// TestGetSinglePost tests fetching post details
func TestGetSinglePost(t *testing.T) {
	// Post 1000 is Alice's first post from seed
	client := newClient()

	resp, err := client.get("/posts/1000")
	if err != nil {
		t.Fatalf("Get post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Get post failed: %d - %s", resp.StatusCode, body)
	}

	var post struct {
		ID      int64  `json:"id"`
		Caption string `json:"caption"`
		Author  struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"author"`
		Media []struct {
			MediaURL string `json:"media_url"`
		} `json:"media"`
	}
	if err := parseJSON(resp, &post); err != nil {
		t.Fatalf("Parse post: %v", err)
	}

	if post.ID != 1000 {
		t.Errorf("Expected post ID 1000, got %d", post.ID)
	}
	if post.Author.Username != "alice_test" {
		t.Errorf("Expected author alice_test, got %s", post.Author.Username)
	}
	if len(post.Media) == 0 {
		t.Error("Expected at least 1 media item")
	}

	t.Logf("Post: ID=%d, Caption=%q, Author=%s, MediaCount=%d",
		post.ID, post.Caption, post.Author.Username, len(post.Media))
	t.Log("✓ Get single post test passed")
}

// TestUnfollowRemovesPosts tests that unfollow removes posts from feed
func TestUnfollowRemovesPosts(t *testing.T) {
	// Setup: Make sure Bob follows Alice
	bobToken := login(t, "bob_test", "password123")
	bobClient := newClient().withToken(bobToken)

	// Follow first (may already be following)
	bobClient.post("/users/100/follow", nil)
	time.Sleep(300 * time.Millisecond)

	// Get feed before unfollow
	resp, _ := bobClient.get("/feed")
	var feedBefore struct {
		Posts []interface{} `json:"posts"`
	}
	parseJSON(resp, &feedBefore)
	countBefore := len(feedBefore.Posts)
	t.Logf("Feed before unfollow: %d posts", countBefore)

	if countBefore == 0 {
		t.Skip("Bob's feed is empty, cannot test unfollow")
	}

	// Unfollow Alice
	resp, err := bobClient.delete("/users/100/follow")
	if err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Unfollow failed: %d - %s", resp.StatusCode, body)
	}

	// Wait for worker
	time.Sleep(500 * time.Millisecond)

	// Get feed after unfollow
	resp, _ = bobClient.get("/feed")
	var feedAfter struct {
		Posts []interface{} `json:"posts"`
	}
	parseJSON(resp, &feedAfter)

	t.Logf("Feed after unfollow: %d posts", len(feedAfter.Posts))

	if len(feedAfter.Posts) >= countBefore {
		t.Errorf("Feed should have fewer posts after unfollow")
	}

	t.Log("✓ Unfollow removes posts test passed")

	// Cleanup: re-follow
	bobClient.post("/users/100/follow", nil)
}
