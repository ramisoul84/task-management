package middleware

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	redis *redis.Client
}

func NewRateLimiter(redis *redis.Client) *RateLimiter {
	return &RateLimiter{redis: redis}
}

// GlobalLimit limits requests per IP across all routes
func (r *RateLimiter) GlobalLimit(max int, window time.Duration) fiber.Handler {
	return r.limit("global", max, window)
}

// AuthLimit limits auth requests per IP (stricter)
func (r *RateLimiter) AuthLimit(max int, window time.Duration) fiber.Handler {
	return r.limit("auth", max, window)
}

// limit creates a rate limiter for a specific group
func (r *RateLimiter) limit(group string, max int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Fail open if Redis is nil or unreachable
		if r.redis == nil {
			return c.Next()
		}

		key := fmt.Sprintf("rate:%s:%s", group, c.IP())

		count, err := r.increment(c, key, window)
		if err != nil {
			// Fail open on Redis error
			return c.Next()
		}

		if count > int64(max) {
			c.Set("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			return fiber.NewError(fiber.StatusTooManyRequests, "too many requests")
		}

		return c.Next()
	}
}

// increment increments the counter and sets expiry on first request
func (r *RateLimiter) increment(c *fiber.Ctx, key string, window time.Duration) (int64, error) {
	ctx := c.UserContext()

	count, err := r.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// Set expiry on first request in window
	if count == 1 {
		r.redis.Expire(ctx, key, window)
	}

	return count, nil
}
