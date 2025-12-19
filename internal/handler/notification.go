package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

type NotificationHandler struct {
	notifService *service.NotificationService
}

func NewNotificationHandler(notifService *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{
		notifService: notifService,
	}
}

// List handles GET /notifications
// Returns all notifications for the authenticated user (follows + aggregated likes/comments).
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	// Parse limit from query params
	limit := 20 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			httputil.WriteBadRequest(w, "Invalid limit parameter")
			return
		}
		limit = parsed
	}

	notifications, err := h.notifService.GetNotifications(r.Context(), userID, limit)
	if err != nil {
		log.Printf("[ERROR] List notifications: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to get notifications")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, notifications)
}

// MarkRead handles PATCH /notifications/read
// Marks specific notifications as read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	var req model.MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	if len(req.NotificationIDs) == 0 {
		httputil.WriteBadRequest(w, "notification_ids is required")
		return
	}

	err := h.notifService.MarkAsRead(r.Context(), userID, req.NotificationIDs)
	if err != nil {
		log.Printf("[ERROR] Mark notifications read: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to mark notifications as read")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Notifications marked as read",
	})
}

// MarkAllRead handles POST /notifications/read-all
// Marks all notifications as read for the authenticated user.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	err := h.notifService.MarkAllAsRead(r.Context(), userID)
	if err != nil {
		log.Printf("[ERROR] Mark all notifications read: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to mark all notifications as read")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "All notifications marked as read",
	})
}

// GetUnreadCount handles GET /notifications/unread-count
// Returns the count of unread notifications (for badge display).
func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	count, err := h.notifService.GetUnreadCount(r.Context(), userID)
	if err != nil {
		log.Printf("[ERROR] Get unread count: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to get unread count")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int{
		"unread_count": count,
	})
}

// RegisterToken handles POST /devices/token
// Registers a device token for push notifications.
func (h *NotificationHandler) RegisterToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	var req model.RegisterTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.Token == "" {
		httputil.WriteBadRequest(w, "token is required")
		return
	}

	err := h.notifService.RegisterDeviceToken(r.Context(), userID, req.Token, req.Platform)
	if err != nil {
		log.Printf("[ERROR] Register device token: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to register device token")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Device token registered",
	})
}

// RemoveToken handles DELETE /devices/token
// Removes a device token (e.g., on logout).
func (h *NotificationHandler) RemoveToken(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.Token == "" {
		httputil.WriteBadRequest(w, "token is required")
		return
	}

	err := h.notifService.RemoveDeviceToken(r.Context(), req.Token)
	if err != nil {
		log.Printf("[ERROR] Remove device token: err=%v", err)
		httputil.WriteInternalError(w, "Failed to remove device token")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Device token removed",
	})
}
