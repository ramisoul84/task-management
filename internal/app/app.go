// Package app wires dependencies together and owns the process lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/repository"
	httpserver "github.com/ramisoul84/task-management/internal/server/http"
	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/redisx"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type App struct {
	cfg    *config.Config
	log    zerolog.Logger
	db     *database.DB
	redis  *redisx.Client
	server *httpserver.Server
}

// New connects to infrastructure and builds the HTTP server.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	log := logger.New(cfg.Log)
	log.Info().Str("env", cfg.App.Env).Msg("starting application")

	if cfg.WeakSecret() {
		log.Warn().Msg("SECRET is shorter than 32 bytes; this is rejected in production")
	}

	db, err := database.Open(ctx, cfg.DB)
	if err != nil {
		return nil, err
	}
	log.Info().Str("host", cfg.DB.Host).Msg("mysql connected")

	rdb, err := redisx.Open(ctx, cfg.Redis)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	log.Info().Str("addr", cfg.Redis.Addr).Msg("redis connected")

	// Utilities
	hasher := auth.NewHasher(cfg.Auth.BcryptCost)
	tokens := auth.NewTokenService(cfg.Auth.Secret, cfg.Auth.Issuer, cfg.Auth.AccessTokenTTL)
	v := validator.New()

	// Repositories
	userRepo := repository.NewUserRepository(db)
	teamRepo := repository.NewTeamRepository(db)
	taskRepo := repository.NewTaskRepository(db)

	// Caches
	tokenCache := cache.NewTokenCache(rdb, cfg.Auth.RefreshTokenTTL)
	taskCache := cache.NewTaskCache(rdb, cfg.Cache.TaskListTTL)

	// Services
	authSvc := service.NewAuthService(userRepo, tokenCache, tokens, hasher, cfg.Auth.AccessTokenTTL)
	teamSvc := service.NewTeamService(db, teamRepo, userRepo, taskRepo, taskCache)
	taskSvc := service.NewTaskService(db, taskRepo, teamRepo, taskCache)

	// Handlers
	handlers := httpserver.Handlers{
		Auth: handler.NewAuthHandler(authSvc, v, cfg.Auth),
		Team: handler.NewTeamHandler(teamSvc, v),
		Task: handler.NewTaskHandler(taskSvc, v),
	}

	server := httpserver.New(httpserver.Deps{
		Config:     cfg,
		Log:        log,
		Tokens:     tokens,
		TokenCache: tokenCache,
		Redis:      rdb.Client,
		Handlers:   handlers,
		Checks: map[string]httpserver.HealthCheck{
			"mysql": db.Health,
			"redis": rdb.Health,
		},
	})

	return &App{cfg: cfg, log: log, db: db, redis: rdb, server: server}, nil
}

// Run serves HTTP until ctx is cancelled or the server fails, then shuts
// everything down gracefully.
func (a *App) Run(ctx context.Context) error {
	serverErr := make(chan error, 1)
	go func() {
		a.log.Info().Int("port", a.cfg.HTTP.Port).Msg("http server listening")
		serverErr <- a.server.Listen()
	}()

	var runErr error
	select {
	case err := <-serverErr:
		if err != nil {
			runErr = fmt.Errorf("http server: %w", err)
		}
	case <-ctx.Done():
		a.log.Info().Msg("shutdown signal received")
	}

	return errors.Join(runErr, a.shutdown())
}

func (a *App) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTP.ShutdownTimeout)
	defer cancel()

	var errs []error

	// Stop accepting requests first and drain in-flight ones, then close
	// the backends they depend on.
	if err := a.server.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	if err := a.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("mysql close: %w", err))
	}
	if err := a.redis.Close(); err != nil {
		errs = append(errs, fmt.Errorf("redis close: %w", err))
	}

	if len(errs) > 0 {
		a.log.Error().Errs("errors", errs).Msg("shutdown finished with errors")
		return errors.Join(errs...)
	}

	a.log.Info().Msg("shutdown complete")
	return nil
}
