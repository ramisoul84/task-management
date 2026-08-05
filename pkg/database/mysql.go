package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// Config holds MySQL connection settings.
type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	TLS             string // "false", "true", "skip-verify", "preferred" or a registered TLS config name
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DSN builds a go-sql-driver DSN. Using mysql.Config (instead of Sprintf)
// correctly escapes credentials containing '@', '/', ':' etc.
//
// The session is pinned to UTC on both sides (Go's loc and MySQL's
// time_zone) so TIMESTAMP columns round-trip without implicit conversion,
// whatever the host or container timezone is.
func (c Config) DSN() string {
	mc := mysql.NewConfig()
	mc.User = c.User
	mc.Passwd = c.Password
	mc.Net = "tcp"
	mc.Addr = net.JoinHostPort(c.Host, c.Port)
	mc.DBName = c.Name
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.TLSConfig = c.TLS
	mc.Params = map[string]string{
		"charset":   "utf8mb4",
		"time_zone": "'+00:00'",
	}

	return mc.FormatDSN()
}

// DB wraps sqlx.DB and adds transaction propagation through context.
type DB struct {
	*sqlx.DB
}

// Open connects to MySQL, configures the pool and verifies connectivity.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	db, err := sqlx.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	return &DB{DB: db}, nil
}

// Wrap adapts an existing sqlx connection (useful in tests).
func Wrap(db *sqlx.DB) *DB {
	return &DB{DB: db}
}

// Health pings the database.
func (db *DB) Health(ctx context.Context) error {
	if db == nil || db.DB == nil {
		return errors.New("database is not initialized")
	}
	return db.PingContext(ctx)
}

// Close closes the pool. It is safe to call on a nil DB.
func (db *DB) Close() error {
	if db == nil || db.DB == nil {
		return nil
	}
	return db.DB.Close()
}

// MySQL error numbers we translate into domain errors.
const (
	errDuplicateEntry   = 1062
	errNoReferencedRow  = 1452
	errLockWaitTimeout  = 1205
	errDeadlockDetected = 1213
)

func mysqlErrNumber(err error) uint16 {
	if me, ok := errors.AsType[*mysql.MySQLError](err); ok {
		return me.Number
	}
	return 0
}

// IsDuplicateKey reports whether err is a unique-constraint violation.
func IsDuplicateKey(err error) bool { return mysqlErrNumber(err) == errDuplicateEntry }

// IsForeignKeyViolation reports whether err is a missing-parent FK violation.
func IsForeignKeyViolation(err error) bool { return mysqlErrNumber(err) == errNoReferencedRow }

// IsRetryable reports whether err is a transient lock error worth retrying.
func IsRetryable(err error) bool {
	n := mysqlErrNumber(err)
	return n == errDeadlockDetected || n == errLockWaitTimeout
}
