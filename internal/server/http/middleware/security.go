package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// SecureHeaders sets conservative security headers suitable for a JSON API.
func SecureHeaders(hsts bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		if hsts {
			c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		return c.Next()
	}
}
