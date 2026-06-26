package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type TaskService interface {
	CreateTask(ctx context.Context, creatorID string, input *domain.CreateTaskRequest) (*domain.Task, error)
	ListTasksByTeam(ctx context.Context, userID string, query *domain.TaskListQuery) (*domain.TaskListResponse, error)
	UpdateTask(ctx context.Context, userID string, taskID string, input *domain.UpdateTaskRequest) (*domain.Task, error)
	GetTaskHistory(ctx context.Context, userID string, taskID string) ([]*domain.TaskHistory, error)
	AddComment(ctx context.Context, userID string, taskID string, input *domain.CreateCommentRequest) (*domain.TaskComment, error)
	ListComments(ctx context.Context, userID string, taskID string) ([]*domain.TaskComment, error)
}

type taskService struct {
	taskRepo repository.TaskRepository
	teamRepo repository.TeamRepository
	rbacSvc  RBACService
	log      *logger.Logger
}

func NewTaskService(
	taskRepo repository.TaskRepository,
	teamRepo repository.TeamRepository,
	rbacSvc RBACService,
	log *logger.Logger,
) TaskService {
	return &taskService{
		taskRepo: taskRepo,
		teamRepo: teamRepo,
		rbacSvc:  rbacSvc,
		log:      log,
	}
}

// Create - inserts a new task
func (s *taskService) CreateTask(ctx context.Context, creatorID string, input *domain.CreateTaskRequest) (*domain.Task, error) {
	const op = "taskService.CreateTask"

	// Input validation
	if creatorID == "" {
		return nil, fmt.Errorf("%s: creator ID is required: %w", op, domain.ErrInvalidInput)
	}
	if input == nil {
		return nil, fmt.Errorf("%s: input is required: %w", op, domain.ErrInvalidInput)
	}
	if input.TeamID == "" {
		return nil, fmt.Errorf("%s: team ID is required: %w", op, domain.ErrInvalidInput)
	}
	if input.Title == "" {
		return nil, fmt.Errorf("%s: title is required: %w", op, domain.ErrInvalidInput)
	}

	// Check permission (any team member can create tasks)
	if err := s.rbacSvc.CanCreateTask(ctx, input.TeamID, creatorID); err != nil {
		return nil, err
	}

	// Set default assignee to creator if not provided
	if input.AssigneeID == nil || *input.AssigneeID == "" {
		input.AssigneeID = &creatorID
	}

	// Validate assignee is a team member
	isMember, err := s.teamRepo.IsTeamMember(ctx, input.TeamID, *input.AssigneeID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", input.TeamID).
			Str("assignee_id", *input.AssigneeID).
			Msg("failed to check assignee membership")
		return nil, fmt.Errorf("%s: check assignee membership: %w", op, err)
	}

	if !isMember {
		s.log.Warn().
			Str("op", op).
			Str("team_id", input.TeamID).
			Str("assignee_id", *input.AssigneeID).
			Msg("assignee is not a team member")
		return nil, fmt.Errorf("%s: assignee must be a team member: %w", op, domain.ErrAssigneeNotInTeam)
	}

	// Create task object
	now := time.Now()
	task := &domain.Task{
		ID:          uuid.NewString(),
		TeamID:      input.TeamID,
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Status:      domain.TaskStatusTodo,
		CreatedBy:   creatorID,
		AssigneeID:  input.AssigneeID,
		CreatedAt:   now,
		UpdatedAt:   now,
		ClosedAt:    nil,
		Version:     1,
	}

	// Save to database
	if err := s.taskRepo.Create(ctx, task); err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("task_id", task.ID).
			Str("team_id", task.TeamID).
			Str("creator_id", creatorID).
			Msg("failed to create task in repository")
		return nil, fmt.Errorf("%s: create task: %w", op, err)
	}

	s.log.Info().
		Str("op", op).
		Str("task_id", task.ID).
		Str("team_id", task.TeamID).
		Str("creator_id", creatorID).
		Str("assignee_id", *input.AssigneeID).
		Msg("task created successfully")

	return task, nil
}

// ListTasksByTeam - lists tasks with filters and pagination
func (s *taskService) ListTasksByTeam(ctx context.Context, userID string, query *domain.TaskListQuery) (*domain.TaskListResponse, error) {
	const op = "taskService.ListTasksByTeam"

	// Input validation
	if userID == "" {
		return nil, fmt.Errorf("%s: user ID is required: %w", op, domain.ErrInvalidInput)
	}
	if query == nil {
		return nil, fmt.Errorf("%s: query is required: %w", op, domain.ErrInvalidInput)
	}
	if query.TeamID == "" {
		return nil, fmt.Errorf("%s: team ID is required: %w", op, domain.ErrInvalidInput)
	}

	isMember, err := s.teamRepo.IsTeamMember(ctx, query.TeamID, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", query.TeamID).
			Str("user_id", userID).
			Msg("failed to check team membership")
		return nil, fmt.Errorf("%s: check membership: %w", op, err)
	}
	if !isMember {
		s.log.Warn().
			Str("op", op).
			Str("team_id", query.TeamID).
			Str("user_id", userID).
			Msg("user is not a team member")
		return nil, domain.ErrForbidden
	}

	tasks, err := s.taskRepo.ListByTeam(ctx, query)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", query.TeamID).
			Msg("failed to list tasks")
		return nil, fmt.Errorf("%s: list tasks: %w", op, err)
	}

	// Get total count for pagination
	total, err := s.taskRepo.CountByTeam(ctx, query)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", query.TeamID).
			Msg("failed to count tasks")
		return nil, fmt.Errorf("%s: count tasks: %w", op, err)
	}

	// Build response
	response := &domain.TaskListResponse{
		Tasks:   tasks,
		Total:   total,
		Limit:   query.Limit,
		Offset:  query.Offset,
		HasMore: query.Offset+len(tasks) < total,
	}

	return response, nil
}

// UpdateTask - updates task with permission checks and history recording
func (s *taskService) UpdateTask(ctx context.Context, userID string, taskID string, input *domain.UpdateTaskRequest) (*domain.Task, error) {
	const op = "taskService.UpdateTask"

	// Input validation
	if userID == "" {
		return nil, fmt.Errorf("%s: user ID is required: %w", op, domain.ErrInvalidInput)
	}
	if taskID == "" {
		return nil, fmt.Errorf("%s: task ID is required: %w", op, domain.ErrInvalidInput)
	}
	if input == nil {
		return nil, fmt.Errorf("%s: input is required: %w", op, domain.ErrInvalidInput)
	}

	// Get current task
	currentTask, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to get task")
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}

	// Check version conflict
	if input.Version != currentTask.Version {
		s.log.Warn().
			Str("op", op).
			Str("task_id", taskID).
			Int("expected_version", input.Version).
			Int("current_version", currentTask.Version).
			Msg("version conflict detected")
		return nil, domain.ErrConflict
	}

	// Build updates map
	updates := make(map[string]interface{})

	// Check title update
	if input.Title != nil {
		if err := s.rbacSvc.CanEditTask(ctx, currentTask.TeamID, userID, currentTask.CreatedBy); err != nil {
			return nil, err
		}
		updates["title"] = strings.TrimSpace(*input.Title)
	}

	// Check description update
	if input.Description != nil {
		if err := s.rbacSvc.CanEditTask(ctx, currentTask.TeamID, userID, currentTask.CreatedBy); err != nil {
			return nil, err
		}
		updates["description"] = strings.TrimSpace(*input.Description)
	}

	// Check status update
	if input.Status != nil {
		if err := s.rbacSvc.CanChangeTaskStatus(ctx, currentTask.TeamID, userID, currentTask.CreatedBy, currentTask.AssigneeID); err != nil {
			return nil, err
		}
		updates["status"] = *input.Status

		// Handle closed_at
		if *input.Status == domain.TaskStatusDone && currentTask.ClosedAt == nil {
			now := time.Now()
			updates["closed_at"] = now
		}
		if *input.Status != domain.TaskStatusDone && currentTask.ClosedAt != nil {
			updates["closed_at"] = nil
		}
	}

	// Check assignee update
	if input.AssigneeID != nil {
		if err := s.rbacSvc.CanReassignTask(ctx, currentTask.TeamID, userID, currentTask.CreatedBy); err != nil {
			return nil, err
		}

		// Validate new assignee is team member
		if *input.AssigneeID != "" {
			isMember, err := s.teamRepo.IsTeamMember(ctx, currentTask.TeamID, *input.AssigneeID)
			if err != nil {
				s.log.Error().Err(err).Str("op", op).Str("team_id", currentTask.TeamID).Str("assignee_id", *input.AssigneeID).Msg("failed to check assignee membership")
				return nil, fmt.Errorf("%s: check assignee membership: %w", op, err)
			}
			if !isMember {
				return nil, domain.ErrAssigneeNotInTeam
			}
			updates["assignee_id"] = *input.AssigneeID
		} else {
			updates["assignee_id"] = nil
		}
	}

	// Check if there are any updates
	if len(updates) == 0 {
		return nil, domain.ErrInvalidInput
	}

	// Update task with history
	updatedTask, err := s.taskRepo.UpdateWithHistory(ctx, taskID, updates, userID, currentTask.Version)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, domain.ErrConflict
		}
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to update task")
		return nil, fmt.Errorf("%s: update task: %w", op, err)
	}

	s.log.Info().
		Str("op", op).
		Str("task_id", taskID).
		Str("user_id", userID).
		Interface("changes", updates).
		Msg("task updated successfully")

	return updatedTask, nil
}

// GetTaskHistory - returns task change history
func (s *taskService) GetTaskHistory(ctx context.Context, userID string, taskID string) ([]*domain.TaskHistory, error) {
	const op = "taskService.GetTaskHistory"

	// Input validation
	if userID == "" {
		return nil, fmt.Errorf("%s: user ID is required: %w", op, domain.ErrInvalidInput)
	}
	if taskID == "" {
		return nil, fmt.Errorf("%s: task ID is required: %w", op, domain.ErrInvalidInput)
	}

	// Get task to check team
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to get task")
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}

	// Check if user is team member
	isMember, err := s.teamRepo.IsTeamMember(ctx, task.TeamID, userID)
	if err != nil {
		s.log.Error().Err(err).Str("op", op).Str("team_id", task.TeamID).Str("user_id", userID).Msg("failed to check membership")
		return nil, fmt.Errorf("%s: check membership: %w", op, err)
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	// Get history
	history, err := s.taskRepo.GetHistory(ctx, taskID)
	if err != nil {
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to get history")
		return nil, fmt.Errorf("%s: get history: %w", op, err)
	}

	return history, nil
}

// AddComment - adds a comment to a task
func (s *taskService) AddComment(ctx context.Context, userID string, taskID string, input *domain.CreateCommentRequest) (*domain.TaskComment, error) {
	const op = "taskService.AddComment"

	// Input validation
	if userID == "" {
		return nil, fmt.Errorf("%s: user ID is required: %w", op, domain.ErrInvalidInput)
	}
	if taskID == "" {
		return nil, fmt.Errorf("%s: task ID is required: %w", op, domain.ErrInvalidInput)
	}
	if input == nil || input.Content == "" {
		return nil, fmt.Errorf("%s: content is required: %w", op, domain.ErrInvalidInput)
	}

	// Get task to check team
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to get task")
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}

	// Check permission (any team member can comment)
	if err := s.rbacSvc.CanCommentOnTask(ctx, task.TeamID, userID); err != nil {
		return nil, err
	}

	// Create comment
	comment := &domain.TaskComment{
		ID:        uuid.NewString(),
		TaskID:    taskID,
		UserID:    userID,
		Content:   strings.TrimSpace(input.Content),
		CreatedAt: time.Now(),
	}

	if err := s.taskRepo.AddComment(ctx, comment); err != nil {
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Str("user_id", userID).Msg("failed to add comment")
		return nil, fmt.Errorf("%s: add comment: %w", op, err)
	}

	s.log.Info().Str("op", op).Str("task_id", taskID).Str("user_id", userID).Str("comment_id", comment.ID).Msg("comment added")

	return comment, nil
}

// ListComments - returns all comments for a task
func (s *taskService) ListComments(ctx context.Context, userID string, taskID string) ([]*domain.TaskComment, error) {
	const op = "taskService.ListComments"

	// Input validation
	if userID == "" {
		return nil, fmt.Errorf("%s: user ID is required: %w", op, domain.ErrInvalidInput)
	}
	if taskID == "" {
		return nil, fmt.Errorf("%s: task ID is required: %w", op, domain.ErrInvalidInput)
	}

	// Get task to check team
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to get task")
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}

	// Check if user is team member
	isMember, err := s.teamRepo.IsTeamMember(ctx, task.TeamID, userID)
	if err != nil {
		s.log.Error().Err(err).Str("op", op).Str("team_id", task.TeamID).Str("user_id", userID).Msg("failed to check membership")
		return nil, fmt.Errorf("%s: check membership: %w", op, err)
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	// Get comments
	comments, err := s.taskRepo.ListComments(ctx, taskID)
	if err != nil {
		s.log.Error().Err(err).Str("op", op).Str("task_id", taskID).Msg("failed to list comments")
		return nil, fmt.Errorf("%s: list comments: %w", op, err)
	}

	return comments, nil
}
