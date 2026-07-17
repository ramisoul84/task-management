package middleware

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/ramisoul84/task-management/internal/domain"
)

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set("X-Request-ID", requestID)
		c.Locals(domain.RequestIDKey, requestID)

		ctx := context.WithValue(c.UserContext(), domain.RequestIDKey, requestID)
		c.SetUserContext(ctx)

		return c.Next()
	}
}
