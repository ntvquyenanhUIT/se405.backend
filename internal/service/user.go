package service

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/repository"
)

// UserService handles business logic for user operations
type UserService struct {
	repo       repository.UserRepository
	followRepo repository.FollowRepository
}

func NewUserService(repo repository.UserRepository, followRepo repository.FollowRepository) *UserService {
	return &UserService{
		repo:       repo,
		followRepo: followRepo,
	}
}

// Register creates a new user account with optional avatar metadata.
func (s *UserService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
	if strings.TrimSpace(req.Username) == "" {
		return nil, fmt.Errorf("username is required")
	}

	if strings.TrimSpace(req.Password) == "" {
		return nil, fmt.Errorf("password is required")
	}

	if (req.AvatarURL == nil) != (req.AvatarKey == nil) {
		return nil, fmt.Errorf("avatar_url and avatar_key must both be provided or both omitted")
	}

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

// GetProfile retrieves a user's profile with follow relationship status.
//
// Design decision - Two-query approach:
// 1. Fetch user by ID (fails fast with 404 if user doesn't exist)
// 2. Check if viewer follows this user (only if viewer is authenticated and not viewing self)
//
// Alternative: Single query with LEFT JOIN on follows table
// Trade-offs:
//
//	Current approach:
//	  ✅ Clear separation: user existence check vs follow status check
//	  ✅ Graceful degradation (follow check failure doesn't block profile)
//	  ✅ Simpler SQL (no conditional JOIN logic)
//	  ⚠️ Two DB roundtrips
//	LEFT JOIN approach:
//	  ✅ Single DB roundtrip
//	  ❌ More complex SQL with conditional JOIN
//	  ❌ Follow check failure = entire request fails
//
// Current approach prioritizes simplicity and robustness. Optimize if profiling shows issues.
func (s *UserService) GetProfile(ctx context.Context, userID int64, viewerID *int64) (*model.ProfileResponse, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	profile := &model.ProfileResponse{
		User:        user,
		IsFollowing: false,
	}

	if viewerID != nil && *viewerID != userID {
		isFollowing, err := s.followRepo.Exists(ctx, *viewerID, userID)
		if err == nil {
			profile.IsFollowing = isFollowing
		}
	}

	return profile, nil
}

// Search finds users by username with optional follow status enrichment.
// Uses batch query (CheckFollows with ANY($1)) to avoid N+1 problem when checking
// follow relationships for multiple users. See GetFollowers for detailed explanation
// of the two-query approach vs JOIN trade-offs.
func (s *UserService) Search(ctx context.Context, query string, limit int, viewerID *int64) ([]model.UserSummary, error) {
	users, err := s.repo.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	if viewerID != nil && len(users) > 0 {
		userIDs := make([]int64, len(users))
		for i, user := range users {
			userIDs[i] = user.ID
		}

		followMap, err := s.followRepo.CheckFollows(ctx, *viewerID, userIDs)
		if err == nil {
			for i := range users {
				users[i].IsFollowing = followMap[users[i].ID]
			}
		}
	}

	return users, nil
}
