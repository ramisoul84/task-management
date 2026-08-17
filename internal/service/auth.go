package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type AuthService interface {
	Register(ctx context.Context, email, password, name string) (*domain.User, error)
	Login(ctx context.Context, email, password string) (*domain.AuthResult, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type authSvc struct {
	userRepo         repository.UserRepository
	refreshTokenRepo repository.RefreshTokenRepository
	token            auth.TokenService
	hasher           auth.Hasher
	log              *logger.Logger
	accessExpiry     time.Duration
}

func NewAuthService(
	userRepo repository.UserRepository,
	refreshTokenRepo repository.RefreshTokenRepository,
	token auth.TokenService,
	hasher auth.Hasher,
	log *logger.Logger,
	accessExpiry time.Duration,
) AuthService {
	return &authSvc{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		token:            token,
		hasher:           hasher,
		log:              log,
		accessExpiry:     accessExpiry,
	}
}

func (s *authSvc) Register(ctx context.Context, email, password, name string) (*domain.User, error) {
	email = normalizeEmail(email)
	name = capitalizeFirst(name)

	if email == "" || password == "" || name == "" {
		return nil, domain.ErrInvalidInput
	}

	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		s.log.Error().Err(err).Msg("hash password")
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil, domain.ErrAlreadyExists
		}

		s.log.Error().Err(err).Msg("create user")
		return nil, fmt.Errorf("create user: %w", err)
	}

	s.log.Info().
		Str("user_id", user.ID).
		Msg("user registered")
	return user, nil
}

func (s *authSvc) Login(ctx context.Context, email, password string) (*domain.AuthResult, error) {
	email = normalizeEmail(email)

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().Msg("invalid credentials")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().Err(err).Msg("get user")
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		s.log.Warn().Msg("invalid credentials")
		return nil, domain.ErrInvalidCredentials
	}

	accessToken, err := s.token.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		s.log.Error().Err(err).Msg("generate access token")
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.token.GenerateRefreshToken()
	if err != nil {
		s.log.Error().Err(err).Msg("generate refresh token")
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	refreshHash := s.token.HashRefreshToken(refreshToken)

	err = s.refreshTokenRepo.Set(ctx, refreshHash, user.ID)
	if err != nil {
		s.log.Error().Err(err).Msg("save refresh token")
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return &domain.AuthResult{
		User: user,
		Token: &domain.TokenPair{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresIn:    int64(s.accessExpiry.Seconds()),
		},
	}, nil
}

func (s *authSvc) Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		return nil, domain.ErrInvalidCredentials
	}

	oldHash := s.token.HashRefreshToken(refreshToken)

	userID, err := s.refreshTokenRepo.Get(ctx, oldHash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().Msg("refresh token not found")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().Err(err).Msg("get refresh token")
		return nil, fmt.Errorf("get refresh token: %w", err)
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().Msg("user not found")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().Err(err).Msg("get user")
		return nil, fmt.Errorf("get user: %w", err)
	}

	accessToken, err := s.token.GenerateAccessToken(
		user.ID,
		user.Email,
	)
	if err != nil {
		s.log.Error().Err(err).Msg("generate access token")
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	newRefreshToken, err := s.token.GenerateRefreshToken()
	if err != nil {
		s.log.Error().Err(err).Msg("generate refresh token")
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	newHash := s.token.HashRefreshToken(newRefreshToken)

	if err := s.refreshTokenRepo.Rotate(ctx, oldHash, newHash, user.ID); err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			s.log.Warn().Msg("refresh token reuse detected")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().Err(err).Msg("rotate refresh token")
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	return &domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.accessExpiry.Seconds()),
	}, nil
}

func (s *authSvc) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		return nil
	}

	hash := s.token.HashRefreshToken(refreshToken)

	if err := s.refreshTokenRepo.Delete(ctx, hash); err != nil {
		s.log.Error().Err(err).Msg("delete refresh token")
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

// helper

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func capitalizeFirst(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return s
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
