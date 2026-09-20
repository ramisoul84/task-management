//go:build integration

package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

// Seeder inserts rows directly with SQL so tests control every column
// (timestamps in particular). Every ID is random, so tests never collide
// and can run against the same database.
type Seeder struct {
	T  *testing.T
	DB *database.DB
}

func (s Seeder) exec(q string, args ...any) {
	s.T.Helper()
	if _, err := s.DB.ExecContext(context.Background(), q, args...); err != nil {
		s.T.Fatalf("seed %q: %v", q, err)
	}
}

func (s Seeder) User(name string) string {
	s.T.Helper()
	id := uuid.NewString()
	s.exec(`INSERT INTO users (id, email, password_hash, name) VALUES (?, ?, 'x', ?)`, id, id+"@test.local", name)
	return id
}

func (s Seeder) Team(ownerID string) string {
	s.T.Helper()
	id := uuid.NewString()
	s.exec(`INSERT INTO teams (id, name, created_by) VALUES (?, 'team', ?)`, id, ownerID)
	s.Member(id, ownerID, domain.RoleOwner)
	return id
}

func (s Seeder) Member(teamID, userID string, role domain.Role) {
	s.T.Helper()
	s.exec(`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`, teamID, userID, role)
}

// TaskSpec describes a task to seed; zero values get sensible defaults.
type TaskSpec struct {
	Title     string
	Status    domain.TaskStatus
	Priority  domain.TaskPriority
	Assignee  string
	CreatedAt time.Time
	ClosedAt  *time.Time
	DueAt     *time.Time
}

func (s Seeder) Task(teamID, creatorID string, spec TaskSpec) string {
	s.T.Helper()
	id := uuid.NewString()
	if spec.Title == "" {
		spec.Title = "task " + id[:8]
	}
	if spec.Status == "" {
		spec.Status = domain.TaskStatusTodo
	}
	if spec.Priority == "" {
		spec.Priority = domain.PriorityMedium
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC().Truncate(time.Second)
	}
	var assignee *string
	if spec.Assignee != "" {
		assignee = &spec.Assignee
	}
	s.exec(`
		INSERT INTO tasks (id, team_id, title, description, status, priority, due_at, created_by, assignee_id,
		                   created_at, updated_at, closed_at, version)
		VALUES (?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		id, teamID, spec.Title, spec.Status, spec.Priority, spec.DueAt, creatorID, assignee,
		spec.CreatedAt, spec.CreatedAt, spec.ClosedAt,
	)
	return id
}

func (s Seeder) Comment(taskID, userID string) {
	s.T.Helper()
	s.exec(`INSERT INTO task_comments (id, task_id, user_id, content) VALUES (?, ?, ?, 'c')`, uuid.NewString(), taskID, userID)
}
