package service

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/repository"
)

// UserService handles business logic for user operations
type UserService struct {
	repo repository.UserRepository
}

// NewUserService creates a new UserService with the given repository
func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// Register creates a new user account with optional avatar metadata.
func (s *UserService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
	// Check if username already exists
	exists, err := s.repo.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to check username: %w", err)
	}
	if exists {
		return nil, model.ErrUsernameExists
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user struct
	user := &model.User{
		Username:       req.Username,
		PasswordHashed: string(hashedPassword),
		IsNewUser:      true, // New users need onboarding
		AvatarURL:      req.AvatarURL,
		AvatarKey:      req.AvatarKey,
	}

	// Set display name if provided
	if req.DisplayName != "" {
		user.DisplayName = &req.DisplayName
	}

	// Save to database
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Login authenticates a user with username and password.
func (s *UserService) Login(ctx context.Context, req *model.LoginRequest) (*model.User, error) {
	// Get user by username
	user, err := s.repo.GetByUsername(ctx, req.Username)
	if err != nil {
		// Don't reveal whether username exists or not
		return nil, model.ErrInvalidCredentials
	}

	// Compare password with hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHashed), []byte(req.Password))
	if err != nil {
		return nil, model.ErrInvalidCredentials
	}

	return user, nil
}

// GetByID retrieves a user by ID.
func (s *UserService) GetByID(ctx context.Context, id int64) (*model.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return user, nil
}
