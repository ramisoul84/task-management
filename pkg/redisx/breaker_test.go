package redisx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"

	"github.com/ramisoul84/task-management/pkg/redisx"
)

func TestCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)

	rdb := &redisx.Client{Client: redis.NewClient(&redis.Options{
		Addr:        mr.Addr(),
		MaxRetries:  -1,
		DialTimeout: 200 * time.Millisecond,
	})}
	t.Cleanup(func() { _ = rdb.Close() })

	var transitions []string
	cb := redisx.WithCircuitBreaker(rdb, redisx.BreakerConfig{
		ConsecutiveFailures: 3,
		OpenTimeout:         100 * time.Millisecond,
		OnStateChange: func(from, to gobreaker.State) {
			transitions = append(transitions, from.String()+"->"+to.String())
		},
	})

	// Misses are healthy answers and must never trip the breaker.
	for range 10 {
		if err := rdb.Get(ctx, "missing").Err(); !errors.Is(err, redis.Nil) {
			t.Fatalf("Get missing = %v, want redis.Nil", err)
		}
	}
	if cb.State() != gobreaker.StateClosed {
		t.Fatal("cache misses must not open the breaker")
	}

	// Redis goes away: after 3 failures the breaker opens.
	addr := mr.Addr()
	mr.Close()
	for range 3 {
		_ = rdb.Ping(ctx).Err()
	}
	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("state = %s, want open", cb.State())
	}

	// While open, commands fail immediately without touching the network.
	start := time.Now()
	err := rdb.Set(ctx, "k", "v", 0).Err()
	if !errors.Is(err, redisx.ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Fatalf("open breaker took %v; must fail fast", time.Since(start))
	}
	if _, err := rdb.Pipelined(ctx, func(p redis.Pipeliner) error {
		p.Incr(ctx, "n")
		return nil
	}); !errors.Is(err, redisx.ErrCircuitOpen) {
		t.Fatalf("pipeline err = %v, want ErrCircuitOpen", err)
	}

	// Redis comes back; after the open timeout a probe closes the breaker.
	mr2 := miniredis.NewMiniRedis()
	if err := mr2.StartAddr(addr); err != nil {
		t.Skipf("cannot rebind %s: %v", addr, err)
	}
	t.Cleanup(mr2.Close)

	time.Sleep(150 * time.Millisecond)
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	// Half-open closes after MaxRequests consecutive successes.
	for range 5 {
		_ = rdb.Ping(ctx).Err()
	}
	if cb.State() != gobreaker.StateClosed {
		t.Fatalf("state = %s, want closed after successful probes", cb.State())
	}

	want := []string{"closed->open", "open->half-open", "half-open->closed"}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for i := range want {
		if transitions[i] != want[i] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}
