package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid user ID")
		return
	}

	var viewerID *int64
	if id, ok := middleware.GetUserIDFromContext(r.Context()); ok {
		viewerID = &id
	}

	// TODO: This endpoint will need modification when implementing user posts
	// The profile response should include paginated posts (e.g., latest 10 posts with cursor)
	// Example: profile.Posts, profile.PostsCursor, profile.HasMorePosts
	profile, err := h.userService.GetProfile(r.Context(), userID, viewerID)
	if err != nil {
		// TODO: Replace with proper logger (slog/zap) in production
		log.Printf("[ERROR] GetProfile handler: %v", err)
		httputil.WriteNotFound(w, "User not found")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, profile)
}

func (h *UserHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		httputil.WriteBadRequest(w, "Query parameter 'q' is required")
		return
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

	users, err := h.userService.Search(r.Context(), query, limit, viewerID)
	if err != nil {
		// TODO: Replace with proper logger (slog/zap) in production
		log.Printf("[ERROR] Search handler: %v", err)
		httputil.WriteInternalError(w, "Failed to search users")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
	})
}
