//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/internal/testutil"
	"github.com/ramisoul84/task-management/pkg/database"
)

var db *database.DB

func TestMain(m *testing.M) {
	ctx := context.Background()
	mysql, err := testutil.StartMySQL(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup:", err)
		os.Exit(1)
	}
	db = mysql.DB

	code := m.Run()
	mysql.Close()
	os.Exit(code)
}

func hoursAgo(h float64) time.Time {
	return time.Now().UTC().Add(-time.Duration(h * float64(time.Hour))).Truncate(time.Second)
}

func ptr[T any](v T) *T { return &v }

// TestIntegration_TeamStats is the integration test the specification
// requires for the SQL report. The seed is built so every metric has an
// exact expected value, including traps: a tie for third place, closed
// tasks outside the 30-day window, and another team's data that must
// never leak in.
func TestIntegration_TeamStats(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	repo := repository.NewTeamRepository(db)

	owner := seed.User("Owner")
	p1, p2, p3, p4 := seed.User("Perf A"), seed.User("Perf B"), seed.User("Perf C"), seed.User("Perf D")
	team := seed.Team(owner)
	for _, p := range []string{p1, p2, p3, p4} {
		seed.Member(team, p, domain.RoleMember)
	}

	// done tasks: (assignee, closed hours ago, hours from creation to close)
	type done struct {
		who       string
		closedAgo float64
		took      float64
	}
	var doneTasks []string
	for _, d := range []done{
		{p1, 24, 10}, {p1, 48, 10}, {p1, 72, 10}, // P1: 3 recent
		{p2, 24, 20}, {p2, 24, 20}, // P2: 2 recent
		{p3, 24, 30},                         // P3: 1 recent ...
		{p3, 40 * 24, 40}, {p3, 41 * 24, 40}, // ... + 2 outside the 30-day window
		{p4, 24, 50}, // P4: 1 recent; ties P3, loses on name
	} {
		closed := hoursAgo(d.closedAgo)
		id := seed.Task(team, owner, testutil.TaskSpec{
			Status: domain.TaskStatusDone, Assignee: d.who,
			CreatedAt: closed.Add(-time.Duration(d.took * float64(time.Hour))), ClosedAt: &closed,
		})
		doneTasks = append(doneTasks, id)
	}

	past := hoursAgo(5)
	seed.Task(team, owner, testutil.TaskSpec{Status: domain.TaskStatusTodo, DueAt: &past}) // overdue
	seed.Task(team, owner, testutil.TaskSpec{Status: domain.TaskStatusTodo})
	seed.Task(team, owner, testutil.TaskSpec{Status: domain.TaskStatusInProgress})
	seed.Task(team, owner, testutil.TaskSpec{Status: domain.TaskStatusReview})
	seed.Task(team, owner, testutil.TaskSpec{Status: domain.TaskStatusCancelled, DueAt: &past}) // past due but closed: not overdue

	for _, id := range doneTasks[:4] {
		seed.Comment(id, owner)
	}

	// Noise in another team: P1 closes 5 tasks there, with comments.
	other := seed.Team(seed.User("Other owner"))
	seed.Member(other, p1, domain.RoleMember)
	for range 5 {
		closed := hoursAgo(1)
		id := seed.Task(other, p1, testutil.TaskSpec{Status: domain.TaskStatusDone, Assignee: p1, CreatedAt: hoursAgo(2), ClosedAt: &closed})
		seed.Comment(id, p1)
	}

	stats, err := repo.GetStats(ctx, team)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if stats.TotalTasks != 14 {
		t.Errorf("TotalTasks = %d, want 14", stats.TotalTasks)
	}
	wantStatus := map[domain.TaskStatus]int{
		domain.TaskStatusTodo: 2, domain.TaskStatusInProgress: 1, domain.TaskStatusReview: 1,
		domain.TaskStatusDone: 9, domain.TaskStatusCancelled: 1,
	}
	for s, want := range wantStatus {
		if got := stats.TasksByStatus[s]; got != want {
			t.Errorf("TasksByStatus[%s] = %d, want %d", s, got, want)
		}
	}
	if stats.OverdueTasks != 1 {
		t.Errorf("OverdueTasks = %d, want 1", stats.OverdueTasks)
	}
	if stats.TotalComments != 4 {
		t.Errorf("TotalComments = %d, want 4 (other team's comments must be excluded)", stats.TotalComments)
	}

	// All done tasks count toward the average: (3*10 + 2*20 + 30 + 2*40 + 50) / 9.
	if want := 230.0 / 9; math.Abs(stats.AvgCloseTime-want) > 0.01 {
		t.Errorf("AvgCloseTime = %.3f h, want %.3f h", stats.AvgCloseTime, want)
	}

	wantTop := []struct {
		id     string
		closed int
	}{{p1, 3}, {p2, 2}, {p3, 1}}
	if len(stats.TopPerformers) != len(wantTop) {
		t.Fatalf("TopPerformers = %+v, want 3 entries", stats.TopPerformers)
	}
	for i, w := range wantTop {
		got := stats.TopPerformers[i]
		if got.UserID != w.id || got.ClosedTasks != w.closed {
			t.Errorf("TopPerformers[%d] = %+v, want %s with %d", i, got, w.id, w.closed)
		}
	}
}

func TestIntegration_TeamStats_EmptyTeam(t *testing.T) {
	seed := testutil.Seeder{T: t, DB: db}
	team := seed.Team(seed.User("Lonely"))

	stats, err := repository.NewTeamRepository(db).GetStats(context.Background(), team)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTasks != 0 || stats.TotalComments != 0 || stats.AvgCloseTime != 0 || len(stats.TopPerformers) != 0 {
		t.Fatalf("empty team stats = %+v", stats)
	}
	if stats.TopPerformers == nil {
		t.Fatal("top_performers must serialise as [] not null")
	}
}

func TestIntegration_TaskList_FiltersSortingAndSearch(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	repo := repository.NewTaskRepository(db)

	owner := seed.User("Owner")
	dev := seed.User("Dev")
	team := seed.Team(owner)
	seed.Member(team, dev, domain.RoleMember)

	soon, later := time.Now().UTC().Add(24*time.Hour), time.Now().UTC().Add(72*time.Hour)
	urgent := seed.Task(team, owner, testutil.TaskSpec{Title: "Fix 100% CPU", Priority: domain.PriorityUrgent, DueAt: &later, Assignee: dev})
	low := seed.Task(team, owner, testutil.TaskSpec{Title: "Polish_ui", Priority: domain.PriorityLow, DueAt: &soon})
	none := seed.Task(team, owner, testutil.TaskSpec{Title: "Docs", Status: domain.TaskStatusDone})

	list := func(q domain.TaskListQuery) []string {
		t.Helper()
		q.TeamID = team
		if q.Limit == 0 {
			q.Limit = 50
		}
		tasks, err := repo.List(ctx, &q, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(tasks))
		for i, task := range tasks {
			ids[i] = task.ID
		}
		return ids
	}
	expect := func(name string, got []string, want ...string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	expect("status", list(domain.TaskListQuery{Status: domain.TaskStatusDone}), none)
	expect("assignee", list(domain.TaskListQuery{AssigneeID: dev}), urgent)
	expect("priority", list(domain.TaskListQuery{Priority: domain.PriorityLow}), low)
	expect("search is case-insensitive", list(domain.TaskListQuery{Search: "docs"}), none)
	expect("% matches literally", list(domain.TaskListQuery{Search: "100%"}), urgent)
	expect("_ matches literally", list(domain.TaskListQuery{Search: "_"}), low)
	expect("priority desc", list(domain.TaskListQuery{Sort: domain.SortPriority}), urgent, none, low)
	expect("due asc, nulls last", list(domain.TaskListQuery{Sort: domain.SortDueAt, Order: "asc"}), low, urgent, none)
	expect("due desc, nulls last", list(domain.TaskListQuery{Sort: domain.SortDueAt, Order: "desc"}), urgent, low, none)

	n, err := repo.Count(ctx, &domain.TaskListQuery{TeamID: team, Search: "%"})
	if err != nil || n != 1 {
		t.Errorf("Count(q=%%) = %d, %v; want 1", n, err)
	}
}

func TestIntegration_TaskList_CursorWalkWithTiedTimestamps(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	repo := repository.NewTaskRepository(db)

	owner := seed.User("Owner")
	team := seed.Team(owner)

	// 7 tasks sharing one created_at second: only the id tie-breaker keeps
	// pages disjoint.
	same := time.Now().UTC().Truncate(time.Second)
	want := map[string]bool{}
	for range 7 {
		want[seed.Task(team, owner, testutil.TaskSpec{CreatedAt: same})] = true
	}

	for _, order := range []string{"desc", "asc"} {
		seen := map[string]bool{}
		var after *domain.TaskCursor
		for page := 0; ; page++ {
			if page > 5 {
				t.Fatalf("%s: too many pages", order)
			}
			q := &domain.TaskListQuery{TeamID: team, Limit: 3, Order: order}
			tasks, err := repo.List(ctx, q, after)
			if err != nil {
				t.Fatal(err)
			}
			for _, task := range tasks {
				if seen[task.ID] {
					t.Fatalf("%s: task %s returned twice", order, task.ID)
				}
				seen[task.ID] = true
			}
			if len(tasks) < 3 {
				break
			}
			c := domain.CursorAfter(tasks[len(tasks)-1], "", order)
			after = &c
		}
		if len(seen) != len(want) {
			t.Fatalf("%s: walked %d tasks, want %d", order, len(seen), len(want))
		}
	}
}

func TestIntegration_TaskUpdate_VersionGuard(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	repo := repository.NewTaskRepository(db)

	owner := seed.User("Owner")
	id := seed.Task(seed.Team(owner), owner, testutil.TaskSpec{})

	task, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	task.Title = "first"
	if err := repo.Update(ctx, task, 1); err != nil {
		t.Fatalf("first update: %v", err)
	}
	if task.Version != 2 {
		t.Fatalf("version = %d, want 2", task.Version)
	}

	task.Title = "stale"
	if err := repo.Update(ctx, task, 1); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale update err = %v, want ErrVersionConflict", err)
	}
	got, _ := repo.GetByID(ctx, id)
	if got.Title != "first" {
		t.Fatalf("title = %q: stale write must not land", got.Title)
	}
}

func TestIntegration_UnassignUserRecordsHistory(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	repo := repository.NewTaskRepository(db)

	owner, dev := seed.User("Owner"), seed.User("Dev")
	team := seed.Team(owner)
	seed.Member(team, dev, domain.RoleMember)
	t1 := seed.Task(team, owner, testutil.TaskSpec{Assignee: dev})
	t2 := seed.Task(team, owner, testutil.TaskSpec{Assignee: dev})
	untouched := seed.Task(team, owner, testutil.TaskSpec{Assignee: owner})

	var n int
	err := db.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		n, err = repo.UnassignUser(ctx, team, dev, owner)
		return err
	})
	if err != nil || n != 2 {
		t.Fatalf("UnassignUser = %d, %v; want 2", n, err)
	}

	for _, id := range []string{t1, t2} {
		task, _ := repo.GetByID(ctx, id)
		if task.AssigneeID != nil || task.Version != 2 {
			t.Fatalf("task %s: assignee=%v version=%d", id, task.AssigneeID, task.Version)
		}
		history, err := repo.ListHistory(ctx, id)
		if err != nil || len(history) != 1 {
			t.Fatalf("history = %v, %v", history, err)
		}
		var changes map[string]map[string]any
		if err := json.Unmarshal(history[0].Changes, &changes); err != nil {
			t.Fatal(err)
		}
		if changes["assignee_id"]["old"] != dev || changes["assignee_id"]["new"] != nil {
			t.Fatalf("history changes = %s", history[0].Changes)
		}
		if history[0].ChangedBy != owner || history[0].Action != domain.HistoryUpdated {
			t.Fatalf("history entry = %+v", history[0])
		}
	}

	if task, _ := repo.GetByID(ctx, untouched); task.AssigneeID == nil {
		t.Fatal("other assignees must be untouched")
	}
}

func TestIntegration_ConstraintErrorsAreTranslated(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	teams := repository.NewTeamRepository(db)
	users := repository.NewUserRepository(db)

	owner := seed.User("Owner")
	team := seed.Team(owner)

	err := teams.AddMember(ctx, &domain.TeamMember{TeamID: team, UserID: owner, Role: domain.RoleMember})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("duplicate member: %v, want ErrAlreadyExists", err)
	}

	err = teams.AddMember(ctx, &domain.TeamMember{TeamID: team, UserID: "00000000-0000-0000-0000-000000000000", Role: domain.RoleMember})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown user: %v, want ErrNotFound", err)
	}

	u, _ := users.GetByID(ctx, owner)
	dup := &domain.User{ID: "11111111-1111-1111-1111-111111111111", Email: u.Email, PasswordHash: "x", Name: "dup", CreatedAt: time.Now()}
	if err := users.Create(ctx, dup); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("duplicate email: %v, want ErrAlreadyExists", err)
	}
}

func TestIntegration_TimestampsRoundTripInUTC(t *testing.T) {
	ctx := context.Background()
	seed := testutil.Seeder{T: t, DB: db}
	repo := repository.NewTaskRepository(db)

	owner := seed.User("Owner")
	due := time.Date(2040, 3, 4, 5, 6, 7, 0, time.UTC) // beyond TIMESTAMP's 2038 limit
	id := seed.Task(seed.Team(owner), owner, testutil.TaskSpec{DueAt: ptr(due)})

	task, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !task.DueAt.Equal(due) || task.DueAt.Location() != time.UTC {
		t.Fatalf("due_at = %v, want %v UTC", task.DueAt, due)
	}
}
