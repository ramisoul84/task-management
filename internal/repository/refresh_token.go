package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/ramisoul84/task-management/pkg/logger"
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
	log        *logger.Logger
	refreshTTL time.Duration
}

func NewRefreshTokenRepository(cache *cache.Cache, log *logger.Logger, refreshTTL time.Duration) RefreshTokenRepository {
	return &refreshTokenRepository{
		cache: cache,
		log:   log,
	}
}

func (r *refreshTokenRepository) Set(ctx context.Context, hash, userID string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Set(ctx, key, userID, r.refreshTTL).Err(); err != nil {
		r.log.Error().Err(err).Msg("save refresh token")
		return fmt.Errorf("save refresh token: %w", err)
	}

	r.log.Info().Msg("refresh token saved")

	return nil
}

func (r *refreshTokenRepository) Get(ctx context.Context, hash string) (string, error) {
	key := refreshTokenKey(hash)

	hashedRefreshToken, err := r.cache.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			r.log.Warn().Msg("refresh token not found")
			return "", domain.ErrNotFound
		}

		r.log.Error().Err(err).Msg("get refresh token")
		return "", fmt.Errorf("get refresh token: %w", err)
	}

	r.log.Info().Msg("refresh token found")

	return hashedRefreshToken, nil
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
		r.log.Error().Err(err).Msg("rotate refresh token")
		return fmt.Errorf("rotate refresh token: %w", err)
	}

	r.log.Info().Msg("refresh token rotated")

	return nil
}

func (r *refreshTokenRepository) Delete(ctx context.Context, hash string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Del(ctx, key).Err(); err != nil {
		r.log.Error().Err(err).Msg("delete refresh token")
		return fmt.Errorf("delete refresh token: %w", err)
	}

	r.log.Info().Msg("refresh token deleted")

	return nil
}

func refreshTokenKey(hashedRefreshToken string) string {
	return "auth:refresh:" + hashedRefreshToken
}
