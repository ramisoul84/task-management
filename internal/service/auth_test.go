package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/auth"
)

type authFixture struct {
	s        *store
	sessions *fakeTokenCache
	hasher   *plainHasher
	tokens   auth.TokenService
	svc      service.AuthService
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	f := &authFixture{
		s:        newStore(),
		sessions: newFakeTokenCache(),
		hasher:   &plainHasher{},
		tokens:   auth.NewTokenService("test-secret-test-secret-test-secret", "test", 15*time.Minute),
	}
	f.svc = service.NewAuthService(userRepo{f.s}, f.sessions, f.tokens, f.hasher, 15*time.Minute)
	return f
}

func (f *authFixture) register(t *testing.T) *domain.User {
	t.Helper()
	u, err := f.svc.Register(context.Background(), &domain.RegisterRequest{
		Email: "  Rami@Example.COM ", Password: "s3cret-pass", Name: "  Rami Suliman ",
	})
	wantErr(t, err, nil)
	return u
}

func TestRegister(t *testing.T) {
	f := newAuthFixture(t)
	u := f.register(t)

	if u.Email != "rami@example.com" {
		t.Fatalf("email = %q, want normalised", u.Email)
	}
	if u.Name != "Rami Suliman" {
		t.Fatalf("name = %q; names must not be re-cased", u.Name)
	}
	if u.PasswordHash != "hashed:s3cret-pass" || u.CreatedAt.IsZero() {
		t.Fatalf("unexpected user: %+v", u)
	}

	_, err := f.svc.Register(context.Background(), &domain.RegisterRequest{Email: "rami@example.com", Password: "x", Name: "x"})
	wantErr(t, err, domain.ErrAlreadyExists)
}

func TestLogin(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	f.register(t)

	t.Run("success issues tokens and stores refresh session", func(t *testing.T) {
		res, err := f.svc.Login(ctx, &domain.LoginRequest{Email: "RAMI@example.com", Password: "s3cret-pass"})
		wantErr(t, err, nil)
		if res.Token.AccessToken == "" || res.Token.RefreshToken == "" || res.Token.ExpiresIn != 900 {
			t.Fatalf("unexpected tokens: %+v", res.Token)
		}
		if _, err := f.sessions.GetRefreshToken(ctx, f.tokens.HashRefreshToken(res.Token.RefreshToken)); err != nil {
			t.Fatal("refresh token must be stored (hashed)")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, err := f.svc.Login(ctx, &domain.LoginRequest{Email: "rami@example.com", Password: "nope"})
		wantErr(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("unknown email still performs a hash comparison", func(t *testing.T) {
		before := f.hasher.compares
		_, err := f.svc.Login(ctx, &domain.LoginRequest{Email: "ghost@example.com", Password: "whatever"})
		wantErr(t, err, domain.ErrInvalidCredentials)
		if f.hasher.compares != before+1 {
			t.Fatal("missing-user path must cost a bcrypt comparison (timing side channel)")
		}
	})
}

func TestRefresh_RotatesAndRejectsReuse(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	f.register(t)

	login, err := f.svc.Login(ctx, &domain.LoginRequest{Email: "rami@example.com", Password: "s3cret-pass"})
	wantErr(t, err, nil)

	pair, err := f.svc.Refresh(ctx, login.Token.RefreshToken)
	wantErr(t, err, nil)
	if pair.RefreshToken == login.Token.RefreshToken {
		t.Fatal("refresh token must rotate")
	}

	_, err = f.svc.Refresh(ctx, login.Token.RefreshToken)
	wantErr(t, err, domain.ErrInvalidCredentials)

	_, err = f.svc.Refresh(ctx, "")
	wantErr(t, err, domain.ErrInvalidCredentials)
}

func TestLogout_RevokesBothTokens(t *testing.T) {
	ctx := context.Background()
	f := newAuthFixture(t)
	f.register(t)

	login, err := f.svc.Login(ctx, &domain.LoginRequest{Email: "rami@example.com", Password: "s3cret-pass"})
	wantErr(t, err, nil)

	wantErr(t, f.svc.Logout(ctx, login.Token.AccessToken, login.Token.RefreshToken), nil)

	claims, err := f.tokens.ValidateAccessToken(login.Token.AccessToken)
	wantErr(t, err, nil)
	if banned, _ := f.sessions.IsAccessTokenBlacklisted(ctx, claims.JTI); !banned {
		t.Fatal("access token must be blacklisted")
	}
	if _, err := f.svc.Refresh(ctx, login.Token.RefreshToken); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatal("refresh token must be revoked")
	}

	// Idempotent with nothing to revoke.
	wantErr(t, f.svc.Logout(ctx, "", ""), nil)
}
