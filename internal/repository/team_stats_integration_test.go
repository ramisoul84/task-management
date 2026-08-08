//go:build integration

// Package repository_test contains integration tests that hit a real MySQL
// instance. They're excluded from `go test ./...` by default and only run
// with the `integration` build tag, since they need a live database.
//
// Run against the MySQL from docker-compose.yml (adjust host/port/creds to
// match your .env), e.g.:
//
//	docker compose up -d mysql --wait
//	TEST_DB_DSN="root:yourpass@tcp(localhost:3306)/task_management?parseTime=true" \
//		go test -tags=integration ./internal/repository/... -run TestGetTeamStats -v
//
// The target database must already have migrations applied (the app's own
// startup migrator does this, or run `migrate` manually against migrations/).
package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set; skipping integration test")
	}

	sqlxDB, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}

	db := database.Wrap(sqlxDB)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func TestGetTeamStats_Integration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	teamRepo := repository.NewTeamRepository(db)

	teamID := uuid.NewString()
	ownerID := uuid.NewString()
	performerAID := uuid.NewString()
	performerBID := uuid.NewString()

	// ---- seed: users, team, tasks, comments ----
	seedUser(t, db, ownerID, "owner@example.com", "Owner")
	seedUser(t, db, performerAID, "a@example.com", "Performer A")
	seedUser(t, db, performerBID, "b@example.com", "Performer B")

	mustExec(t, db, `INSERT INTO teams (id, name, created_by, created_at) VALUES (?, ?, ?, NOW())`,
		teamID, "Stats Test Team", ownerID)
	mustExec(t, db, `INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, 'owner')`, teamID, ownerID)
	mustExec(t, db, `INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, 'member')`, teamID, performerAID)
	mustExec(t, db, `INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, 'member')`, teamID, performerBID)

	now := time.Now()

	// Performer A: 2 done tasks closed within last 30 days -> top performer #1
	task1 := seedTask(t, db, teamID, ownerID, performerAID, domain.TaskStatusDone, now.Add(-48*time.Hour), &now)
	task2 := seedTask(t, db, teamID, ownerID, performerAID, domain.TaskStatusDone, now.Add(-72*time.Hour), &now)

	// Performer B: 1 done task closed within last 30 days -> top performer #2
	closedB := now.Add(-time.Hour)
	seedTask(t, db, teamID, ownerID, performerBID, domain.TaskStatusDone, now.Add(-24*time.Hour), &closedB)

	// Other statuses, should be counted but not close-time/top-performer eligible
	seedTask(t, db, teamID, ownerID, performerAID, domain.TaskStatusTodo, now, nil)
	seedTask(t, db, teamID, ownerID, performerBID, domain.TaskStatusInProgress, now, nil)
	seedTask(t, db, teamID, ownerID, performerAID, domain.TaskStatusReview, now, nil)
	seedTask(t, db, teamID, ownerID, performerBID, domain.TaskStatusCancelled, now, nil)

	// Comments on two of the done tasks
	mustExec(t, db, `INSERT INTO task_comments (id, task_id, user_id, content, created_at) VALUES (?, ?, ?, ?, NOW())`,
		uuid.NewString(), task1, ownerID, "Nice work")
	mustExec(t, db, `INSERT INTO task_comments (id, task_id, user_id, content, created_at) VALUES (?, ?, ?, ?, NOW())`,
		uuid.NewString(), task2, performerAID, "Thanks!")

	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM teams WHERE id = ?`, teamID) // cascades team_members, tasks, task_comments
		mustExec(t, db, `DELETE FROM users WHERE id IN (?, ?, ?)`, ownerID, performerAID, performerBID)
	})

	// ---- act ----
	stats, err := teamRepo.GetStats(ctx, teamID)
	if err != nil {
		t.Fatalf("GetTeamStats() error = %v", err)
	}

	// ---- assert ----
	if stats.TotalTasks != 7 {
		t.Errorf("TotalTasks = %d, want 7", stats.TotalTasks)
	}

	wantByStatus := map[domain.TaskStatus]int{
		domain.TaskStatusTodo:       1,
		domain.TaskStatusInProgress: 1,
		domain.TaskStatusReview:     1,
		domain.TaskStatusDone:       3,
		domain.TaskStatusCancelled:  1,
	}
	for status, want := range wantByStatus {
		if got := stats.TasksByStatus[status]; got != want {
			t.Errorf("TasksByStatus[%s] = %d, want %d", status, got, want)
		}
	}

	if stats.TotalComments != 2 {
		t.Errorf("TotalComments = %d, want 2", stats.TotalComments)
	}

	if stats.AvgCloseTime <= 0 {
		t.Errorf("AvgCloseTime = %v, want > 0", stats.AvgCloseTime)
	}

	if len(stats.TopPerformers) != 2 {
		t.Fatalf("TopPerformers len = %d, want 2", len(stats.TopPerformers))
	}
	if stats.TopPerformers[0].UserID != performerAID || stats.TopPerformers[0].ClosedTasks != 2 {
		t.Errorf("TopPerformers[0] = %+v, want performer A with 2 closed tasks", stats.TopPerformers[0])
	}
	if stats.TopPerformers[1].UserID != performerBID || stats.TopPerformers[1].ClosedTasks != 1 {
		t.Errorf("TopPerformers[1] = %+v, want performer B with 1 closed task", stats.TopPerformers[1])
	}
}

// ---- helpers ----

func seedUser(t *testing.T, db *database.DB, id, email, name string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, name, created_at) VALUES (?, ?, 'x', ?, NOW())`,
		id, email, name)
}

func seedTask(
	t *testing.T,
	db *database.DB,
	teamID, createdBy, assigneeID string,
	status domain.TaskStatus,
	createdAt time.Time,
	closedAt *time.Time,
) string {
	t.Helper()

	id := uuid.NewString()
	mustExec(t, db, `
		INSERT INTO tasks (id, team_id, title, description, status, created_by, assignee_id, created_at, updated_at, closed_at, version)
		VALUES (?, ?, ?, '', ?, ?, ?, ?, ?, ?, 1)`,
		id, teamID, "Test task "+id[:8], status, createdBy, assigneeID, createdAt, createdAt, closedAt,
	)

	return id
}

func mustExec(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
