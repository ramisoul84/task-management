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

type TokenRepository interface {
	SetRefreshToken(ctx context.Context, hash, userID string) error
	GetRefreshToken(ctx context.Context, hash string) (string, error)
	RotateRefreshToken(ctx context.Context, oldHash string, newHash string, userID string) error
	DeleteRefreshToken(ctx context.Context, hash string) error
	BlacklistAccessToken(ctx context.Context, jti string, ttl time.Duration) error
	IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type tokenRepository struct {
	cache      *cache.Cache
	refreshTTL time.Duration
}

func NewTokenRepository(cache *cache.Cache, refreshTTL time.Duration) TokenRepository {
	return &tokenRepository{
		cache:      cache,
		refreshTTL: refreshTTL,
	}
}

func (r *tokenRepository) SetRefreshToken(ctx context.Context, hash, userID string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Client.Set(ctx, key, userID, r.refreshTTL).Err(); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}

	return nil
}

func (r *tokenRepository) GetRefreshToken(ctx context.Context, hash string) (string, error) {
	key := refreshTokenKey(hash)

	userID, err := r.cache.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", domain.ErrNotFound
		}

		return "", fmt.Errorf("get refresh token: %w", err)
	}

	return userID, nil
}

func (r *tokenRepository) RotateRefreshToken(ctx context.Context, oldHash string, newHash string, userID string) error {
	oldKey := refreshTokenKey(oldHash)
	newKey := refreshTokenKey(newHash)

	_, err := r.cache.Client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, oldKey)
		pipe.Set(ctx, newKey, userID, r.refreshTTL)

		return nil
	})

	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}

	return nil
}

func (r *tokenRepository) DeleteRefreshToken(ctx context.Context, hash string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

func (r *tokenRepository) BlacklistAccessToken(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}

	key := accessTokenKey(jti)
	if err := r.cache.Client.Set(ctx, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("cache: blacklist %s: %w", jti, err)
	}

	return nil
}

func (r *tokenRepository) IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	n, err := r.cache.Client.Exists(ctx, "auth:bl:"+jti).Result()
	if err != nil {
		return false, fmt.Errorf("cache: check blacklist %s: %w", jti, err)
	}

	return n > 0, nil
}

func refreshTokenKey(hashedRefreshToken string) string {
	return "auth:refresh:" + hashedRefreshToken
}

func accessTokenKey(jti string) string {
	return "auth:bl:" + jti
}
