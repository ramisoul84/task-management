package database

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/ramisoul84/task-management/internal/config"
)

type DB struct {
	*sqlx.DB
}

func NewMySQL(cfg config.DatabaseConfig) (*DB, error) {
	const op = "database.NewMySQL"

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&tls=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.TLS,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sqlx.ConnectContext(ctx, "mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: connect: %w", op, err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnLifetime)
	db.SetConnMaxIdleTime(cfg.ConnIdleTime)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s: ping: %w", op, err)
	}

	return &DB{DB: db}, nil
}

func (db *DB) Health(ctx context.Context) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("database is not initialized")
	}
	return db.DB.PingContext(ctx)
}

func (db *DB) Close() error {
	if db == nil || db.DB == nil {
		return nil
	}
	return db.DB.Close()
}
