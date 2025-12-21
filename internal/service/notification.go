package service

import (
	"context"
	"log"

	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/repository"
)

// NotificationService handles notification-related business logic.
// It manages both polling (in-app) and push (Expo Push) notifications.
type NotificationService struct {
	notifRepo repository.NotificationRepository
	tokenRepo repository.DeviceTokenRepository
	userRepo  repository.UserRepository
	expoPush  *ExpoPushClient // Can be nil if push not configured
}

func NewNotificationService(
	notifRepo repository.NotificationRepository,
	tokenRepo repository.DeviceTokenRepository,
	userRepo repository.UserRepository,
	expoPush *ExpoPushClient,
) *NotificationService {
	return &NotificationService{
		notifRepo: notifRepo,
		tokenRepo: tokenRepo,
		userRepo:  userRepo,
		expoPush:  expoPush,
	}
}

// GetNotifications returns all notifications for a user.
// - Follow notifications are returned individually (not aggregated)
// - Like/Comment notifications are aggregated by post (e.g., "user1 and 5 others liked your post")
// Unread count is computed from the fetched data (no extra query).
func (s *NotificationService) GetNotifications(ctx context.Context, userID int64, limit int) (*model.NotificationListResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Get follow notifications (not aggregated) + their unread count
	follows, err, followUnread := s.notifRepo.GetFollowNotifications(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	// Get aggregated like/comment notifications + their unread count
	aggregated, err, aggUnread := s.notifRepo.GetAggregatedNotifications(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	// Total unread = follows unread + aggregated unread (computed from fetched data)
	unreadCount := followUnread + aggUnread

	return &model.NotificationListResponse{
		Follows:     follows,
		Aggregated:  aggregated,
		UnreadCount: unreadCount,
	}, nil
}

// MarkAsRead marks specific notifications as read.
func (s *NotificationService) MarkAsRead(ctx context.Context, userID int64, notificationIDs []int64) error {
	return s.notifRepo.MarkAsRead(ctx, userID, notificationIDs)
}

// MarkAllAsRead marks all notifications for a user as read.
func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID int64) error {
	return s.notifRepo.MarkAllAsRead(ctx, userID)
}

// GetUnreadCount returns the number of unread notifications (for badge display).
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.notifRepo.GetUnreadCount(ctx, userID)
}

// RegisterDeviceToken stores or updates a device's Expo push token.
// This is called when:
// - User logs in on a new device
// - Expo push token is refreshed by the mobile app
//
// The token is unique, so if the same token exists for a different user,
// it will be reassigned to the current user (device changed hands).
func (s *NotificationService) RegisterDeviceToken(ctx context.Context, userID int64, token, platform string) error {
	// For Expo, platform can be "expo", "ios", or "android"
	// We'll store it as-is for reference
	if platform == "" {
		platform = "expo"
	}

	return s.tokenRepo.Upsert(ctx, userID, token, platform)
}

// RemoveDeviceToken removes a device token (e.g., on logout).
func (s *NotificationService) RemoveDeviceToken(ctx context.Context, token string) error {
	return s.tokenRepo.Delete(ctx, token)
}

// CreateNotification creates a notification and optionally sends a push notification.
// This is called by other services (follow, like, comment) or by workers.
func (s *NotificationService) CreateNotification(
	ctx context.Context,
	userID, actorID int64,
	notifType string,
	postID, commentID *int64,
) error {
	// Don't notify yourself
	if userID == actorID {
		return nil
	}

	// Insert notification into database
	if err := s.notifRepo.Create(ctx, userID, actorID, notifType, postID, commentID); err != nil {
		return err
	}

	// Send push notification (async, don't block)
	if s.expoPush != nil {
		go s.sendPushNotification(context.Background(), userID, actorID, notifType, postID)
	}

	return nil
}

// sendPushNotification sends a push notification to all of the user's devices.
// This is called asynchronously - errors are logged but don't fail the request.
func (s *NotificationService) sendPushNotification(ctx context.Context, userID, actorID int64, notifType string, postID *int64) {
	// Get device tokens for the recipient
	tokens, err := s.tokenRepo.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[NotificationService] Failed to get device tokens for user %d: %v", userID, err)
		return
	}

	if len(tokens) == 0 {
		return // User has no registered devices
	}

	// Get actor info for the notification message
	actor, err := s.userRepo.GetByID(ctx, actorID)
	if err != nil {
		log.Printf("[NotificationService] Failed to get actor %d: %v", actorID, err)
		return
	}

	// Build notification message
	title, body := s.buildPushMessage(actor.Username, notifType)

	// Extract token strings
	tokenStrings := make([]string, len(tokens))
	for i, t := range tokens {
		tokenStrings[i] = t.Token
	}

	// Build data payload for navigation
	data := map[string]interface{}{
		"type":     notifType,
		"actor_id": actorID,
	}
	if postID != nil {
		data["post_id"] = *postID
	}
	
	// Send push notification via Expo
	if err := s.expoPush.SendToTokens(tokenStrings, title, body, data); err != nil {
		log.Printf("[NotificationService] Failed to send push to user %d: %v", userID, err)
	}
}

// buildPushMessage creates the title and body for a push notification.
func (s *NotificationService) buildPushMessage(actorUsername, notifType string) (title, body string) {
	switch notifType {
	case model.NotificationTypeFollow:
		title = "New Follower"
		body = actorUsername + " started following you"
	case model.NotificationTypeLike:
		title = "New Like"
		body = actorUsername + " liked your post"
	case model.NotificationTypeComment:
		title = "New Comment"
		body = actorUsername + " commented on your post"
	default:
		title = "Iamstagram"
		body = "You have a new notification"
	}
	return
}
