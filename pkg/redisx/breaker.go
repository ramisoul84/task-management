package redisx

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
)

// ErrCircuitOpen is returned for Redis commands while the breaker is open.
var ErrCircuitOpen = errors.New("redis: circuit breaker open")

// BreakerConfig tunes the Redis circuit breaker.
type BreakerConfig struct {
	// Trip after this many consecutive failed commands.
	ConsecutiveFailures uint32
	// Stay open this long before letting a probe request through.
	OpenTimeout time.Duration
	// OnStateChange is called on every transition (for logs/metrics).
	OnStateChange func(from, to gobreaker.State)
}

// WithCircuitBreaker installs a breaker on every command and pipeline the
// client sends.
//
// Without it, a Redis outage makes every request that touches the cache,
// the rate limiter or the token blacklist wait out dial timeouts and
// retries. Once open, commands fail in microseconds with ErrCircuitOpen and
// callers take their fallback path (read MySQL, fail open, ...). After
// OpenTimeout a single probe is let through; success closes the breaker.
func WithCircuitBreaker(c *Client, cfg BreakerConfig) *gobreaker.CircuitBreaker[struct{}] {
	if cfg.ConsecutiveFailures == 0 {
		cfg.ConsecutiveFailures = 5
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = 15 * time.Second
	}

	cb := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name: "redis",
		// Requests allowed through while half-open. Must be > 1: when the
		// probe opens a fresh connection, go-redis sends its handshake
		// (HELLO, CLIENT SETINFO) through these same hooks, and a single
		// slot would be consumed by the probe itself, re-opening the
		// breaker forever.
		MaxRequests: 5,
		Timeout:     cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailures
		},
		IsSuccessful: isHealthy,
		OnStateChange: func(_ string, from, to gobreaker.State) {
			if cfg.OnStateChange != nil {
				cfg.OnStateChange(from, to)
			}
		},
	})

	c.AddHook(breakerHook{cb: cb})
	return cb
}

// isHealthy decides which outcomes count against Redis. A cache miss is a
// normal answer and a cancelled request is the client's doing; only
// transport-level problems mean Redis is unhealthy.
func isHealthy(err error) bool {
	if err == nil || errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
		return true
	}
	if _, ok := errors.AsType[net.Error](err); ok || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Server-side replies (WRONGTYPE, NOSCRIPT, ...) prove Redis is up.
	_, ok := errors.AsType[redis.Error](err)
	return ok
}

type breakerHook struct {
	cb *gobreaker.CircuitBreaker[struct{}]
}

func (h breakerHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h breakerHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		_, err := h.cb.Execute(func() (struct{}, error) {
			return struct{}{}, next(ctx, cmd)
		})
		if isOpen(err) {
			cmd.SetErr(ErrCircuitOpen)
			return ErrCircuitOpen
		}
		return err
	}
}

func (h breakerHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		_, err := h.cb.Execute(func() (struct{}, error) {
			return struct{}{}, next(ctx, cmds)
		})
		if isOpen(err) {
			for _, cmd := range cmds {
				cmd.SetErr(ErrCircuitOpen)
			}
			return ErrCircuitOpen
		}
		return err
	}
}

func isOpen(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}
