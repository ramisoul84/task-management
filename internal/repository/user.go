package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, userID string) (*domain.User, error)
}

type userRepo struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) UserRepository {
	return &userRepo{db: db}
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
			return domain.ErrAlreadyExists
		}

		return fmt.Errorf("create user: %w", err)
	}

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
			return nil, domain.ErrNotFound
		}

		return nil, fmt.Errorf("get user by email: %w", err)
	}

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
			return nil, domain.ErrNotFound
		}

		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &user, nil
}
