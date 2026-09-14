package cache

import (
	"context"

	"github.com/ramisoul84/task-management/internal/domain"
)

// LookupRecorder receives cache lookup outcomes (implemented by metrics).
type LookupRecorder interface {
	CacheLookup(cache, result string)
}

// Instrument wraps a TaskCache and reports hit/miss/error for each Get.
func Instrument(c TaskCache, rec LookupRecorder) TaskCache {
	return &instrumented{TaskCache: c, rec: rec}
}

type instrumented struct {
	TaskCache
	rec LookupRecorder
}

func (i *instrumented) Get(ctx context.Context, q *domain.TaskListQuery) (*domain.TaskListResponse, Stamp, error) {
	page, stamp, err := i.TaskCache.Get(ctx, q)
	switch {
	case err != nil:
		i.rec.CacheLookup("task_list", "error")
	case page != nil:
		i.rec.CacheLookup("task_list", "hit")
	default:
		i.rec.CacheLookup("task_list", "miss")
	}
	return page, stamp, err
}
