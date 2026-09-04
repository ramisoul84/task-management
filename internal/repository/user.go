package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	UpdateName(ctx context.Context, id, name string) error
	UpdatePasswordHash(ctx context.Context, id, hash string) error
}

type userRepo struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) UserRepository {
	return &userRepo{db: db}
}

const userColumns = `id, email, password_hash, name, created_at`

func (r *userRepo) Create(ctx context.Context, u *domain.User) error {
	const q = `INSERT INTO users (id, email, password_hash, name, created_at) VALUES (?, ?, ?, ?, ?)`

	if _, err := r.db.Q(ctx).ExecContext(ctx, q, u.ID, u.Email, u.PasswordHash, u.Name, u.CreatedAt); err != nil {
		if database.IsDuplicateKey(err) {
			return domain.ErrAlreadyExists.WithMessage("a user with this email already exists")
		}
		return fmt.Errorf("userRepo.Create: %w", err)
	}
	return nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.getOne(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email)
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return r.getOne(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
}

func (r *userRepo) getOne(ctx context.Context, q string, arg any) (*domain.User, error) {
	var u domain.User
	if err := r.db.Q(ctx).GetContext(ctx, &u, q, arg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound.WithMessage("user not found")
		}
		return nil, fmt.Errorf("userRepo.get: %w", err)
	}
	return &u, nil
}

func (r *userRepo) UpdateName(ctx context.Context, id, name string) error {
	return r.update(ctx, `UPDATE users SET name = ? WHERE id = ?`, name, id)
}

func (r *userRepo) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	return r.update(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
}

func (r *userRepo) update(ctx context.Context, q, value, id string) error {
	if _, err := r.db.Q(ctx).ExecContext(ctx, q, value, id); err != nil {
		return fmt.Errorf("userRepo.update: %w", err)
	}
	return nil
}
