package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gofiber/fiber/v2"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/logger"
)

// Recover turns a panic in any handler into a 500 response and logs the
// stack trace, instead of crashing the request goroutine.
func Recover() fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.FromContext(c.UserContext()).Error().
					Str("panic", fmt.Sprint(r)).
					Bytes("stack", debug.Stack()).
					Msg("panic recovered")
				err = domain.ErrInternal.Wrap(fmt.Errorf("panic: %v", r))
			}
		}()
		return c.Next()
	}
}
