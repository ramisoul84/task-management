// main.go
package main

import (
	"os"

	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/pkg/logger"
)

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	cfg := config.Load(env)

	// Create logger
	log := logger.New(
		cfg.Logger.Level,
		cfg.App.Name,
		cfg.IsDevelopment(),
		cfg.Logger.Output,
	)

	// Use logger
	log.Info().
		Str("env", cfg.App.Env).
		Msg("application starting")

	log.Debug().Msg("debug message")
	log.Warn().Msg("warning message")
	log.Error().Msg("error message")
}
