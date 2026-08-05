package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// CORS configures cross-origin access for browser clients.
func CORS(origins []string, allowCredentials bool) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     strings.Join(origins, ","),
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,If-Match,X-Request-ID",
		AllowCredentials: allowCredentials,
		ExposeHeaders:    "Content-Length,Retry-After,ETag,X-Request-ID,X-RateLimit-Limit,X-RateLimit-Remaining,X-RateLimit-Reset",
		MaxAge:           600,
	})
}
