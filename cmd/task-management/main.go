// Command task-management runs the Task Management REST API.
//
// Usage:
//
//	task-management                 start the HTTP server (default)
//	task-management serve           same as above
//	task-management migrate up      apply pending migrations
//	task-management migrate down N  roll back N migrations (default 1)
//	task-management migrate version print the current schema version
//	task-management healthcheck     probe /livez (for container health checks)
//	task-management version         print build information
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ramisoul84/task-management/internal/app"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/pkg/migrator"
)

// Set at build time via -ldflags (see Makefile / Dockerfile).
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// @title						Task Management API
// @version					1.0
// @description				REST API for managing tasks inside teams: role-based access control, task history, comments, Redis-cached listings and team analytics.
// @contact.name				Rami Suliman
// @license.name				MIT
// @host						localhost:8080
// @BasePath					/api/v1
// @schemes					http https
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Type "Bearer" followed by a space and the access token.
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "version":
		fmt.Printf("task-management %s (commit %s, built %s)\n", version, commit, buildTime)
		return nil
	case "healthcheck":
		return healthcheck()
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.App.Version == "dev" {
		cfg.App.Version = version
		cfg.Log.Version = version
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "serve":
		return serve(ctx, cfg)
	case "migrate":
		return migrateCmd(ctx, cfg, args)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func serve(ctx context.Context, cfg *config.Config) error {
	if cfg.App.MigrateOnStart {
		if err := migrateUp(ctx, cfg); err != nil {
			return err
		}
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}

	return application.Run(ctx)
}

func migrateCmd(ctx context.Context, cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate up|down [N]|version")
	}

	m, err := migrator.New(ctx, cfg.DB.DSN(), cfg.App.MigrationsDir)
	if err != nil {
		return err
	}
	defer m.Close()

	switch args[0] {
	case "up":
		return m.Up()
	case "down":
		steps := 1
		if len(args) > 1 {
			if steps, err = strconv.Atoi(args[1]); err != nil {
				return fmt.Errorf("invalid step count %q", args[1])
			}
		}
		return m.Down(steps)
	case "version":
		v, dirty, err := m.Version()
		if err != nil {
			return err
		}
		fmt.Printf("version=%d dirty=%t\n", v, dirty)
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q", args[0])
	}
}

func migrateUp(ctx context.Context, cfg *config.Config) error {
	m, err := migrator.New(ctx, cfg.DB.DSN(), cfg.App.MigrationsDir)
	if err != nil {
		return err
	}
	defer m.Close()
	return m.Up()
}

// healthcheck lets distroless images (no curl/wget) run a Docker HEALTHCHECK.
func healthcheck() error {
	port := 8080
	if v := os.Getenv("HTTP_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid HTTP_PORT %q", v)
		}
		port = p
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/livez", port)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody) //nolint:gosec // loopback probe; port is a validated integer
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // see above
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
