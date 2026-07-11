package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ramisoul84/task-management/internal/app"
	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/pkg/migrator"
)

// @title Task Management API
// @version 1.0
// @description REST API for managing tasks within teams with RBAC
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg := config.Load(os.Getenv("APP_ENV"))

	if err := migrator.Run(cfg.DB, cfg.App.MigrationsDir); err != nil {
		log.Fatalf("migration: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("task-management: %v", err)
	}

	serverErr := application.Start()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Printf("task-management: server error: %v", err)
		}
	case <-ctx.Done():
		log.Println("shutdown signal received")
	}

	if err := application.Shutdown(); err != nil {
		log.Printf("task-management: shutdown: %v", err)
		os.Exit(1)
	}

	log.Println("application stopped")
}
