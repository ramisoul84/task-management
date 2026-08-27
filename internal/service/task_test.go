package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
)

const (
	teamA = "team-a"
	teamB = "team-b"
	taskT = "task-1"
)

type taskFixture struct {
	s     *store
	tx    *fakeTx
	cache *fakeTaskCache
	svc   service.TaskService
}

// newTaskFixture builds team A (owner, admin, creator, assignee, other) and
// team B (outsider) with one task in team A created by "creator" and
// assigned to "assignee".
func newTaskFixture(t *testing.T) *taskFixture {
	t.Helper()
	s := newStore()
	for _, u := range []string{"owner", "admin", "creator", "assignee", "other", "outsider"} {
		s.addUser(u)
	}
	s.addTeam(teamA, "owner")
	s.addMember(teamA, "admin", admin)
	s.addMember(teamA, "creator", member)
	s.addMember(teamA, "assignee", member)
	s.addMember(teamA, "other", member)
	s.addTeam(teamB, "outsider")

	assignee := "assignee"
	s.addTask(domain.Task{ID: taskT, TeamID: teamA, Title: "Original", CreatedBy: "creator", AssigneeID: &assignee})

	tx := &fakeTx{}
	cache := newFakeTaskCache()
	return &taskFixture{
		s:     s,
		tx:    tx,
		cache: cache,
		svc:   service.NewTaskService(tx, taskRepo{s}, teamRepo{s}, cache),
	}
}

func ptr[T any](v T) *T { return &v }

func TestCreateTask(t *testing.T) {
	ctx := context.Background()

	t.Run("member creates task; defaults assignee to self", func(t *testing.T) {
		f := newTaskFixture(t)
		task, err := f.svc.CreateTask(ctx, "other", &domain.CreateTaskRequest{TeamID: teamA, Title: "  New task  "})
		wantErr(t, err, nil)

		if task.CreatedBy != "other" || task.AssigneeID == nil || *task.AssigneeID != "other" {
			t.Fatalf("creator/assignee = %s/%v, want other/other", task.CreatedBy, task.AssigneeID)
		}
		if task.Title != "New task" || task.Status != domain.TaskStatusTodo || task.Version != 1 {
			t.Fatalf("unexpected task: %+v", task)
		}
		if len(f.cache.invalidated) != 1 || f.cache.invalidated[0] != teamA {
			t.Fatalf("cache invalidations = %v, want [team-a]", f.cache.invalidated)
		}
		if f.tx.calls != 1 {
			t.Fatal("task and its history must be written in one transaction")
		}
		if len(f.s.history) != 1 || f.s.history[0].Action != domain.HistoryCreated || f.s.history[0].ChangedBy != "other" {
			t.Fatalf("expected one 'created' history entry, got %+v", f.s.history)
		}
	})

	t.Run("non-member cannot create in someone else's team", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.CreateTask(ctx, "outsider", &domain.CreateTaskRequest{TeamID: teamA, Title: "Sneaky"})
		wantErr(t, err, domain.ErrForbidden)
	})

	t.Run("assignee from another team is rejected", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.CreateTask(ctx, "creator", &domain.CreateTaskRequest{
			TeamID: teamA, Title: "Task", AssigneeID: ptr("outsider"),
		})
		wantErr(t, err, domain.ErrAssigneeNotInTeam)
	})

	t.Run("repository failure propagates", func(t *testing.T) {
		f := newTaskFixture(t)
		f.s.failNext = errBoom
		_, err := f.svc.CreateTask(ctx, "creator", &domain.CreateTaskRequest{TeamID: teamA, Title: "Task"})
		if !errors.Is(err, errBoom) {
			t.Fatalf("error = %v, want boom", err)
		}
	})
}

func TestListTasks(t *testing.T) {
	ctx := context.Background()

	t.Run("applies default page size and caches result", func(t *testing.T) {
		f := newTaskFixture(t)
		q := &domain.TaskListQuery{TeamID: teamA}

		res, err := f.svc.ListTasks(ctx, "other", q)
		wantErr(t, err, nil)
		if res.Total != 1 || len(res.Tasks) != 1 || res.Limit != service.DefaultPageSize || res.HasMore {
			t.Fatalf("unexpected page: %+v", res)
		}
		if len(f.cache.entries) != 1 {
			t.Fatalf("expected result to be cached")
		}

		// Second call is served from cache even if the store changes.
		f.s.addTask(domain.Task{ID: "task-2", TeamID: teamA, CreatedBy: "owner"})
		res, err = f.svc.ListTasks(ctx, "other", &domain.TaskListQuery{TeamID: teamA})
		wantErr(t, err, nil)
		if res.Total != 1 {
			t.Fatalf("expected cached total 1, got %d", res.Total)
		}
	})

	t.Run("non-member is rejected before the cache is read", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.ListTasks(ctx, "outsider", &domain.TaskListQuery{TeamID: teamA})
		wantErr(t, err, domain.ErrForbidden)
	})

	t.Run("cache errors fall back to the database", func(t *testing.T) {
		f := newTaskFixture(t)
		f.cache.getErr = errBoom
		res, err := f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA})
		wantErr(t, err, nil)
		if res.Total != 1 {
			t.Fatalf("total = %d, want 1", res.Total)
		}
	})

	t.Run("limit is capped", func(t *testing.T) {
		f := newTaskFixture(t)
		res, err := f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA, Limit: 10_000})
		wantErr(t, err, nil)
		if res.Limit != service.MaxPageSize {
			t.Fatalf("limit = %d, want %d", res.Limit, service.MaxPageSize)
		}
	})
}

func TestListTasks_CursorPagination(t *testing.T) {
	ctx := context.Background()
	f := newTaskFixture(t)
	for _, id := range []string{"task-2", "task-3", "task-4", "task-5"} {
		f.s.addTask(domain.Task{ID: id, TeamID: teamA, CreatedBy: "owner"})
	}

	var (
		seen   []string
		cursor string
		pages  int
	)
	for {
		res, err := f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA, Limit: 2, Cursor: cursor})
		wantErr(t, err, nil)
		pages++
		for _, task := range res.Tasks {
			seen = append(seen, task.ID)
		}
		if res.Total != 5 {
			t.Fatalf("total = %d, want 5", res.Total)
		}
		if !res.HasMore {
			if res.NextCursor != "" {
				t.Fatal("last page must not carry a next_cursor")
			}
			break
		}
		if res.NextCursor == "" {
			t.Fatal("has_more page must carry a next_cursor")
		}
		cursor = res.NextCursor
	}

	want := []string{"task-5", "task-4", "task-3", "task-2", "task-1"}
	if pages != 3 || strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("pages=%d seen=%v, want 3 pages of %v", pages, seen, want)
	}
}

func TestListTasks_CursorValidation(t *testing.T) {
	ctx := context.Background()
	f := newTaskFixture(t)

	_, err := f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA, Cursor: "%%%garbage"})
	wantErr(t, err, domain.ErrInvalidInput)

	_, err = f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA, Sort: domain.SortPriority, Cursor: "x"})
	wantErr(t, err, domain.ErrInvalidInput)

	// A cursor issued for one ordering cannot be replayed against another.
	c := domain.CursorAfter(&domain.Task{ID: "task-1", CreatedAt: time.Now()}, "", "").Encode()
	_, err = f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA, Order: "asc", Cursor: c})
	wantErr(t, err, domain.ErrInvalidInput)
}

func TestUpdateTask_Permissions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		user string
		req  domain.UpdateTaskRequest
		want error
	}{
		{"owner edits title", "owner", domain.UpdateTaskRequest{Title: ptr("Owner edit")}, nil},
		{"admin reassigns", "admin", domain.UpdateTaskRequest{AssigneeID: ptr("other")}, nil},
		{"creator edits everything", "creator", domain.UpdateTaskRequest{
			Title: ptr("New"), Description: ptr("d"), Status: ptr(domain.TaskStatusReview), AssigneeID: ptr("other"),
		}, nil},
		{"assignee changes status", "assignee", domain.UpdateTaskRequest{Status: ptr(domain.TaskStatusInProgress)}, nil},
		{"assignee cannot edit title", "assignee", domain.UpdateTaskRequest{Title: ptr("Hijack")}, domain.ErrForbidden},
		{"assignee cannot reassign", "assignee", domain.UpdateTaskRequest{AssigneeID: ptr("other")}, domain.ErrForbidden},
		{"unrelated member cannot edit", "other", domain.UpdateTaskRequest{Status: ptr(domain.TaskStatusDone)}, domain.ErrForbidden},
		{"outsider sees not found", "outsider", domain.UpdateTaskRequest{Title: ptr("x")}, domain.ErrNotFound},
		{"assignee must be in team", "owner", domain.UpdateTaskRequest{AssigneeID: ptr("outsider")}, domain.ErrAssigneeNotInTeam},
		{"creator sets priority", "creator", domain.UpdateTaskRequest{Priority: ptr(domain.PriorityUrgent)}, nil},
		{"assignee cannot change priority", "assignee", domain.UpdateTaskRequest{Priority: ptr(domain.PriorityUrgent)}, domain.ErrForbidden},
		{"assignee cannot change due date", "assignee", domain.UpdateTaskRequest{
			DueAt: domain.Nullable[time.Time]{Set: true, Valid: true, Value: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
		}, domain.ErrForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTaskFixture(t)
			tc.req.Version = 1

			_, err := f.svc.UpdateTask(ctx, tc.user, taskT, &tc.req)
			wantErr(t, err, tc.want)

			stored := f.s.tasks[taskT]
			if tc.want != nil && stored.Version != 1 {
				t.Fatalf("rejected update must not persist (version %d)", stored.Version)
			}
			if tc.want == nil && stored.Version != 2 {
				t.Fatalf("accepted update must bump version, got %d", stored.Version)
			}
		})
	}
}

func TestUpdateTask_HistoryAndLifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("records old and new values atomically", func(t *testing.T) {
		f := newTaskFixture(t)

		task, err := f.svc.UpdateTask(ctx, "creator", taskT, &domain.UpdateTaskRequest{
			Title: ptr("Renamed"), Status: ptr(domain.TaskStatusDone), Version: 1,
		})
		wantErr(t, err, nil)

		if f.tx.calls != 1 {
			t.Fatalf("expected update to run in one transaction, got %d", f.tx.calls)
		}
		if task.ClosedAt == nil {
			t.Fatal("closed_at must be set when moving to done")
		}
		if len(f.s.history) != 1 {
			t.Fatalf("history entries = %d, want 1", len(f.s.history))
		}

		var changes map[string]map[string]any
		if err := json.Unmarshal(f.s.history[0].Changes, &changes); err != nil {
			t.Fatal(err)
		}
		if changes["title"]["old"] != "Original" || changes["title"]["new"] != "Renamed" {
			t.Fatalf("title change = %v", changes["title"])
		}
		if changes["status"]["old"] != "todo" || changes["status"]["new"] != "done" {
			t.Fatalf("status change = %v", changes["status"])
		}
		if _, ok := changes["closed_at"]; !ok {
			t.Fatal("closed_at change missing from history")
		}
		if f.s.history[0].ChangedBy != "creator" || f.s.history[0].Action != domain.HistoryUpdated {
			t.Fatalf("unexpected history entry: %+v", f.s.history[0])
		}
	})

	t.Run("reopening clears closed_at", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.UpdateTask(ctx, "owner", taskT, &domain.UpdateTaskRequest{Status: ptr(domain.TaskStatusDone), Version: 1})
		wantErr(t, err, nil)

		task, err := f.svc.UpdateTask(ctx, "owner", taskT, &domain.UpdateTaskRequest{Status: ptr(domain.TaskStatusTodo), Version: 2})
		wantErr(t, err, nil)
		if task.ClosedAt != nil {
			t.Fatal("closed_at must be cleared when reopening")
		}
	})

	t.Run("empty string unassigns", func(t *testing.T) {
		f := newTaskFixture(t)
		task, err := f.svc.UpdateTask(ctx, "owner", taskT, &domain.UpdateTaskRequest{AssigneeID: ptr(""), Version: 1})
		wantErr(t, err, nil)
		if task.AssigneeID != nil {
			t.Fatalf("assignee = %v, want nil", *task.AssigneeID)
		}
	})

	t.Run("stale version is a conflict", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.UpdateTask(ctx, "owner", taskT, &domain.UpdateTaskRequest{Title: ptr("First"), Version: 1})
		wantErr(t, err, nil)

		// A second writer that read version 1 must not overwrite the first.
		_, err = f.svc.UpdateTask(ctx, "admin", taskT, &domain.UpdateTaskRequest{Title: ptr("Second"), Version: 1})
		wantErr(t, err, domain.ErrVersionConflict)
		if f.s.tasks[taskT].Title != "First" {
			t.Fatalf("title = %q, lost update", f.s.tasks[taskT].Title)
		}
	})

	t.Run("no-op update writes nothing", func(t *testing.T) {
		f := newTaskFixture(t)
		task, err := f.svc.UpdateTask(ctx, "owner", taskT, &domain.UpdateTaskRequest{Title: ptr("Original"), Version: 1})
		wantErr(t, err, nil)
		if task.Version != 1 || len(f.s.history) != 0 || len(f.cache.invalidated) != 0 {
			t.Fatalf("no-op must not write: version=%d history=%d", task.Version, len(f.s.history))
		}
	})

	t.Run("empty request is invalid", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.UpdateTask(ctx, "owner", taskT, &domain.UpdateTaskRequest{Version: 1})
		wantErr(t, err, domain.ErrInvalidInput)
	})

	t.Run("unknown task", func(t *testing.T) {
		f := newTaskFixture(t)
		_, err := f.svc.UpdateTask(ctx, "owner", "missing", &domain.UpdateTaskRequest{Title: ptr("x"), Version: 1})
		wantErr(t, err, domain.ErrNotFound)
	})
}

func TestTask_PriorityAndDueDate(t *testing.T) {
	ctx := context.Background()

	t.Run("defaults to medium priority", func(t *testing.T) {
		f := newTaskFixture(t)
		task, err := f.svc.CreateTask(ctx, "owner", &domain.CreateTaskRequest{TeamID: teamA, Title: "Defaults"})
		wantErr(t, err, nil)
		if task.Priority != domain.PriorityMedium || task.DueAt != nil {
			t.Fatalf("priority=%s due=%v", task.Priority, task.DueAt)
		}
	})

	t.Run("due date is normalised to UTC", func(t *testing.T) {
		f := newTaskFixture(t)
		local := time.Date(2030, 6, 1, 12, 30, 15, 999, time.FixedZone("MSK", 3*3600))
		task, err := f.svc.CreateTask(ctx, "owner", &domain.CreateTaskRequest{TeamID: teamA, Title: "Due", DueAt: &local})
		wantErr(t, err, nil)
		if task.DueAt.Location() != time.UTC || task.DueAt.Hour() != 9 || task.DueAt.Nanosecond() != 0 {
			t.Fatalf("due_at = %v, want 09:30:15 UTC", task.DueAt)
		}
	})

	t.Run("absurd due date is rejected", func(t *testing.T) {
		f := newTaskFixture(t)
		old := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
		_, err := f.svc.CreateTask(ctx, "owner", &domain.CreateTaskRequest{TeamID: teamA, Title: "Old", DueAt: &old})
		wantErr(t, err, domain.ErrInvalidInput)
	})

	t.Run("set then clear due date, both recorded in history", func(t *testing.T) {
		f := newTaskFixture(t)
		due := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

		task, err := f.svc.UpdateTask(ctx, "creator", taskT, &domain.UpdateTaskRequest{
			DueAt: domain.Nullable[time.Time]{Set: true, Valid: true, Value: due}, Version: 1,
		})
		wantErr(t, err, nil)
		if task.DueAt == nil || !task.DueAt.Equal(due) {
			t.Fatalf("due_at = %v", task.DueAt)
		}

		task, err = f.svc.UpdateTask(ctx, "creator", taskT, &domain.UpdateTaskRequest{
			DueAt: domain.Nullable[time.Time]{Set: true}, Version: 2,
		})
		wantErr(t, err, nil)
		if task.DueAt != nil {
			t.Fatal("explicit null must clear the due date")
		}
		if len(f.s.history) != 2 {
			t.Fatalf("history entries = %d, want 2", len(f.s.history))
		}
	})

	t.Run("overdue filter", func(t *testing.T) {
		f := newTaskFixture(t)
		past := time.Now().Add(-time.Hour)
		f.s.addTask(domain.Task{ID: "late", TeamID: teamA, CreatedBy: "owner", DueAt: &past})
		f.s.addTask(domain.Task{ID: "late-but-done", TeamID: teamA, CreatedBy: "owner", DueAt: &past, Status: domain.TaskStatusDone})

		res, err := f.svc.ListTasks(ctx, "owner", &domain.TaskListQuery{TeamID: teamA, Overdue: true})
		wantErr(t, err, nil)
		if res.Total != 1 || res.Tasks[0].ID != "late" {
			t.Fatalf("overdue tasks = %+v", res.Tasks)
		}
	})
}

func TestGetTask(t *testing.T) {
	ctx := context.Background()
	f := newTaskFixture(t)

	task, err := f.svc.GetTask(ctx, "other", taskT)
	wantErr(t, err, nil)
	if task.ID != taskT {
		t.Fatalf("got task %s", task.ID)
	}

	_, err = f.svc.GetTask(ctx, "outsider", taskT)
	wantErr(t, err, domain.ErrNotFound)
}

func TestDeleteTask(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		user string
		want error
	}{
		{"owner", nil},
		{"admin", nil},
		{"creator", nil},
		{"assignee", domain.ErrForbidden},
		{"other", domain.ErrForbidden},
		{"outsider", domain.ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.user, func(t *testing.T) {
			f := newTaskFixture(t)
			wantErr(t, f.svc.DeleteTask(ctx, tc.user, taskT), tc.want)

			_, exists := f.s.tasks[taskT]
			if exists == (tc.want == nil) {
				t.Fatalf("task exists = %v after delete by %s", exists, tc.user)
			}
			if tc.want == nil && len(f.cache.invalidated) != 1 {
				t.Fatal("delete must invalidate the team's task cache")
			}
		})
	}
}

func TestHistoryAndComments_Visibility(t *testing.T) {
	ctx := context.Background()
	f := newTaskFixture(t)

	t.Run("members can comment and read", func(t *testing.T) {
		c, err := f.svc.AddComment(ctx, "other", taskT, &domain.CreateCommentRequest{Content: "  hello  "})
		wantErr(t, err, nil)
		if c.Content != "hello" || c.UserID != "other" {
			t.Fatalf("unexpected comment: %+v", c)
		}

		comments, err := f.svc.ListComments(ctx, "assignee", taskT)
		wantErr(t, err, nil)
		if len(comments) != 1 {
			t.Fatalf("comments = %d, want 1", len(comments))
		}

		_, err = f.svc.GetHistory(ctx, "other", taskT)
		wantErr(t, err, nil)
	})

	t.Run("outsiders cannot see that the task exists", func(t *testing.T) {
		_, err := f.svc.ListComments(ctx, "outsider", taskT)
		wantErr(t, err, domain.ErrNotFound)
		_, err = f.svc.GetHistory(ctx, "outsider", taskT)
		wantErr(t, err, domain.ErrNotFound)
		_, err = f.svc.AddComment(ctx, "outsider", taskT, &domain.CreateCommentRequest{Content: "x"})
		wantErr(t, err, domain.ErrNotFound)
	})
}

func TestComments_EditAndDelete(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (*taskFixture, string) {
		f := newTaskFixture(t)
		c, err := f.svc.AddComment(ctx, "assignee", taskT, &domain.CreateCommentRequest{Content: "first"})
		wantErr(t, err, nil)
		return f, c.ID
	}

	t.Run("author edits", func(t *testing.T) {
		f, id := setup(t)
		f.s.comments[0].CreatedAt = f.s.comments[0].CreatedAt.Add(-time.Minute)
		c, err := f.svc.EditComment(ctx, "assignee", taskT, id, &domain.CreateCommentRequest{Content: " second "})
		wantErr(t, err, nil)
		if c.Content != "second" || !c.Edited {
			t.Fatalf("unexpected comment: %+v", c)
		}
	})

	t.Run("nobody else edits, not even the owner", func(t *testing.T) {
		f, id := setup(t)
		_, err := f.svc.EditComment(ctx, "owner", taskT, id, &domain.CreateCommentRequest{Content: "censored"})
		wantErr(t, err, domain.ErrForbidden)
	})

	t.Run("comment must belong to the task in the URL", func(t *testing.T) {
		f, id := setup(t)
		f.s.addTask(domain.Task{ID: "task-2", TeamID: teamA, CreatedBy: "owner"})
		_, err := f.svc.EditComment(ctx, "assignee", "task-2", id, &domain.CreateCommentRequest{Content: "x"})
		wantErr(t, err, domain.ErrNotFound)
	})

	for _, tc := range []struct {
		user string
		want error
	}{
		{"assignee", nil}, // author
		{"admin", nil},    // moderation
		{"owner", nil},    // moderation
		{"other", domain.ErrForbidden},
		{"outsider", domain.ErrNotFound},
	} {
		t.Run("delete by "+tc.user, func(t *testing.T) {
			f, id := setup(t)
			wantErr(t, f.svc.DeleteComment(ctx, tc.user, taskT, id), tc.want)
			if (len(f.s.comments) == 0) != (tc.want == nil) {
				t.Fatalf("comments left = %d", len(f.s.comments))
			}
		})
	}
}
