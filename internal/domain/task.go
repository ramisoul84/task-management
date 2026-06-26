package domain

import (
	"encoding/json"
	"time"
)

type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusReview     TaskStatus = "review"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type Task struct {
	ID          string     `json:"id" db:"id"`
	TeamID      string     `json:"team_id" db:"team_id"`
	Title       string     `json:"title" db:"title"`
	Description string     `json:"description,omitempty" db:"description"`
	Status      TaskStatus `json:"status" db:"status"`
	CreatedBy   string     `json:"created_by" db:"created_by"`
	AssigneeID  *string    `json:"assignee_id,omitempty" db:"assignee_id"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty" db:"closed_at"`
	Version     int        `json:"version" db:"version"`
}

type CreateTaskRequest struct {
	TeamID      string  `json:"team_id" validate:"required,uuid"`
	Title       string  `json:"title" validate:"required,min=3,max=255"`
	Description string  `json:"description" validate:"omitempty,max=5000"`
	AssigneeID  *string `json:"assignee_id,omitempty" validate:"omitempty,uuid"`
}

type TaskListQuery struct {
	TeamID     string     `query:"team_id" validate:"required,uuid"`
	Status     TaskStatus `query:"status" validate:"omitempty,oneof=todo in_progress review done cancelled"`
	AssigneeID string     `query:"assignee_id" validate:"omitempty,uuid"`
	Limit      int        `query:"limit" validate:"omitempty,min=1,max=100"`
	Offset     int        `query:"offset" validate:"omitempty,min=0"`
}

type TaskListResponse struct {
	Tasks   []*Task `json:"tasks"`
	Total   int     `json:"total"`
	Limit   int     `json:"limit"`
	Offset  int     `json:"offset"`
	HasMore bool    `json:"has_more"`
}

type UpdateTaskRequest struct {
	Title       *string     `json:"title,omitempty" validate:"omitempty,min=3,max=255"`
	Description *string     `json:"description,omitempty" validate:"omitempty,max=5000"`
	Status      *TaskStatus `json:"status,omitempty" validate:"omitempty,oneof=todo in_progress done"`
	AssigneeID  *string     `json:"assignee_id,omitempty" validate:"omitempty,uuid"`
	Version     int         `json:"version" validate:"required,min=1"`
}

type CreateCommentRequest struct {
	Content string `json:"content" validate:"required,min=1,max=5000"`
}

type TaskComment struct {
	ID        string    `json:"id" db:"id"`
	TaskID    string    `json:"task_id" db:"task_id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Content   string    `json:"content" db:"content"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type TaskHistory struct {
	ID        int64           `json:"id" db:"id"`
	TaskID    string          `json:"task_id" db:"task_id"`
	ChangedBy string          `json:"changed_by" db:"changed_by"`
	Changes   json.RawMessage `json:"changes" db:"changes"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}
