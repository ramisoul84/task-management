package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/redisx"
)

func newRedis(t *testing.T) (*miniredis.Miniredis, *redisx.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, &redisx.Client{Client: rdb}
}

func page(total int) *domain.TaskListResponse {
	return &domain.TaskListResponse{Tasks: []*domain.Task{}, Total: total, Limit: 20}
}

func TestTaskCache_HitMissAndFilters(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedis(t)
	c := cache.NewTaskCache(rdb, 5*time.Minute)

	q := &domain.TaskListQuery{TeamID: "t1", Limit: 20}
	got, stamp, err := c.Get(ctx, q)
	if err != nil || got != nil || stamp == "" {
		t.Fatalf("first Get = (%v, %q, %v), want miss with stamp", got, stamp, err)
	}

	if err := c.Set(ctx, q, stamp, page(3)); err != nil {
		t.Fatal(err)
	}

	got, _, err = c.Get(ctx, q)
	if err != nil || got == nil || got.Total != 3 {
		t.Fatalf("Get after Set = (%v, %v), want hit", got, err)
	}

	// A different filter is a different entry.
	other := &domain.TaskListQuery{TeamID: "t1", Status: domain.TaskStatusDone, Limit: 20}
	if got, _, _ := c.Get(ctx, other); got != nil {
		t.Fatal("different filters must not share a cache entry")
	}
}

func TestTaskCache_InvalidationIsScopedToTeam(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedis(t)
	c := cache.NewTaskCache(rdb, 5*time.Minute)

	a := &domain.TaskListQuery{TeamID: "team-a", Limit: 20}
	b := &domain.TaskListQuery{TeamID: "team-b", Limit: 20}
	for _, q := range []*domain.TaskListQuery{a, b} {
		_, s, _ := c.Get(ctx, q)
		_ = c.Set(ctx, q, s, page(1))
	}

	if err := c.InvalidateTeam(ctx, "team-a"); err != nil {
		t.Fatal(err)
	}

	if got, _, _ := c.Get(ctx, a); got != nil {
		t.Fatal("team-a entry must be invalidated")
	}
	if got, _, _ := c.Get(ctx, b); got == nil {
		t.Fatal("team-b entry must survive team-a invalidation")
	}
}

// The scenario delete-based invalidation gets wrong: a reader misses,
// loads from the DB, a writer changes a task and invalidates, then the
// reader stores its (now stale) result.
func TestTaskCache_StaleWriteAfterInvalidationIsNeverServed(t *testing.T) {
	ctx := context.Background()
	_, rdb := newRedis(t)
	c := cache.NewTaskCache(rdb, 5*time.Minute)
	q := &domain.TaskListQuery{TeamID: "t1", Limit: 20}

	_, readerStamp, _ := c.Get(ctx, q) // reader: miss, goes to MySQL...

	if err := c.InvalidateTeam(ctx, "t1"); err != nil { // ...writer commits a change
		t.Fatal(err)
	}

	if err := c.Set(ctx, q, readerStamp, page(1)); err != nil { // ...reader stores stale page
		t.Fatal(err)
	}

	if got, _, _ := c.Get(ctx, q); got != nil {
		t.Fatalf("stale page (total=%d) was served after invalidation", got.Total)
	}
}

func TestTaskCache_TTL(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newRedis(t)
	c := cache.NewTaskCache(rdb, 5*time.Minute)
	q := &domain.TaskListQuery{TeamID: "t1", Limit: 20}

	_, s, _ := c.Get(ctx, q)
	_ = c.Set(ctx, q, s, page(1))

	mr.FastForward(5*time.Minute + time.Second)

	if got, _, _ := c.Get(ctx, q); got != nil {
		t.Fatal("entry must expire after the TTL")
	}
}

func TestTaskCache_RedisDown(t *testing.T) {
	ctx := context.Background()
	mr, rdb := newRedis(t)
	c := cache.NewTaskCache(rdb, 5*time.Minute)
	mr.Close()

	got, stamp, err := c.Get(ctx, &domain.TaskListQuery{TeamID: "t1"})
	if err == nil || got != nil || stamp != "" {
		t.Fatalf("Get with Redis down = (%v, %q, %v), want error and empty stamp", got, stamp, err)
	}
	// An empty stamp makes Set a no-op rather than an error.
	if err := c.Set(ctx, &domain.TaskListQuery{TeamID: "t1"}, "", page(1)); err != nil {
		t.Fatalf("Set with empty stamp = %v, want nil", err)
	}
}
