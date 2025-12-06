package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"iamstagram_22520060/internal/config"
	"iamstagram_22520060/internal/model"
	"iamstagram_22520060/internal/repository"
)

// AuthService handles authentication-related business logic with refresh token rotation and reuse detection.
type AuthService struct {
	refreshTokenRepo repository.RefreshTokenRepository
	config           *config.Config
}

func NewAuthService(refreshTokenRepo repository.RefreshTokenRepository, cfg *config.Config) *AuthService {
	return &AuthService{
		refreshTokenRepo: refreshTokenRepo,
		config:           cfg,
	}
}

// GenerateTokenPair issues a new access token and persists a refresh token.
func (s *AuthService) GenerateTokenPair(ctx context.Context, userID int64, deviceInfo, ipAddress string) (*model.TokenPair, error) {
	accessToken, err := s.generateAccessToken(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshTokenRaw := uuid.New().String()
	refreshTokenHash := s.hashToken(refreshTokenRaw)

	refreshToken := &model.RefreshToken{
		UserID:    userID,
		TokenHash: refreshTokenHash,
		ExpiresAt: time.Now().Add(time.Duration(s.config.RefreshTokenMaxAge) * time.Second),
	}

	if deviceInfo != "" {
		refreshToken.DeviceInfo = &deviceInfo
	}
	if ipAddress != "" {
		refreshToken.IPAddress = &ipAddress
	}

	if err := s.refreshTokenRepo.Create(ctx, refreshToken); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenRaw,
		ExpiresIn:    s.config.AccessTokenMaxAge,
	}, nil
}

// RefreshTokens validates the refresh token and rotates a new pair.
func (s *AuthService) RefreshTokens(ctx context.Context, refreshTokenRaw, deviceInfo, ipAddress string) (*model.TokenPair, int64, error) {
	tokenHash := s.hashToken(refreshTokenRaw)

	token, err := s.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, 0, model.ErrRefreshTokenNotFound
	}

	if token.IsRevoked() {
		_ = s.revokeTokenFamily(ctx, token)
		// TODO: Log failure if revokeTokenFamily returns an error
		return nil, 0, model.ErrRefreshTokenReused
	}

	if token.IsExpired() {
		return nil, 0, model.ErrRefreshTokenExpired
	}

	newTokenPair, err := s.GenerateTokenPair(ctx, token.UserID, deviceInfo, ipAddress)
	if err != nil {
		return nil, 0, err
	}

	newTokenHash := s.hashToken(newTokenPair.RefreshToken)
	var replacedByID *string
	if newToken, err := s.refreshTokenRepo.FindByTokenHash(ctx, newTokenHash); err == nil && newToken != nil {
		replacedByID = &newToken.ID
	}

	if err := s.refreshTokenRepo.Revoke(ctx, token.ID, replacedByID); err != nil {
		// TODO: Add proper logging here
	}

	return newTokenPair, token.UserID, nil
}

func (s *AuthService) RevokeRefreshToken(ctx context.Context, refreshTokenRaw string) error {
	tokenHash := s.hashToken(refreshTokenRaw)
	token, err := s.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return err
	}
	return s.refreshTokenRepo.Revoke(ctx, token.ID, nil)
}

func (s *AuthService) RevokeAllUserTokens(ctx context.Context, userID int64) error {
	return s.refreshTokenRepo.RevokeAllForUser(ctx, userID)
}

func (s *AuthService) revokeTokenFamily(ctx context.Context, token *model.RefreshToken) error {
	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, token.UserID); err != nil {
		// TODO: Add proper logging here - this is a security-critical operation
		return fmt.Errorf("failed to revoke token family: %w", err)
	}
	return nil
}

func (s *AuthService) generateAccessToken(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Duration(s.config.AccessTokenMaxAge) * time.Second).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

func (s *AuthService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
