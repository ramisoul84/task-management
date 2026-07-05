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

func newTeamService(t *testing.T, team *fakeTeamRepo, user *fakeUserRepo, rbac *fakeRBAC) service.TeamService {
	t.Helper()
	l := logger.New("debug", "test", false, "stdout")
	l.Logger = l.Logger.Level(zerolog.Disabled)
	return service.NewTeamService(team, user, rbac, l)
}

func TestTeamService_CreateTeam(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		team, err := svc.CreateTeam(ctx, "user-1", "  my team  ")

		if err != nil {
			t.Fatalf("CreateTeam() error = %v", err)
		}
		if team.Name != "MY TEAM" {
			t.Errorf("Name = %q, want %q", team.Name, "MY TEAM")
		}
		if team.CreatedBy != "user-1" {
			t.Errorf("CreatedBy = %q, want user-1", team.CreatedBy)
		}
		if team.ID == "" {
			t.Error("ID is empty")
		}
		if len(teamRepo.created) != 1 {
			t.Errorf("CreateWithOwner called %d times, want 1", len(teamRepo.created))
		}
	})

	t.Run("empty_owner", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})
		_, err := svc.CreateTeam(ctx, "", "Team")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})
		_, err := svc.CreateTeam(ctx, "user-1", "   ")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{createErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})
		_, err := svc.CreateTeam(ctx, "user-1", "Team")
		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_ListTeamsByUser(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		want := []*domain.Team{{ID: "team-1", Name: "TEAM-1"}}
		teamRepo := &fakeTeamRepo{teamsList: want}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		got, err := svc.ListTeamsByUser(ctx, "user-1")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(got) != 1 || got[0].ID != "team-1" {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("empty_user", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})
		_, err := svc.ListTeamsByUser(ctx, "")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{listErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})
		_, err := svc.ListTeamsByUser(ctx, "user-1")
		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_InviteMember(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-2": {ID: "user-2"}}}
		svc := newTeamService(t, teamRepo, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(teamRepo.added) != 1 {
			t.Fatalf("AddMember called %d times", len(teamRepo.added))
		}
		m := teamRepo.added[0]
		if m.Role != domain.RoleMember {
			t.Errorf("Role = %v, want member", m.Role)
		}
	})

	t.Run("rbac_denial", func(t *testing.T) {
		rbac := &fakeRBAC{inviteErr: domain.ErrForbidden}
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, rbac)

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("user_not_found", func(t *testing.T) {
		userRepo := &fakeUserRepo{users: map[string]*domain.User{}}
		svc := newTeamService(t, &fakeTeamRepo{}, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "ghost")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrNotFound)
		}
	})

	t.Run("already_member", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{isMember: true}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-2": {ID: "user-2"}}}
		svc := newTeamService(t, teamRepo, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")
		if !errors.Is(err, domain.ErrAlreadyExists) {
			t.Fatalf("error = %v, want %v", err, domain.ErrAlreadyExists)
		}
	})
}

func TestTeamService_ChangeMemberRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.ChangeMemberRole(ctx, "user-1", "team-1", "user-2", domain.RoleAdmin)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(teamRepo.updateCalls) != 1 {
			t.Fatalf("UpdateMemberRole called %d times", len(teamRepo.updateCalls))
		}
	})

	t.Run("rbac_denial", func(t *testing.T) {
		rbac := &fakeRBAC{changeErr: domain.ErrForbidden}
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, rbac)

		err := svc.ChangeMemberRole(ctx, "user-1", "team-1", "user-2", domain.RoleAdmin)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{updateErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.ChangeMemberRole(ctx, "user-1", "team-1", "user-2", domain.RoleAdmin)
		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_RemoveMember(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.RemoveMember(ctx, "user-1", "team-1", "user-2")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(teamRepo.removeCalls) != 1 {
			t.Fatalf("RemoveMember called %d times", len(teamRepo.removeCalls))
		}
	})

	t.Run("rbac_denial", func(t *testing.T) {
		rbac := &fakeRBAC{removeErr: domain.ErrForbidden}
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, rbac)

		err := svc.RemoveMember(ctx, "user-1", "team-1", "user-2")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{removeErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.RemoveMember(ctx, "user-1", "team-1", "user-2")
		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("error = %v, want %v", err, errRepoFailure)
		}
	})
}
