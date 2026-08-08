// Package logger configures zerolog and carries request-scoped loggers
// through context.Context.
package logger

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// Config controls log level, format and destination.
type Config struct {
	Level   string // trace|debug|info|warn|error
	Format  string // json|console
	Output  string // stdout|stderr|<file path>
	Service string
	Version string
}

// New builds the root logger and registers it as the default context
// logger, so FromContext never returns a disabled logger.
func New(cfg Config) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil || level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFunc = func() time.Time { return time.Now().UTC() }
	zerolog.DurationFieldUnit = time.Millisecond

	w := writer(cfg.Output)
	if cfg.Format == "console" {
		w = zerolog.ConsoleWriter{Out: w, TimeFormat: "15:04:05.000"}
	}

	l := zerolog.New(w).
		Level(level).
		With().
		Timestamp().
		Str("service", cfg.Service).
		Str("version", cfg.Version).
		Logger()

	zerolog.DefaultContextLogger = &l

	return l
}

// FromContext returns the request-scoped logger stored in ctx, falling back
// to the root logger.
func FromContext(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

// WithContext stores l in ctx.
func WithContext(ctx context.Context, l zerolog.Logger) context.Context {
	return l.WithContext(ctx)
}

// Nop returns a logger that discards everything (handy in tests).
func Nop() zerolog.Logger { return zerolog.Nop() }

func writer(output string) io.Writer {
	switch output {
	case "", "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	}

	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return os.Stdout
		}
	}

	f, err := os.OpenFile(filepath.Clean(output), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return os.Stdout
	}
	return f
}
