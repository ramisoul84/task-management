package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
	GetByID(ctx context.Context, id string) (*domain.Task, error)
	// GetByIDForUpdate locks the row until the surrounding transaction
	// ends, so concurrent updates are serialised and history is always
	// computed from the latest committed state.
	GetByIDForUpdate(ctx context.Context, id string) (*domain.Task, error)
	List(ctx context.Context, q *domain.TaskListQuery) ([]*domain.Task, error)
	Count(ctx context.Context, q *domain.TaskListQuery) (int, error)
	// Update persists the mutable fields of task if its stored version is
	// still expectedVersion, and bumps the version. Returns
	// ErrVersionConflict otherwise.
	Update(ctx context.Context, task *domain.Task, expectedVersion int) error
	// UnassignUser clears userID as assignee on every task of the team,
	// recording a history entry for each affected task. Returns the number
	// of tasks changed.
	UnassignUser(ctx context.Context, teamID, userID, changedBy string) (int, error)

	AddHistory(ctx context.Context, h *domain.TaskHistory) error
	ListHistory(ctx context.Context, taskID string) ([]*domain.TaskHistory, error)

	AddComment(ctx context.Context, c *domain.TaskComment) error
	ListComments(ctx context.Context, taskID string) ([]*domain.TaskComment, error)
}

type taskRepo struct {
	db *database.DB
}

func NewTaskRepository(db *database.DB) TaskRepository {
	return &taskRepo{db: db}
}

const taskColumns = `id, team_id, title, description, status, created_by, assignee_id,
	created_at, updated_at, closed_at, version`

func (r *taskRepo) Create(ctx context.Context, t *domain.Task) error {
	const q = `
		INSERT INTO tasks (id, team_id, title, description, status, created_by, assignee_id,
		                   created_at, updated_at, closed_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Q(ctx).ExecContext(ctx, q,
		t.ID, t.TeamID, t.Title, t.Description, t.Status, t.CreatedBy, t.AssigneeID,
		t.CreatedAt, t.UpdatedAt, t.ClosedAt, t.Version,
	)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return domain.ErrNotFound.WithMessage("team or assignee not found")
		}
		return fmt.Errorf("taskRepo.Create: %w", err)
	}
	return nil
}

func (r *taskRepo) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	return r.getOne(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
}

func (r *taskRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Task, error) {
	return r.getOne(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ? FOR UPDATE`, id)
}

func (r *taskRepo) getOne(ctx context.Context, q, id string) (*domain.Task, error) {
	var t domain.Task
	if err := r.db.Q(ctx).GetContext(ctx, &t, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound.WithMessage("task not found")
		}
		return nil, fmt.Errorf("taskRepo.get: %w", err)
	}
	return &t, nil
}

// filter builds the shared WHERE clause for List and Count.
func filter(q *domain.TaskListQuery) (string, []any) {
	var sb strings.Builder
	args := []any{q.TeamID}
	sb.WriteString(" WHERE team_id = ?")

	if q.Status != "" {
		sb.WriteString(" AND status = ?")
		args = append(args, q.Status)
	}
	if q.AssigneeID != "" {
		sb.WriteString(" AND assignee_id = ?")
		args = append(args, q.AssigneeID)
	}

	return sb.String(), args
}

func (r *taskRepo) List(ctx context.Context, q *domain.TaskListQuery) ([]*domain.Task, error) {
	where, args := filter(q)

	// (created_at, id) is a total order, so pages never overlap or skip
	// rows even when many tasks share a timestamp.
	query := `SELECT ` + taskColumns + ` FROM tasks` + where +
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, q.Limit, q.Offset)

	tasks := make([]*domain.Task, 0, q.Limit)
	if err := r.db.Q(ctx).SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, fmt.Errorf("taskRepo.List: %w", err)
	}
	return tasks, nil
}

func (r *taskRepo) Count(ctx context.Context, q *domain.TaskListQuery) (int, error) {
	where, args := filter(q)

	var n int
	if err := r.db.Q(ctx).GetContext(ctx, &n, `SELECT COUNT(*) FROM tasks`+where, args...); err != nil {
		return 0, fmt.Errorf("taskRepo.Count: %w", err)
	}
	return n, nil
}

func (r *taskRepo) Update(ctx context.Context, t *domain.Task, expectedVersion int) error {
	const q = `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, assignee_id = ?, closed_at = ?,
		    updated_at = ?, version = version + 1
		WHERE id = ? AND version = ?`

	res, err := r.db.Q(ctx).ExecContext(ctx, q,
		t.Title, t.Description, t.Status, t.AssigneeID, t.ClosedAt, t.UpdatedAt,
		t.ID, expectedVersion,
	)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return domain.ErrNotFound.WithMessage("assignee not found")
		}
		return fmt.Errorf("taskRepo.Update: %w", err)
	}
	if err := requireAffected(res, domain.ErrVersionConflict); err != nil {
		return err
	}

	t.Version = expectedVersion + 1
	return nil
}

func (r *taskRepo) UnassignUser(ctx context.Context, teamID, userID, changedBy string) (int, error) {
	db := r.db.Q(ctx)

	// History first: it needs the old assignee. Both statements are
	// set-based (no per-task round-trips) and run in the caller's tx.
	const historyQ = `
		INSERT INTO task_history (task_id, changed_by, changes, created_at)
		SELECT id, ?, JSON_OBJECT('assignee_id', JSON_OBJECT('old', assignee_id, 'new', NULL)), UTC_TIMESTAMP()
		FROM tasks
		WHERE team_id = ? AND assignee_id = ?
		FOR UPDATE`
	if _, err := db.ExecContext(ctx, historyQ, changedBy, teamID, userID); err != nil {
		return 0, fmt.Errorf("taskRepo.UnassignUser: history: %w", err)
	}

	const updateQ = `
		UPDATE tasks
		SET assignee_id = NULL, updated_at = UTC_TIMESTAMP(), version = version + 1
		WHERE team_id = ? AND assignee_id = ?`
	res, err := db.ExecContext(ctx, updateQ, teamID, userID)
	if err != nil {
		return 0, fmt.Errorf("taskRepo.UnassignUser: update: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("taskRepo.UnassignUser: rows affected: %w", err)
	}
	return int(n), nil
}

func (r *taskRepo) AddHistory(ctx context.Context, h *domain.TaskHistory) error {
	const q = `INSERT INTO task_history (task_id, changed_by, changes, created_at) VALUES (?, ?, ?, ?)`

	res, err := r.db.Q(ctx).ExecContext(ctx, q, h.TaskID, h.ChangedBy, h.Changes, h.CreatedAt)
	if err != nil {
		return fmt.Errorf("taskRepo.AddHistory: %w", err)
	}
	if id, err := res.LastInsertId(); err == nil {
		h.ID = id
	}
	return nil
}

func (r *taskRepo) ListHistory(ctx context.Context, taskID string) ([]*domain.TaskHistory, error) {
	const q = `
		SELECT id, task_id, changed_by, changes, created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY created_at DESC, id DESC`

	history := make([]*domain.TaskHistory, 0)
	if err := r.db.Q(ctx).SelectContext(ctx, &history, q, taskID); err != nil {
		return nil, fmt.Errorf("taskRepo.ListHistory: %w", err)
	}
	return history, nil
}

func (r *taskRepo) AddComment(ctx context.Context, c *domain.TaskComment) error {
	const q = `INSERT INTO task_comments (id, task_id, user_id, content, created_at) VALUES (?, ?, ?, ?, ?)`

	if _, err := r.db.Q(ctx).ExecContext(ctx, q, c.ID, c.TaskID, c.UserID, c.Content, c.CreatedAt); err != nil {
		if database.IsForeignKeyViolation(err) {
			return domain.ErrNotFound.WithMessage("task not found")
		}
		return fmt.Errorf("taskRepo.AddComment: %w", err)
	}
	return nil
}

func (r *taskRepo) ListComments(ctx context.Context, taskID string) ([]*domain.TaskComment, error) {
	const q = `
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY created_at ASC, id ASC`

	comments := make([]*domain.TaskComment, 0)
	if err := r.db.Q(ctx).SelectContext(ctx, &comments, q, taskID); err != nil {
		return nil, fmt.Errorf("taskRepo.ListComments: %w", err)
	}
	return comments, nil
}
