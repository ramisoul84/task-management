package http

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	_ "github.com/ramisoul84/task-management/docs"
	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/logger"
	"github.com/redis/go-redis/v9"
	swagger "github.com/swaggo/fiber-swagger"
)

type Server struct {
	app         *fiber.App
	tokenScv    auth.TokenService
	tokenCache  cache.TokenCache
	log         *logger.Logger
	cfg         *config.Config
	rateLimiter *middleware.RateLimiter
}

func NewServer(
	cfg *config.Config,
	tokenScv auth.TokenService,
	tokenCache cache.TokenCache,
	redisClient *redis.Client,
	log *logger.Logger,
	authHandler *handler.AuthHandler,
	teamHandler *handler.TeamHandler,
	taskHandler *handler.TaskHandler,
) *Server {
	app := fiber.New(fiber.Config{
		AppName:               cfg.App.Name,
		DisableStartupMessage: true,
		ReadTimeout:           cfg.Server.ReadTimeout,
		WriteTimeout:          cfg.Server.WriteTimeout,
		IdleTimeout:           cfg.Server.IdleTimeout,
		BodyLimit:             cfg.Server.BodyLimitMB * 1024 * 1024,
		ErrorHandler:          errorHandler(log),
	})

	// Create rate limiter once
	rateLimiter := middleware.NewRateLimiter(redisClient)

	server := &Server{
		app:         app,
		tokenScv:    tokenScv,
		tokenCache:  tokenCache,
		log:         log,
		cfg:         cfg,
		rateLimiter: rateLimiter,
	}

	server.setupMiddleware()
	server.setupRoutes(authHandler, teamHandler, taskHandler)

	return server
}

func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.Port)
	return s.app.Listen(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info().Msg("shutting down HTTP server")
	return s.app.ShutdownWithContext(ctx)
}

func errorHandler(log *logger.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		message := "internal server error"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		}

		log.Error().
			Err(err).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", code).
			Msg("request error")

		return c.Status(code).JSON(domain.ErrorResponse{
			Error: message,
		})
	}
}

// setupMiddleware configures global middleware
func (s *Server) setupMiddleware() {
	// CORS
	s.app.Use(middleware.CORS(s.cfg.Server.AllowedOrigins, s.cfg.Server.AllowCredentials))

	// Global rate limit
	s.app.Use(s.rateLimiter.GlobalLimit(s.cfg.Server.GlobalRateMax, s.cfg.Server.GlobalRateWindow))
}

func (s *Server) setupRoutes(
	authHandler *handler.AuthHandler,
	teamHandler *handler.TeamHandler,
	taskHandler *handler.TaskHandler,
) {
	// Health check
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now(),
		})
	})

	// Docs
	s.app.Get("/docs", func(c *fiber.Ctx) error {
		return c.Redirect("/docs/index.html")
	})
	s.app.Get("/docs/*", swagger.WrapHandler)

	api := s.app.Group("/api/v1")

	// Auth routes with stricter rate limit (use s.rateLimiter)
	auth := api.Group("/auth", s.rateLimiter.AuthLimit(s.cfg.Server.AuthRateMax, s.cfg.Server.AuthRateWindow))
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	auth.Post("/logout", authHandler.Logout)

	// Protected routes
	protected := api.Group("", middleware.Auth(s.tokenScv, s.tokenCache))

	// Team routes
	teams := protected.Group("/teams")
	teams.Post("/", teamHandler.Create)
	teams.Get("/", teamHandler.List)
	teams.Post("/:id/invite", teamHandler.Invite)
	teams.Get("/:id/members", teamHandler.ListMembers)
	teams.Patch("/:id/members/:userID/role", teamHandler.ChangeMemberRole)
	teams.Delete("/:id/members/:userID", teamHandler.RemoveMember)
	teams.Get("/:id/stats", teamHandler.GetStats)

	// Task routes
	tasks := protected.Group("/tasks")
	tasks.Post("/", taskHandler.Create)
	tasks.Get("/", taskHandler.ListByTeam)
	tasks.Put("/:id", taskHandler.Update)
	tasks.Get("/:id/history", taskHandler.GetHistory)
	tasks.Post("/:id/comments", taskHandler.AddComment)
	tasks.Get("/:id/comments", taskHandler.ListComments)
}
