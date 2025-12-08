package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

type FollowHandler struct {
	followService *service.FollowService
}

func NewFollowHandler(followService *service.FollowService) *FollowHandler {
	return &FollowHandler{
		followService: followService,
	}
}

func (h *FollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	followerID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	followeeIDStr := chi.URLParam(r, "id")
	followeeID, err := strconv.ParseInt(followeeIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid user ID")
		return
	}

	if err := h.followService.Follow(r.Context(), followerID, followeeID); err != nil {
		switch {
		case errors.Is(err, model.ErrCannotFollowSelf):
			httputil.WriteBadRequest(w, err.Error())
		case errors.Is(err, model.ErrAlreadyFollowing):
			httputil.WriteConflict(w, err.Error())
		case errors.Is(err, model.ErrUserNotFound):
			httputil.WriteNotFound(w, err.Error())
		default:
			// TODO: Replace with proper logger (slog/zap) in production
			log.Printf("[ERROR] Follow handler: %v", err)
			httputil.WriteInternalError(w, "Failed to follow user")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Successfully followed user",
	})
}

func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	followerID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	followeeIDStr := chi.URLParam(r, "id")
	followeeID, err := strconv.ParseInt(followeeIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid user ID")
		return
	}

	if err := h.followService.Unfollow(r.Context(), followerID, followeeID); err != nil {
		switch {
		case errors.Is(err, model.ErrNotFollowing):
			httputil.WriteNotFound(w, err.Error())
		default:
			// TODO: Replace with proper logger (slog/zap) in production
			log.Printf("[ERROR] Unfollow handler: %v", err)
			httputil.WriteInternalError(w, "Failed to unfollow user")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Successfully unfollowed user",
	})
}

func (h *FollowHandler) GetFollowers(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid user ID")
		return
	}

	cursorStr := r.URL.Query().Get("cursor")
	var cursor *time.Time
	if cursorStr != "" {
		parsed, err := time.Parse(time.RFC3339, cursorStr)
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid cursor format")
			return
		}
		cursor = &parsed
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			httputil.WriteBadRequest(w, "Limit must be between 1 and 100")
			return
		}
		limit = parsedLimit
	}

	var viewerID *int64
	if id, ok := middleware.GetUserIDFromContext(r.Context()); ok {
		viewerID = &id
	}

	result, err := h.followService.GetFollowers(r.Context(), userID, cursor, limit, viewerID)
	if err != nil {
		// TODO: Replace with proper logger (slog/zap) in production
		log.Printf("[ERROR] GetFollowers handler: %v", err)
		httputil.WriteInternalError(w, "Failed to fetch followers")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *FollowHandler) GetFollowing(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid user ID")
		return
	}

	cursorStr := r.URL.Query().Get("cursor")
	var cursor *time.Time
	if cursorStr != "" {
		parsed, err := time.Parse(time.RFC3339, cursorStr)
		if err != nil {
			httputil.WriteBadRequest(w, "Invalid cursor format")
			return
		}
		cursor = &parsed
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			httputil.WriteBadRequest(w, "Limit must be between 1 and 100")
			return
		}
		limit = parsedLimit
	}

	var viewerID *int64
	if id, ok := middleware.GetUserIDFromContext(r.Context()); ok {
		viewerID = &id
	}

	result, err := h.followService.GetFollowing(r.Context(), userID, cursor, limit, viewerID)
	if err != nil {
		// TODO: Replace with proper logger (slog/zap) in production
		log.Printf("[ERROR] GetFollowing handler: %v", err)
		httputil.WriteInternalError(w, "Failed to fetch following")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
