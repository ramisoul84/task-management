package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/redisx"
)

type TaskCache interface {
	Get(ctx context.Context, query *domain.TaskListQuery) (*domain.TaskListResponse, bool, error)
	Set(ctx context.Context, query *domain.TaskListQuery, response *domain.TaskListResponse) error
	InvalidateTeam(ctx context.Context, teamID string) error
}

type taskCache struct {
	cache   *redisx.Client
	taskTTL time.Duration
}

func NewTaskCache(cache *redisx.Client, taskTTL time.Duration) TaskCache {
	return &taskCache{
		cache:   cache,
		taskTTL: taskTTL,
	}
}

// Get retrieves a cached task list.
// Returns (nil, false, nil) on cache miss.
func (c *taskCache) Get(ctx context.Context, query *domain.TaskListQuery) (*domain.TaskListResponse, bool, error) {
	const op = "taskCache.GetTasks"

	key := c.generateKey(query)

	data, err := c.cache.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil // Cache miss
		}
		return nil, false, fmt.Errorf("%s: get from cache: %w", op, err)
	}

	var response domain.TaskListResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, false, fmt.Errorf("%s: unmarshal cached data: %w", op, err)
	}

	return &response, true, nil
}

// Set stores a task list in cache and tracks the key in the team's
// Redis set for efficient invalidation.
func (c *taskCache) Set(ctx context.Context, query *domain.TaskListQuery, response *domain.TaskListResponse) error {
	const op = "taskCache.SetTasks"

	key := c.generateKey(query)

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("%s: marshal response: %w", op, err)
	}

	// Store the cached data
	if err := c.cache.Set(ctx, key, data, c.taskTTL).Err(); err != nil {
		return fmt.Errorf("%s: set cache: %w", op, err)
	}

	// Track the key in the team's set for direct invalidation
	setKey := c.teamKeysSet(query.TeamID)
	pipe := c.cache.Pipeline()
	pipe.SAdd(ctx, setKey, key)
	pipe.Expire(ctx, setKey, c.taskTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("%s: track key in set: %w", op, err)
	}

	return nil
}

// InvalidateTeam removes all cached task lists for a team.
// Uses set-based tracking for O(1) deletion, with SCAN fallback for legacy keys.
func (c *taskCache) InvalidateTeam(ctx context.Context, teamID string) error {
	const op = "taskCache.InvalidateTeamTasks"

	setKey := c.teamKeysSet(teamID)

	// Get all tracked keys
	members, err := c.cache.SMembers(ctx, setKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("%s: read key set: %w", op, err)
	}

	// Batch delete tracked keys + the set itself
	if len(members) > 0 {
		pipe := c.cache.Pipeline()
		pipe.Del(ctx, members...)
		pipe.Del(ctx, setKey)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("%s: delete tracked keys: %w", op, err)
		}
	}

	// Defensive fallback: remove any untracked legacy keys
	pattern := fmt.Sprintf("tasks:team:%s:*", teamID)
	iter := c.cache.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.cache.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("%s: delete untracked key %s: %w", op, iter.Val(), err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("%s: scan keys: %w", op, err)
	}

	return nil
}

// teamKeysSet returns the Redis key for the set that tracks cached
// task-list keys for a given team.
func (c *taskCache) teamKeysSet(teamID string) string {
	return fmt.Sprintf("tasks:team:%s:keys", teamID)
}

// generateKey creates a unique cache key based on query parameters.
// Different filters produce different keys.
func (c *taskCache) generateKey(query *domain.TaskListQuery) string {
	return fmt.Sprintf("tasks:team:%s:status:%s:assignee:%s:limit:%d:offset:%d",
		query.TeamID,
		query.Status,
		query.AssigneeID,
		query.Limit,
		query.Offset,
	)
}
