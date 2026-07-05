package app

import (
	"context"
	"fmt"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/repository"
	fiberServer "github.com/ramisoul84/task-management/internal/server/http"
	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/service"
	authUtil "github.com/ramisoul84/task-management/pkg/auth"
	cacheRedis "github.com/ramisoul84/task-management/pkg/cache"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type App struct {
	cfg        *config.Config
	log        *logger.Logger
	db         *database.DB
	redisCache *cacheRedis.Cache
	server     *fiberServer.Server
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
	redisCache, err := cacheRedis.New(cfg.Cache)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("redis: %w", err)
	}
	log.Info().Msg("redis connected")

	// utlities
	hasher := authUtil.NewHasher(cfg.Security.Cost)
	token := authUtil.NewTokenService(cfg.Security)
	requestValidator := validator.New()

	// repositories
	userRepo := repository.NewUserRepository(db)
	teamRepo := repository.NewTeamRepository(db)
	taskRepo := repository.NewTaskRepository(db)

	// Caches
	tokenCache := cache.NewTokenCache(redisCache, cfg.Security.RefreshTokenTTL)
	taskCache := cache.NewTaskCache(redisCache, cfg.CacheTTL.TaskListTTL)

	// services
	authSvc := service.NewAuthService(userRepo, tokenCache, token, hasher, log, cfg.Security.AccessTokenTTL)
	rbacService := service.NewRBACService(teamRepo)
	teamSvc := service.NewTeamService(teamRepo, userRepo, rbacService, log)
	taskSvc := service.NewTaskService(taskRepo, taskCache, teamRepo, rbacService, log)

	// handlers
	authHandler := handler.NewAuthHandler(authSvc, requestValidator, cfg.Security)
	teamHandler := handler.NewTeamHandler(teamSvc, requestValidator)
	taskHandler := handler.NewTaskHandler(taskSvc, requestValidator)

	// Server
	server := fiberServer.NewServer(cfg, token, tokenCache, redisCache.Client, log, authHandler, teamHandler, taskHandler)

	return &App{
		cfg:        cfg,
		log:        log,
		db:         db,
		redisCache: redisCache,
		server:     server,
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

	if err := a.redisCache.Close(); err != nil {
		a.log.Error().Err(err).Msg("cache close error")
	}
	a.log.Info().Msg("cache closed")

	a.log.Info().Msg("shutdown complete")
	return nil
}
