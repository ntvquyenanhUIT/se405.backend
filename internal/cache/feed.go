package cache

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// FeedCachePrefix is the key prefix for user feed caches
	FeedCachePrefix = "feed:user:"

	// FeedCacheCap is the maximum number of posts to cache per user
	FeedCacheCap = 500

	// FeedCacheTTL is the TTL for feed cache (7 days)
	FeedCacheTTL = 7 * 24 * time.Hour
)

// PostScore represents a post with its timestamp score for caching
type PostScore struct {
	PostID    int64
	Timestamp int64 // Unix timestamp
}

// FeedCache defines the interface for feed cache operations.
// Using an interface enables testing with mocks and potential future backends.
type FeedCache interface {
	// AddPost adds a post to a user's feed cache.
	// Uses pipeline: ZADD + ZREMRANGEBYRANK (maintain cap) + EXPIRE (refresh TTL)
	AddPost(ctx context.Context, userID, postID int64, timestamp int64) error

	// RemovePost removes a post from a user's feed cache.
	// Uses ZREM.
	RemovePost(ctx context.Context, userID, postID int64) error

	// GetFeed retrieves post IDs from a user's feed cache.
	// If cursor is nil, returns newest posts. Otherwise returns posts older than cursor.
	// Returns post IDs, their scores (timestamps), and any error.
	GetFeed(ctx context.Context, userID int64, cursorScore *float64, limit int) (postIDs []int64, scores []float64, err error)

	// GetScore returns the timestamp score for a post in a user's feed cache.
	// Returns (score, found, error). found=false if post is not in cache.
	GetScore(ctx context.Context, userID, postID int64) (score int64, found bool, err error)

	// WarmCache bulk-inserts posts into a user's feed cache.
	// Uses pipelined ZADD commands + EXPIRE for efficiency.
	WarmCache(ctx context.Context, userID int64, posts []PostScore) error

	// Size returns the number of posts in a user's feed cache.
	Size(ctx context.Context, userID int64) (int64, error)

	// Exists checks if a user has a feed cache entry.
	// Returns false if the key doesn't exist (new user or TTL expired).
	// Service layer should warm the cache when this returns false.
	Exists(ctx context.Context, userID int64) (bool, error)
}

// RedisFeedCache implements FeedCache using Redis Sorted Sets.
type RedisFeedCache struct {
	client *redis.Client
}

// NewFeedCache creates a new FeedCache backed by Redis.
func NewFeedCache(client *redis.Client) FeedCache {
	return &RedisFeedCache{client: client}
}

// feedKey returns the Redis key for a user's feed cache.
func feedKey(userID int64) string {
	return fmt.Sprintf("%s%d", FeedCachePrefix, userID)
}

// AddPost adds a post to a user's feed cache using a pipeline.
// Pipeline: ZADD + ZREMRANGEBYRANK (trim to cap) + EXPIRE (refresh TTL)
func (c *RedisFeedCache) AddPost(ctx context.Context, userID, postID int64, timestamp int64) error {
	key := feedKey(userID)
	startTime := time.Now()

	pipe := c.client.Pipeline()

	// Add post with timestamp as score
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(timestamp),
		Member: strconv.FormatInt(postID, 10),
	})

	// Maintain cap: remove oldest posts beyond the cap
	// ZREMRANGEBYRANK removes [start, stop] inclusive, 0 is lowest score (oldest)
	// We keep the highest FeedCacheCap scores (newest), remove the rest
	pipe.ZRemRangeByRank(ctx, key, 0, int64(-FeedCacheCap-1))

	// Refresh TTL
	pipe.Expire(ctx, key, FeedCacheTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("[FeedCache] AddPost FAILED: user=%d post=%d err=%v", userID, postID, err)
		return fmt.Errorf("add post to feed: %w", err)
	}

	log.Printf("[FeedCache] AddPost OK: user=%d post=%d timestamp=%d duration=%v",
		userID, postID, timestamp, time.Since(startTime))
	return nil
}

// RemovePost removes a post from a user's feed cache.
func (c *RedisFeedCache) RemovePost(ctx context.Context, userID, postID int64) error {
	key := feedKey(userID)
	startTime := time.Now()
	member := strconv.FormatInt(postID, 10)

	removed, err := c.client.ZRem(ctx, key, member).Result()
	if err != nil {
		log.Printf("[FeedCache] RemovePost FAILED: user=%d post=%d err=%v", userID, postID, err)
		return fmt.Errorf("remove post from feed: %w", err)
	}

	log.Printf("[FeedCache] RemovePost OK: user=%d post=%d removed=%d duration=%v",
		userID, postID, removed, time.Since(startTime))
	return nil
}

// GetFeed retrieves post IDs from a user's feed cache.
// If cursorScore is nil, returns the newest posts (ZREVRANGE).
// If cursorScore is provided, returns posts with score < cursorScore (ZREVRANGEBYSCORE).
func (c *RedisFeedCache) GetFeed(ctx context.Context, userID int64, cursorScore *float64, limit int) ([]int64, []float64, error) {
	key := feedKey(userID)
	startTime := time.Now()

	var results []redis.Z
	var err error

	if cursorScore == nil {
		// No cursor: get newest posts
		results, err = c.client.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
		log.Printf("[FeedCache] GetFeed (no cursor): user=%d limit=%d", userID, limit)
	} else {
		// With cursor: get posts older than cursor (exclusive)
		// Use "(" prefix for exclusive range
		results, err = c.client.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
			Min:    "-inf",
			Max:    fmt.Sprintf("(%f", *cursorScore), // exclusive
			Offset: 0,
			Count:  int64(limit),
		}).Result()
		log.Printf("[FeedCache] GetFeed (with cursor): user=%d cursorScore=%.0f limit=%d", userID, *cursorScore, limit)
	}

	if err != nil {
		log.Printf("[FeedCache] GetFeed FAILED: user=%d err=%v", userID, err)
		return nil, nil, fmt.Errorf("get feed: %w", err)
	}

	// Refresh TTL on access
	c.client.Expire(ctx, key, FeedCacheTTL)

	postIDs := make([]int64, len(results))
	scores := make([]float64, len(results))

	for i, z := range results {
		id, err := strconv.ParseInt(z.Member.(string), 10, 64)
		if err != nil {
			log.Printf("[FeedCache] GetFeed parse error: member=%v err=%v", z.Member, err)
			return nil, nil, fmt.Errorf("parse post id: %w", err)
		}
		postIDs[i] = id
		scores[i] = z.Score
	}

	log.Printf("[FeedCache] GetFeed OK: user=%d returned=%d duration=%v",
		userID, len(postIDs), time.Since(startTime))
	return postIDs, scores, nil
}

// GetScore returns the timestamp score for a post in a user's feed cache.
// Returns (score, found, error).
func (c *RedisFeedCache) GetScore(ctx context.Context, userID, postID int64) (int64, bool, error) {
	key := feedKey(userID)
	member := strconv.FormatInt(postID, 10)

	score, err := c.client.ZScore(ctx, key, member).Result()
	if err == redis.Nil {
		log.Printf("[FeedCache] GetScore: user=%d post=%d NOT_FOUND", userID, postID)
		return 0, false, nil
	}
	if err != nil {
		log.Printf("[FeedCache] GetScore FAILED: user=%d post=%d err=%v", userID, postID, err)
		return 0, false, fmt.Errorf("get score: %w", err)
	}

	log.Printf("[FeedCache] GetScore OK: user=%d post=%d score=%.0f", userID, postID, score)
	return int64(score), true, nil
}

// WarmCache bulk-inserts posts into a user's feed cache using a pipeline.
func (c *RedisFeedCache) WarmCache(ctx context.Context, userID int64, posts []PostScore) error {
	if len(posts) == 0 {
		log.Printf("[FeedCache] WarmCache: user=%d posts=0 (nothing to warm)", userID)
		return nil
	}

	key := feedKey(userID)
	startTime := time.Now()

	pipe := c.client.Pipeline()

	// Batch ZADD commands
	members := make([]redis.Z, len(posts))
	for i, p := range posts {
		members[i] = redis.Z{
			Score:  float64(p.Timestamp),
			Member: strconv.FormatInt(p.PostID, 10),
		}
	}
	pipe.ZAdd(ctx, key, members...)

	// Maintain cap after bulk insert
	pipe.ZRemRangeByRank(ctx, key, 0, int64(-FeedCacheCap-1))

	// Set TTL
	pipe.Expire(ctx, key, FeedCacheTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("[FeedCache] WarmCache FAILED: user=%d posts=%d err=%v", userID, len(posts), err)
		return fmt.Errorf("warm cache: %w", err)
	}

	log.Printf("[FeedCache] WarmCache OK: user=%d posts=%d duration=%v",
		userID, len(posts), time.Since(startTime))
	return nil
}

// Size returns the number of posts in a user's feed cache.
func (c *RedisFeedCache) Size(ctx context.Context, userID int64) (int64, error) {
	key := feedKey(userID)

	size, err := c.client.ZCard(ctx, key).Result()
	if err != nil {
		log.Printf("[FeedCache] Size FAILED: user=%d err=%v", userID, err)
		return 0, fmt.Errorf("get cache size: %w", err)
	}

	log.Printf("[FeedCache] Size: user=%d size=%d", userID, size)
	return size, nil
}

// Exists checks if a user has a feed cache entry.
func (c *RedisFeedCache) Exists(ctx context.Context, userID int64) (bool, error) {
	key := feedKey(userID)

	exists, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		log.Printf("[FeedCache] Exists FAILED: user=%d err=%v", userID, err)
		return false, fmt.Errorf("check cache exists: %w", err)
	}

	found := exists > 0
	log.Printf("[FeedCache] Exists: user=%d found=%t", userID, found)
	return found, nil
}
