// internal/app/app.go
package app

import (
	"context"
	"fmt"

	"github.com/ramisoul84/task-management/internal/config"
	fiberServer "github.com/ramisoul84/task-management/internal/server/http"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type App struct {
	cfg    *config.Config
	log    *logger.Logger
	db     *database.DB
	cache  *cache.Cache
	server *fiberServer.Server
}

func New(cfg *config.Config) (*App, error) {
	log := logger.New(cfg.Logger.Level, cfg.App.Name, cfg.IsDevelopment(), cfg.Logger.Output)
	log.Info().Str("env", cfg.App.Env).Msg("starting application")

	// Database
	db, err := database.NewMySQL(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	log.Info().Msg("database connected")

	// Redis
	redisCache, err := cache.New(cfg.Cache)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("redis: %w", err)
	}
	log.Info().Msg("redis connected")

	// Server
	server := fiberServer.NewServer(cfg, log)

	return &App{
		cfg:    cfg,
		log:    log,
		db:     db,
		cache:  redisCache,
		server: server,
	}, nil
}

func (a *App) Start() <-chan error {
	errChan := make(chan error, 1)

	go func() {
		a.log.Info().Int("port", a.cfg.Server.Port).Msg("server starting")
		if err := a.server.Run(); err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	return errChan
}

func (a *App) Shutdown() error {
	a.log.Info().Msg("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(ctx); err != nil {
		a.log.Error().Err(err).Msg("server shutdown error")
	}
	a.log.Info().Msg("server stopped")

	if err := a.db.Close(); err != nil {
		a.log.Error().Err(err).Msg("database close error")
	}
	a.log.Info().Msg("database closed")

	if err := a.cache.Close(); err != nil {
		a.log.Error().Err(err).Msg("cache close error")
	}
	a.log.Info().Msg("cache closed")

	a.log.Info().Msg("shutdown complete")
	return nil
}
