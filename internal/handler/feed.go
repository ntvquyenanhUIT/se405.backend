package handler

import (
	"log"
	"net/http"
	"strconv"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

type FeedHandler struct {
	feedService *service.FeedService
}

func NewFeedHandler(feedService *service.FeedService) *FeedHandler {
	return &FeedHandler{
		feedService: feedService,
	}
}

// GetFeed handles GET /feed
// Returns paginated feed for the authenticated user.
//
// Query params:
//   - cursor: optional, compound cursor for pagination (format: "timestamp_id")
//   - limit: optional, number of posts per page (default 10, max 50)
func (h *FeedHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	// Parse query params
	var cursor *string
	if c := r.URL.Query().Get("cursor"); c != "" {
		cursor = &c
	}

	limit := 10 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			httputil.WriteBadRequest(w, "Invalid limit parameter")
			return
		}
		limit = parsed
	}

	// Get feed
	feed, err := h.feedService.GetFeed(r.Context(), userID, cursor, limit)
	if err != nil {
		log.Printf("[ERROR] GetFeed handler: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to get feed")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, feed)
}
