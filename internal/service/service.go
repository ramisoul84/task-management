// Package service holds the business logic: authorisation, invariants and
// transaction boundaries. It knows nothing about HTTP.
package service

import (
	"context"
	"errors"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
)

// Transactor runs fn atomically. Repository calls made with the ctx passed
// to fn join the transaction. *database.DB implements it.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// memberRole returns userID's role in teamID. A non-member gets
// ErrForbidden: the team exists, the caller simply has no access to it.
func memberRole(ctx context.Context, teams repository.TeamRepository, teamID, userID string) (domain.Role, error) {
	role, err := teams.GetMemberRole(ctx, teamID, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.ErrForbidden.WithMessage("you are not a member of this team")
	}
	return role, err
}

// visibleTask loads a task the caller may see. To avoid
// leaking which task IDs exist, a task in a team the caller does not
// belong to is reported as not found.
func visibleTask(
	ctx context.Context,
	tasks repository.TaskRepository,
	teams repository.TeamRepository,
	taskID, userID string,
) (*domain.Task, error) {
	task, err := tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	ok, err := teams.IsMember(ctx, task.TeamID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound.WithMessage("task not found")
	}
	return task, nil
}
