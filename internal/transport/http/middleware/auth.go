package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"iamstagram_22520060/internal/httputil"
	"iamstagram_22520060/internal/model"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserIDKey is the context key for the authenticated user's ID
	UserIDKey contextKey = "user_id"
)

// AuthMiddleware creates a middleware that validates JWT tokens
// Checks Authorization header first (for mobile), then falls back to cookie (for web)
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string

			// 1. Try Authorization header first (mobile apps)
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				// Expected format: "Bearer <token>"
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
					tokenString = parts[1]
				}
			}

			// 2. Fall back to cookie (web browsers)
			if tokenString == "" {
				cookie, err := r.Cookie("access_token")
				if err == nil && cookie.Value != "" {
					tokenString = cookie.Value
				}
			}

			// No token found in either location
			if tokenString == "" {
				httputil.WriteUnauthorized(w, "Missing authentication token")
				return
			}

			// Parse and validate token
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})

			if err != nil {
				if strings.Contains(err.Error(), "expired") {
					httputil.WriteUnauthorizedWithCode(w, model.CodeTokenExpired, "Access token has expired")
					return
				}
				httputil.WriteUnauthorizedWithCode(w, model.CodeTokenInvalid, "Invalid authentication token")
				return
			}

			// Extract claims
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || !token.Valid {
				httputil.WriteUnauthorizedWithCode(w, model.CodeTokenInvalid, "Invalid authentication token")
				return
			}

			// Get user_id from claims
			userIDFloat, ok := claims["user_id"].(float64)
			if !ok {
				httputil.WriteUnauthorizedWithCode(w, model.CodeTokenInvalid, "Invalid token claims")
				return
			}
			userID := int64(userIDFloat)

			// Add user_id to context
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserIDFromContext extracts the user ID from the request context
// Returns the user ID and true if found, or 0 and false if not found
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserIDKey).(int64)
	return userID, ok
}
