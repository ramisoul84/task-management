package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/redisx"
)

type TokenCache interface {
	SetRefreshToken(ctx context.Context, hash, userID string) error
	GetRefreshToken(ctx context.Context, hash string) (string, error)
	RotateRefreshToken(ctx context.Context, oldHash string, newHash string, userID string) error
	DeleteRefreshToken(ctx context.Context, hash string) error
	BlacklistAccessToken(ctx context.Context, jti string, ttl time.Duration) error
	IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type tokenCache struct {
	cache      *redisx.Client
	refreshTTL time.Duration
}

func NewTokenCache(cache *redisx.Client, refreshTTL time.Duration) TokenCache {
	return &tokenCache{
		cache:      cache,
		refreshTTL: refreshTTL,
	}
}

func (r *tokenCache) SetRefreshToken(ctx context.Context, hash, userID string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Set(ctx, key, userID, r.refreshTTL).Err(); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}

	return nil
}

func (r *tokenCache) GetRefreshToken(ctx context.Context, hash string) (string, error) {
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

func (r *tokenCache) RotateRefreshToken(ctx context.Context, oldHash string, newHash string, userID string) error {
	oldKey := refreshTokenKey(oldHash)
	newKey := refreshTokenKey(newHash)

	// Atomically fetch-and-delete the old token. If it no longer exists (or
	// belongs to a different user than expected), it was already consumed by
	// a previous rotation — this is a replay/reuse attempt on a stale token.
	oldVal, err := r.cache.GetDel(ctx, oldKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return domain.ErrInvalidCredentials
		}
		return fmt.Errorf("rotate refresh token: get old token: %w", err)
	}

	if oldVal != userID {
		return domain.ErrInvalidCredentials
	}

	if err := r.cache.Set(ctx, newKey, userID, r.refreshTTL).Err(); err != nil {
		return fmt.Errorf("rotate refresh token: set new token: %w", err)
	}

	return nil
}

func (r *tokenCache) DeleteRefreshToken(ctx context.Context, hash string) error {
	key := refreshTokenKey(hash)

	if err := r.cache.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

func (r *tokenCache) BlacklistAccessToken(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}

	key := accessTokenKey(jti)
	if err := r.cache.Set(ctx, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("cache: blacklist %s: %w", jti, err)
	}

	return nil
}

func (r *tokenCache) IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	n, err := r.cache.Exists(ctx, accessTokenKey(jti)).Result()
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
