// internal/server/http/server.go
package http

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type Server struct {
	app  *fiber.App
	log  *logger.Logger
	addr string
}

func NewServer(cfg *config.Config, log *logger.Logger) *Server {
	app := fiber.New(fiber.Config{
		AppName:               cfg.App.Name,
		DisableStartupMessage: true,
		ReadTimeout:           cfg.Server.ReadTimeout,
		WriteTimeout:          cfg.Server.WriteTimeout,
		IdleTimeout:           cfg.Server.IdleTimeout,
		BodyLimit:             cfg.Server.BodyLimitMB * 1024 * 1024,
		ErrorHandler:          errorHandler(log),
	})

	setupRoutes(app)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	return &Server{
		app:  app,
		log:  log,
		addr: addr,
	}
}

func (s *Server) Run() error {
	return s.app.Listen(s.addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info().Msg("shutting down HTTP server")
	return s.app.ShutdownWithContext(ctx)
}

func errorHandler(log *logger.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		log.Error().
			Err(err).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Msg("request error")

		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		return c.Status(code).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
}

func setupRoutes(app *fiber.App) {
	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now(),
		})
	})
}
