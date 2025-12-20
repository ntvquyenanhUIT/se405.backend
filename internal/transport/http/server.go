package http

import (
	"context"
	"fmt"
	"log"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"

	"iamstagram_22520060/internal/cache"
	"iamstagram_22520060/internal/config"
	"iamstagram_22520060/internal/database"
	"iamstagram_22520060/internal/handler"
	"iamstagram_22520060/internal/queue"
	iredis "iamstagram_22520060/internal/redis"
	"iamstagram_22520060/internal/repository"
	"iamstagram_22520060/internal/service"
	"iamstagram_22520060/internal/worker"
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

	// Connect to Redis
	redisClient, err := iredis.NewClient(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("failed to create redis client: %w", err)
	}
	defer redisClient.Close()

	// Verify Redis connection (fail fast if unreachable)
	ctx := context.Background()
	if err := redisClient.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}
	log.Printf("Connected to Redis at %s", cfg.RedisURL)

	// Create Redis components
	feedCache := cache.NewFeedCache(redisClient.Client)
	publisher := queue.NewPublisher(redisClient.Client)
	consumer := queue.NewConsumer(redisClient.Client)

	// Create repositories
	userRepo := repository.NewUserRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	followRepo := repository.NewFollowRepository(db)
	postRepo := repository.NewPostRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	notifRepo := repository.NewNotificationRepository(db)
	deviceTokenRepo := repository.NewDeviceTokenRepository(db)

	// Create services (with publisher for event-driven services)
	userService := service.NewUserService(userRepo, followRepo)
	authService := service.NewAuthService(refreshTokenRepo, cfg)
	followService := service.NewFollowService(followRepo, userRepo, db, publisher)
	mediaService, err := service.NewMediaService(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize media service: %w", err)
	}
	postService := service.NewPostService(postRepo, userRepo, publisher, db)
	feedService := service.NewFeedService(feedCache, postRepo, followRepo, userRepo)
	commentService := service.NewCommentService(commentRepo, postRepo, userRepo, db, publisher)

	// Initialize Expo Push client for push notifications
	// Unlike FCM, Expo Push doesn't require any credentials!
	expoPushClient := service.NewExpoPushClient()
	log.Println("Expo Push client initialized - push notifications enabled")
	notifService := service.NewNotificationService(notifRepo, deviceTokenRepo, userRepo, expoPushClient)

	// Create worker components
	workerHandler := worker.NewHandler(feedCache, followRepo, postRepo)
	workerHandler.SetNotificationCreator(notifService) // Enable notification handling
	workerManager := worker.NewManager(consumer, workerHandler, worker.DefaultManagerConfig())

	// Start worker goroutines
	if err := workerManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start worker manager: %w", err)
	}
	log.Println("Worker manager started")

	// Create handlers
	authHandler := handler.NewAuthHandler(userService, authService, mediaService, cfg)
	userHandler := handler.NewUserHandler(userService)
	followHandler := handler.NewFollowHandler(followService)
	feedHandler := handler.NewFeedHandler(feedService)
	postHandler := handler.NewPostHandler(postService)
	mediaHandler := handler.NewMediaHandler(mediaService)
	commentHandler := handler.NewCommentHandler(commentService)
	notifHandler := handler.NewNotificationHandler(notifService)

	// Create router with dependencies
	router := NewRouter(RouterConfig{
		AuthHandler:         authHandler,
		UserHandler:         userHandler,
		FollowHandler:       followHandler,
		FeedHandler:         feedHandler,
		PostHandler:         postHandler,
		MediaHandler:        mediaHandler,
		CommentHandler:      commentHandler,
		NotificationHandler: notifHandler,
		JWTSecret:           cfg.JWTSecret,
	})

	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("Starting server on %s", addr)
	log.Printf("Routes:")
	log.Printf("  POST   /auth/register         - Register new user")
	log.Printf("  POST   /auth/login            - Login and get tokens")
	log.Printf("  POST   /auth/refresh          - Refresh tokens")
	log.Printf("  POST   /auth/logout           - Logout (protected)")
	log.Printf("  POST   /auth/logout-all       - Logout all devices (protected)")
	log.Printf("  GET    /me                    - Get current user (protected)")
	log.Printf("  GET    /users/search          - Search users (optional auth)")
	log.Printf("  GET    /users/:id             - Get user profile (optional auth)")
	log.Printf("  GET    /users/:id/followers   - Get user followers (optional auth)")
	log.Printf("  GET    /users/:id/following   - Get users following (optional auth)")
	log.Printf("  GET    /users/:id/posts       - Get user posts (optional auth)")
	log.Printf("  POST   /users/:id/follow      - Follow user (protected)")
	log.Printf("  DELETE /users/:id/follow      - Unfollow user (protected)")
	log.Printf("  GET    /feed                  - Get feed (protected)")
	log.Printf("  POST   /posts                 - Create post (protected)")
	log.Printf("  GET    /posts/:id             - Get post (optional auth)")
	log.Printf("  DELETE /posts/:id             - Delete post (protected)")
	log.Printf("  POST   /posts/:id/likes       - Like post (protected)")
	log.Printf("  DELETE /posts/:id/likes       - Unlike post (protected)")
	log.Printf("  GET    /posts/:id/likes       - Get post likers (protected)")
	log.Printf("  POST   /posts/:id/comments    - Create comment (protected)")
	log.Printf("  DELETE /posts/:id/comments/:id- Delete comment (protected)")
	log.Printf("  GET    /posts/:id/comments    - Get comments (protected)")

	// Setup graceful shutdown
	server := &stdhttp.Server{
		Addr:    addr,
		Handler: router,
	}

	// Channel to listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Run server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		return err
	case <-shutdown:
		log.Println("Shutting down gracefully...")

		// Stop worker manager first
		workerManager.Stop()

		// Shutdown HTTP server
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}

		log.Println("Server stopped")
		return nil
	}
}
