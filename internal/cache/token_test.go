package cache_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
)

func TestTokenCache_RefreshRotation(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedis(t)
	c := cache.NewTokenCache(rdb, time.Hour)

	if err := c.SetRefreshToken(ctx, "old", "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.RotateRefreshToken(ctx, "old", "new", "user-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := c.GetRefreshToken(ctx, "old"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("old token must be consumed, got %v", err)
	}
	if uid, err := c.GetRefreshToken(ctx, "new"); err != nil || uid != "user-1" {
		t.Fatalf("new token = (%q, %v)", uid, err)
	}

	// Replaying the consumed token fails.
	if err := c.RotateRefreshToken(ctx, "old", "newer", "user-1"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("reuse = %v, want ErrInvalidCredentials", err)
	}
}

// Two requests racing with the same refresh token: exactly one may win.
func TestTokenCache_ConcurrentRotationHasOneWinner(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedis(t)
	c := cache.NewTokenCache(rdb, time.Hour)
	_ = c.SetRefreshToken(ctx, "tok", "user-1")

	const n = 20
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.RotateRefreshToken(ctx, "tok", string(rune('a'+i)), "user-1"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("%d concurrent rotations succeeded, want exactly 1", wins)
	}
}

func TestTokenCache_Blacklist(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newRedis(t)
	c := cache.NewTokenCache(rdb, time.Hour)

	if err := c.BlacklistAccessToken(ctx, "jti-1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if banned, _ := c.IsAccessTokenBlacklisted(ctx, "jti-1"); !banned {
		t.Fatal("jti must be blacklisted")
	}
	if banned, _ := c.IsAccessTokenBlacklisted(ctx, "jti-2"); banned {
		t.Fatal("unrelated jti must not be blacklisted")
	}

	mr.FastForward(time.Minute + time.Second)
	if banned, _ := c.IsAccessTokenBlacklisted(ctx, "jti-1"); banned {
		t.Fatal("blacklist entry must expire with the token")
	}
}
