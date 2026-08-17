package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/redis/go-redis/v9"
)

type RefreshTokenRepository interface {
	Set(ctx context.Context, hash, userID string) error
	Get(ctx context.Context, hash string) (string, error)
	Rotate(ctx context.Context, oldHash string, newHash string, userID string) error
	Delete(ctx context.Context, hash string) error
}

type refreshTokenRepository struct {
	cache      *cache.Cache
	refreshTTL time.Duration
}

func NewRefreshTokenRepository(cache *cache.Cache, refreshTTL time.Duration) RefreshTokenRepository {
	return &refreshTokenRepository{
		cache:      cache,
		refreshTTL: refreshTTL,
	}
}

func (r *refreshTokenRepository) Set(ctx context.Context, hash, userID string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Set(ctx, key, userID, r.refreshTTL).Err(); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}

	return nil
}

func (r *refreshTokenRepository) Get(ctx context.Context, hash string) (string, error) {
	key := refreshTokenKey(hash)

	userID, err := r.cache.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", domain.ErrNotFound
		}

		return "", fmt.Errorf("get refresh token: %w", err)
	}

	return userID, nil
}

func (r *refreshTokenRepository) Rotate(ctx context.Context, oldHash string, newHash string, userID string) error {
	oldKey := refreshTokenKey(oldHash)
	newKey := refreshTokenKey(newHash)

	_, err := r.cache.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, oldKey)
		pipe.Set(ctx, newKey, userID, r.refreshTTL)

		return nil
	})

	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}

	return nil
}

func (r *refreshTokenRepository) Delete(ctx context.Context, hash string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

func refreshTokenKey(hashedRefreshToken string) string {
	return "auth:refresh:" + hashedRefreshToken
}
