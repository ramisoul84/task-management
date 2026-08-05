//go:build integration

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

// TestTeamRepositoryGetTeamStats runs against a migrated MySQL database. It is
// intentionally opt-in because it needs external infrastructure:
// INTEGRATION_MYSQL_DSN='user:password@tcp(localhost:3306)/task_management?parseTime=true' go test -tags=integration ./internal/repository
func TestTeamRepositoryGetTeamStats(t *testing.T) {
	dsn := os.Getenv("INTEGRATION_MYSQL_DSN")
	if dsn == "" {
		t.Skip("INTEGRATION_MYSQL_DSN is not set")
	}

	sqlDB, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	repo := NewTeamRepository(database.Wrap(sqlDB))
	ctx := context.Background()

	ownerID, assigneeID, teamID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	for _, user := range []struct{ id, email string }{{ownerID, ownerID + "@example.test"}, {assigneeID, assigneeID + "@example.test"}} {
		if _, err := sqlDB.ExecContext(ctx, "INSERT INTO users (id, email, password_hash, name) VALUES (?, ?, 'hash', ?)", user.id, user.email, user.id); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		_, _ = sqlDB.ExecContext(ctx, "DELETE FROM teams WHERE id = ?", teamID)
		_, _ = sqlDB.ExecContext(ctx, "DELETE FROM users WHERE id IN (?, ?)", ownerID, assigneeID)
	}()
	if _, err := sqlDB.ExecContext(ctx, "INSERT INTO teams (id, name, created_by) VALUES (?, 'stats test', ?)", teamID, ownerID); err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		id   string
		role domain.Role
	}{{ownerID, domain.RoleOwner}, {assigneeID, domain.RoleMember}} {
		if _, err := sqlDB.ExecContext(ctx, "INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)", teamID, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}

	statuses := []domain.TaskStatus{domain.TaskStatusTodo, domain.TaskStatusInProgress, domain.TaskStatusReview, domain.TaskStatusCancelled, domain.TaskStatusDone}
	for _, status := range statuses {
		taskID := uuid.NewString()
		var closedAt interface{}
		if status == domain.TaskStatusDone {
			closedAt = now
		}
		if _, err := sqlDB.ExecContext(ctx, "INSERT INTO tasks (id, team_id, title, status, created_by, assignee_id, created_at, closed_at, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)", taskID, teamID, string(status), status, ownerID, assigneeID, now.Add(-2*time.Hour), closedAt); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.ExecContext(ctx, "INSERT INTO task_comments (id, task_id, user_id, content) VALUES (?, ?, ?, 'comment')", uuid.NewString(), taskID, ownerID); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := repo.GetStats(ctx, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTasks != len(statuses) || stats.TotalComments != len(statuses) {
		t.Fatalf("stats = %+v, want %d tasks and comments", stats, len(statuses))
	}
	for _, status := range statuses {
		if stats.TasksByStatus[status] != 1 {
			t.Errorf("status %q = %d, want 1", status, stats.TasksByStatus[status])
		}
	}
	if len(stats.TopPerformers) != 1 || stats.TopPerformers[0].UserID != assigneeID || stats.TopPerformers[0].ClosedTasks != 1 {
		t.Errorf("top performers = %+v, want assignee with one closed task", stats.TopPerformers)
	}
}
