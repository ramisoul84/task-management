package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// loadDotenv loads the appropriate .env file for the given environment.
func loadDotenv(env string) {
	file := ".env.dev"
	if strings.ToLower(env) == "production" {
		file = ".env.prod"
	}

	if err := godotenv.Load(file); err != nil {
		log.Printf("warning: could not load %s: %v", file, err)
	}
}

// getEnv returns the env var value or the fallback if not set.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustGetEnv returns the env var value or panics at startup if not set.
func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return v
}

// getEnvInt returns an integer env var or the fallback if not set.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("environment variable %q must be an integer, got %q", key, v))
	}
	return n
}

// getEnvDuration returns a time.Duration env var (e.g. "15s", "1m") or fallback.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("environment variable %q must be a valid duration (e.g. 15s, 1m), got %q", key, v))
	}
	return d
}

// getEnvBool returns a boolean env var or the fallback if not set.
func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("environment variable %q must be a boolean (e.g. true, false), got %q", key, v))
	}

	return b
}
