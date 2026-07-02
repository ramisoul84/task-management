package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/redis/go-redis/v9"
)

type TaskCache interface {
	GetTasks(ctx context.Context, query *domain.TaskListQuery) (*domain.TaskListResponse, bool, error)
	SetTasks(ctx context.Context, query *domain.TaskListQuery, response *domain.TaskListResponse) error
	InvalidateTeamTasks(ctx context.Context, teamID string) error
}

type taskCache struct {
	cache   *cache.Cache
	taskTTL time.Duration
}

func NewTaskCache(cache *cache.Cache, taskTTL time.Duration) TaskCache {
	return &taskCache{
		cache:   cache,
		taskTTL: taskTTL,
	}
}

// GetTasks retrieves cached task list
func (c *taskCache) GetTasks(ctx context.Context, query *domain.TaskListQuery) (*domain.TaskListResponse, bool, error) {
	const op = "taskCache.GetTasks"

	key := c.generateKey(query)

	data, err := c.cache.Client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%s: get from cache: %w", op, err)
	}

	var response domain.TaskListResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, false, fmt.Errorf("%s: unmarshal cached data: %w", op, err)
	}

	return &response, true, nil
}

// SetTasks stores task list in cache
func (c *taskCache) SetTasks(ctx context.Context, query *domain.TaskListQuery, response *domain.TaskListResponse) error {
	const op = "taskCache.SetTasks"

	key := c.generateKey(query)

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("%s: marshal response: %w", op, err)
	}

	if err := c.cache.Client.Set(ctx, key, data, c.taskTTL).Err(); err != nil {
		return fmt.Errorf("%s: set cache: %w", op, err)
	}

	return nil
}

// InvalidateTeamTasks removes all cached task lists for a team
func (c *taskCache) InvalidateTeamTasks(ctx context.Context, teamID string) error {
	const op = "taskCache.InvalidateTeamTasks"

	pattern := fmt.Sprintf("tasks:team:%s:*", teamID)

	iter := c.cache.Client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.cache.Client.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("%s: delete key %s: %w", op, iter.Val(), err)
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("%s: scan keys: %w", op, err)
	}

	return nil
}

// generateKey creates a unique cache key based on query parameters
func (c *taskCache) generateKey(query *domain.TaskListQuery) string {
	return fmt.Sprintf("tasks:team:%s:status:%s:assignee:%s:limit:%d:offset:%d",
		query.TeamID,
		query.Status,
		query.AssigneeID,
		query.Limit,
		query.Offset,
	)
}
