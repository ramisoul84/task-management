// Package http assembles the Fiber application: middleware, routes and
// health endpoints.
package http

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	swagger "github.com/swaggo/fiber-swagger"

	_ "github.com/ramisoul84/task-management/docs" // swagger spec
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/internal/server/http/handler"
	"github.com/ramisoul84/task-management/internal/server/http/middleware"
	"github.com/ramisoul84/task-management/pkg/auth"
)

// HealthCheck reports whether a dependency is usable.
type HealthCheck func(ctx context.Context) error

// Handlers groups the resource handlers.
type Handlers struct {
	Auth *handler.AuthHandler
	Team *handler.TeamHandler
	Task *handler.TaskHandler
}

// Deps are the server's collaborators.
type Deps struct {
	Config     *config.Config
	Log        zerolog.Logger
	Tokens     auth.TokenService
	TokenCache middleware.TokenBlacklist
	Redis      *redis.Client
	Handlers   Handlers
	Checks     map[string]HealthCheck
}

type Server struct {
	app  *fiber.App
	deps Deps
}

func New(deps Deps) *Server {
	cfg := deps.Config.HTTP

	fcfg := fiber.Config{
		AppName:               deps.Config.App.Name,
		DisableStartupMessage: true,
		CaseSensitive:         true,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		IdleTimeout:           cfg.IdleTimeout,
		BodyLimit:             cfg.BodyLimitBytes,
		ErrorHandler:          handler.ErrorHandler,
	}

	// Only honour X-Forwarded-For from known proxies; otherwise any client
	// could spoof its IP and dodge rate limiting.
	if len(cfg.TrustedProxies) > 0 {
		fcfg.EnableTrustedProxyCheck = true
		fcfg.TrustedProxies = cfg.TrustedProxies
		fcfg.ProxyHeader = fiber.HeaderXForwardedFor
	}

	s := &Server{app: fiber.New(fcfg), deps: deps}
	s.middleware()
	s.routes()

	return s
}

func (s *Server) Listen() error {
	return s.app.Listen(fmt.Sprintf(":%d", s.deps.Config.HTTP.Port))
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

// App exposes the Fiber app (used by HTTP tests via app.Test).
func (s *Server) App() *fiber.App { return s.app }

// middleware installs the global chain. Order matters:
// request ID first (so everything is correlated), access log next (so it
// sees the final status), then recover (so panics become logged 500s).
func (s *Server) middleware() {
	cfg := s.deps.Config

	s.app.Use(middleware.RequestID(s.deps.Log))
	s.app.Use(middleware.AccessLog())
	s.app.Use(middleware.Recover())
	s.app.Use(middleware.SecureHeaders(cfg.IsProduction()))
	s.app.Use(middleware.CORS(cfg.HTTP.AllowedOrigins, cfg.HTTP.AllowCredentials))
	s.app.Use(middleware.Timeout(cfg.HTTP.RequestTimeout))
}

func (s *Server) routes() {
	cfg := s.deps.Config
	h := s.deps.Handlers

	// Probes: liveness never touches dependencies (a DB outage must not get
	// the pod restarted); readiness does.
	s.app.Get("/livez", s.livez)
	s.app.Get("/readyz", s.readyz)
	s.app.Get("/health", s.readyz)

	if cfg.HTTP.EnableDocs {
		s.app.Get("/docs", func(c *fiber.Ctx) error { return c.Redirect("/docs/index.html") })
		s.app.Get("/docs/*", swagger.WrapHandler)
	}

	limiter := middleware.NewRateLimiter(s.deps.Redis)

	api := s.app.Group("/api/v1")
	if cfg.RateLimit.Enabled {
		api.Use(limiter.GlobalLimit(cfg.RateLimit.GlobalMax, cfg.RateLimit.GlobalWindow))
	}

	authLimit := func(c *fiber.Ctx) error { return c.Next() }
	if cfg.RateLimit.Enabled {
		authLimit = limiter.AuthLimit(cfg.RateLimit.AuthMax, cfg.RateLimit.AuthWindow)
	}

	// Public auth endpoints. The spec puts register/login directly under
	// /api/v1; the /auth/* aliases are what the web client uses.
	api.Post("/register", authLimit, h.Auth.Register)
	api.Post("/login", authLimit, h.Auth.Login)

	authGroup := api.Group("/auth", authLimit)
	authGroup.Post("/register", h.Auth.Register)
	authGroup.Post("/login", h.Auth.Login)
	authGroup.Post("/refresh", h.Auth.Refresh)
	authGroup.Post("/logout", h.Auth.Logout)

	// Everything below requires a valid access token.
	protected := api.Group("", middleware.Auth(s.deps.Tokens, s.deps.TokenCache))

	teams := protected.Group("/teams")
	teams.Post("/", h.Team.Create)
	teams.Get("/", h.Team.List)
	teams.Post("/:id/invite", h.Team.Invite)
	teams.Get("/:id/members", h.Team.ListMembers)
	teams.Patch("/:id/members/:userID/role", h.Team.ChangeMemberRole)
	teams.Delete("/:id/members/:userID", h.Team.RemoveMember)
	teams.Get("/:id/stats", h.Team.GetStats)

	tasks := protected.Group("/tasks")
	tasks.Post("/", h.Task.Create)
	tasks.Get("/", h.Task.ListByTeam)
	tasks.Get("/:id", h.Task.Get)
	tasks.Put("/:id", h.Task.Update)
	tasks.Delete("/:id", h.Task.Delete)
	tasks.Get("/:id/history", h.Task.GetHistory)
	tasks.Post("/:id/comments", h.Task.AddComment)
	tasks.Get("/:id/comments", h.Task.ListComments)
}

func (s *Server) livez(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

func (s *Server) readyz(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
	defer cancel()

	status := fiber.StatusOK
	checks := make(map[string]string, len(s.deps.Checks))
	for name, check := range s.deps.Checks {
		if err := check(ctx); err != nil {
			status = fiber.StatusServiceUnavailable
			checks[name] = "down"
			continue
		}
		checks[name] = "up"
	}

	state := "ok"
	if status != fiber.StatusOK {
		state = "degraded"
	}

	return c.Status(status).JSON(fiber.Map{
		"status":  state,
		"checks":  checks,
		"version": s.deps.Config.App.Version,
		"time":    time.Now().UTC(),
	})
}
