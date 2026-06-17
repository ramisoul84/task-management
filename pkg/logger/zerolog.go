// pkg/logger/logger.go
package logger

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

type Logger struct {
	zerolog.Logger
}

func New(lvl, appName string, dev bool, output string) *Logger {
	level, err := zerolog.ParseLevel(lvl)
	if err != nil {
		level = zerolog.InfoLevel
	}

	writer := getWriter(output)

	var zl zerolog.Logger
	if dev {
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        writer,
			TimeFormat: "15:04:05",
		}).Level(level).
			With().
			Str("app_name", appName).
			Timestamp().
			Logger()
	} else {
		zl = zerolog.New(writer).
			Level(level).
			With().
			Str("app_name", appName).
			Timestamp().
			Logger()
	}

	return &Logger{Logger: zl}
}

func getWriter(output string) io.Writer {
	switch output {
	case "stderr":
		return os.Stderr
	case "", "stdout":
		return os.Stdout
	default:
		dir := filepath.Dir(output)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return os.Stdout
			}
		}

		file, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return os.Stdout
		}
		return file
	}
}
