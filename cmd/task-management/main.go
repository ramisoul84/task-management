package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ramisoul84/task-management/internal/app"
	"github.com/ramisoul84/task-management/internal/config"
)

func main() {
	cfg := config.Load(os.Getenv("APP_ENV"))

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("task-management: %v", err)
	}

	serverErr := application.Run()

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
}
