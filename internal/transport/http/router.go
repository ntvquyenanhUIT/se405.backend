package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"iamstagram_22520060/internal/handler"
	"iamstagram_22520060/internal/httputil"
	authmw "iamstagram_22520060/internal/transport/http/middleware"
)

// RouterConfig holds the dependencies needed to create routes
type RouterConfig struct {
	AuthHandler   *handler.AuthHandler
	UserHandler   *handler.UserHandler
	FollowHandler *handler.FollowHandler
	FeedHandler   *handler.FeedHandler
	PostHandler   *handler.PostHandler
	MediaHandler  *handler.MediaHandler
	JWTSecret     string
}

// NewRouter creates and configures a new Chi router with all route groups
func NewRouter(cfg RouterConfig) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health check endpoint (useful for deployment/monitoring)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.WriteJSON(w, 200, map[string]string{"status": "ok"})
	})

	// Public routes - no authentication required
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", cfg.AuthHandler.Register)
		r.Post("/login", cfg.AuthHandler.Login)
		r.Post("/refresh", cfg.AuthHandler.Refresh)
	})

	// Public user endpoints with optional authentication
	r.Route("/users", func(r chi.Router) {
		r.With(authmw.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/search", cfg.UserHandler.Search)
		r.With(authmw.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/{id}", cfg.UserHandler.GetProfile)
		r.With(authmw.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/{id}/followers", cfg.FollowHandler.GetFollowers)
		r.With(authmw.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/{id}/following", cfg.FollowHandler.GetFollowing)
		r.With(authmw.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/{id}/posts", cfg.PostHandler.GetUserPosts)
	})

	// Public post endpoint with optional authentication
	r.With(authmw.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/posts/{id}", cfg.PostHandler.GetByID)

	// Protected routes - require authentication
	r.Group(func(r chi.Router) {
		r.Use(authmw.AuthMiddleware(cfg.JWTSecret))

		// Current user endpoints
		r.Get("/me", cfg.AuthHandler.Me)

		// Auth actions that require authentication
		r.Post("/auth/logout", cfg.AuthHandler.Logout)
		r.Post("/auth/logout-all", cfg.AuthHandler.LogoutAll)

		// Follow/unfollow actions require authentication
		r.Post("/users/{id}/follow", cfg.FollowHandler.Follow)
		r.Delete("/users/{id}/follow", cfg.FollowHandler.Unfollow)

		// Feed endpoint
		r.Get("/feed", cfg.FeedHandler.GetFeed)

		// Post endpoints
		r.Post("/posts", cfg.PostHandler.Create)
		r.Delete("/posts/{id}", cfg.PostHandler.Delete)

		// Media endpoints (direct-to-R2 uploads)
		r.Post("/media/posts/presign", cfg.MediaHandler.PresignPostUpload)
		r.Post("/media/posts/presign/batch", cfg.MediaHandler.PresignPostUploadBatch)

		r.Route("/notifications", func(r chi.Router) {
			// Notification endpoints (to be implemented)
		})
	})

	return r
}
