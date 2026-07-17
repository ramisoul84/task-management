package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/logger"
)

func Logger(log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		requestID, ok := c.Locals(domain.RequestIDKey).(string)
		if !ok || requestID == "" {
			requestID = uuid.NewString()
			c.Locals(domain.RequestIDKey, requestID)

			ctx := context.WithValue(c.UserContext(), domain.RequestIDKey, requestID)
			c.SetUserContext(ctx)
		}

		log.Info().
			Str("request_id", requestID).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Str("ip", c.IP()).
			Msg("request started")

		err := c.Next()

		duration := time.Since(start)

		if c.Response().StatusCode() >= 400 {
			log.Error().
				Str("request_id", requestID).
				Str("method", c.Method()).
				Str("path", c.Path()).
				Int("status", c.Response().StatusCode()).
				Dur("duration_ms", duration).
				Msg("request failed")
		} else {
			log.Info().
				Str("request_id", requestID).
				Str("method", c.Method()).
				Str("path", c.Path()).
				Int("status", c.Response().StatusCode()).
				Dur("duration_ms", duration).
				Msg("request completed")
		}

		return err
	}
}
