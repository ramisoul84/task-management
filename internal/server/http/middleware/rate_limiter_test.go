package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
)

func limitedApp(t *testing.T, maxRequests int) (*fiber.App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })

	app := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler})
	app.Use(middleware.NewRateLimiter(rdb).GlobalLimit(maxRequests, time.Minute))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app, mr
}

type result struct {
	StatusCode int
	Header     http.Header
}

func get(t *testing.T, app *fiber.App) result {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return result{StatusCode: resp.StatusCode, Header: resp.Header}
}

func TestRateLimiter_BlocksAfterLimitAndSetsHeaders(t *testing.T) {
	app, _ := limitedApp(t, 2)

	for i, wantRemaining := range []string{"1", "0"} {
		resp := get(t, app)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i+1, resp.StatusCode)
		}
		if got := resp.Header.Get("X-RateLimit-Remaining"); got != wantRemaining {
			t.Fatalf("request %d: remaining %q, want %q", i+1, got, wantRemaining)
		}
	}

	resp := get(t, app)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

func TestRateLimiter_WindowResets(t *testing.T) {
	app, mr := limitedApp(t, 1)

	_ = get(t, app)
	if get(t, app).StatusCode != http.StatusTooManyRequests {
		t.Fatal("second request must be limited")
	}

	mr.FastForward(time.Minute + time.Second)

	if resp := get(t, app); resp.StatusCode != http.StatusOK {
		t.Fatalf("after window: status %d, want 200", resp.StatusCode)
	}
}

func TestRateLimiter_CounterAlwaysHasExpiry(t *testing.T) {
	app, mr := limitedApp(t, 10)
	_ = get(t, app)

	for _, k := range mr.Keys() {
		if mr.TTL(k) <= 0 {
			t.Fatalf("key %s has no expiry; a client could be locked out forever", k)
		}
	}
}

func TestRateLimiter_FailsOpenWhenRedisIsDown(t *testing.T) {
	app, mr := limitedApp(t, 1)
	mr.Close()

	if resp := get(t, app); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200 (fail open)", resp.StatusCode)
	}
}
