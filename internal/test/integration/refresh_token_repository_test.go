//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
)

func TestRefreshTokenRepository_Rotate(t *testing.T) {
	c := newCache(t)
	repo := repository.NewRefreshTokenRepository(c, time.Hour)
	ctx := context.Background()

	const (
		oldHash = "old-hash"
		newHash = "new-hash"
		userID  = "user-1"
	)

	if err := repo.Set(ctx, oldHash, userID); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}

	got, err := repo.Get(ctx, oldHash)
	if err != nil {
		t.Fatalf("Get() old error = %v, want nil", err)
	}
	if got != userID {
		t.Errorf("Get() = %q, want %q", got, userID)
	}

	if err := repo.Rotate(ctx, oldHash, newHash, userID); err != nil {
		t.Fatalf("Rotate() error = %v, want nil", err)
	}

	got, err = repo.Get(ctx, newHash)
	if err != nil {
		t.Fatalf("Get() new error = %v, want nil", err)
	}
	if got != userID {
		t.Errorf("Get() new = %q, want %q", got, userID)
	}

	if _, err := repo.Get(ctx, oldHash); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get() old after rotate error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestRefreshTokenRepository_KeyExpiresAfterTTL(t *testing.T) {
	c := newCache(t)
	// A short real TTL: Redis treats 0 as "no expiry", so the expiry path can
	// only be exercised with an actual duration.
	repo := repository.NewRefreshTokenRepository(c, 200*time.Millisecond)
	ctx := context.Background()

	if err := repo.Set(ctx, "short-lived", "user-1"); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := repo.Get(ctx, "short-lived")
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("token is still alive after its TTL")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRefreshTokenRepository_Delete(t *testing.T) {
	c := newCache(t)
	repo := repository.NewRefreshTokenRepository(c, time.Hour)
	ctx := context.Background()

	if err := repo.Set(ctx, "hash-1", "user-1"); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}

	if err := repo.Delete(ctx, "hash-1"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	if _, err := repo.Get(ctx, "hash-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get() after delete error = %v, want %v", err, domain.ErrNotFound)
	}
}
