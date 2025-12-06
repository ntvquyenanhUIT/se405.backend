package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"iamstagram_22520060/internal/model"
)

// =============================================================================
// MOCK REPOSITORY
// =============================================================================
//
// In unit tests, we don't want to hit a real database. Instead, we create a
// "mock" that implements the same interface but returns controlled responses.
//
// This is the KEY insight: because UserService depends on the UserRepository
// INTERFACE (not the concrete implementation), we can swap in a mock.

type mockUserRepository struct {
	// These functions let each test define custom behavior
	createFn           func(ctx context.Context, user *model.User) error
	getByIDFn          func(ctx context.Context, id int64) (*model.User, error)
	getByUsernameFn    func(ctx context.Context, username string) (*model.User, error)
	existsByUsernameFn func(ctx context.Context, username string) (bool, error)

	// Track calls for assertions
	createCalls []createCall
}

type createCall struct {
	User *model.User
}

func (m *mockUserRepository) Create(ctx context.Context, user *model.User) error {
	m.createCalls = append(m.createCalls, createCall{User: user})
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return nil
}

func (m *mockUserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, model.ErrUserNotFound
}

func (m *mockUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	if m.getByUsernameFn != nil {
		return m.getByUsernameFn(ctx, username)
	}
	return nil, model.ErrUserNotFound
}

func (m *mockUserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	if m.existsByUsernameFn != nil {
		return m.existsByUsernameFn(ctx, username)
	}
	return false, nil
}

// =============================================================================
// REGISTER TESTS
// =============================================================================

func TestUserService_Register_Success(t *testing.T) {
	// ARRANGE: Set up test data and mocks
	mockRepo := &mockUserRepository{
		existsByUsernameFn: func(ctx context.Context, username string) (bool, error) {
			return false, nil // Username doesn't exist
		},
		createFn: func(ctx context.Context, user *model.User) error {
			// Simulate database setting ID and timestamps
			user.ID = 1
			user.CreatedAt = time.Now()
			user.UpdatedAt = time.Now()
			return nil
		},
	}
	svc := NewUserService(mockRepo)

	req := &model.RegisterRequest{
		Username:    "testuser",
		Password:    "securepassword123",
		DisplayName: "Test User",
	}

	// ACT: Call the method we're testing
	user, err := svc.Register(context.Background(), req)

	// ASSERT: Check the results
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if user == nil {
		t.Fatal("expected user, got nil")
	}

	if user.Username != req.Username {
		t.Errorf("username = %q, want %q", user.Username, req.Username)
	}

	if user.DisplayName == nil || *user.DisplayName != req.DisplayName {
		t.Errorf("display_name = %v, want %q", user.DisplayName, req.DisplayName)
	}

	if !user.IsNewUser {
		t.Error("expected IsNewUser to be true for new registration")
	}

	// Verify password was hashed (not stored in plain text!)
	if user.PasswordHashed == req.Password {
		t.Error("password should be hashed, not stored in plain text")
	}

	// Verify the hash is valid bcrypt
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHashed), []byte(req.Password))
	if err != nil {
		t.Error("password hash should be valid bcrypt hash")
	}

	// Verify Create was called exactly once
	if len(mockRepo.createCalls) != 1 {
		t.Errorf("Create called %d times, want 1", len(mockRepo.createCalls))
	}
}

func TestUserService_Register_UsernameExists(t *testing.T) {
	mockRepo := &mockUserRepository{
		existsByUsernameFn: func(ctx context.Context, username string) (bool, error) {
			return true, nil // Username already exists
		},
	}
	svc := NewUserService(mockRepo)

	req := &model.RegisterRequest{
		Username: "existinguser",
		Password: "password123",
	}

	user, err := svc.Register(context.Background(), req)

	// Should return ErrUsernameExists
	if !errors.Is(err, model.ErrUsernameExists) {
		t.Errorf("error = %v, want %v", err, model.ErrUsernameExists)
	}

	if user != nil {
		t.Error("user should be nil when registration fails")
	}

	// Verify Create was NOT called
	if len(mockRepo.createCalls) != 0 {
		t.Error("Create should not be called when username exists")
	}
}

func TestUserService_Register_CheckUsernameError(t *testing.T) {
	dbError := errors.New("database connection failed")
	mockRepo := &mockUserRepository{
		existsByUsernameFn: func(ctx context.Context, username string) (bool, error) {
			return false, dbError // Database error
		},
	}
	svc := NewUserService(mockRepo)

	req := &model.RegisterRequest{
		Username: "testuser",
		Password: "password123",
	}

	_, err := svc.Register(context.Background(), req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The original error should be wrapped
	if !errors.Is(err, dbError) {
		t.Errorf("error should wrap original database error")
	}
}

func TestUserService_Register_CreateError(t *testing.T) {
	dbError := errors.New("insert failed")
	mockRepo := &mockUserRepository{
		existsByUsernameFn: func(ctx context.Context, username string) (bool, error) {
			return false, nil
		},
		createFn: func(ctx context.Context, user *model.User) error {
			return dbError
		},
	}
	svc := NewUserService(mockRepo)

	req := &model.RegisterRequest{
		Username: "testuser",
		Password: "password123",
	}

	_, err := svc.Register(context.Background(), req)

	if !errors.Is(err, dbError) {
		t.Errorf("error should wrap create error")
	}
}

func TestUserService_Register_WithoutDisplayName(t *testing.T) {
	mockRepo := &mockUserRepository{
		existsByUsernameFn: func(ctx context.Context, username string) (bool, error) {
			return false, nil
		},
		createFn: func(ctx context.Context, user *model.User) error {
			user.ID = 1
			return nil
		},
	}
	svc := NewUserService(mockRepo)

	req := &model.RegisterRequest{
		Username: "testuser",
		Password: "password123",
		// DisplayName intentionally omitted
	}

	user, err := svc.Register(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.DisplayName != nil {
		t.Errorf("display_name should be nil when not provided, got %v", user.DisplayName)
	}
}

// =============================================================================
// LOGIN TESTS - Table-Driven (THE Go idiom)
// =============================================================================

func TestUserService_Login(t *testing.T) {
	validPassword := "correctpassword"
	validHash, _ := bcrypt.GenerateFromPassword([]byte(validPassword), bcrypt.MinCost)

	testUser := &model.User{
		ID:             1,
		Username:       "testuser",
		PasswordHashed: string(validHash),
	}

	tests := []struct {
		name          string
		username      string
		password      string
		mockGetByUser func(ctx context.Context, username string) (*model.User, error)
		wantErr       error
		wantUser      bool
	}{
		{
			name:     "successful login",
			username: "testuser",
			password: validPassword,
			mockGetByUser: func(ctx context.Context, username string) (*model.User, error) {
				return testUser, nil
			},
			wantErr:  nil,
			wantUser: true,
		},
		{
			name:     "user not found",
			username: "nonexistent",
			password: "anypassword",
			mockGetByUser: func(ctx context.Context, username string) (*model.User, error) {
				return nil, model.ErrUserNotFound
			},
			wantErr:  model.ErrInvalidCredentials, // Don't reveal user doesn't exist
			wantUser: false,
		},
		{
			name:     "wrong password",
			username: "testuser",
			password: "wrongpassword",
			mockGetByUser: func(ctx context.Context, username string) (*model.User, error) {
				return testUser, nil
			},
			wantErr:  model.ErrInvalidCredentials,
			wantUser: false,
		},
		{
			name:     "database error",
			username: "testuser",
			password: validPassword,
			mockGetByUser: func(ctx context.Context, username string) (*model.User, error) {
				return nil, errors.New("database error")
			},
			wantErr:  model.ErrInvalidCredentials, // Don't reveal internal errors
			wantUser: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockUserRepository{
				getByUsernameFn: tt.mockGetByUser,
			}
			svc := NewUserService(mockRepo)

			req := &model.LoginRequest{
				Username: tt.username,
				Password: tt.password,
			}

			user, err := svc.Login(context.Background(), req)

			// Check error
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check user
			if tt.wantUser && user == nil {
				t.Error("expected user, got nil")
			}
			if !tt.wantUser && user != nil {
				t.Error("expected nil user")
			}
		})
	}
}

// =============================================================================
// GETBYID TESTS
// =============================================================================

func TestUserService_GetByID(t *testing.T) {
	tests := []struct {
		name      string
		userID    int64
		mockGetFn func(ctx context.Context, id int64) (*model.User, error)
		wantErr   error
		wantUser  bool
	}{
		{
			name:   "user found",
			userID: 1,
			mockGetFn: func(ctx context.Context, id int64) (*model.User, error) {
				return &model.User{ID: id, Username: "testuser"}, nil
			},
			wantErr:  nil,
			wantUser: true,
		},
		{
			name:   "user not found",
			userID: 999,
			mockGetFn: func(ctx context.Context, id int64) (*model.User, error) {
				return nil, model.ErrUserNotFound
			},
			wantErr:  model.ErrUserNotFound,
			wantUser: false,
		},
		{
			name:   "database error",
			userID: 1,
			mockGetFn: func(ctx context.Context, id int64) (*model.User, error) {
				return nil, errors.New("connection timeout")
			},
			wantErr:  nil, // We expect some error
			wantUser: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockUserRepository{
				getByIDFn: tt.mockGetFn,
			}
			svc := NewUserService(mockRepo)

			user, err := svc.GetByID(context.Background(), tt.userID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			}

			if tt.wantUser && user == nil {
				t.Error("expected user, got nil")
			}
			if !tt.wantUser && user != nil {
				t.Error("expected nil user")
			}
		})
	}
}
