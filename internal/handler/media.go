package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

type MediaHandler struct {
	mediaService *service.MediaService
}

func NewMediaHandler(mediaService *service.MediaService) *MediaHandler {
	return &MediaHandler{mediaService: mediaService}
}

// PresignPostUpload handles POST /media/posts/presign
// Returns a presigned URL for uploading post media directly to R2.
func (h *MediaHandler) PresignPostUpload(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB is plenty for JSON
	var req model.PresignPostUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, err.Error())
		return
	}

	req.ContentType = strings.TrimSpace(req.ContentType)
	if req.ContentType == "" {
		httputil.WriteBadRequest(w, "content_type is required")
		return
	}
	if req.FileSize > 0 && req.FileSize > model.MaxPostMediaSize {
		httputil.WriteBadRequestWithCode(w, model.CodeFileTooLarge, "Media exceeds 10MB limit")
		return
	}

	res, err := h.mediaService.PresignPostUpload(r.Context(), req.ContentType)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidImageType):
			httputil.WriteBadRequestWithCode(w, model.CodeInvalidImageType, "Unsupported image type. Allowed: jpeg, png, gif, webp")
		default:
			httputil.WriteInternalError(w, "Failed to create upload URL")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, res)
}

// PresignPostUploadBatch handles POST /media/posts/presign/batch
// Returns presigned URLs for uploading multiple post media items directly to R2.
func (h *MediaHandler) PresignPostUploadBatch(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Authentication required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB is plenty for JSON
	var req model.PresignPostUploadBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	if len(req.Items) == 0 {
		httputil.WriteBadRequest(w, "items is required")
		return
	}
	if len(req.Items) > model.MaxPostMediaCount {
		httputil.WriteBadRequest(w, fmt.Sprintf("too many items (max %d)", model.MaxPostMediaCount))
		return
	}

	items := make([]model.PresignPostUploadResponse, 0, len(req.Items))
	for i := range req.Items {
		item := req.Items[i]
		item.ContentType = strings.TrimSpace(item.ContentType)
		if item.ContentType == "" {
			httputil.WriteBadRequest(w, fmt.Sprintf("items[%d].content_type is required", i))
			return
		}
		if item.FileSize > 0 && item.FileSize > model.MaxPostMediaSize {
			httputil.WriteBadRequestWithCode(w, model.CodeFileTooLarge, fmt.Sprintf("items[%d] exceeds 10MB limit", i))
			return
		}

		res, err := h.mediaService.PresignPostUpload(r.Context(), item.ContentType)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrInvalidImageType):
				httputil.WriteBadRequestWithCode(w, model.CodeInvalidImageType, fmt.Sprintf("items[%d] unsupported image type. Allowed: jpeg, png, gif, webp", i))
			default:
				httputil.WriteInternalError(w, "Failed to create upload URL")
			}
			return
		}

		items = append(items, *res)
	}

	httputil.WriteJSON(w, http.StatusOK, model.PresignPostUploadBatchResponse{Items: items})
}
