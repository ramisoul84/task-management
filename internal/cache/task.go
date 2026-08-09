package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/redisx"
)

// Stamp identifies the cache generation a miss was observed in. Pass it
// back to Set so a result computed from stale data can never be served.
type Stamp string

// TaskCache caches task-list pages per team and filter combination.
type TaskCache interface {
	// Get returns the cached page, or nil on a miss together with the
	// Stamp to use for Set.
	Get(ctx context.Context, q *domain.TaskListQuery) (*domain.TaskListResponse, Stamp, error)
	// Set stores a page computed after the Get that produced stamp.
	Set(ctx context.Context, q *domain.TaskListQuery, stamp Stamp, page *domain.TaskListResponse) error
	// InvalidateTeam makes every cached page of the team unreachable.
	InvalidateTeam(ctx context.Context, teamID string) error
}

// genTTL bounds how long a team's generation counter lives after its last
// invalidation. It must be much longer than the page TTL: once the counter
// expires it restarts at 0, which is safe only because every page written
// under generation 0 has long since expired.
const genTTL = 24 * time.Hour

// taskCache uses generation-based invalidation:
//
//	tasks:{team}:gen            -> N       (bumped by INCR on every change)
//	tasks:{team}:g<N>:<filters> -> page    (expires after the page TTL)
//
// Invalidation is a single O(1) INCR instead of deleting keys, and it is
// race-free. A reader that missed in generation N and queries MySQL while
// a writer bumps the generation to N+1 stores its now-stale page under N,
// where no future reader looks. With delete-based invalidation that stale
// page would be served for the full TTL.
//
// The {team} hash tag keeps a team's keys in one Redis Cluster slot.
type taskCache struct {
	rdb *redisx.Client
	ttl time.Duration
}

func NewTaskCache(rdb *redisx.Client, ttl time.Duration) TaskCache {
	return &taskCache{rdb: rdb, ttl: ttl}
}

func genKey(teamID string) string {
	return "tasks:{" + teamID + "}:gen"
}

func pageKey(q *domain.TaskListQuery, gen Stamp) string {
	return fmt.Sprintf("tasks:{%s}:g%s:s=%s:a=%s:l=%d:o=%d",
		q.TeamID, gen, q.Status, q.AssigneeID, q.Limit, q.Offset)
}

func (c *taskCache) generation(ctx context.Context, teamID string) (Stamp, error) {
	gen, err := c.rdb.Get(ctx, genKey(teamID)).Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	if err != nil {
		return "", fmt.Errorf("taskCache: read generation: %w", err)
	}
	return Stamp(gen), nil
}

func (c *taskCache) Get(ctx context.Context, q *domain.TaskListQuery) (*domain.TaskListResponse, Stamp, error) {
	gen, err := c.generation(ctx, q.TeamID)
	if err != nil {
		return nil, "", err
	}

	data, err := c.rdb.Get(ctx, pageKey(q, gen)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, gen, nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("taskCache: get page: %w", err)
	}

	var page domain.TaskListResponse
	if err := json.Unmarshal(data, &page); err != nil {
		// Corrupt or incompatible entry: treat as a miss and overwrite it.
		return nil, gen, nil //nolint:nilerr // deliberate: self-healing cache
	}
	return &page, gen, nil
}

func (c *taskCache) Set(ctx context.Context, q *domain.TaskListQuery, stamp Stamp, page *domain.TaskListResponse) error {
	if stamp == "" {
		return nil
	}
	if _, err := strconv.ParseUint(string(stamp), 10, 64); err != nil {
		return fmt.Errorf("taskCache: invalid stamp %q", stamp)
	}

	data, err := json.Marshal(page)
	if err != nil {
		return fmt.Errorf("taskCache: marshal page: %w", err)
	}
	if err := c.rdb.Set(ctx, pageKey(q, stamp), data, c.ttl).Err(); err != nil {
		return fmt.Errorf("taskCache: set page: %w", err)
	}
	return nil
}

func (c *taskCache) InvalidateTeam(ctx context.Context, teamID string) error {
	key := genKey(teamID)
	_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Incr(ctx, key)
		p.Expire(ctx, key, genTTL)
		return nil
	})
	if err != nil {
		return fmt.Errorf("taskCache: bump generation: %w", err)
	}
	return nil
}
