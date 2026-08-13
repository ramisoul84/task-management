package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/logger"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

type TaskService interface {
	CreateTask(ctx context.Context, userID string, req *domain.CreateTaskRequest) (*domain.Task, error)
	ListTasks(ctx context.Context, userID string, q *domain.TaskListQuery) (*domain.TaskListResponse, error)
	GetTask(ctx context.Context, userID, taskID string) (*domain.Task, error)
	UpdateTask(ctx context.Context, userID, taskID string, req *domain.UpdateTaskRequest) (*domain.Task, error)
	DeleteTask(ctx context.Context, userID, taskID string) error
	GetHistory(ctx context.Context, userID, taskID string) ([]*domain.TaskHistory, error)
	AddComment(ctx context.Context, userID, taskID string, req *domain.CreateCommentRequest) (*domain.TaskComment, error)
	ListComments(ctx context.Context, userID, taskID string) ([]*domain.TaskComment, error)
}

type taskService struct {
	tx    Transactor
	tasks repository.TaskRepository
	teams repository.TeamRepository
	cache cache.TaskCache
	now   func() time.Time
}

func NewTaskService(
	tx Transactor,
	tasks repository.TaskRepository,
	teams repository.TeamRepository,
	taskCache cache.TaskCache,
) TaskService {
	return &taskService{
		tx:    tx,
		tasks: tasks,
		teams: teams,
		cache: taskCache,
		now:   func() time.Time { return time.Now().UTC().Truncate(time.Second) },
	}
}

// CreateTask creates a task in a team the caller belongs to. The caller is
// the creator; the assignee defaults to the caller and must be a member.
func (s *taskService) CreateTask(ctx context.Context, userID string, req *domain.CreateTaskRequest) (*domain.Task, error) {
	if _, err := memberRole(ctx, s.teams, req.TeamID, userID); err != nil {
		return nil, err
	}

	assignee := userID
	if req.AssigneeID != nil && *req.AssigneeID != "" {
		assignee = *req.AssigneeID
	}
	if err := s.requireMember(ctx, req.TeamID, assignee); err != nil {
		return nil, err
	}

	now := s.now()
	task := &domain.Task{
		ID:          uuid.NewString(),
		TeamID:      req.TeamID,
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Status:      domain.TaskStatusTodo,
		CreatedBy:   userID,
		AssigneeID:  &assignee,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}

	// The task and its "created" history entry commit together.
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.tasks.Create(ctx, task); err != nil {
			return err
		}
		return s.recordHistory(ctx, task, userID, domain.HistoryCreated, domain.TaskChanges{
			"title":       {New: task.Title},
			"description": {New: task.Description},
			"status":      {New: task.Status},
			"assignee_id": {New: task.AssigneeID},
		})
	})
	if err != nil {
		return nil, err
	}

	s.invalidate(ctx, task.TeamID)

	logger.FromContext(ctx).Info().Str("task_id", task.ID).Str("team_id", task.TeamID).Msg("task created")
	return task, nil
}

// ListTasks returns a page of the team's tasks. Membership is checked
// before the cache is consulted, so cached data never leaks across teams.
func (s *taskService) ListTasks(ctx context.Context, userID string, q *domain.TaskListQuery) (*domain.TaskListResponse, error) {
	if _, err := memberRole(ctx, s.teams, q.TeamID, userID); err != nil {
		return nil, err
	}

	normalizePage(q)

	// A cache failure is not a request failure: fall through to MySQL.
	cached, stamp, err := s.cache.Get(ctx, q)
	if err != nil {
		logger.FromContext(ctx).Warn().Err(err).Msg("task cache read failed")
	}
	if cached != nil {
		return cached, nil
	}

	tasks, err := s.tasks.List(ctx, q)
	if err != nil {
		return nil, err
	}
	total, err := s.tasks.Count(ctx, q)
	if err != nil {
		return nil, err
	}

	resp := &domain.TaskListResponse{
		Tasks:   tasks,
		Total:   total,
		Limit:   q.Limit,
		Offset:  q.Offset,
		HasMore: q.Offset+len(tasks) < total,
	}

	if err := s.cache.Set(ctx, q, stamp, resp); err != nil {
		logger.FromContext(ctx).Warn().Err(err).Msg("task cache write failed")
	}

	return resp, nil
}

// GetTask returns a single task visible to the caller.
func (s *taskService) GetTask(ctx context.Context, userID, taskID string) (*domain.Task, error) {
	task, err := visibleTask(ctx, s.tasks, s.teams, taskID, userID)
	return task, err
}

// DeleteTask permanently removes a task with its history and comments.
// Owner, admin or the task's creator only.
func (s *taskService) DeleteTask(ctx context.Context, userID, taskID string) error {
	var teamID string

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		task, err := s.tasks.GetByIDForUpdate(ctx, taskID)
		if err != nil {
			return err
		}
		role, err := s.teams.GetMemberRole(ctx, task.TeamID, userID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound.WithMessage("task not found")
		}
		if err != nil {
			return err
		}
		if !TaskPermissionsFor(role, userID, task).Delete {
			return domain.ErrForbidden.WithMessage("only the task creator or a team manager can delete a task")
		}
		teamID = task.TeamID
		return s.tasks.Delete(ctx, task.ID)
	})
	if err != nil {
		return err
	}

	s.invalidate(ctx, teamID)
	logger.FromContext(ctx).Info().Str("task_id", taskID).Str("team_id", teamID).Msg("task deleted")
	return nil
}

// UpdateTask applies a partial update.
//
// Concurrency: the row is locked (SELECT ... FOR UPDATE) for the whole
// transaction and the client's version must match the locked row. Two
// concurrent writers are therefore serialised: the second one sees the
// bumped version and gets ErrVersionConflict instead of silently
// overwriting the first. Permissions and the history diff are computed
// from that same locked, up-to-date row, and the update plus its history
// entry commit atomically.
func (s *taskService) UpdateTask(ctx context.Context, userID, taskID string, req *domain.UpdateTaskRequest) (*domain.Task, error) {
	if req.IsEmpty() {
		return nil, domain.ErrInvalidInput.WithMessage("at least one field must be provided")
	}

	var (
		updated *domain.Task
		changed bool
	)

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		task, err := s.tasks.GetByIDForUpdate(ctx, taskID)
		if err != nil {
			return err
		}

		role, err := s.teams.GetMemberRole(ctx, task.TeamID, userID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound.WithMessage("task not found")
		}
		if err != nil {
			return err
		}

		if req.Version != task.Version {
			return domain.ErrVersionConflict.WithMessage(
				fmt.Sprintf("task was modified (current version %d, yours %d); reload and retry", task.Version, req.Version))
		}

		changes, err := s.applyUpdate(ctx, role, userID, task, req)
		if err != nil {
			return err
		}
		updated = task
		if len(changes) == 0 {
			return nil // nothing actually differs: no write, no history
		}
		changed = true

		expected := task.Version
		task.UpdatedAt = s.now()
		if err := s.tasks.Update(ctx, task, expected); err != nil {
			return err
		}

		return s.recordHistory(ctx, task, userID, domain.HistoryUpdated, changes)
	})
	if err != nil {
		return nil, err
	}

	if changed {
		s.invalidate(ctx, updated.TeamID)
		logger.FromContext(ctx).Info().Str("task_id", updated.ID).Int("version", updated.Version).Msg("task updated")
	}

	return updated, nil
}

// applyUpdate checks permissions for every field the request touches,
// mutates task in place and returns the old/new diff of fields that really
// changed.
func (s *taskService) applyUpdate(
	ctx context.Context,
	role domain.Role,
	userID string,
	task *domain.Task,
	req *domain.UpdateTaskRequest,
) (domain.TaskChanges, error) {
	perms := TaskPermissionsFor(role, userID, task)
	if !perms.Any() {
		return nil, domain.ErrForbidden.WithMessage("you can only edit tasks you created or are assigned to")
	}

	changes := domain.TaskChanges{}

	if req.Title != nil {
		if title := strings.TrimSpace(*req.Title); title != task.Title {
			if !perms.EditContent {
				return nil, domain.ErrForbidden.WithMessage("only the task creator or a team manager can change the title")
			}
			changes["title"] = domain.FieldChange{Old: task.Title, New: title}
			task.Title = title
		}
	}

	if req.Description != nil {
		if desc := strings.TrimSpace(*req.Description); desc != task.Description {
			if !perms.EditContent {
				return nil, domain.ErrForbidden.WithMessage("only the task creator or a team manager can change the description")
			}
			changes["description"] = domain.FieldChange{Old: task.Description, New: desc}
			task.Description = desc
		}
	}

	if req.Status != nil && *req.Status != task.Status {
		if !perms.ChangeStatus {
			return nil, domain.ErrForbidden.WithMessage("you cannot change the status of this task")
		}
		changes["status"] = domain.FieldChange{Old: task.Status, New: *req.Status}
		task.Status = *req.Status

		// closed_at tracks when the task reached "done"; reopening clears it.
		switch {
		case task.Status == domain.TaskStatusDone && task.ClosedAt == nil:
			now := s.now()
			changes["closed_at"] = domain.FieldChange{Old: nil, New: now}
			task.ClosedAt = &now
		case task.Status != domain.TaskStatusDone && task.ClosedAt != nil:
			changes["closed_at"] = domain.FieldChange{Old: *task.ClosedAt, New: nil}
			task.ClosedAt = nil
		}
	}

	if req.AssigneeID != nil {
		var next *string
		if *req.AssigneeID != "" {
			next = req.AssigneeID
		}
		if !sameAssignee(task.AssigneeID, next) {
			if !perms.Reassign {
				return nil, domain.ErrForbidden.WithMessage("assignees can change the status but cannot reassign the task")
			}
			if next != nil {
				if err := s.requireMember(ctx, task.TeamID, *next); err != nil {
					return nil, err
				}
			}
			changes["assignee_id"] = domain.FieldChange{Old: task.AssigneeID, New: next}
			task.AssigneeID = next
		}
	}

	return changes, nil
}

func (s *taskService) GetHistory(ctx context.Context, userID, taskID string) ([]*domain.TaskHistory, error) {
	if _, err := visibleTask(ctx, s.tasks, s.teams, taskID, userID); err != nil {
		return nil, err
	}
	return s.tasks.ListHistory(ctx, taskID)
}

func (s *taskService) AddComment(ctx context.Context, userID, taskID string, req *domain.CreateCommentRequest) (*domain.TaskComment, error) {
	if _, err := visibleTask(ctx, s.tasks, s.teams, taskID, userID); err != nil {
		return nil, err
	}

	comment := &domain.TaskComment{
		ID:        uuid.NewString(),
		TaskID:    taskID,
		UserID:    userID,
		Content:   strings.TrimSpace(req.Content),
		CreatedAt: s.now(),
	}
	if err := s.tasks.AddComment(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *taskService) ListComments(ctx context.Context, userID, taskID string) ([]*domain.TaskComment, error) {
	if _, err := visibleTask(ctx, s.tasks, s.teams, taskID, userID); err != nil {
		return nil, err
	}
	return s.tasks.ListComments(ctx, taskID)
}

func (s *taskService) recordHistory(
	ctx context.Context,
	task *domain.Task,
	userID string,
	action domain.HistoryAction,
	changes domain.TaskChanges,
) error {
	raw, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("marshal task changes: %w", err)
	}
	return s.tasks.AddHistory(ctx, &domain.TaskHistory{
		TaskID:    task.ID,
		ChangedBy: userID,
		Action:    action,
		Changes:   raw,
		CreatedAt: task.UpdatedAt,
	})
}

// requireMember fails with ErrAssigneeNotInTeam when userID is not in teamID.
func (s *taskService) requireMember(ctx context.Context, teamID, userID string) error {
	ok, err := s.teams.IsMember(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrAssigneeNotInTeam
	}
	return nil
}

func (s *taskService) invalidate(ctx context.Context, teamID string) {
	if err := s.cache.InvalidateTeam(ctx, teamID); err != nil {
		logger.FromContext(ctx).Warn().Err(err).Str("team_id", teamID).Msg("task cache invalidation failed")
	}
}

func normalizePage(q *domain.TaskListQuery) {
	if q.Limit <= 0 {
		q.Limit = DefaultPageSize
	}
	if q.Limit > MaxPageSize {
		q.Limit = MaxPageSize
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
}

func sameAssignee(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
