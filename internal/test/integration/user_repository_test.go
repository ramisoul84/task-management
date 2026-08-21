//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
)

func TestUserRepository_CreateAndGetByEmail(t *testing.T) {
	db := newDB(t)
	repo := repository.NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{
		ID:           uuid.NewString(),
		Email:        "alice@example.com",
		PasswordHash: "hashed-secret",
		Name:         "Alice",
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := repo.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetByEmail() error = %v, want nil", err)
	}
	if got.ID != user.ID || got.Email != user.Email || got.PasswordHash != user.PasswordHash {
		t.Errorf("GetByEmail() = %+v, want %+v", got, user)
	}
}

func TestUserRepository_GetByEmail_UnknownEmail(t *testing.T) {
	db := newDB(t)
	repo := repository.NewUserRepository(db)

	_, err := repo.GetByEmail(context.Background(), "ghost@example.com")

	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByEmail() error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestUserRepository_GetByID(t *testing.T) {
	db := newDB(t)
	repo := repository.NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{
		ID:           uuid.NewString(),
		Email:        "bob@example.com",
		PasswordHash: "hashed-secret",
		Name:         "Bob",
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if got.ID != user.ID {
		t.Errorf("GetByID() ID = %q, want %q", got.ID, user.ID)
	}
}

func TestUserRepository_Create_DuplicateEmail(t *testing.T) {
	db := newDB(t)
	repo := repository.NewUserRepository(db)
	ctx := context.Background()

	first := &domain.User{
		ID:           uuid.NewString(),
		Email:        "carol@example.com",
		PasswordHash: "hashed-secret",
		Name:         "Carol",
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v, want nil", err)
	}

	second := &domain.User{
		ID:           uuid.NewString(),
		Email:        first.Email,
		PasswordHash: "another-hash",
		Name:         "Carol II",
	}

	err := repo.Create(ctx, second)

	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("second Create() error = %v, want %v", err, domain.ErrAlreadyExists)
	}
}
