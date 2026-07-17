package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type AuthService interface {
	Register(ctx context.Context, email, password, name string) (*domain.User, error)
	Login(ctx context.Context, email, password string) (*domain.AuthResult, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
	Logout(ctx context.Context, accessToken, refreshToken string) error
}

type authSvc struct {
	userRepo     repository.UserRepository
	tokenCache   cache.TokenCache
	token        auth.TokenService
	hasher       auth.Hasher
	log          *logger.Logger
	accessExpiry time.Duration
}

func NewAuthService(
	userRepo repository.UserRepository,
	tokenCache cache.TokenCache,
	token auth.TokenService,
	hasher auth.Hasher,
	log *logger.Logger,
	accessExpiry time.Duration,
) AuthService {
	return &authSvc{
		userRepo:     userRepo,
		tokenCache:   tokenCache,
		token:        token,
		hasher:       hasher,
		log:          log,
		accessExpiry: accessExpiry,
	}
}

func (s *authSvc) Register(ctx context.Context, email, password, name string) (*domain.User, error) {
	const op = "authSvc.Register"
	requestID := logger.RequestIDFromContext(ctx)

	email = normalizeEmail(email)
	name = capitalizeFirst(name)

	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("email", email).
			Msg("hash password failed")
		return nil, fmt.Errorf("%s: hash password: %w", op, err)
	}

	user := &domain.User{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			s.log.Warn().
				Str("request_id", requestID).
				Str("op", op).
				Str("email", email).
				Msg("registration failed: email already exists")
			return nil, domain.ErrAlreadyExists
		}

		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("email", email).
			Msg("create user failed")
		return nil, fmt.Errorf("%s: create user: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("user_id", user.ID).
		Str("email", user.Email).
		Str("name", user.Name).
		Msg("user registered successfully")

	return user, nil
}

func (s *authSvc) Login(ctx context.Context, email, password string) (*domain.AuthResult, error) {
	const op = "authSvc.Login"
	requestID := logger.RequestIDFromContext(ctx)

	email = normalizeEmail(email)

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().
				Str("request_id", requestID).
				Str("op", op).
				Str("email", email).
				Msg("login failed: user not found")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("email", email).
			Msg("get user failed")
		return nil, fmt.Errorf("%s: get user: %w", op, err)
	}

	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Str("email", email).
			Msg("login failed: wrong password")
		return nil, domain.ErrInvalidCredentials
	}

	accessToken, err := s.token.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Msg("generate access token failed")
		return nil, fmt.Errorf("%s: generate access token: %w", op, err)
	}

	refreshToken, err := s.token.GenerateRefreshToken()
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Msg("generate refresh token failed")
		return nil, fmt.Errorf("%s: generate refresh token: %w", op, err)
	}

	refreshHash := s.token.HashRefreshToken(refreshToken)

	if err := s.tokenCache.SetRefreshToken(ctx, refreshHash, user.ID); err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Msg("save refresh token failed")
		return nil, fmt.Errorf("%s: save refresh token: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("user_id", user.ID).
		Str("email", user.Email).
		Msg("user logged in successfully")

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
	const op = "authSvc.Refresh"
	requestID := logger.RequestIDFromContext(ctx)

	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Msg("refresh failed: empty token")
		return nil, domain.ErrInvalidCredentials
	}

	oldHash := s.token.HashRefreshToken(refreshToken)

	userID, err := s.tokenCache.GetRefreshToken(ctx, oldHash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().
				Str("request_id", requestID).
				Str("op", op).
				Msg("refresh failed: token not found")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Msg("get refresh token failed")
		return nil, fmt.Errorf("%s: get refresh token: %w", op, err)
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().
				Str("request_id", requestID).
				Str("op", op).
				Str("user_id", userID).
				Msg("refresh failed: user not found")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", userID).
			Msg("get user failed")
		return nil, fmt.Errorf("%s: get user: %w", op, err)
	}

	accessToken, err := s.token.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Msg("generate access token failed")
		return nil, fmt.Errorf("%s: generate access token: %w", op, err)
	}

	newRefreshToken, err := s.token.GenerateRefreshToken()
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Msg("generate refresh token failed")
		return nil, fmt.Errorf("%s: generate refresh token: %w", op, err)
	}

	newHash := s.token.HashRefreshToken(newRefreshToken)

	if err := s.tokenCache.RotateRefreshToken(ctx, oldHash, newHash, user.ID); err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			s.log.Warn().
				Str("request_id", requestID).
				Str("op", op).
				Str("user_id", user.ID).
				Msg("refresh failed: token reuse detected")
			return nil, domain.ErrInvalidCredentials
		}

		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", user.ID).
			Msg("rotate refresh token failed")
		return nil, fmt.Errorf("%s: rotate refresh token: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("user_id", user.ID).
		Str("email", user.Email).
		Msg("token refreshed successfully")

	return &domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.accessExpiry.Seconds()),
	}, nil
}

func (s *authSvc) Logout(ctx context.Context, accessToken, refreshToken string) error {
	const op = "authSvc.Logout"
	requestID := logger.RequestIDFromContext(ctx)

	// Revoke the access token for the remainder of its lifetime so a stolen or
	// stale token cannot be used after logout. The blacklist entry expires on
	// its own once the token would have naturally expired.
	if accessToken != "" {
		if claims, err := s.token.ValidateAccessToken(accessToken); err == nil && claims != nil {
			ttl := time.Until(claims.ExpiresAt)
			if ttl > 0 {
				if err := s.tokenCache.BlacklistAccessToken(ctx, claims.JTI, ttl); err != nil {
					s.log.Error().
						Err(err).
						Str("request_id", requestID).
						Str("op", op).
						Str("jti", claims.JTI).
						Dur("ttl", ttl).
						Msg("blacklist access token failed")
				} else {
					s.log.Info().
						Str("request_id", requestID).
						Str("op", op).
						Str("jti", claims.JTI).
						Dur("ttl", ttl).
						Msg("access token blacklisted")
				}
			}
		} else if err != nil {
			s.log.Warn().
				Err(err).
				Str("request_id", requestID).
				Str("op", op).
				Msg("validate access token failed - skipping blacklist")
		}
	}

	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		s.log.Info().
			Str("request_id", requestID).
			Str("op", op).
			Msg("logout: no refresh token provided")
		return nil
	}

	hash := s.token.HashRefreshToken(refreshToken)

	if err := s.tokenCache.DeleteRefreshToken(ctx, hash); err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Msg("delete refresh token failed")
		return fmt.Errorf("%s: delete refresh token: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Msg("user logged out successfully")

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
