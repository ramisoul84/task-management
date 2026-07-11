package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
	"github.com/ramisoul84/task-management/pkg/logger"
)

func newTaskService(
	t *testing.T,
	taskRepo *fakeTaskRepo,
	taskCache *fakeTaskCache,
	teamRepo *fakeTeamRepo,
	rbac *fakeRBAC,
) service.TaskService {
	t.Helper()

	l := logger.New("debug", "test", false, "stdout")
	l.Logger = l.Logger.Level(zerolog.Disabled)

	return service.NewTaskService(taskRepo, taskCache, teamRepo, rbac, l)
}

// ==================== TESTS ====================

func TestTaskService_CreateTask(t *testing.T) {
	ctx := context.Background()

	validInput := &domain.CreateTaskRequest{
		TeamID:      "team-1",
		Title:       "Fix login bug",
		Description: "Users cannot login",
	}

	t.Run("success_creates_task", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{}
		taskCache := &fakeTaskCache{}
		teamRepo := &fakeTeamRepo{isMember: true}
		rbac := &fakeRBAC{}
		svc := newTaskService(t, taskRepo, taskCache, teamRepo, rbac)

		task, err := svc.CreateTask(ctx, "user-1", validInput)

		if err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
		if task == nil {
			t.Fatal("CreateTask() = nil")
		}
		if task.ID == "" {
			t.Error("task.ID is empty")
		}
		if task.TeamID != "team-1" {
			t.Errorf("TeamID = %q, want team-1", task.TeamID)
		}
		if task.Title != "Fix login bug" {
			t.Errorf("Title = %q, want 'Fix login bug'", task.Title)
		}
		if task.Status != domain.TaskStatusTodo {
			t.Errorf("Status = %q, want todo", task.Status)
		}
		if task.CreatedBy != "user-1" {
			t.Errorf("CreatedBy = %q, want user-1", task.CreatedBy)
		}
		if task.AssigneeID == nil || *task.AssigneeID != "user-1" {
			t.Error("AssigneeID should default to creator")
		}
		if task.Version != 1 {
			t.Errorf("Version = %d, want 1", task.Version)
		}
		if len(taskRepo.created) != 1 {
			t.Errorf("Create called %d times, want 1", len(taskRepo.created))
		}
		if taskCache.invCalled != 1 {
			t.Errorf("InvalidateTeamTasks called %d times, want 1", taskCache.invCalled)
		}
	})

	t.Run("empty_creator_id", func(t *testing.T) {
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})
		_, err := svc.CreateTask(ctx, "", validInput)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("nil_input", func(t *testing.T) {
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})
		_, err := svc.CreateTask(ctx, "user-1", nil)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("empty_title", func(t *testing.T) {
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})
		input := &domain.CreateTaskRequest{TeamID: "team-1", Title: ""}
		_, err := svc.CreateTask(ctx, "user-1", input)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("rbac_denial", func(t *testing.T) {
		rbac := &fakeRBAC{}
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, &fakeTeamRepo{isMember: false}, rbac)
		_, err := svc.CreateTask(ctx, "user-1", validInput)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("assignee_not_in_team", func(t *testing.T) {
		assigneeID := "ghost"
		input := &domain.CreateTaskRequest{
			TeamID:     "team-1",
			Title:      "Test",
			AssigneeID: &assigneeID,
		}
		teamRepo := &fakeTeamRepo{isMember: false} // assignee is not member
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		_, err := svc.CreateTask(ctx, "user-1", input)
		if !errors.Is(err, domain.ErrAssigneeNotInTeam) {
			t.Fatalf("error = %v, want %v", err, domain.ErrAssigneeNotInTeam)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{createErr: errTaskRepoFailure}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		_, err := svc.CreateTask(ctx, "user-1", validInput)
		if !errors.Is(err, errTaskRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errTaskRepoFailure)
		}
	})
}

func TestTaskService_ListTasksByTeam(t *testing.T) {
	ctx := context.Background()

	query := &domain.TaskListQuery{
		TeamID: "team-1",
		Limit:  20,
		Offset: 0,
	}

	t.Run("success_with_cache_miss", func(t *testing.T) {
		wantTasks := []*domain.Task{{ID: "task-1", TeamID: "team-1", Title: "Task 1"}}
		taskRepo := &fakeTaskRepo{tasksList: wantTasks, count: 1}
		taskCache := &fakeTaskCache{found: false}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, taskCache, teamRepo, &fakeRBAC{})

		result, err := svc.ListTasksByTeam(ctx, "user-1", query)

		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.Total != 1 {
			t.Errorf("Total = %d, want 1", result.Total)
		}
		if len(result.Tasks) != 1 {
			t.Errorf("Tasks len = %d, want 1", len(result.Tasks))
		}
		if result.HasMore {
			t.Error("HasMore = true, want false")
		}
	})

	t.Run("success_with_cache_hit", func(t *testing.T) {
		cached := &domain.TaskListResponse{
			Tasks: []*domain.Task{{ID: "cached-task"}},
			Total: 1,
		}
		taskCache := &fakeTaskCache{cached: cached, found: true}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, &fakeTaskRepo{}, taskCache, teamRepo, &fakeRBAC{})

		result, err := svc.ListTasksByTeam(ctx, "user-1", query)

		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.Tasks[0].ID != "cached-task" {
			t.Errorf("got %q, want cached-task", result.Tasks[0].ID)
		}
	})

	t.Run("not_team_member", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{isMember: false}
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		_, err := svc.ListTasksByTeam(ctx, "user-1", query)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("empty_user_id", func(t *testing.T) {
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})
		_, err := svc.ListTasksByTeam(ctx, "", query)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{listErr: errTaskRepoFailure}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		_, err := svc.ListTasksByTeam(ctx, "user-1", query)
		if !errors.Is(err, errTaskRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errTaskRepoFailure)
		}
	})
}

func TestTaskService_UpdateTask(t *testing.T) {
	ctx := context.Background()

	currentTask := &domain.Task{
		ID:        "task-1",
		TeamID:    "team-1",
		Title:     "Old title",
		Status:    domain.TaskStatusTodo,
		CreatedBy: "user-1",
		Version:   1,
	}

	t.Run("success_update_title", func(t *testing.T) {
		title := "New title"
		input := &domain.UpdateTaskRequest{
			Title:   &title,
			Version: 1,
		}
		taskRepo := &fakeTaskRepo{task: currentTask, updatedTask: currentTask}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		task, err := svc.UpdateTask(ctx, "user-1", "task-1", input)

		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if task == nil {
			t.Fatal("task = nil")
		}
		if len(taskRepo.updateCalls) != 1 {
			t.Fatalf("UpdateWithHistory called %d times", len(taskRepo.updateCalls))
		}
	})

	t.Run("version_conflict", func(t *testing.T) {
		input := &domain.UpdateTaskRequest{
			Title:   strPtr("New"),
			Version: 99,
		}
		taskRepo := &fakeTaskRepo{task: currentTask}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})

		_, err := svc.UpdateTask(ctx, "user-1", "task-1", input)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("error = %v, want %v", err, domain.ErrConflict)
		}
	})

	t.Run("task_not_found", func(t *testing.T) {
		input := &domain.UpdateTaskRequest{Version: 1}
		taskRepo := &fakeTaskRepo{getErr: domain.ErrNotFound}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})

		_, err := svc.UpdateTask(ctx, "user-1", "ghost", input)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrNotFound)
		}
	})

	t.Run("no_changes", func(t *testing.T) {
		input := &domain.UpdateTaskRequest{Version: 1}
		taskRepo := &fakeTaskRepo{task: currentTask}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})

		_, err := svc.UpdateTask(ctx, "user-1", "task-1", input)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("update_error", func(t *testing.T) {
		title := "New"
		input := &domain.UpdateTaskRequest{Title: &title, Version: 1}
		taskRepo := &fakeTaskRepo{task: currentTask, updateErr: errTaskRepoFailure}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})

		_, err := svc.UpdateTask(ctx, "user-1", "task-1", input)
		if !errors.Is(err, errTaskRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errTaskRepoFailure)
		}
	})
}

func TestTaskService_GetTaskHistory(t *testing.T) {
	ctx := context.Background()

	task := &domain.Task{ID: "task-1", TeamID: "team-1"}

	t.Run("success", func(t *testing.T) {
		wantHistory := []*domain.TaskHistory{{ID: 1, TaskID: "task-1"}}
		taskRepo := &fakeTaskRepo{task: task, history: wantHistory}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		history, err := svc.GetTaskHistory(ctx, "user-1", "task-1")

		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(history) != 1 {
			t.Errorf("len = %d, want 1", len(history))
		}
	})

	t.Run("not_team_member", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{task: task}
		teamRepo := &fakeTeamRepo{isMember: false}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		_, err := svc.GetTaskHistory(ctx, "user-1", "task-1")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("task_not_found", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{getErr: domain.ErrNotFound}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})

		_, err := svc.GetTaskHistory(ctx, "user-1", "ghost")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrNotFound)
		}
	})
}

func TestTaskService_AddComment(t *testing.T) {
	ctx := context.Background()

	task := &domain.Task{ID: "task-1", TeamID: "team-1"}

	t.Run("success", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{task: task}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		input := &domain.CreateCommentRequest{Content: "Great work!"}
		comment, err := svc.AddComment(ctx, "user-1", "task-1", input)

		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if comment == nil {
			t.Fatal("comment = nil")
		}
		if comment.Content != "Great work!" {
			t.Errorf("Content = %q", comment.Content)
		}
		if taskRepo.addedComment == nil {
			t.Error("AddComment not called")
		}
	})

	t.Run("empty_content", func(t *testing.T) {
		svc := newTaskService(t, &fakeTaskRepo{}, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})
		_, err := svc.AddComment(ctx, "user-1", "task-1", &domain.CreateCommentRequest{Content: ""})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("task_not_found", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{getErr: domain.ErrNotFound}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})
		_, err := svc.AddComment(ctx, "user-1", "ghost", &domain.CreateCommentRequest{Content: "Test"})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrNotFound)
		}
	})
}

func TestTaskService_ListComments(t *testing.T) {
	ctx := context.Background()

	task := &domain.Task{ID: "task-1", TeamID: "team-1"}

	t.Run("success", func(t *testing.T) {
		wantComments := []*domain.TaskComment{{ID: "c1", TaskID: "task-1"}}
		taskRepo := &fakeTaskRepo{task: task, comments: wantComments}
		teamRepo := &fakeTeamRepo{isMember: true}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		comments, err := svc.ListComments(ctx, "user-1", "task-1")

		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(comments) != 1 {
			t.Errorf("len = %d, want 1", len(comments))
		}
	})

	t.Run("not_team_member", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{task: task}
		teamRepo := &fakeTeamRepo{isMember: false}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, teamRepo, &fakeRBAC{})

		_, err := svc.ListComments(ctx, "user-1", "task-1")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("task_not_found", func(t *testing.T) {
		taskRepo := &fakeTaskRepo{getErr: domain.ErrNotFound}
		svc := newTaskService(t, taskRepo, &fakeTaskCache{}, &fakeTeamRepo{}, &fakeRBAC{})

		_, err := svc.ListComments(ctx, "user-1", "ghost")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrNotFound)
		}
	})
}

// Helper
func strPtr(s string) *string {
	return &s
}
