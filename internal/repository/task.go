package repository

import (
	"context"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
	ListByTeam(ctx context.Context, filter *domain.TaskListQuery) ([]*domain.Task, error)
	CountByTeam(ctx context.Context, filter *domain.TaskListQuery) (int, error)
}

type taskRepo struct {
	db *database.DB
}

func NewTaskRepository(db *database.DB) TaskRepository {
	return &taskRepo{db: db}
}

func (r *taskRepo) Create(ctx context.Context, task *domain.Task) error {
	const op = "taskRepo.Create"

	const insertQuery = `
		INSERT INTO tasks (
			id,
			team_id,
			title,
			description,
			status,
			created_by,
			assignee_id,
			created_at,
			updated_at,
			closed_at,
			version
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(
		ctx,
		insertQuery,
		task.ID,
		task.TeamID,
		task.Title,
		task.Description,
		task.Status,
		task.CreatedBy,
		task.AssigneeID,
		task.CreatedAt,
		task.UpdatedAt,
		task.ClosedAt,
		task.Version,
	)
	if err != nil {
		return fmt.Errorf("%s: insert task (id=%s): %w", op, task.ID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: get rows affected (id=%s): %w", op, task.ID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%s: no rows inserted (id=%s): %w", op, task.ID, domain.ErrNotFound)
	}

	return nil
}

func (r *taskRepo) ListByTeam(ctx context.Context, filter *domain.TaskListQuery) ([]*domain.Task, error) {
	const op = "taskRepo.ListByTeam"

	query := `
		SELECT
			id,
			team_id,
			title,
			description,
			status,
			created_by,
			assignee_id,
			created_at,
			updated_at,
			closed_at,
			version
		FROM tasks
		WHERE team_id = ?
	`

	args := []any{filter.TeamID}

	// Add filters
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.AssigneeID != "" {
		query += " AND assignee_id = ?"
		args = append(args, filter.AssigneeID)
	}

	// Stable ordering required for pagination; newest first
	query += " ORDER BY created_at DESC, id DESC"

	// Add pagination
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	tasks := make([]*domain.Task, 0)

	if err := r.db.SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, fmt.Errorf("%s: select tasks: %w", op, err)
	}

	return tasks, nil
}

// CountByTeam returns total count of tasks matching filters
func (r *taskRepo) CountByTeam(ctx context.Context, filter *domain.TaskListQuery) (int, error) {
	const op = "taskRepo.CountByTeam"

	query := `
		SELECT COUNT(*)
		FROM tasks
		WHERE team_id = ?
	`

	args := []any{filter.TeamID}

	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.AssigneeID != "" {
		query += " AND assignee_id = ?"
		args = append(args, filter.AssigneeID)
	}

	var count int
	if err := r.db.GetContext(ctx, &count, query, args...); err != nil {
		return 0, fmt.Errorf("%s: count tasks: %w", op, err)
	}

	return count, nil
}
