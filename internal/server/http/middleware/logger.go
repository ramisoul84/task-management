package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/ramisoul84/task-management/pkg/logger"
)

// AccessLog writes exactly one structured line per request, after the
// response status is final.
//
// Errors returned down the chain are rendered here (via the app's
// ErrorHandler) rather than after this middleware returns, so the logged
// status always matches what the client received.
func AccessLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		chainErr := c.Next()
		if chainErr != nil {
			if err := c.App().ErrorHandler(c, chainErr); err != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		status := c.Response().StatusCode()

		var ev *zerolog.Event
		l := logger.FromContext(c.UserContext())
		switch {
		case status >= fiber.StatusInternalServerError:
			ev = l.Error()
		case status >= fiber.StatusBadRequest:
			ev = l.Warn()
		default:
			ev = l.Info()
		}

		if chainErr != nil && status < fiber.StatusInternalServerError {
			// 5xx causes are already logged by the error handler.
			ev = ev.AnErr("reason", chainErr)
		}
		if uid, ok := UserID(c); ok {
			ev = ev.Str("user_id", uid)
		}

		ev.Str("method", c.Method()).
			Str("path", c.Path()).
			Str("route", c.Route().Path).
			Int("status", status).
			Dur("latency", time.Since(start)).
			Int("bytes", len(c.Response().Body())).
			Str("ip", c.IP()).
			Str("user_agent", c.Get(fiber.HeaderUserAgent)).
			Msg("http request")

		return nil
	}
}
