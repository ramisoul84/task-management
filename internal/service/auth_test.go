package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/logger"
)

func newAuthService(
	t *testing.T,
	userRepo *fakeUserRepo,
	tokenCache *fakeTokenCache,
	tokenSvc *fakeTokenService,
	hasher *fakeHasher,
) service.AuthService {
	t.Helper()

	l := logger.New("debug", "test", false, "stdout")
	l.Logger = l.Logger.Level(zerolog.Disabled)

	return service.NewAuthService(userRepo, tokenCache, tokenSvc, hasher, l, 15*time.Minute)
}

func TestAuthService_Register(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		userRepo := &fakeUserRepo{}
		hasher := &fakeHasher{hash: "hashed_password"}
		svc := newAuthService(t, userRepo, &fakeTokenCache{}, &fakeTokenService{}, hasher)

		user, err := svc.Register(ctx, "  TEST@Example.com ", "StrongPass123!", "  john doe  ")

		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		if user == nil {
			t.Fatal("Register() = nil")
		}
		if user.Email != "test@example.com" {
			t.Errorf("Email = %q, want test@example.com", user.Email)
		}
		if user.Name != "John doe" {
			t.Errorf("Name = %q, want 'John doe'", user.Name)
		}
		if user.ID == "" {
			t.Error("ID is empty")
		}
		if user.PasswordHash != "hashed_password" {
			t.Errorf("PasswordHash = %q", user.PasswordHash)
		}
	})

	t.Run("hash_error", func(t *testing.T) {
		hasher := &fakeHasher{hashErr: errRepoFailure}
		svc := newAuthService(t, &fakeUserRepo{}, &fakeTokenCache{}, &fakeTokenService{}, hasher)

		_, err := svc.Register(ctx, "test@example.com", "password", "John")
		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errRepoFailure)
		}
	})

	t.Run("duplicate_email", func(t *testing.T) {
		userRepo := &fakeUserRepo{err: domain.ErrAlreadyExists}
		hasher := &fakeHasher{hash: "hash"}
		svc := newAuthService(t, userRepo, &fakeTokenCache{}, &fakeTokenService{}, hasher)

		_, err := svc.Register(ctx, "test@example.com", "password", "John")
		if !errors.Is(err, domain.ErrAlreadyExists) {
			t.Fatalf("error = %v, want %v", err, domain.ErrAlreadyExists)
		}
	})
}

func TestAuthService_Login(t *testing.T) {
	ctx := context.Background()

	user := &domain.User{
		ID:           "user-1",
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
		Name:         "John Doe",
	}

	t.Run("success", func(t *testing.T) {
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-1": user}}
		tokenCache := &fakeTokenCache{}
		tokenSvc := &fakeTokenService{
			accessToken:  "access-token-123",
			refreshToken: "refresh-token-456",
		}
		hasher := &fakeHasher{}
		svc := newAuthService(t, userRepo, tokenCache, tokenSvc, hasher)

		result, err := svc.Login(ctx, "TEST@example.com", "password")

		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if result == nil {
			t.Fatal("Login() = nil")
		}
		if result.User.ID != "user-1" {
			t.Errorf("User.ID = %q", result.User.ID)
		}
		if result.Token.AccessToken != "access-token-123" {
			t.Errorf("AccessToken = %q", result.Token.AccessToken)
		}
		if result.Token.RefreshToken != "refresh-token-456" {
			t.Errorf("RefreshToken = %q", result.Token.RefreshToken)
		}
	})

	t.Run("user_not_found", func(t *testing.T) {
		userRepo := &fakeUserRepo{users: map[string]*domain.User{}}
		svc := newAuthService(t, userRepo, &fakeTokenCache{}, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Login(ctx, "ghost@example.com", "password")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("wrong_password", func(t *testing.T) {
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-1": user}}
		hasher := &fakeHasher{compareErr: errors.New("wrong password")}
		svc := newAuthService(t, userRepo, &fakeTokenCache{}, &fakeTokenService{}, hasher)

		_, err := svc.Login(ctx, "test@example.com", "wrong")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})
}

func TestAuthService_Refresh(t *testing.T) {
	ctx := context.Background()

	user := &domain.User{ID: "user-1", Email: "test@example.com"}

	t.Run("success", func(t *testing.T) {
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-1": user}}
		tokenCache := &fakeTokenCache{refreshToken: "user-1"}
		tokenSvc := &fakeTokenService{
			accessToken:  "new-access-token",
			refreshToken: "new-refresh-token",
		}
		svc := newAuthService(t, userRepo, tokenCache, tokenSvc, &fakeHasher{})

		result, err := svc.Refresh(ctx, "valid-refresh-token")

		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if result.AccessToken != "new-access-token" {
			t.Errorf("AccessToken = %q", result.AccessToken)
		}
		if result.RefreshToken != "new-refresh-token" {
			t.Errorf("RefreshToken = %q", result.RefreshToken)
		}
	})

	t.Run("empty_token", func(t *testing.T) {
		svc := newAuthService(t, &fakeUserRepo{}, &fakeTokenCache{}, &fakeTokenService{}, &fakeHasher{})
		_, err := svc.Refresh(ctx, "")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("token_not_found", func(t *testing.T) {
		tokenCache := &fakeTokenCache{getErr: domain.ErrNotFound}
		svc := newAuthService(t, &fakeUserRepo{}, tokenCache, &fakeTokenService{}, &fakeHasher{})

		_, err := svc.Refresh(ctx, "invalid-token")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})
}

func TestAuthService_Logout(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		tokenCache := &fakeTokenCache{}
		tokenSvc := &fakeTokenService{
			claims: &auth.TokenClaims{
				JTI:       "token-id-1",
				ExpiresAt: time.Now().Add(15 * time.Minute),
			},
		}
		svc := newAuthService(t, &fakeUserRepo{}, tokenCache, tokenSvc, &fakeHasher{})

		err := svc.Logout(ctx, "access-token", "refresh-token")
		if err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
	})

	t.Run("empty_refresh_token_returns_nil", func(t *testing.T) {
		svc := newAuthService(t, &fakeUserRepo{}, &fakeTokenCache{}, &fakeTokenService{}, &fakeHasher{})

		err := svc.Logout(ctx, "", "")
		if err != nil {
			t.Fatalf("Logout() error = %v, want nil", err)
		}
	})
}
