package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type AuthService interface {
	Register(ctx context.Context, req *domain.RegisterRequest) (*domain.User, error)
	Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResult, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, accessToken, refreshToken string) error
}

type authService struct {
	users     repository.UserRepository
	tokens    auth.TokenService
	sessions  cache.TokenCache
	hasher    auth.Hasher
	accessTTL time.Duration
	now       func() time.Time

	// dummyHash is compared against when the email is unknown, so a login
	// for a missing user costs the same bcrypt work as a wrong password and
	// response timing does not reveal which emails are registered.
	dummyHash string
}

func NewAuthService(
	users repository.UserRepository,
	sessions cache.TokenCache,
	tokens auth.TokenService,
	hasher auth.Hasher,
	accessTTL time.Duration,
) AuthService {
	dummy, _ := hasher.Hash("timing-equaliser-not-a-real-password")

	return &authService{
		users:     users,
		tokens:    tokens,
		sessions:  sessions,
		hasher:    hasher,
		accessTTL: accessTTL,
		now:       func() time.Time { return time.Now().UTC().Truncate(time.Second) },
		dummyHash: dummy,
	}
}

func (s *authService) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.User, error) {
	hash, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		ID:           uuid.NewString(),
		Email:        normalizeEmail(req.Email),
		PasswordHash: hash,
		Name:         strings.TrimSpace(req.Name),
		CreatedAt:    s.now(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info().Str("user_id", user.ID).Msg("user registered")
	return user, nil
}

func (s *authService) Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResult, error) {
	user, err := s.users.GetByEmail(ctx, normalizeEmail(req.Email))
	if errors.Is(err, domain.ErrNotFound) {
		_ = s.hasher.Compare(s.dummyHash, req.Password)
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := s.hasher.Compare(user.PasswordHash, req.Password); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	pair, err := s.issueTokens(ctx, user)
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info().Str("user_id", user.ID).Msg("user logged in")
	return &domain.AuthResult{User: user, Token: pair}, nil
}

// Refresh exchanges a refresh token for a new token pair. Refresh tokens
// are single-use: rotation atomically consumes the old one, so a replayed
// (stolen) token is rejected.
func (s *authService) Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, domain.ErrInvalidCredentials.WithMessage("refresh token required")
	}

	oldHash := s.tokens.HashRefreshToken(refreshToken)

	userID, err := s.sessions.GetRefreshToken(ctx, oldHash)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidCredentials.WithMessage("refresh token is invalid or expired")
	}
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	access, err := s.tokens.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}
	refresh, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	if err := s.sessions.RotateRefreshToken(ctx, oldHash, s.tokens.HashRefreshToken(refresh), user.ID); err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			logger.FromContext(ctx).Warn().Str("user_id", user.ID).Msg("refresh token reuse detected")
		}
		return nil, err
	}

	return &domain.TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(s.accessTTL.Seconds())}, nil
}

// Logout revokes the access token (for its remaining lifetime) and the
// refresh token. It is idempotent and succeeds even with missing tokens.
func (s *authService) Logout(ctx context.Context, accessToken, refreshToken string) error {
	if accessToken != "" {
		if claims, err := s.tokens.ValidateAccessToken(accessToken); err == nil {
			if ttl := time.Until(claims.ExpiresAt); ttl > 0 {
				if err := s.sessions.BlacklistAccessToken(ctx, claims.JTI, ttl); err != nil {
					return err
				}
			}
		}
	}

	if refreshToken = strings.TrimSpace(refreshToken); refreshToken != "" {
		if err := s.sessions.DeleteRefreshToken(ctx, s.tokens.HashRefreshToken(refreshToken)); err != nil {
			return err
		}
	}

	return nil
}

func (s *authService) issueTokens(ctx context.Context, user *domain.User) (*domain.TokenPair, error) {
	access, err := s.tokens.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}
	refresh, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := s.sessions.SetRefreshToken(ctx, s.tokens.HashRefreshToken(refresh), user.ID); err != nil {
		return nil, err
	}
	return &domain.TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(s.accessTTL.Seconds())}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
