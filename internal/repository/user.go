package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, userID string) (*domain.User, error)
}

type userRepo struct {
	db  *database.DB
	log *logger.Logger
}

func NewUserRepository(db *database.DB, log *logger.Logger) UserRepository {
	return &userRepo{
		db:  db,
		log: log,
	}
}

func (r *userRepo) Create(ctx context.Context, user *domain.User) error {
	const query = `
		INSERT INTO users (
			id,
			email,
			password_hash,
			name
		)
		VALUES (?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.Name,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError

		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			r.log.Warn().Msg("email already exists")
			return domain.ErrAlreadyExists
		}

		r.log.Error().Err(err).Msg("create user")
		return fmt.Errorf("create user: %w", err)
	}

	r.log.Info().Msg("user created")

	return nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const query = `
		SELECT
			id,
			email,
			password_hash,
			name,
			created_at
		FROM users
		WHERE email = ?
	`

	var user domain.User

	if err := r.db.GetContext(ctx, &user, query, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.log.Warn().Msg("user not found")
			return nil, domain.ErrNotFound
		}

		r.log.Error().Err(err).Msg("get user by email")
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	r.log.Info().Msg("user found")
	return &user, nil
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	const query = `
		SELECT
			id,
			email,
			password_hash,
			name,
			created_at
		FROM users
		WHERE id = ?
	`

	var user domain.User

	if err := r.db.GetContext(ctx, &user, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.log.Warn().Msg("user not found")
			return nil, domain.ErrNotFound
		}

		r.log.Error().Err(err).Msg("get user by id")
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	r.log.Info().Msg("user found")

	return &user, nil
}
