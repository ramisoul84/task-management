//go:build integration

// Package testutil starts disposable infrastructure for integration tests.
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/migrator"
)

// MySQLImage can be overridden with TEST_MYSQL_IMAGE.
const MySQLImage = "mysql:8.4.11"

// MySQL is a migrated, throwaway MySQL server.
type MySQL struct {
	DB        *database.DB
	Config    database.Config
	container testcontainers.Container
}

// StartMySQL boots MySQL in Docker and applies every migration in
// migrations/. Call it once from TestMain and Close it afterwards.
func StartMySQL(ctx context.Context) (*MySQL, error) {
	image := MySQLImage
	if v := os.Getenv("TEST_MYSQL_IMAGE"); v != "" {
		image = v
	}

	c, err := tcmysql.Run(ctx, image,
		tcmysql.WithDatabase("tm_test"),
		tcmysql.WithUsername("tm"),
		tcmysql.WithPassword("p@ss/w:rd"), // exercises DSN escaping
	)
	if err != nil {
		return nil, fmt.Errorf("start mysql: %w", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := c.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, err
	}

	cfg := database.Config{
		Host: host, Port: port.Port(),
		User: "tm", Password: "p@ss/w:rd", Name: "tm_test", TLS: "false",
		MaxOpenConns: 20, MaxIdleConns: 5, ConnMaxLifetime: time.Minute,
	}

	m, err := migrator.New(ctx, cfg.DSN(), MigrationsDir())
	if err != nil {
		return nil, err
	}
	defer m.Close()
	if err := m.Up(); err != nil {
		return nil, err
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &MySQL{DB: db, Config: cfg, container: c}, nil
}

// Close stops the container.
func (m *MySQL) Close() {
	_ = m.DB.Close()
	_ = testcontainers.TerminateContainer(m.container)
}

// MigrationsDir is the absolute path of the repository's migrations/.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}
