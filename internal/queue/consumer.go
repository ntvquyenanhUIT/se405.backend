package queue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Message represents a message read from a Redis stream.
type Message struct {
	ID    string    // Redis message ID (e.g., "1702000000000-0")
	Event FeedEvent // Parsed event data
}

// Consumer defines the interface for consuming events from a stream.
type Consumer interface {
	// EnsureGroup creates the consumer group if it doesn't exist.
	// Should be called at worker startup.
	EnsureGroup(ctx context.Context, stream, group string) error

	// Read reads messages from the stream for this consumer.
	// Uses XREADGROUP to read pending or new messages.
	// count: max messages to read per call
	// block: how long to block waiting for new messages (0 = forever)
	Read(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]Message, error)

	// Ack acknowledges that a message has been processed.
	// Removes the message from the consumer's pending list.
	Ack(ctx context.Context, stream, group string, messageIDs ...string) error

	// Pending returns the number of pending (unacknowledged) messages for the group.
	Pending(ctx context.Context, stream, group string) (int64, error)
}

// RedisConsumer implements Consumer using Redis Streams.
type RedisConsumer struct {
	client *redis.Client
}

// NewConsumer creates a new Consumer backed by Redis Streams.
func NewConsumer(client *redis.Client) Consumer {
	return &RedisConsumer{client: client}
}

// EnsureGroup creates the consumer group if it doesn't exist.
// Uses XGROUP CREATE with MKSTREAM to create both stream and group.
// The "0" ID means the group will read all existing messages from the beginning.
// For new deployments, use "$" to read only new messages.
func (c *RedisConsumer) EnsureGroup(ctx context.Context, stream, group string) error {
	// Try to create the group
	// XGROUP CREATE stream group id [MKSTREAM]
	// "0" = start from beginning of stream
	// MKSTREAM = create stream if it doesn't exist
	err := c.client.XGroupCreateMkStream(ctx, stream, group, "0").Err()

	if err != nil {
		// "BUSYGROUP" means group already exists - that's fine
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			log.Printf("[Consumer] EnsureGroup: stream=%s group=%s (already exists)", stream, group)
			return nil
		}
		log.Printf("[Consumer] EnsureGroup FAILED: stream=%s group=%s err=%v", stream, group, err)
		return fmt.Errorf("create consumer group: %w", err)
	}

	log.Printf("[Consumer] EnsureGroup OK: stream=%s group=%s (created)", stream, group)
	return nil
}

// Read reads messages from the stream using XREADGROUP.
// First reads pending messages (">"), then blocks for new messages.
func (c *RedisConsumer) Read(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]Message, error) {
	startTime := time.Now()

	// XREADGROUP GROUP group consumer [COUNT count] [BLOCK ms] STREAMS stream id
	// ">" means read only new messages not yet delivered to any consumer
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()

	if err == redis.Nil {
		// Timeout - no new messages
		return nil, nil
	}
	if err != nil {
		log.Printf("[Consumer] Read FAILED: stream=%s group=%s consumer=%s err=%v", stream, group, consumer, err)
		return nil, fmt.Errorf("xreadgroup: %w", err)
	}

	// Parse messages
	var messages []Message
	for _, s := range streams {
		for _, msg := range s.Messages {
			event, err := ParseFeedEvent(msg.Values)
			if err != nil {
				log.Printf("[Consumer] Read parse error: msgID=%s err=%v", msg.ID, err)
				continue // Skip malformed messages
			}
			messages = append(messages, Message{
				ID:    msg.ID,
				Event: event,
			})
		}
	}

	log.Printf("[Consumer] Read OK: stream=%s group=%s consumer=%s count=%d duration=%v",
		stream, group, consumer, len(messages), time.Since(startTime))

	// Log each message for debugging
	for _, m := range messages {
		log.Printf("[Consumer]   -> msgID=%s type=%s", m.ID, m.Event.Type)
	}

	return messages, nil
}

// Ack acknowledges messages using XACK.
func (c *RedisConsumer) Ack(ctx context.Context, stream, group string, messageIDs ...string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	acked, err := c.client.XAck(ctx, stream, group, messageIDs...).Result()
	if err != nil {
		log.Printf("[Consumer] Ack FAILED: stream=%s group=%s ids=%v err=%v", stream, group, messageIDs, err)
		return fmt.Errorf("xack: %w", err)
	}

	log.Printf("[Consumer] Ack OK: stream=%s group=%s acked=%d ids=%v", stream, group, acked, messageIDs)
	return nil
}

// Pending returns the count of pending messages for the consumer group.
func (c *RedisConsumer) Pending(ctx context.Context, stream, group string) (int64, error) {
	info, err := c.client.XPending(ctx, stream, group).Result()
	if err != nil {
		log.Printf("[Consumer] Pending FAILED: stream=%s group=%s err=%v", stream, group, err)
		return 0, fmt.Errorf("xpending: %w", err)
	}

	log.Printf("[Consumer] Pending: stream=%s group=%s count=%d", stream, group, info.Count)
	return info.Count, nil
}

// ReadPending reads messages that were delivered but not yet acknowledged.
// Useful for recovering from crashes - process messages that were in-flight.
func (c *RedisConsumer) ReadPending(ctx context.Context, stream, group, consumer string, count int64) ([]Message, error) {
	startTime := time.Now()

	// Use "0" instead of ">" to read pending messages
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, "0"},
		Count:    count,
	}).Result()

	if err == redis.Nil {
		log.Printf("[Consumer] ReadPending: stream=%s group=%s consumer=%s (no pending)", stream, group, consumer)
		return nil, nil
	}
	if err != nil {
		log.Printf("[Consumer] ReadPending FAILED: stream=%s group=%s consumer=%s err=%v", stream, group, consumer, err)
		return nil, fmt.Errorf("xreadgroup pending: %w", err)
	}

	var messages []Message
	for _, s := range streams {
		for _, msg := range s.Messages {
			event, err := ParseFeedEvent(msg.Values)
			if err != nil {
				log.Printf("[Consumer] ReadPending parse error: msgID=%s err=%v", msg.ID, err)
				continue
			}
			messages = append(messages, Message{
				ID:    msg.ID,
				Event: event,
			})
		}
	}

	log.Printf("[Consumer] ReadPending OK: stream=%s group=%s consumer=%s count=%d duration=%v",
		stream, group, consumer, len(messages), time.Since(startTime))

	return messages, nil
}
