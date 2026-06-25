package service

import (
	"context"
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
