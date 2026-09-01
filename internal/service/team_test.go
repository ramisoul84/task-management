package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
)

type teamFixture struct {
	s     *store
	tx    *fakeTx
	cache *fakeTaskCache
	svc   service.TeamService
}

func newTeamFixture(t *testing.T) *teamFixture {
	t.Helper()
	s := newStore()
	for _, u := range []string{"owner", "admin", "member", "newbie", "outsider"} {
		s.addUser(u)
	}
	s.addTeam(teamA, "owner")
	s.addMember(teamA, "admin", admin)
	s.addMember(teamA, "member", member)

	tx := &fakeTx{}
	cache := newFakeTaskCache()
	return &teamFixture{
		s:     s,
		tx:    tx,
		cache: cache,
		svc:   service.NewTeamService(tx, teamRepo{s}, userRepo{s}, taskRepo{s}, cache),
	}
}

func TestCreateTeam(t *testing.T) {
	f := newTeamFixture(t)

	team, err := f.svc.CreateTeam(context.Background(), "newbie", "  Platform Team ")
	wantErr(t, err, nil)

	if team.Name != "Platform Team" {
		t.Fatalf("name = %q; names must keep their original casing", team.Name)
	}
	if team.CreatedBy != "newbie" || team.Role != owner {
		t.Fatalf("unexpected team: %+v", team)
	}
	if f.tx.calls != 1 {
		t.Fatal("team and owner membership must be created in one transaction")
	}
	if f.s.members[team.ID]["newbie"] != owner {
		t.Fatal("creator must become owner")
	}
}

func TestListTeams_OnlyOwnTeams(t *testing.T) {
	f := newTeamFixture(t)
	f.s.addTeam(teamB, "outsider")

	teams, err := f.svc.ListTeams(context.Background(), "member")
	wantErr(t, err, nil)
	if len(teams) != 1 || teams[0].ID != teamA || teams[0].Role != member {
		t.Fatalf("teams = %+v, want only team-a as member", teams)
	}
}

func TestInviteMember(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		actor string
		user  string
		want  error
	}{
		{"owner invites", "owner", "newbie", nil},
		{"admin invites", "admin", "newbie", nil},
		{"member cannot invite", "member", "newbie", domain.ErrForbidden},
		{"outsider cannot invite", "outsider", "newbie", domain.ErrForbidden},
		{"unknown user", "owner", "ghost", domain.ErrNotFound},
		{"already a member", "owner", "member", domain.ErrAlreadyExists},
		{"cannot invite self", "owner", "owner", domain.ErrInvalidInput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTeamFixture(t)
			_, err := f.svc.InviteMember(ctx, tc.actor, teamA, &domain.InviteMemberRequest{UserID: tc.user})
			wantErr(t, err, tc.want)

			if tc.want == nil && f.s.members[teamA][tc.user] != member {
				t.Fatalf("invitee role = %q, want member", f.s.members[teamA][tc.user])
			}
		})
	}
}

func TestChangeMemberRole(t *testing.T) {
	ctx := context.Background()

	t.Run("owner promotes member", func(t *testing.T) {
		f := newTeamFixture(t)
		wantErr(t, f.svc.ChangeMemberRole(ctx, "owner", teamA, "member", admin), nil)
		if f.s.members[teamA]["member"] != admin {
			t.Fatal("role not updated")
		}
	})

	t.Run("admin cannot demote owner", func(t *testing.T) {
		f := newTeamFixture(t)
		wantErr(t, f.svc.ChangeMemberRole(ctx, "admin", teamA, "owner", member), domain.ErrForbidden)
	})

	t.Run("unknown target", func(t *testing.T) {
		f := newTeamFixture(t)
		wantErr(t, f.svc.ChangeMemberRole(ctx, "owner", teamA, "outsider", admin), domain.ErrNotFound)
	})
}

func TestRemoveMember_UnassignsTheirTasks(t *testing.T) {
	ctx := context.Background()
	f := newTeamFixture(t)

	assignee := "member"
	f.s.addTask(domain.Task{ID: "t1", TeamID: teamA, CreatedBy: "owner", AssigneeID: &assignee})
	other := "admin"
	f.s.addTask(domain.Task{ID: "t2", TeamID: teamA, CreatedBy: "owner", AssigneeID: &other})

	wantErr(t, f.svc.RemoveMember(ctx, "owner", teamA, "member"), nil)

	if _, still := f.s.members[teamA]["member"]; still {
		t.Fatal("member not removed")
	}
	if f.s.tasks["t1"].AssigneeID != nil {
		t.Fatal("removed member's task must be unassigned")
	}
	if f.s.tasks["t2"].AssigneeID == nil {
		t.Fatal("other members' tasks must be untouched")
	}
	if len(f.s.history) != 1 {
		t.Fatalf("history entries = %d, want 1", len(f.s.history))
	}
	var changes domain.TaskChanges
	_ = json.Unmarshal(f.s.history[0].Changes, &changes)
	if _, ok := changes["assignee_id"]; !ok {
		t.Fatalf("history must record the unassignment: %s", f.s.history[0].Changes)
	}
	if len(f.cache.invalidated) != 1 {
		t.Fatal("task cache must be invalidated after unassigning")
	}
}

func TestRemoveMember_Rules(t *testing.T) {
	ctx := context.Background()
	f := newTeamFixture(t)

	wantErr(t, f.svc.RemoveMember(ctx, "admin", teamA, "owner"), domain.ErrForbidden)
	wantErr(t, f.svc.RemoveMember(ctx, "member", teamA, "admin"), domain.ErrForbidden)
	wantErr(t, f.svc.RemoveMember(ctx, "owner", teamA, "owner"), domain.ErrInvalidInput)
	wantErr(t, f.svc.RemoveMember(ctx, "owner", teamA, "outsider"), domain.ErrNotFound)
}

func TestGetStats_ManagersOnly(t *testing.T) {
	ctx := context.Background()
	f := newTeamFixture(t)

	_, err := f.svc.GetStats(ctx, "owner", teamA)
	wantErr(t, err, nil)
	_, err = f.svc.GetStats(ctx, "admin", teamA)
	wantErr(t, err, nil)
	_, err = f.svc.GetStats(ctx, "member", teamA)
	wantErr(t, err, domain.ErrForbidden)
	_, err = f.svc.GetStats(ctx, "outsider", teamA)
	wantErr(t, err, domain.ErrForbidden)
}

func TestInviteMember_ByEmailWithRole(t *testing.T) {
	ctx := context.Background()
	f := newTeamFixture(t)

	added, err := f.svc.InviteMember(ctx, "owner", teamA, &domain.InviteMemberRequest{Email: "NEWBIE@example.com", Role: admin})
	wantErr(t, err, nil)
	if added.UserID != "newbie" || added.Role != admin || f.s.members[teamA]["newbie"] != admin {
		t.Fatalf("added = %+v", added)
	}

	_, err = f.svc.InviteMember(ctx, "owner", teamA, &domain.InviteMemberRequest{Email: "nobody@example.com"})
	wantErr(t, err, domain.ErrNotFound)

	_, err = f.svc.InviteMember(ctx, "owner", teamA, &domain.InviteMemberRequest{UserID: "outsider", Role: owner})
	wantErr(t, err, domain.ErrInvalidInput)
}

func TestTeamLifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("get requires membership and reports role", func(t *testing.T) {
		f := newTeamFixture(t)
		team, err := f.svc.GetTeam(ctx, "admin", teamA)
		wantErr(t, err, nil)
		if team.Role != admin {
			t.Fatalf("role = %s", team.Role)
		}
		_, err = f.svc.GetTeam(ctx, "outsider", teamA)
		wantErr(t, err, domain.ErrForbidden)
	})

	t.Run("managers rename, members cannot", func(t *testing.T) {
		f := newTeamFixture(t)
		team, err := f.svc.RenameTeam(ctx, "admin", teamA, "  Renamed ")
		wantErr(t, err, nil)
		if team.Name != "Renamed" {
			t.Fatalf("name = %q", team.Name)
		}
		_, err = f.svc.RenameTeam(ctx, "member", teamA, "Nope")
		wantErr(t, err, domain.ErrForbidden)
	})

	t.Run("only the owner deletes", func(t *testing.T) {
		f := newTeamFixture(t)
		wantErr(t, f.svc.DeleteTeam(ctx, "admin", teamA), domain.ErrForbidden)
		wantErr(t, f.svc.DeleteTeam(ctx, "outsider", teamA), domain.ErrForbidden)
		wantErr(t, f.svc.DeleteTeam(ctx, "owner", teamA), nil)
		if _, ok := f.s.teams[teamA]; ok {
			t.Fatal("team not deleted")
		}
		if len(f.cache.invalidated) != 1 {
			t.Fatal("deleting a team must invalidate its task cache")
		}
	})

	t.Run("members leave and their tasks are unassigned; owner cannot leave", func(t *testing.T) {
		f := newTeamFixture(t)
		assignee := "member"
		f.s.addTask(domain.Task{ID: "t1", TeamID: teamA, CreatedBy: "owner", AssigneeID: &assignee})

		wantErr(t, f.svc.LeaveTeam(ctx, "member", teamA), nil)
		if _, still := f.s.members[teamA]["member"]; still {
			t.Fatal("member still in team")
		}
		if f.s.tasks["t1"].AssigneeID != nil {
			t.Fatal("leaving must unassign the member's tasks")
		}
		wantErr(t, f.svc.LeaveTeam(ctx, "owner", teamA), domain.ErrForbidden)
		wantErr(t, f.svc.LeaveTeam(ctx, "outsider", teamA), domain.ErrForbidden)
	})

	t.Run("ownership transfer keeps exactly one owner", func(t *testing.T) {
		f := newTeamFixture(t)
		wantErr(t, f.svc.TransferOwnership(ctx, "admin", teamA, "member"), domain.ErrForbidden)
		wantErr(t, f.svc.TransferOwnership(ctx, "owner", teamA, "outsider"), domain.ErrNotFound)
		wantErr(t, f.svc.TransferOwnership(ctx, "owner", teamA, "owner"), domain.ErrInvalidInput)

		wantErr(t, f.svc.TransferOwnership(ctx, "owner", teamA, "member"), nil)
		if f.s.members[teamA]["member"] != owner || f.s.members[teamA]["owner"] != admin {
			t.Fatalf("roles after transfer = %v", f.s.members[teamA])
		}
		owners := 0
		for _, r := range f.s.members[teamA] {
			if r == owner {
				owners++
			}
		}
		if owners != 1 {
			t.Fatalf("team has %d owners", owners)
		}
	})
}
