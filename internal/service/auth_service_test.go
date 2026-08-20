package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/logger"
)

const testAccessExpiry = 15 * time.Minute

func newAuthService(t *testing.T, user *fakeUserRepo, refresh *fakeRefreshTokenRepo, token *fakeTokenService, hasher *fakeHasher) service.AuthService {
	t.Helper()

	l := logger.New("debug", "test", false)
	l.Logger = l.Logger.Level(zerolog.Disabled)

	return service.NewAuthService(user, refresh, token, hasher, l, testAccessExpiry)
}

func TestAuthService_Register(t *testing.T) {
	ctx := context.Background()

	t.Run("success_normalizes_and_creates_user", func(t *testing.T) {
		userRepo := &fakeUserRepo{}
		hasher := &fakeHasher{hashOut: "hashed-secret"}
		tokenSvc := &fakeTokenService{}
		svc := newAuthService(t, userRepo, &fakeRefreshTokenRepo{}, tokenSvc, hasher)

		user, err := svc.Register(ctx, "  User@Example.COM  ", "secret", "jane doe")

		if err != nil {
			t.Fatalf("Register() error = %v, want nil", err)
		}
		if user.Email != "user@example.com" {
			t.Errorf("user.Email = %q, want %q", user.Email, "user@example.com")
		}
		if user.Name != "Jane doe" {
			t.Errorf("user.Name = %q, want %q", user.Name, "Jane doe")
		}
		if user.PasswordHash != "hashed-secret" {
			t.Errorf("user.PasswordHash = %q, want %q", user.PasswordHash, "hashed-secret")
		}
		if user.ID == "" {
			t.Error("user.ID is empty, want a generated ID")
		}
		if hasher.hashCalls != 1 {
			t.Errorf("hasher.Hash called %d time(s), want 1", hasher.hashCalls)
		}
	})

	t.Run("empty_email_is_invalid_input", func(t *testing.T) {
		svc := newAuthService(t, &fakeUserRepo{}, &fakeRefreshTokenRepo{}, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Register(ctx, "", "secret", "Jane")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("Register() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("empty_password_is_invalid_input", func(t *testing.T) {
		svc := newAuthService(t, &fakeUserRepo{}, &fakeRefreshTokenRepo{}, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Register(ctx, "user@example.com", "", "Jane")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("Register() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("duplicate_email_is_already_exists", func(t *testing.T) {
		userRepo := &fakeUserRepo{err: domain.ErrAlreadyExists}
		hasher := &fakeHasher{hashOut: "hashed-secret"}
		svc := newAuthService(t, userRepo, &fakeRefreshTokenRepo{}, &fakeTokenService{}, hasher)

		_, err := svc.Register(ctx, "user@example.com", "secret", "Jane")

		if !errors.Is(err, domain.ErrAlreadyExists) {
			t.Fatalf("Register() error = %v, want %v", err, domain.ErrAlreadyExists)
		}
	})

	t.Run("hash_failure_is_wrapped", func(t *testing.T) {
		hasher := &fakeHasher{hashErr: errRepoFailure}
		svc := newAuthService(t, &fakeUserRepo{}, &fakeRefreshTokenRepo{}, &fakeTokenService{}, hasher)

		_, err := svc.Register(ctx, "user@example.com", "secret", "Jane")

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("Register() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}

func TestAuthService_Login(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown_email_is_invalid_credentials", func(t *testing.T) {
		userRepo := &fakeUserRepo{users: map[string]*domain.User{}}
		svc := newAuthService(t, userRepo, &fakeRefreshTokenRepo{}, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Login(ctx, "ghost@example.com", "secret")

		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("wrong_password_is_invalid_credentials", func(t *testing.T) {
		user := &domain.User{ID: "user-1", Email: "user@example.com", PasswordHash: "hashed-secret"}
		userRepo := &fakeUserRepo{emails: map[string]*domain.User{user.Email: user}}
		hasher := &fakeHasher{compareErr: domain.ErrInvalidCredentials}
		svc := newAuthService(t, userRepo, &fakeRefreshTokenRepo{}, &fakeTokenService{}, hasher)

		_, err := svc.Login(ctx, "user@example.com", "wrong")

		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("success_returns_tokens_and_stores_refresh", func(t *testing.T) {
		user := &domain.User{ID: "user-1", Email: "user@example.com", PasswordHash: "hashed-secret"}
		userRepo := &fakeUserRepo{emails: map[string]*domain.User{user.Email: user}}
		refreshRepo := &fakeRefreshTokenRepo{tokens: map[string]string{}}
		tokenSvc := &fakeTokenService{accessToken: "access-1", refreshToken: "refresh-1"}
		hasher := &fakeHasher{}
		svc := newAuthService(t, userRepo, refreshRepo, tokenSvc, hasher)

		result, err := svc.Login(ctx, "user@example.com", "secret")

		if err != nil {
			t.Fatalf("Login() error = %v, want nil", err)
		}
		if result.User != user {
			t.Error("Login() returned a different user than the stored one")
		}
		if result.Token.AccessToken != "access-1" || result.Token.RefreshToken != "refresh-1" {
			t.Errorf("Login() tokens = %+v, want access-1/refresh-1", result.Token)
		}
		if result.Token.ExpiresIn != int64(testAccessExpiry.Seconds()) {
			t.Errorf("Login() ExpiresIn = %d, want %d", result.Token.ExpiresIn, int64(testAccessExpiry.Seconds()))
		}
		if tokenSvc.lastAccessUserID != user.ID {
			t.Errorf("access token generated for %q, want %q", tokenSvc.lastAccessUserID, user.ID)
		}
		if _, ok := refreshRepo.tokens["hash-refresh-1"]; !ok {
			t.Error("refresh token hash was not stored")
		}
		if refreshRepo.tokens["hash-refresh-1"] != user.ID {
			t.Errorf("refresh token mapped to %q, want %q", refreshRepo.tokens["hash-refresh-1"], user.ID)
		}
	})
}

func TestAuthService_Refresh(t *testing.T) {
	ctx := context.Background()

	t.Run("blank_token_is_invalid_credentials", func(t *testing.T) {
		svc := newAuthService(t, &fakeUserRepo{}, &fakeRefreshTokenRepo{}, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Refresh(ctx, "   ")

		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Refresh() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("unknown_token_is_invalid_credentials", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{tokens: map[string]string{}}
		svc := newAuthService(t, &fakeUserRepo{}, refreshRepo, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Refresh(ctx, "refresh-1")

		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Refresh() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("deleted_user_is_invalid_credentials", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{tokens: map[string]string{"hash-refresh-1": "user-1"}}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{}}
		svc := newAuthService(t, userRepo, refreshRepo, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Refresh(ctx, "refresh-1")

		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Refresh() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("reuse_of_rotated_token_is_invalid_credentials", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{
			tokens:    map[string]string{"hash-refresh-1": "user-1"},
			rotateErr: domain.ErrInvalidCredentials,
		}
		user := &domain.User{ID: "user-1", Email: "user@example.com"}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{user.ID: user}}
		svc := newAuthService(t, userRepo, refreshRepo, &fakeTokenService{refreshToken: "refresh-2"}, &fakeHasher{})

		_, err := svc.Refresh(ctx, "refresh-1")

		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Refresh() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("success_rotates_token_and_returns_new_pair", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{tokens: map[string]string{"hash-refresh-1": "user-1"}}
		user := &domain.User{ID: "user-1", Email: "user@example.com"}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{user.ID: user}}
		tokenSvc := &fakeTokenService{accessToken: "access-2", refreshToken: "refresh-2"}
		svc := newAuthService(t, userRepo, refreshRepo, tokenSvc, &fakeHasher{})

		pair, err := svc.Refresh(ctx, "refresh-1")

		if err != nil {
			t.Fatalf("Refresh() error = %v, want nil", err)
		}
		if pair.AccessToken != "access-2" || pair.RefreshToken != "refresh-2" {
			t.Errorf("Refresh() pair = %+v, want access-2/refresh-2", pair)
		}
		if len(refreshRepo.rotates) != 1 {
			t.Fatalf("Rotate called %d time(s), want 1", len(refreshRepo.rotates))
		}
		call := refreshRepo.rotates[0]
		if call.oldHash != "hash-refresh-1" || call.newHash != "hash-refresh-2" || call.userID != user.ID {
			t.Errorf("Rotate got %+v, want {hash-refresh-1 hash-refresh-2 user-1}", call)
		}
	})
}

func TestAuthService_Logout(t *testing.T) {
	ctx := context.Background()

	t.Run("blank_token_is_noop", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{tokens: map[string]string{}}
		svc := newAuthService(t, &fakeUserRepo{}, refreshRepo, &fakeTokenService{}, &fakeHasher{})

		err := svc.Logout(ctx, "   ")

		if err != nil {
			t.Fatalf("Logout() error = %v, want nil", err)
		}
		if len(refreshRepo.deleteCalls) != 0 {
			t.Errorf("Delete called %d time(s), want 0", len(refreshRepo.deleteCalls))
		}
	})

	t.Run("success_deletes_refresh_token", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{tokens: map[string]string{"hash-refresh-1": "user-1"}}
		svc := newAuthService(t, &fakeUserRepo{}, refreshRepo, &fakeTokenService{}, &fakeHasher{})

		err := svc.Logout(ctx, "refresh-1")

		if err != nil {
			t.Fatalf("Logout() error = %v, want nil", err)
		}
		if len(refreshRepo.deleteCalls) != 1 {
			t.Fatalf("Delete called %d time(s), want 1", len(refreshRepo.deleteCalls))
		}
		if refreshRepo.deleteCalls[0] != "hash-refresh-1" {
			t.Errorf("Delete got %q, want %q", refreshRepo.deleteCalls[0], "hash-refresh-1")
		}
		if _, ok := refreshRepo.tokens["hash-refresh-1"]; ok {
			t.Error("refresh token still present after logout")
		}
	})

	t.Run("delete_failure_is_wrapped", func(t *testing.T) {
		refreshRepo := &fakeRefreshTokenRepo{
			tokens:    map[string]string{"hash-refresh-1": "user-1"},
			deleteErr: errRepoFailure,
		}
		svc := newAuthService(t, &fakeUserRepo{}, refreshRepo, &fakeTokenService{}, &fakeHasher{})

		err := svc.Logout(ctx, "refresh-1")

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("Logout() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}
