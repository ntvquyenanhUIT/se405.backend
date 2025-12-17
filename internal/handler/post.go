package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

type PostHandler struct {
	postService *service.PostService
}

func NewPostHandler(postService *service.PostService) *PostHandler {
	return &PostHandler{
		postService: postService,
	}
}

// Create handles POST /posts
// Creates a new post for the authenticated user.
func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	var req model.CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	post, err := h.postService.Create(r.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrNoMediaProvided):
			httputil.WriteBadRequest(w, "At least one media item is required")
		case errors.Is(err, model.ErrTooManyMedia):
			httputil.WriteBadRequest(w, "Too many media items (max 10)")
		case errors.Is(err, model.ErrCaptionTooLong):
			httputil.WriteBadRequest(w, "Caption too long (max 2200 characters)")
		default:
			log.Printf("[ERROR] Create post handler: user=%d err=%v", userID, err)
			httputil.WriteInternalError(w, "Failed to create post")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, post)
}

// GetByID handles GET /posts/:id
// Returns a single post with full details.
func (h *PostHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	postIDStr := chi.URLParam(r, "id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid post ID")
		return
	}

	// Get viewer ID if authenticated (for like status, etc.)
	var viewerID *int64
	if id, ok := middleware.GetUserIDFromContext(r.Context()); ok {
		viewerID = &id
	}

	post, err := h.postService.GetByID(r.Context(), postID, viewerID)
	if err != nil {
		if errors.Is(err, model.ErrPostNotFound) {
			httputil.WriteNotFound(w, "Post not found")
			return
		}
		log.Printf("[ERROR] Get post handler: post=%d err=%v", postID, err)
		httputil.WriteInternalError(w, "Failed to get post")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, post)
}

// Delete handles DELETE /posts/:id
// Soft-deletes a post (only owner can delete).
func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	postIDStr := chi.URLParam(r, "id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid post ID")
		return
	}

	err = h.postService.Delete(r.Context(), postID, userID)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrPostNotFound):
			httputil.WriteNotFound(w, "Post not found")
		case errors.Is(err, model.ErrNotPostOwner):
			httputil.WriteForbidden(w, "You can only delete your own posts")
		default:
			log.Printf("[ERROR] Delete post handler: user=%d post=%d err=%v", userID, postID, err)
			httputil.WriteInternalError(w, "Failed to delete post")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Post deleted successfully",
	})
}

// GetUserPosts handles GET /users/:id/posts
// Returns paginated post thumbnails for a user's profile grid.
func (h *PostHandler) GetUserPosts(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid user ID")
		return
	}

	// Parse query params
	var cursor *string
	if c := r.URL.Query().Get("cursor"); c != "" {
		cursor = &c
	}

	limit := 12 // default (3x4 grid)
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			httputil.WriteBadRequest(w, "Invalid limit parameter")
			return
		}
		limit = parsed
	}

	posts, err := h.postService.GetUserPosts(r.Context(), userID, cursor, limit)
	if err != nil {
		log.Printf("[ERROR] Get user posts handler: user=%d err=%v", userID, err)
		httputil.WriteInternalError(w, "Failed to get user posts")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, posts)
}
