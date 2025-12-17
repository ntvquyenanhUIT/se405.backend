package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Client wraps the Redis client with application-specific configuration.
// We use a single shared client across the application to reuse connection pooling.
type Client struct {
	*redis.Client
}

// NewClient creates a new Redis client from the given URL.
// URL format: redis://[:password@]host:port[/db]
// Example: redis://localhost:6379 or redis://:password@localhost:6379/0
func NewClient(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(opts)

	return &Client{Client: client}, nil
}

// Ping verifies the connection to Redis.
// Call this on application startup to fail fast if Redis is unreachable.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.Client.Close()
}
