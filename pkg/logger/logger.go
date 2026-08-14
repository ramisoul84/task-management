package logger

import (
	"os"

	"github.com/rs/zerolog"
)

type Logger struct {
	zerolog.Logger
}

func New(lvl, appName string, dev bool) *Logger {
	level, err := zerolog.ParseLevel(lvl)
	if err != nil {
		level = zerolog.InfoLevel
	}

	var zl zerolog.Logger

	if dev {
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}).Level(level).
			With().
			Str("app_name", appName).
			Timestamp().
			Logger()
	} else {
		zl = zerolog.New(os.Stdout).
			Level(level).
			With().
			Str("app_name", appName).
			Timestamp().
			Logger()
	}

	return &Logger{Logger: zl}
}
