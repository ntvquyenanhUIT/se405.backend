package queue

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event types for the feed stream
const (
	EventPostCreated    = "post_created"
	EventPostDeleted    = "post_deleted"
	EventUserFollowed   = "user_followed"
	EventUserUnfollowed = "user_unfollowed"
)

// Stream names
const (
	StreamFeed = "stream:feed"
)

// Consumer group name for feed workers
const (
	ConsumerGroupFeed = "feed_workers"
)

// FeedEvent represents an event published to the feed stream.
// All feed-related events share this structure.
type FeedEvent struct {
	Type      string `json:"type"`      // EventPostCreated, EventPostDeleted, EventUserFollowed
	Timestamp int64  `json:"timestamp"` // Unix timestamp when event occurred

	// Post events (PostCreated, PostDeleted)
	PostID   int64 `json:"post_id,omitempty"`
	AuthorID int64 `json:"author_id,omitempty"`

	// Follow event (UserFollowed)
	FollowerID int64 `json:"follower_id,omitempty"`
	FolloweeID int64 `json:"followee_id,omitempty"`
}

// NewPostCreatedEvent creates an event for when a user creates a post.
// Worker will fan-out this post to all followers' feed caches.
func NewPostCreatedEvent(postID, authorID int64) FeedEvent {
	return FeedEvent{
		Type:      EventPostCreated,
		Timestamp: time.Now().Unix(),
		PostID:    postID,
		AuthorID:  authorID,
	}
}

// NewPostDeletedEvent creates an event for when a user deletes a post.
// Worker will remove this post from all followers' feed caches.
func NewPostDeletedEvent(postID, authorID int64) FeedEvent {
	return FeedEvent{
		Type:      EventPostDeleted,
		Timestamp: time.Now().Unix(),
		PostID:    postID,
		AuthorID:  authorID,
	}
}

// NewUserFollowedEvent creates an event for when a user follows another.
// Worker will backfill recent posts from followee into follower's feed cache.
func NewUserFollowedEvent(followerID, followeeID int64) FeedEvent {
	return FeedEvent{
		Type:       EventUserFollowed,
		Timestamp:  time.Now().Unix(),
		FollowerID: followerID,
		FolloweeID: followeeID,
	}
}

// NewUserUnfollowedEvent creates an event for when a user unfollows another.
// Worker will remove followee's posts from follower's feed cache.
func NewUserUnfollowedEvent(followerID, followeeID int64) FeedEvent {
	return FeedEvent{
		Type:       EventUserUnfollowed,
		Timestamp:  time.Now().Unix(),
		FollowerID: followerID,
		FolloweeID: followeeID,
	}
}

// ToMap converts the event to a map for Redis XADD.
// Redis Streams store field-value pairs, so we serialize to JSON in a "data" field.
func (e FeedEvent) ToMap() (map[string]interface{}, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	return map[string]interface{}{
		"type": e.Type,
		"data": string(data),
	}, nil
}

// ParseFeedEvent parses a FeedEvent from Redis stream message values.
func ParseFeedEvent(values map[string]interface{}) (FeedEvent, error) {
	data, ok := values["data"].(string)
	if !ok {
		return FeedEvent{}, fmt.Errorf("missing or invalid 'data' field")
	}

	var event FeedEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return FeedEvent{}, fmt.Errorf("unmarshal event: %w", err)
	}
	return event, nil
}
