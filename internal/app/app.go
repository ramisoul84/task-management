package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/internal/server/http"
	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/ramisoul84/task-management/pkg/validator"
)

type App struct {
	log    *logger.Logger
	server *http.Server
	db     *database.DB
	cache  *cache.Cache
	cfg    *config.Config
}

func New(cfg *config.Config) (*App, error) {
	log := logger.New(cfg.Logging.Level, cfg.App.Name, cfg.IsDevelopment())
	log.Info().Msg("starting task management")

	db, err := database.NewMySQL(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	log.Info().Msg("database connected")

	redisCache, err := cache.New(cfg.Cache)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	log.Info().Msg("cache connected")

	// utilities
	hasher := auth.NewHasher(cfg.Security.Cost)
	token := auth.NewTokenService(cfg.Security)
	requestValidator := validator.New()

	// Repositories
	userRepo := repository.NewUserRepository(db, log)
	refreshTokenRepo := repository.NewRefreshTokenRepository(redisCache, log, cfg.Security.RefreshTokenTTL)
	teamRepo := repository.NewTeamRepository(db, log)

	// Services
	authService := service.NewAuthService(
		userRepo,
		refreshTokenRepo,
		token,
		hasher,
		log,
		cfg.Security.AccessTokenTTL,
	)

	rbacService := service.NewRBACService(teamRepo, log)

	teamService := service.NewTeamService(
		teamRepo,
		userRepo,
		rbacService,
		log,
	)

	// Handlers
	authHandler := handler.NewAuthHandler(authService, requestValidator, cfg.Security)
	teamHandler := handler.NewTeamHandler(teamService, requestValidator)

	// HTTP server
	server := http.NewServer(
		cfg,
		log,
		db,
		redisCache,
		token,
		authHandler,
		teamHandler,
	)

	return &App{
		log:    log,
		server: server,
		db:     db,
		cache:  redisCache,
		cfg:    cfg,
	}, nil
}

func (a *App) Run() <-chan error {
	return a.server.Run()
}

func (a *App) Shutdown() error {
	a.log.Info().Msg("shutting down application")

	done := make(chan error, 1)
	go func() {
		var err error
		if shutdownErr := a.server.Shutdown(); shutdownErr != nil {
			err = errors.Join(err, fmt.Errorf("shutdown HTTP server: %w", shutdownErr))
		}
		if cacheErr := a.cache.Close(); cacheErr != nil {
			err = errors.Join(err, fmt.Errorf("close Redis: %w", cacheErr))
		}
		if dbErr := a.db.Close(); dbErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", dbErr))
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("application shutdown: %w", err)
		}
		a.log.Info().Msg("application shutdown complete")
		return nil
	case <-time.After(a.cfg.HTTP.ShutdownTimeout):
		return fmt.Errorf("application shutdown: timed out after %s", a.cfg.HTTP.ShutdownTimeout)
	}
}
