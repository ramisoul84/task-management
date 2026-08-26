package domain

import (
	"encoding/json"
	"time"
)

// TaskStatus is the workflow state of a task.
type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusReview     TaskStatus = "review"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// TaskStatuses lists every valid status.
var TaskStatuses = []TaskStatus{
	TaskStatusTodo, TaskStatusInProgress, TaskStatusReview, TaskStatusDone, TaskStatusCancelled,
}

// IsTerminal reports whether no further work is expected.
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusDone || s == TaskStatusCancelled
}

// TaskPriority orders tasks by urgency.
type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityMedium TaskPriority = "medium"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

// TaskPriorities lists every valid priority, lowest first.
var TaskPriorities = []TaskPriority{PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent}

// Sort fields accepted by GET /tasks.
const (
	SortCreatedAt = "created_at"
	SortUpdatedAt = "updated_at"
	SortDueAt     = "due_at"
	SortPriority  = "priority"
)

// Task is a unit of work inside a team.
type Task struct {
	ID          string       `json:"id" db:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	TeamID      string       `json:"team_id" db:"team_id" example:"123e4567-e89b-12d3-a456-426614174001"`
	Title       string       `json:"title" db:"title" example:"Fix login bug"`
	Description string       `json:"description,omitempty" db:"description" example:"Users cannot login with Google OAuth"`
	Status      TaskStatus   `json:"status" db:"status" example:"todo"`
	Priority    TaskPriority `json:"priority" db:"priority" example:"medium"`
	DueAt       *time.Time   `json:"due_at,omitempty" db:"due_at" example:"2024-02-01T17:00:00Z"`
	CreatedBy   string       `json:"created_by" db:"created_by" example:"123e4567-e89b-12d3-a456-426614174002"`
	AssigneeID  *string      `json:"assignee_id,omitempty" db:"assignee_id" example:"123e4567-e89b-12d3-a456-426614174003"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at" example:"2024-01-15T10:30:00Z"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at" example:"2024-01-15T10:30:00Z"`
	ClosedAt    *time.Time   `json:"closed_at,omitempty" db:"closed_at" example:"2024-01-16T15:00:00Z"`
	Version     int          `json:"version" db:"version" example:"1"`
}

// IsOverdue reports whether the task is past due and still open.
func (t *Task) IsOverdue(now time.Time) bool {
	return t.DueAt != nil && t.DueAt.Before(now) && !t.Status.IsTerminal()
}

// IsAssignee reports whether userID is the task's assignee.
func (t *Task) IsAssignee(userID string) bool {
	return t.AssigneeID != nil && *t.AssigneeID == userID
}

// CreateTaskRequest is the body of POST /tasks.
type CreateTaskRequest struct {
	TeamID      string       `json:"team_id" validate:"required,uuid" example:"123e4567-e89b-12d3-a456-426614174001"`
	Title       string       `json:"title" validate:"required,notblank,min=3,max=255" example:"Fix login bug"`
	Description string       `json:"description" validate:"max=5000" example:"Users cannot login with Google OAuth"`
	AssigneeID  *string      `json:"assignee_id,omitempty" validate:"omitempty,uuid" example:"123e4567-e89b-12d3-a456-426614174003"`
	Priority    TaskPriority `json:"priority,omitempty" validate:"omitempty,oneof=low medium high urgent" example:"high"`
	DueAt       *time.Time   `json:"due_at,omitempty" example:"2024-02-01T17:00:00Z"`
}

// TaskListQuery holds GET /tasks filters, sorting and pagination.
type TaskListQuery struct {
	TeamID     string       `query:"team_id" validate:"required,uuid" example:"123e4567-e89b-12d3-a456-426614174001"`
	Status     TaskStatus   `query:"status" validate:"omitempty,oneof=todo in_progress review done cancelled" example:"todo"`
	AssigneeID string       `query:"assignee_id" validate:"omitempty,uuid" example:"123e4567-e89b-12d3-a456-426614174003"`
	Priority   TaskPriority `query:"priority" validate:"omitempty,oneof=low medium high urgent" example:"high"`
	Overdue    bool         `query:"overdue" example:"false"`
	Search     string       `query:"q" validate:"max=100" example:"login"`
	Sort       string       `query:"sort" validate:"omitempty,oneof=created_at updated_at due_at priority" example:"created_at"`
	Order      string       `query:"order" validate:"omitempty,oneof=asc desc" example:"desc"`
	Limit      int          `query:"limit" validate:"gte=0,lte=100" example:"20"`
	Offset     int          `query:"offset" validate:"gte=0" example:"0"`
	// Cursor is the next_cursor of a previous page (keyset pagination).
	// It cannot be combined with offset.
	Cursor string `query:"cursor" validate:"omitempty,max=512,excluded_with=Offset" example:""`
}

// TaskListResponse is a page of tasks.
type TaskListResponse struct {
	Tasks   []*Task `json:"tasks"`
	Total   int     `json:"total" example:"50"`
	Limit   int     `json:"limit" example:"20"`
	Offset  int     `json:"offset" example:"0"`
	HasMore bool    `json:"has_more" example:"true"`
	// NextCursor fetches the following page via ?cursor=. Present when
	// has_more is true and the sort supports keyset pagination
	// (created_at, updated_at).
	NextCursor string `json:"next_cursor,omitempty" example:"eyJzIjoiY3JlYXRlZF9hdCIsIm8iOiJkZXNjIiwidCI6IjIwMjQtMDEtMTVUMTA6MzA6MDBaIiwiaSI6IjEyMyJ9"`
}

// UpdateTaskRequest is the body of PUT /tasks/{id}. Omitted fields are left
// unchanged; send "assignee_id": "" to unassign. Version must equal the
// task's current version (optimistic concurrency control).
type UpdateTaskRequest struct {
	Title       *string       `json:"title,omitempty" validate:"omitempty,notblank,min=3,max=255" example:"Fix critical login bug"`
	Description *string       `json:"description,omitempty" validate:"omitempty,max=5000" example:"Users cannot login with Google OAuth on mobile"`
	Status      *TaskStatus   `json:"status,omitempty" validate:"omitempty,oneof=todo in_progress review done cancelled" example:"in_progress"`
	AssigneeID  *string       `json:"assignee_id,omitempty" validate:"omitempty,uuid" example:"123e4567-e89b-12d3-a456-426614174003"`
	Priority    *TaskPriority `json:"priority,omitempty" validate:"omitempty,oneof=low medium high urgent" example:"urgent"`
	// DueAt: omit to keep, null to clear, a timestamp to set.
	DueAt   Nullable[time.Time] `json:"due_at" swaggertype:"string" format:"date-time" extensions:"x-nullable" example:"2024-02-01T17:00:00Z"`
	Version int                 `json:"version" validate:"required,min=1" example:"1"`
}

// IsEmpty reports whether the request changes nothing.
func (r *UpdateTaskRequest) IsEmpty() bool {
	return r.Title == nil && r.Description == nil && r.Status == nil && r.AssigneeID == nil &&
		r.Priority == nil && !r.DueAt.Set
}

// CreateCommentRequest is the body of POST /tasks/{id}/comments.
type CreateCommentRequest struct {
	Content string `json:"content" validate:"required,notblank,max=5000" example:"This bug is caused by a race condition in the auth flow"`
}

// TaskComment is a comment on a task.
type TaskComment struct {
	ID        string    `json:"id" db:"id" example:"123e4567-e89b-12d3-a456-426614174004"`
	TaskID    string    `json:"task_id" db:"task_id" example:"123e4567-e89b-12d3-a456-426614174000"`
	UserID    string    `json:"user_id" db:"user_id" example:"123e4567-e89b-12d3-a456-426614174002"`
	Content   string    `json:"content" db:"content" example:"This bug is caused by a race condition"`
	CreatedAt time.Time `json:"created_at" db:"created_at" example:"2024-01-15T11:00:00Z"`
}

// FieldChange captures a field's value before and after a change.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// TaskChanges maps field name to its change, e.g.
// {"status": {"old": "todo", "new": "done"}}.
type TaskChanges map[string]FieldChange

// HistoryAction says what kind of event a history entry records.
type HistoryAction string

const (
	HistoryCreated HistoryAction = "created"
	HistoryUpdated HistoryAction = "updated"
)

// TaskHistory is one audit-log entry: who changed what, and when. For
// "created" entries every initial field is recorded with old = null.
type TaskHistory struct {
	ID        int64           `json:"id" db:"id" example:"1"`
	TaskID    string          `json:"task_id" db:"task_id" example:"123e4567-e89b-12d3-a456-426614174000"`
	ChangedBy string          `json:"changed_by" db:"changed_by" example:"123e4567-e89b-12d3-a456-426614174002"`
	Action    HistoryAction   `json:"action" db:"action" example:"updated" enums:"created,updated"`
	Changes   json.RawMessage `json:"changes" db:"changes" swaggertype:"object"`
	CreatedAt time.Time       `json:"created_at" db:"created_at" example:"2024-01-15T11:00:00Z"`
}
