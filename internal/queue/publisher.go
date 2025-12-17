package queue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Publisher defines the interface for publishing events to a stream.
type Publisher interface {
	// Publish adds an event to the specified stream.
	// Returns the message ID assigned by Redis.
	Publish(ctx context.Context, stream string, event FeedEvent) (messageID string, err error)
}

// RedisPublisher implements Publisher using Redis Streams.
type RedisPublisher struct {
	client *redis.Client
}

// NewPublisher creates a new Publisher backed by Redis Streams.
func NewPublisher(client *redis.Client) Publisher {
	return &RedisPublisher{client: client}
}

// Publish adds an event to the stream using XADD.
// Uses "*" for auto-generated message ID (timestamp-sequence).
func (p *RedisPublisher) Publish(ctx context.Context, stream string, event FeedEvent) (string, error) {
	startTime := time.Now()

	values, err := event.ToMap()
	if err != nil {
		log.Printf("[Publisher] Publish FAILED: stream=%s type=%s err=%v", stream, event.Type, err)
		return "", fmt.Errorf("serialize event: %w", err)
	}

	// XADD stream * field value [field value ...]
	// "*" means Redis auto-generates the message ID
	messageID, err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()

	if err != nil {
		log.Printf("[Publisher] Publish FAILED: stream=%s type=%s err=%v", stream, event.Type, err)
		return "", fmt.Errorf("xadd to stream: %w", err)
	}

	log.Printf("[Publisher] Publish OK: stream=%s type=%s msgID=%s duration=%v",
		stream, event.Type, messageID, time.Since(startTime))

	// Log event details for debugging
	switch event.Type {
	case EventPostCreated, EventPostDeleted:
		log.Printf("[Publisher]   -> post=%d author=%d", event.PostID, event.AuthorID)
	case EventUserFollowed:
		log.Printf("[Publisher]   -> follower=%d followee=%d", event.FollowerID, event.FolloweeID)
	}

	return messageID, nil
}

// PublishPostCreated is a convenience method for publishing post created events.
func (p *RedisPublisher) PublishPostCreated(ctx context.Context, postID, authorID int64) (string, error) {
	event := NewPostCreatedEvent(postID, authorID)
	return p.Publish(ctx, StreamFeed, event)
}

// PublishPostDeleted is a convenience method for publishing post deleted events.
func (p *RedisPublisher) PublishPostDeleted(ctx context.Context, postID, authorID int64) (string, error) {
	event := NewPostDeletedEvent(postID, authorID)
	return p.Publish(ctx, StreamFeed, event)
}

// PublishUserFollowed is a convenience method for publishing user followed events.
func (p *RedisPublisher) PublishUserFollowed(ctx context.Context, followerID, followeeID int64) (string, error) {
	event := NewUserFollowedEvent(followerID, followeeID)
	return p.Publish(ctx, StreamFeed, event)
}
