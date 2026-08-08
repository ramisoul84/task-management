package middleware

import (
	"regexp"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/ramisoul84/task-management/pkg/logger"
)

const (
	HeaderRequestID = "X-Request-ID"
	requestIDKey    = "request_id"
)

// Incoming IDs are echoed into logs and headers, so only accept short,
// boring values to avoid log injection.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,64}$`)

// RequestID assigns every request an ID (reusing a well-formed incoming
// X-Request-ID), returns it in the response, and attaches a child logger
// carrying it to the request context. Everything downstream that logs via
// logger.FromContext is then correlated automatically.
func RequestID(base zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = uuid.NewString()
		}

		c.Set(HeaderRequestID, id)
		c.Locals(requestIDKey, id)

		l := base.With().Str("request_id", id).Logger()
		c.SetUserContext(logger.WithContext(c.UserContext(), l))

		return c.Next()
	}
}

// GetRequestID returns the current request's ID.
func GetRequestID(c *fiber.Ctx) string {
	id, _ := c.Locals(requestIDKey).(string)
	return id
}
