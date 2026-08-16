package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/pkg/auth"
)

type contextKey string

const userIDKey contextKey = "user_id"

func Auth(tokens auth.TokenService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get(fiber.HeaderAuthorization)

		const prefix = "Bearer "

		if !strings.HasPrefix(header, prefix) {
			return fiber.ErrUnauthorized
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))

		if token == "" {
			return fiber.ErrUnauthorized
		}

		claims, err := tokens.ValidateAccessToken(token)
		if err != nil {
			return fiber.ErrUnauthorized
		}

		c.Locals(userIDKey, claims.UserID)

		return c.Next()
	}
}

func UserID(c *fiber.Ctx) (string, bool) {
	userID, ok := c.Locals(userIDKey).(string)

	return userID, ok
}
