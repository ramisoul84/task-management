package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Timeout puts a deadline on the request context. Repositories and caches
// use that context, so slow queries are cancelled instead of piling up.
func Timeout(d time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if d <= 0 {
			return c.Next()
		}
		ctx, cancel := context.WithTimeout(c.UserContext(), d)
		defer cancel()
		c.SetUserContext(ctx)
		return c.Next()
	}
}
