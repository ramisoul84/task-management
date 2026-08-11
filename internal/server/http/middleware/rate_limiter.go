package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/logger"
)

// fixedWindow increments the counter and guarantees it has an expiry in a
// single atomic step. Doing INCR and EXPIRE as two commands risks a counter
// that never expires (if the EXPIRE is lost), locking a client out forever.
//
// Returns {count, ttl_ms}.
var fixedWindow = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {count, ttl}
`)

// RateLimiter is a Redis-backed fixed-window limiter keyed by client IP,
// so limits hold across all API replicas.
type RateLimiter struct {
	rdb redis.Scripter
}

func NewRateLimiter(rdb redis.Scripter) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// GlobalLimit applies to every API request.
func (r *RateLimiter) GlobalLimit(maxRequests int, window time.Duration) fiber.Handler {
	return r.limit("global", maxRequests, window)
}

// AuthLimit is a stricter limit for credential endpoints (brute force).
func (r *RateLimiter) AuthLimit(maxRequests int, window time.Duration) fiber.Handler {
	return r.limit("auth", maxRequests, window)
}

func (r *RateLimiter) limit(scope string, maxRequests int, window time.Duration) fiber.Handler {
	limit := strconv.Itoa(maxRequests)

	return func(c *fiber.Ctx) error {
		count, ttl, err := r.hit(c.UserContext(), "rate:"+scope+":"+c.IP(), window)
		if err != nil {
			// Fail open: an unavailable limiter must not take the API down.
			logger.FromContext(c.UserContext()).Warn().Err(err).Str("scope", scope).Msg("rate limiter unavailable")
			return c.Next()
		}

		resetSecs := strconv.FormatInt(int64((ttl+time.Second-1)/time.Second), 10)
		c.Set("X-RateLimit-Limit", limit)
		c.Set("X-RateLimit-Remaining", strconv.FormatInt(max(int64(maxRequests)-count, 0), 10))
		c.Set("X-RateLimit-Reset", resetSecs)

		if count > int64(maxRequests) {
			c.Set(fiber.HeaderRetryAfter, resetSecs)
			return domain.ErrTooManyRequests.WithMessage("rate limit exceeded, retry after " + resetSecs + "s")
		}
		return c.Next()
	}
}

func (r *RateLimiter) hit(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error) {
	res, err := fixedWindow.Run(ctx, r.rdb, []string{key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return 0, 0, err
	}
	return res[0], time.Duration(res[1]) * time.Millisecond, nil
}
