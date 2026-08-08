// Package redisx builds a configured go-redis client.
package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds Redis connection settings.
type Config struct {
	Addr         string
	Password     string
	DB           int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
	MinIdleConns int
}

// Client wraps redis.Client with health and nil-safe close helpers.
type Client struct {
	*redis.Client
}

// Open connects to Redis and verifies connectivity.
func Open(ctx context.Context, cfg Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,

		MaxRetries:      cfg.MaxRetries,
		MinRetryBackoff: 100 * time.Millisecond,
		MaxRetryBackoff: time.Second,

		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,

		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return &Client{Client: rdb}, nil
}

// Health pings Redis.
func (c *Client) Health(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return errors.New("redis is not initialized")
	}
	return c.Ping(ctx).Err()
}

// Close closes the client. It is safe to call on a nil Client.
func (c *Client) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Close()
}
