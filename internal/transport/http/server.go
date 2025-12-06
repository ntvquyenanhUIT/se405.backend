package http

import (
	"context"
	"fmt"
	"log"
	stdhttp "net/http"

	"iamstagram_22520060/internal/config"
	"iamstagram_22520060/internal/database"
	"iamstagram_22520060/internal/handler"
	"iamstagram_22520060/internal/repository"
	"iamstagram_22520060/internal/service"
)

func Run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create repositories
	userRepo := repository.NewUserRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)

	// Create services
	userService := service.NewUserService(userRepo)
	authService := service.NewAuthService(refreshTokenRepo, cfg)
	mediaService, err := service.NewMediaService(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize media service: %w", err)
	}

	// Create handlers
	authHandler := handler.NewAuthHandler(userService, authService, mediaService, cfg)

	// Create router with dependencies
	router := NewRouter(RouterConfig{
		AuthHandler: authHandler,
		JWTSecret:   cfg.JWTSecret,
	})

	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("Starting server on %s", addr)
	log.Printf("Routes:")
	log.Printf("  POST /auth/register     - Register new user")
	log.Printf("  POST /auth/login        - Login and get tokens")
	log.Printf("  POST /auth/refresh      - Refresh tokens")
	log.Printf("  POST /auth/logout       - Logout (protected)")
	log.Printf("  POST /auth/logout-all   - Logout all devices (protected)")
	log.Printf("  GET  /me                - Get current user (protected)")

	return stdhttp.ListenAndServe(addr, router)
}
