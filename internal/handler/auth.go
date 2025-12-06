package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"iamstagram_22520060/internal/config"
	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/transport/http/middleware"
)

// AuthHandler groups auth-related HTTP endpoints and their dependencies.
type AuthHandler struct {
	userService  *service.UserService
	authService  *service.AuthService
	mediaService *service.MediaService
	config       *config.Config
}

// NewAuthHandler wires dependencies for authentication endpoints.
func NewAuthHandler(userService *service.UserService, authService *service.AuthService, mediaService *service.MediaService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		userService:  userService,
		authService:  authService,
		mediaService: mediaService,
		config:       cfg,
	}
}

// Register handles multipart sign-up with optional avatar upload and default avatar fallback.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	maxFormSize := int64(model.MaxAvatarSizeBytes) + 1024*1024 // allow form overhead
	r.Body = http.MaxBytesReader(w, r.Body, maxFormSize)
	if err := r.ParseMultipartForm(maxFormSize); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			httputil.WriteBadRequest(w, "Content-Type must be multipart/form-data")
			return
		}
		if strings.Contains(err.Error(), "request body too large") {
			httputil.WriteBadRequestWithCode(w, model.CodeFileTooLarge, "Avatar exceeds 5MB limit")
			return
		}
		httputil.WriteBadRequest(w, "Invalid form data")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	displayName := r.FormValue("display_name")

	if username == "" {
		httputil.WriteBadRequest(w, "Username is required")
		return
	}
	if password == "" {
		httputil.WriteBadRequest(w, "Password is required")
		return
	}

	var avatarURL *string
	var avatarKey *string
	file, header, err := r.FormFile("avatar")
	if err == nil {
		defer file.Close()
		upload, uploadErr := h.mediaService.UploadAvatar(r.Context(), file, header)
		if uploadErr != nil {
			switch {
			case errors.Is(uploadErr, model.ErrFileTooLarge):
				httputil.WriteBadRequestWithCode(w, model.CodeFileTooLarge, "Avatar exceeds 5MB limit")
			case errors.Is(uploadErr, model.ErrInvalidImageType):
				httputil.WriteBadRequestWithCode(w, model.CodeInvalidImageType, "Unsupported image type. Allowed: jpeg, png, gif, webp")
			default:
				httputil.WriteInternalError(w, "Failed to upload avatar")
			}
			return
		}
		avatarURL = &upload.URL
		avatarKey = &upload.Key
	} else if err != http.ErrMissingFile {
		httputil.WriteBadRequest(w, "Invalid avatar upload")
		return
	} else {
		if h.config.DefaultAvatarURL != "" {
			avatarURL = &h.config.DefaultAvatarURL
		}
		if h.config.DefaultAvatarKey != "" {
			avatarKey = &h.config.DefaultAvatarKey
		}
	}

	req := model.RegisterRequest{
		Username:    username,
		Password:    password,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		AvatarKey:   avatarKey,
	}

	user, err := h.userService.Register(r.Context(), &req)
	if err != nil {
		if errors.Is(err, model.ErrUsernameExists) {
			httputil.WriteConflict(w, "Username already exists")
			return
		}
		httputil.WriteInternalError(w, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, user)
}

// Login handles user login
// POST /auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	// Basic validation
	if req.Username == "" {
		httputil.WriteBadRequest(w, "Username is required")
		return
	}
	if req.Password == "" {
		httputil.WriteBadRequest(w, "Password is required")
		return
	}

	// Authenticate user
	user, err := h.userService.Login(r.Context(), &req)
	if err != nil {
		if errors.Is(err, model.ErrInvalidCredentials) {
			httputil.WriteUnauthorized(w, "Invalid username or password")
			return
		}
		httputil.WriteInternalError(w, "Failed to login")
		return
	}

	// Get client info for token metadata
	deviceInfo := r.Header.Get("User-Agent")
	ipAddress := h.getClientIP(r)

	// Generate token pair (access + refresh)
	tokenPair, err := h.authService.GenerateTokenPair(r.Context(), user.ID, deviceInfo, ipAddress)
	if err != nil {
		httputil.WriteInternalError(w, "Failed to generate tokens")
		return
	}

	// Return user data with tokens
	response := model.LoginResponse{
		User:         user,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// Me returns the currently authenticated user
// GET /me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by auth middleware)
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Not authenticated")
		return
	}

	user, err := h.userService.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, model.ErrUserNotFound) {
			httputil.WriteNotFound(w, "User not found")
			return
		}
		httputil.WriteInternalError(w, "Failed to get user")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, user)
}

// Refresh handles token refresh
// POST /auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req model.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.RefreshToken == "" {
		httputil.WriteBadRequest(w, "Refresh token is required")
		return
	}

	// Get client info
	deviceInfo := r.Header.Get("User-Agent")
	ipAddress := h.getClientIP(r)

	// Refresh tokens
	tokenPair, _, err := h.authService.RefreshTokens(r.Context(), req.RefreshToken, deviceInfo, ipAddress)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrRefreshTokenNotFound):
			httputil.WriteUnauthorized(w, "Invalid refresh token")
		case errors.Is(err, model.ErrRefreshTokenExpired):
			httputil.WriteUnauthorizedWithCode(w, model.CodeTokenExpired, "Refresh token has expired")
		case errors.Is(err, model.ErrRefreshTokenReused):
			httputil.WriteUnauthorizedWithCode(w, model.CodeTokenReused, "Refresh token reuse detected. Please login again.")
		default:
			httputil.WriteInternalError(w, "Failed to refresh tokens")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tokenPair)
}

// Logout handles user logout
// POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req model.LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.RefreshToken == "" {
		httputil.WriteBadRequest(w, "Refresh token is required")
		return
	}

	// Revoke the refresh token
	err := h.authService.RevokeRefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, model.ErrRefreshTokenNotFound) {
			// Token already revoked or doesn't exist - still return success
			httputil.WriteJSON(w, http.StatusOK, map[string]string{
				"message": "Logged out successfully",
			})
			return
		}
		httputil.WriteInternalError(w, "Failed to logout")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// LogoutAll handles logout from all devices
// POST /auth/logout-all
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by auth middleware)
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		httputil.WriteUnauthorized(w, "Not authenticated")
		return
	}

	// Revoke all refresh tokens for this user
	err := h.authService.RevokeAllUserTokens(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, "Failed to logout from all devices")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out from all devices",
	})
}

// getClientIP extracts the client IP from the request
func (h *AuthHandler) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxied requests)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the list
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	// RemoteAddr is in the format "IP:port", so we need to extract the IP
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
