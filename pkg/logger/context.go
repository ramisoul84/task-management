package logger

import (
	"context"

	"github.com/ramisoul84/task-management/internal/domain"
)

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(domain.RequestIDKey).(string); ok {
		return id
	}
	return "unknown"
}
