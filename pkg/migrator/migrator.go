// Package migrator applies SQL migrations with golang-migrate.
package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file" // file:// source
)

// Migrator wraps golang-migrate. golang-migrate takes a MySQL advisory lock,
// so running it from several replicas at once is safe.
type Migrator struct {
	db *sql.DB
	m  *migrate.Migrate
}

// New prepares a migrator for the given DSN and migrations directory.
func New(ctx context.Context, dsn, dir string) (*Migrator, error) {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("migrator: parse dsn: %w", err)
	}
	// Migration files may contain several statements.
	cfg.MultiStatements = true

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("migrator: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrator: ping: %w", err)
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{MigrationsTable: "schema_migrations"})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrator: driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+filepath.ToSlash(dir), "mysql", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrator: init: %w", err)
	}

	return &Migrator{db: db, m: m}, nil
}

// Up applies all pending migrations. No pending migrations is not an error.
func (m *Migrator) Up() error {
	if err := m.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrator: up: %w", err)
	}
	return nil
}

// Down rolls back the given number of migrations.
func (m *Migrator) Down(steps int) error {
	if steps <= 0 {
		return errors.New("migrator: steps must be positive")
	}
	if err := m.m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrator: down: %w", err)
	}
	return nil
}

// Version returns the current schema version and whether it is dirty.
func (m *Migrator) Version() (version uint, dirty bool, err error) {
	version, dirty, err = m.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return version, dirty, err
}

// Close releases the underlying connection.
func (m *Migrator) Close() error {
	srcErr, dbErr := m.m.Close()
	return errors.Join(srcErr, dbErr)
}
