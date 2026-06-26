package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
	GetByID(ctx context.Context, taskID string) (*domain.Task, error)
	ListByTeam(ctx context.Context, filter *domain.TaskListQuery) ([]*domain.Task, error)
	CountByTeam(ctx context.Context, filter *domain.TaskListQuery) (int, error)
	UpdateWithHistory(ctx context.Context, taskID string, updates map[string]interface{}, changedBy string, expectedVersion int) (*domain.Task, error)
	GetHistory(ctx context.Context, taskID string) ([]*domain.TaskHistory, error)
	AddComment(ctx context.Context, comment *domain.TaskComment) error
	ListComments(ctx context.Context, taskID string) ([]*domain.TaskComment, error)
}

type taskRepo struct {
	db *database.DB
}

func NewTaskRepository(db *database.DB) TaskRepository {
	return &taskRepo{db: db}
}

// Create - inserts a new task
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

// GetByID - retrieves a single task by ID
func (r *taskRepo) GetByID(ctx context.Context, taskID string) (*domain.Task, error) {
	const op = "taskRepo.GetByID"

	const query = `
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
		WHERE id = ?
	`

	var task domain.Task
	if err := r.db.GetContext(ctx, &task, query, taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: task not found (id=%s): %w", op, taskID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: get task (id=%s): %w", op, taskID, err)
	}

	return &task, nil
}

// ListByTeam - lists tasks with filters and pagination
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

// UpdateWithHistory - updates task and records history in transaction
func (r *taskRepo) UpdateWithHistory(
	ctx context.Context,
	taskID string,
	updates map[string]interface{},
	changedBy string,
	expectedVersion int,
) (*domain.Task, error) {
	const op = "taskRepo.UpdateWithHistory"

	// Start transaction
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	// Get current task with row lock
	var currentTask domain.Task
	const getQuery = `
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
		WHERE id = ?
		FOR UPDATE
	`

	if err := tx.GetContext(ctx, &currentTask, getQuery, taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: task not found: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}

	// Check version for optimistic locking
	if currentTask.Version != expectedVersion {
		return nil, fmt.Errorf("%s: expected version %d, got %d: %w",
			op, expectedVersion, currentTask.Version, domain.ErrConflict)
	}

	// Build update query dynamically
	updateQuery := "UPDATE tasks SET version = version + 1, updated_at = NOW()"
	args := []interface{}{}

	if title, ok := updates["title"]; ok {
		updateQuery += ", title = ?"
		args = append(args, title)
	}
	if description, ok := updates["description"]; ok {
		updateQuery += ", description = ?"
		args = append(args, description)
	}
	if status, ok := updates["status"]; ok {
		updateQuery += ", status = ?"
		args = append(args, status)
	}
	if closedAt, ok := updates["closed_at"]; ok {
		updateQuery += ", closed_at = ?"
		args = append(args, closedAt)
	}
	if assigneeID, ok := updates["assignee_id"]; ok {
		updateQuery += ", assignee_id = ?"
		args = append(args, assigneeID)
	}

	updateQuery += " WHERE id = ? AND version = ?"
	args = append(args, taskID, expectedVersion)

	// Execute update
	result, err := tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: update task: %w", op, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("%s: get rows affected: %w", op, err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("%s: no rows updated: %w", op, domain.ErrConflict)
	}

	// Record history
	changesJSON, err := json.Marshal(updates)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal changes: %w", op, err)
	}

	const historyQuery = `
		INSERT INTO task_history (task_id, changed_by, changes, created_at)
		VALUES (?, ?, ?, NOW())
	`

	if _, err := tx.ExecContext(ctx, historyQuery, taskID, changedBy, changesJSON); err != nil {
		return nil, fmt.Errorf("%s: insert history: %w", op, err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: commit transaction: %w", op, err)
	}

	// Get updated task
	updatedTask, err := r.GetByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: get updated task: %w", op, err)
	}

	return updatedTask, nil
}

// GetHistory - retrieves task change history
func (r *taskRepo) GetHistory(ctx context.Context, taskID string) ([]*domain.TaskHistory, error) {
	const op = "taskRepo.GetHistory"

	const query = `
		SELECT
			id,
			task_id,
			changed_by,
			changes,
			created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY created_at DESC
	`

	history := make([]*domain.TaskHistory, 0)
	if err := r.db.SelectContext(ctx, &history, query, taskID); err != nil {
		return nil, fmt.Errorf("%s: select history: %w", op, err)
	}

	return history, nil
}

// AddComment - adds a comment to a task
func (r *taskRepo) AddComment(ctx context.Context, comment *domain.TaskComment) error {
	const op = "taskRepo.AddComment"

	const query = `
		INSERT INTO task_comments (id, task_id, user_id, content, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		comment.ID,
		comment.TaskID,
		comment.UserID,
		comment.Content,
		comment.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("%s: insert comment: %w", op, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: get rows affected: %w", op, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%s: no rows inserted: %w", op, domain.ErrNotFound)
	}

	return nil
}

// ListComments - retrieves all comments for a task
func (r *taskRepo) ListComments(ctx context.Context, taskID string) ([]*domain.TaskComment, error) {
	const op = "taskRepo.ListComments"

	const query = `
		SELECT
			id,
			task_id,
			user_id,
			content,
			created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY created_at ASC
	`

	comments := make([]*domain.TaskComment, 0)
	if err := r.db.SelectContext(ctx, &comments, query, taskID); err != nil {
		return nil, fmt.Errorf("%s: select comments: %w", op, err)
	}

	return comments, nil
}
