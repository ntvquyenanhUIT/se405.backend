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

type CommentHandler struct {
	commentService *service.CommentService
}

func NewCommentHandler(commentService *service.CommentService) *CommentHandler {
	return &CommentHandler{
		commentService: commentService,
	}
}

// Create handles POST /posts/:id/comments
// Creates a comment on a post for the authenticated user.
func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req model.CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	comment, err := h.commentService.Create(r.Context(), postID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrPostNotFound):
			httputil.WriteNotFound(w, "Post not found")
		case errors.Is(err, model.ErrCommentNotFound):
			httputil.WriteNotFound(w, "Parent comment not found")
		case errors.Is(err, model.ErrContentRequired):
			httputil.WriteBadRequest(w, "Comment content is required")
		case errors.Is(err, model.ErrContentTooLong):
			httputil.WriteBadRequest(w, "Comment content too long")
		default:
			log.Printf("[ERROR] Create comment handler: user=%d post=%d err=%v", userID, postID, err)
			httputil.WriteInternalError(w, "Failed to create comment")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, comment)
}

// Delete handles DELETE /posts/:id/comments/:commentId
// Deletes a comment (only owner can delete).
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	commentIDStr := chi.URLParam(r, "commentId")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid comment ID")
		return
	}

	err = h.commentService.Delete(r.Context(), commentID, userID)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrCommentNotFound):
			httputil.WriteNotFound(w, "Comment not found")
		case errors.Is(err, model.ErrNotCommentOwner):
			httputil.WriteForbidden(w, "You can only delete your own comments")
		default:
			log.Printf("[ERROR] Delete comment handler: user=%d comment=%d err=%v", userID, commentID, err)
			httputil.WriteInternalError(w, "Failed to delete comment")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Comment deleted successfully",
	})
}

// Update handles PATCH /posts/:id/comments/:commentId
// Updates a comment's content (only owner can update).
func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	commentIDStr := chi.URLParam(r, "commentId")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid comment ID")
		return
	}

	var req model.UpdateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	comment, err := h.commentService.Update(r.Context(), commentID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrCommentNotFound):
			httputil.WriteNotFound(w, "Comment not found")
		case errors.Is(err, model.ErrNotCommentOwner):
			httputil.WriteForbidden(w, "You can only edit your own comments")
		case errors.Is(err, model.ErrContentRequired):
			httputil.WriteBadRequest(w, "Comment content is required")
		case errors.Is(err, model.ErrContentTooLong):
			httputil.WriteBadRequest(w, "Comment content too long")
		default:
			log.Printf("[ERROR] Update comment handler: user=%d comment=%d err=%v", userID, commentID, err)
			httputil.WriteInternalError(w, "Failed to update comment")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, comment)
}

// List handles GET /posts/:id/comments
// Returns paginated comments for a post.
func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	postIDStr := chi.URLParam(r, "id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		httputil.WriteBadRequest(w, "Invalid post ID")
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

	comments, err := h.commentService.GetByPostID(r.Context(), postID, cursor, limit)
	if err != nil {
		if errors.Is(err, model.ErrPostNotFound) {
			httputil.WriteNotFound(w, "Post not found")
			return
		}
		log.Printf("[ERROR] List comments handler: post=%d err=%v", postID, err)
		httputil.WriteInternalError(w, "Failed to get comments")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, comments)
}
