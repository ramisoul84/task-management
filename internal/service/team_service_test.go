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

	l := logger.New("debug", "test", false)
	l.Logger = l.Logger.Level(zerolog.Disabled)

	return service.NewTeamService(team, user, rbac, l)
}

func TestTeamService_CreateTeam(t *testing.T) {
	ctx := context.Background()

	t.Run("success_returns_uppercased_trimmed_team", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		team, err := svc.CreateTeam(ctx, "user-1", "  my team  ")

		if err != nil {
			t.Fatalf("CreateTeam() error = %v, want nil", err)
		}
		if team == nil {
			t.Fatal("CreateTeam() = nil, want a team")
		}
		if team.Name != "MY TEAM" {
			t.Errorf("team.Name = %q, want %q", team.Name, "MY TEAM")
		}
		if team.CreatedBy != "user-1" {
			t.Errorf("team.CreatedBy = %q, want %q", team.CreatedBy, "user-1")
		}
		if team.ID == "" {
			t.Error("team.ID is empty, want a generated ID")
		}
		if len(teamRepo.created) != 1 {
			t.Fatalf("CreateWithOwner called %d time(s), want 1", len(teamRepo.created))
		}
		if teamRepo.created[0] != team {
			t.Error("CreateWithOwner received a different team than the one returned")
		}
	})

	t.Run("empty_owner_id_is_invalid_input", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		_, err := svc.CreateTeam(ctx, "", "Team")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("CreateTeam() error = %v, want %v", err, domain.ErrInvalidInput)
		}
		if len(teamRepo.created) != 0 {
			t.Error("CreateWithOwner was called, want no call")
		}
	})

	t.Run("empty_name_is_invalid_input", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})

		_, err := svc.CreateTeam(ctx, "user-1", "   ")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("CreateTeam() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("repository_error_is_wrapped", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{createErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		_, err := svc.CreateTeam(ctx, "user-1", "Team")

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("CreateTeam() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_ListTeams(t *testing.T) {
	ctx := context.Background()

	t.Run("success_returns_members_teams", func(t *testing.T) {
		want := []*domain.Team{{ID: "team-1", Name: "TEAM-1"}}
		teamRepo := &fakeTeamRepo{teamsList: want}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		got, err := svc.ListTeams(ctx, "user-1")

		if err != nil {
			t.Fatalf("ListTeams() error = %v, want nil", err)
		}
		if len(got) != 1 || got[0].ID != want[0].ID {
			t.Errorf("ListTeams() = %v, want %v", got, want)
		}
	})

	t.Run("empty_user_id_is_invalid_input", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})

		_, err := svc.ListTeams(ctx, "")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("ListTeams() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("repository_error_is_wrapped", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{listErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		_, err := svc.ListTeams(ctx, "user-1")

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("ListTeams() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_InviteMember(t *testing.T) {
	ctx := context.Background()

	t.Run("success_adds_member_with_member_role", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-2": {ID: "user-2"}}}
		svc := newTeamService(t, teamRepo, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")

		if err != nil {
			t.Fatalf("InviteMember() error = %v, want nil", err)
		}
		if len(teamRepo.added) != 1 {
			t.Fatalf("AddMember called %d time(s), want 1", len(teamRepo.added))
		}
		m := teamRepo.added[0]
		if m.TeamID != "team-1" || m.UserID != "user-2" || m.Role != domain.RoleMember {
			t.Errorf("AddMember got %+v, want {team-1 user-2 member}", m)
		}
	})

	t.Run("empty_ids_are_invalid_input", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.InviteMember(ctx, "", "team-1", "user-2")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("InviteMember() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("inviting_self_is_invalid_input", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-1")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("InviteMember() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("rbac_denial_is_passed_through", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		rbac := &fakeRBAC{inviteErr: domain.ErrForbidden}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, rbac)

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")

		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("InviteMember() error = %v, want %v", err, domain.ErrForbidden)
		}
		if rbac.inviteCalls != 1 {
			t.Errorf("RBAC CanInviteMember consulted %v times, want 1", rbac.inviteCalls)
		}
		if len(teamRepo.added) != 0 {
			t.Error("AddMember was called, want no call")
		}
	})

	t.Run("unknown_user_is_not_found", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{}}
		svc := newTeamService(t, teamRepo, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "ghost")

		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("InviteMember() error = %v, want %v", err, domain.ErrNotFound)
		}
		if len(teamRepo.added) != 0 {
			t.Error("AddMember was called, want no call")
		}
	})

	t.Run("already_member_is_already_exists", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{isMember: true}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-2": {ID: "user-2"}}}
		svc := newTeamService(t, teamRepo, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")

		if !errors.Is(err, domain.ErrAlreadyExists) {
			t.Fatalf("InviteMember() error = %v, want %v", err, domain.ErrAlreadyExists)
		}
		if len(teamRepo.added) != 0 {
			t.Error("AddMember was called, want no call")
		}
	})

	t.Run("add_failure_is_wrapped", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{addErr: errRepoFailure}
		userRepo := &fakeUserRepo{users: map[string]*domain.User{"user-2": {ID: "user-2"}}}
		svc := newTeamService(t, teamRepo, userRepo, &fakeRBAC{})

		err := svc.InviteMember(ctx, "user-1", "team-1", "user-2")

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("InviteMember() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_ChangeMemberRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success_passes_correct_args_to_repository", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.ChangeMemberRole(ctx, "user-1", "team-1", "user-2", domain.RoleAdmin)

		if err != nil {
			t.Fatalf("ChangeMemberRole() error = %v, want nil", err)
		}
		if len(teamRepo.updateCalls) != 1 {
			t.Fatalf("UpdateMemberRole called %d time(s), want 1", len(teamRepo.updateCalls))
		}
		call := teamRepo.updateCalls[0]
		if call.teamID != "team-1" || call.actorID != "user-1" || call.userID != "user-2" || call.role != domain.RoleAdmin {
			t.Errorf("UpdateMemberRole got %+v, want {team-1 user-1 user-2 admin}", call)
		}
	})

	t.Run("empty_ids_are_invalid_input", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.ChangeMemberRole(ctx, "user-1", "", "user-2", domain.RoleAdmin)

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("ChangeMemberRole() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("rbac_denial_is_passed_through", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		rbac := &fakeRBAC{changeErr: domain.ErrForbidden}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, rbac)

		err := svc.ChangeMemberRole(ctx, "user-1", "team-1", "user-2", domain.RoleAdmin)

		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("ChangeMemberRole() error = %v, want %v", err, domain.ErrForbidden)
		}
		if len(teamRepo.updateCalls) != 0 {
			t.Error("UpdateMemberRole was called, want no call")
		}
	})

	t.Run("repository_error_is_wrapped", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{updateErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.ChangeMemberRole(ctx, "user-1", "team-1", "user-2", domain.RoleAdmin)

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("ChangeMemberRole() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}

func TestTeamService_RemoveMember(t *testing.T) {
	ctx := context.Background()

	t.Run("success_passes_correct_args_to_repository", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.RemoveMember(ctx, "user-1", "team-1", "user-2")

		if err != nil {
			t.Fatalf("RemoveMember() error = %v, want nil", err)
		}
		if len(teamRepo.removeCalls) != 1 {
			t.Fatalf("RemoveMember repo call count = %d, want 1", len(teamRepo.removeCalls))
		}
		call := teamRepo.removeCalls[0]
		if call.teamID != "team-1" || call.actorID != "user-1" || call.userID != "user-2" {
			t.Errorf("RemoveMember got %+v, want {team-1 user-1 user-2}", call)
		}
	})

	t.Run("empty_ids_are_invalid_input", func(t *testing.T) {
		svc := newTeamService(t, &fakeTeamRepo{}, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.RemoveMember(ctx, "user-1", "", "user-2")

		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("RemoveMember() error = %v, want %v", err, domain.ErrInvalidInput)
		}
	})

	t.Run("rbac_denial_is_passed_through", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{}
		rbac := &fakeRBAC{removeErr: domain.ErrForbidden}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, rbac)

		err := svc.RemoveMember(ctx, "user-1", "team-1", "user-2")

		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("RemoveMember() error = %v, want %v", err, domain.ErrForbidden)
		}
		if len(teamRepo.removeCalls) != 0 {
			t.Error("RemoveMember repo was called, want no call")
		}
	})

	t.Run("repository_error_is_wrapped", func(t *testing.T) {
		teamRepo := &fakeTeamRepo{removeErr: errRepoFailure}
		svc := newTeamService(t, teamRepo, &fakeUserRepo{}, &fakeRBAC{})

		err := svc.RemoveMember(ctx, "user-1", "team-1", "user-2")

		if !errors.Is(err, errRepoFailure) {
			t.Fatalf("RemoveMember() error = %v, want it to wrap %v", err, errRepoFailure)
		}
	})
}
