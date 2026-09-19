package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/config"
)

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENV_FILE", "/nonexistent") // never pick up a developer's .env.dev
	t.Setenv("APP_ENV", "test")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("SECRET", "short-dev-secret")
}

func TestLoad_Defaults(t *testing.T) {
	baseEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Port != 8080 || cfg.Cache.TaskListTTL != 5*time.Minute || cfg.Auth.AccessTokenTTL != 15*time.Minute {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.WeakSecret() {
		t.Fatal("a 16-byte secret must be reported as weak")
	}
}

func TestLoad_ReportsEveryProblemAtOnce(t *testing.T) {
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "n")
	t.Setenv("SECRET", "")
	t.Setenv("HTTP_PORT", "eighty")
	t.Setenv("ACCESS_TOKEN_TTL", "soon")
	t.Setenv("ALLOW_CREDENTIALS", "true")
	t.Setenv("ALLOWED_ORIGINS", "*")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DB_USER", "DB_PASSWORD", "SECRET", "HTTP_PORT", "ACCESS_TOKEN_TTL", "ALLOW_CREDENTIALS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s:\n%v", want, err)
		}
	}
}

func TestLoad_ProductionRequiresStrongSecret(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "SECRET must be at least 32 bytes") {
		t.Fatalf("err = %v", err)
	}

	t.Setenv("SECRET", strings.Repeat("s", 32))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.CookieSecure {
		t.Fatal("cookies must default to Secure in production")
	}
}
