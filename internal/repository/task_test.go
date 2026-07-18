package repository

import (
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
)

func TestHistoryChangesIncludesOldAndNewValues(t *testing.T) {
	assigneeID := "user-1"
	closedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	task := domain.Task{
		Title:       "Original title",
		Description: "Original description",
		Status:      domain.TaskStatusTodo,
		AssigneeID:  &assigneeID,
		ClosedAt:    &closedAt,
	}

	changes := historyChanges(task, map[string]interface{}{
		"title":       "Updated title",
		"status":      domain.TaskStatusDone,
		"assignee_id": nil,
		"closed_at":   nil,
	})

	if got, want := changes["title"]["old"], "Original title"; got != want {
		t.Errorf("title old = %v, want %v", got, want)
	}
	if got, want := changes["title"]["new"], "Updated title"; got != want {
		t.Errorf("title new = %v, want %v", got, want)
	}
	if got, want := changes["status"]["old"], domain.TaskStatusTodo; got != want {
		t.Errorf("status old = %v, want %v", got, want)
	}
	if got, want := changes["status"]["new"], domain.TaskStatusDone; got != want {
		t.Errorf("status new = %v, want %v", got, want)
	}
	if got := changes["assignee_id"]["old"]; got == nil {
		t.Error("assignee old value is missing")
	}
	if got := changes["assignee_id"]["new"]; got != nil {
		t.Errorf("assignee new = %v, want nil", got)
	}
}
