//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/config"
	"github.com/ramisoul84/task-management/pkg/cache"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/migrator"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// migrationsDir returns the repository's migrations directory as an absolute
// path. golang-migrate resolves relative "file://" sources against the test
// binary's working directory, which varies under `go test ./...`, so the path
// is pinned to this file's own location instead.
func migrationsDir(t *testing.T) string {
	t.Helper()

	if dir := os.Getenv("TEST_MIGRATIONS_DIR"); dir != "" {
		return dir
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate helpers.go")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	return filepath.Join(repoRoot, "migrations")
}

// newDB connects to MySQL (compose test services by default), applies the
// migrations and cleans all tables so every test starts from an empty state.
func newDB(t *testing.T) *database.DB {
	t.Helper()

	cfg := config.MySQLConfig{
		Host:         envOr("TEST_DB_HOST", "127.0.0.1"),
		Port:         envOr("TEST_DB_PORT", "3307"),
		User:         envOr("TEST_DB_USER", "tm"),
		Password:     envOr("TEST_DB_PASSWORD", "tm"),
		DBName:       envOr("TEST_DB_NAME", "task_management"),
		TLS:          "false",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		ConnLifetime: 5 * time.Minute,
		ConnIdleTime: time.Minute,
	}

	if err := migrator.Run(cfg, migrationsDir(t)); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	db, err := database.NewMySQL(cfg)
	if err != nil {
		t.Fatalf("connect mysql: %v", err)
	}

	truncate(t, db)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// newCache connects to Redis (compose test service by default) and flushes
// its database before each test.
func newCache(t *testing.T) *cache.Cache {
	t.Helper()

	c, err := cache.New(config.RedisConfig{
		Addr:         envOr("TEST_REDIS_ADDR", "127.0.0.1:6380"),
		Password:     "",
		DB:           1,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}

	if err := c.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return c
}

func truncate(t *testing.T, db *database.DB) {
	t.Helper()

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, "DELETE FROM team_members"); err != nil {
		t.Fatalf("delete team_members: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM teams"); err != nil {
		t.Fatalf("delete teams: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM users"); err != nil {
		t.Fatalf("delete users: %v", err)
	}
}
