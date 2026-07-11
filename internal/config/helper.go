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

// loadDotenv loads the appropriate .env file based on the current environment.
func loadDotenv(env string) {
	if strings.ToLower(env) == "production" {
		return
	}

	file := ".env.dev"
	if err := godotenv.Load(file); err != nil {
		log.Printf("warning: could not load %s: %v", file, err)
	}
}

// getEnv retrieves an environment variable value or returns the fallback if not set.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

// mustGetEnv retrieves a required environment variable or panics if not set.
// It's used for critical configuration that the application cannot run without.
func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return strings.TrimSpace(v)
}

// getEnvInt retrieves an integer environment variable or returns the fallback if not set.
// It panics if the value is not a valid integer.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return fallback
	}

	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		panic(fmt.Sprintf("environment variable %q must be an integer, got %q", key, v))
	}
	return n
}

// getEnvDuration retrieves a duration environment variable or returns the fallback if not set.
// It accepts values like "15s", "1m", "1h", "1h30m".
// It panics if the value is not a valid duration.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return fallback
	}

	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		panic(fmt.Sprintf("environment variable %q must be a valid duration (e.g. 15s, 1m, 1h), got %q", key, v))
	}
	return d
}

// getEnvBool retrieves a boolean environment variable or returns the fallback if not set.
// It accepts values like "true", "false", "1", "0", "yes", "no", "on", "off".
// It panics if the value is not a valid boolean.
func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return fallback
	}

	v = strings.ToLower(strings.TrimSpace(v))

	switch v {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("environment variable %q must be a boolean (e.g. true, false, 1, 0, yes, no), got %q", key, v))
	}

	return b
}
