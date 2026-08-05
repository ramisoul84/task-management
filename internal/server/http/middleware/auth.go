package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/auth"
)

const userIDKey = "user_id"

// TokenBlacklist reports whether an access token's jti has been revoked.
type TokenBlacklist interface {
	IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

// Auth requires a valid, non-revoked Bearer access token.
func Auth(tokens auth.TokenService, blacklist TokenBlacklist) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, ok := BearerToken(c)
		if !ok {
			return domain.ErrUnauthorized.WithMessage("missing or malformed Authorization header")
		}

		claims, err := tokens.ValidateAccessToken(token)
		if err != nil {
			return domain.ErrUnauthorized.WithMessage("invalid or expired access token")
		}

		// Revoked on logout. Fail closed: if we cannot tell, deny.
		revoked, err := blacklist.IsAccessTokenBlacklisted(c.UserContext(), claims.JTI)
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		if revoked {
			return domain.ErrUnauthorized.WithMessage("access token has been revoked")
		}

		c.Locals(userIDKey, claims.UserID)
		return c.Next()
	}
}

// BearerToken extracts the token from "Authorization: Bearer <token>".
func BearerToken(c *fiber.Ctx) (string, bool) {
	scheme, token, found := strings.Cut(c.Get(fiber.HeaderAuthorization), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// UserID returns the authenticated user's ID.
func UserID(c *fiber.Ctx) (string, bool) {
	id, ok := c.Locals(userIDKey).(string)
	return id, ok
}
