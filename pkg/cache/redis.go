package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/ramisoul84/task-management/internal/config"
	"github.com/redis/go-redis/v9"
)

type Cache struct {
	*redis.Client
}

func New(cfg config.RedisConfig) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,

		MaxRetries:      cfg.MaxRetries,
		MinRetryBackoff: 100 * time.Millisecond,
		MaxRetryBackoff: 1 * time.Second,

		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,

		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("cache: ping: %w", err)
	}

	return &Cache{
		Client: client,
	}, nil
}

func (c *Cache) Health(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return fmt.Errorf("cache is not initialized")
	}

	return c.Client.Ping(ctx).Err()
}

func (c *Cache) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}

	return c.Client.Close()
}
