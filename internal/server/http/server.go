package http

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type Server struct {
	app   *fiber.App
	db    *database.DB
	cache *cache.Cache
	cfg   *config.Config
	log   *logger.Logger
}

func NewServer(cfg *config.Config, log *logger.Logger, db *database.DB, cache *cache.Cache) *Server {
	app := fiber.New(fiber.Config{
		AppName:               cfg.App.Name,
		DisableStartupMessage: true,
		ReadTimeout:           cfg.HTTP.ReadTimeout,
		WriteTimeout:          cfg.HTTP.WriteTimeout,
		IdleTimeout:           cfg.HTTP.IdleTimeout,
		BodyLimit:             cfg.HTTP.BodyLimitMB * 1024 * 1024,
	})

	server := &Server{
		app:   app,
		db:    db,
		cache: cache,
		cfg:   cfg,
		log:   log,
	}

	server.registerRoutes()

	return server
}

func (s *Server) registerRoutes() {
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: s.cfg.HTTP.AllowedOrigins,
	}))

	s.app.Get("/health", s.health)

	api := s.app.Group("/api/v1")
	_ = api
}

func (s *Server) health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), time.Second)
	defer cancel()

	dbErr := s.db.Health(ctx)
	cacheErr := s.cache.Health(ctx)

	if dbErr != nil || cacheErr != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "unavailable",
			"db":     dbErr == nil,
			"cache":  cacheErr == nil,
		})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (s *Server) Run() <-chan error {
	errCh := make(chan error, 1)

	addr := fmt.Sprintf(":%d", s.cfg.HTTP.Port)

	s.log.Info().
		Str("address", addr).
		Msg("HTTP server started")

	go func() {
		if err := s.app.Listen(addr); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	return errCh
}

func (s *Server) Shutdown() error {
	s.log.Info().
		Dur("timeout", s.cfg.HTTP.ShutdownTimeout).
		Msg("shutting down HTTP server")

	return s.app.ShutdownWithTimeout(
		s.cfg.HTTP.ShutdownTimeout,
	)
}
