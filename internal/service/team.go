package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type TeamService interface {
	CreateTeam(ctx context.Context, userID, name string) (*domain.Team, error)
	ListTeams(ctx context.Context, userID string) ([]*domain.Team, error)
	GetTeam(ctx context.Context, userID, teamID string) (*domain.Team, error)
	RenameTeam(ctx context.Context, userID, teamID, name string) (*domain.Team, error)
	DeleteTeam(ctx context.Context, userID, teamID string) error
	InviteMember(ctx context.Context, actorID, teamID string, req *domain.InviteMemberRequest) (*domain.TeamMemberAdded, error)
	LeaveTeam(ctx context.Context, userID, teamID string) error
	TransferOwnership(ctx context.Context, actorID, teamID, newOwnerID string) error
	ListMembers(ctx context.Context, userID, teamID string) ([]*domain.TeamMemberResponse, error)
	ChangeMemberRole(ctx context.Context, actorID, teamID, targetID string, role domain.Role) error
	RemoveMember(ctx context.Context, actorID, teamID, targetID string) error
	GetStats(ctx context.Context, userID, teamID string) (*domain.TeamStats, error)
}

type teamService struct {
	tx        Transactor
	teams     repository.TeamRepository
	users     repository.UserRepository
	tasks     repository.TaskRepository
	taskCache cache.TaskCache
	now       func() time.Time
}

func NewTeamService(
	tx Transactor,
	teams repository.TeamRepository,
	users repository.UserRepository,
	tasks repository.TaskRepository,
	taskCache cache.TaskCache,
) TeamService {
	return &teamService{
		tx:        tx,
		teams:     teams,
		users:     users,
		tasks:     tasks,
		taskCache: taskCache,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// CreateTeam creates the team and its owner membership atomically.
func (s *teamService) CreateTeam(ctx context.Context, userID, name string) (*domain.Team, error) {
	team := &domain.Team{
		ID:        uuid.NewString(),
		Name:      strings.TrimSpace(name),
		CreatedBy: userID,
		CreatedAt: s.now(),
		Role:      domain.RoleOwner,
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.teams.Create(ctx, team); err != nil {
			return err
		}
		return s.teams.AddMember(ctx, &domain.TeamMember{TeamID: team.ID, UserID: userID, Role: domain.RoleOwner})
	})
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info().Str("team_id", team.ID).Str("owner_id", userID).Msg("team created")
	return team, nil
}

// ListTeams returns only the teams the user belongs to.
func (s *teamService) ListTeams(ctx context.Context, userID string) ([]*domain.Team, error) {
	return s.teams.ListByUser(ctx, userID)
}

// GetTeam returns a team the caller belongs to, with their role.
func (s *teamService) GetTeam(ctx context.Context, userID, teamID string) (*domain.Team, error) {
	role, err := memberRole(ctx, s.teams, teamID, userID)
	if err != nil {
		return nil, err
	}
	team, err := s.teams.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	team.Role = role
	return team, nil
}

func (s *teamService) RenameTeam(ctx context.Context, userID, teamID, name string) (*domain.Team, error) {
	role, err := memberRole(ctx, s.teams, teamID, userID)
	if err != nil {
		return nil, err
	}
	if err := CanRenameTeam(role); err != nil {
		return nil, err
	}
	if err := s.teams.Rename(ctx, teamID, strings.TrimSpace(name)); err != nil {
		return nil, err
	}
	return s.GetTeam(ctx, userID, teamID)
}

// DeleteTeam removes the team and everything in it. Owner only.
func (s *teamService) DeleteTeam(ctx context.Context, userID, teamID string) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		role, err := s.teams.GetMemberRoleForUpdate(ctx, teamID, userID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden.WithMessage("you are not a member of this team")
		}
		if err != nil {
			return err
		}
		if err := CanDeleteTeam(role); err != nil {
			return err
		}
		return s.teams.Delete(ctx, teamID)
	})
	if err != nil {
		return err
	}

	s.invalidateTasks(ctx, teamID)
	logger.FromContext(ctx).Info().Str("team_id", teamID).Str("actor_id", userID).Msg("team deleted")
	return nil
}

// InviteMember adds an existing user, found by ID or email, to the team.
func (s *teamService) InviteMember(
	ctx context.Context, actorID, teamID string, req *domain.InviteMemberRequest,
) (*domain.TeamMemberAdded, error) {
	actorRole, err := memberRole(ctx, s.teams, teamID, actorID)
	if err != nil {
		return nil, err
	}

	role := req.Role
	if role == "" {
		role = domain.RoleMember
	}
	if err := CanInvite(actorRole, role); err != nil {
		return nil, err
	}

	var invitee *domain.User
	if req.Email != "" {
		invitee, err = s.users.GetByEmail(ctx, normalizeEmail(req.Email))
	} else {
		invitee, err = s.users.GetByID(ctx, req.UserID)
	}
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrNotFound.WithMessage("no user with this id or email")
	}
	if err != nil {
		return nil, err
	}
	if invitee.ID == actorID {
		return nil, domain.ErrInvalidInput.WithMessage("you are already a member of this team")
	}

	// The (team_id, user_id) primary key makes this race-free: a
	// concurrent duplicate invite fails with ErrAlreadyExists.
	if err := s.teams.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: invitee.ID, Role: role}); err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info().
		Str("team_id", teamID).Str("actor_id", actorID).Str("invited_user_id", invitee.ID).Str("role", string(role)).
		Msg("member invited")
	return &domain.TeamMemberAdded{TeamID: teamID, UserID: invitee.ID, Role: role}, nil
}

// LeaveTeam removes the caller from the team, unassigning their tasks.
func (s *teamService) LeaveTeam(ctx context.Context, userID, teamID string) error {
	var unassigned int

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		role, err := s.teams.GetMemberRoleForUpdate(ctx, teamID, userID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden.WithMessage("you are not a member of this team")
		}
		if err != nil {
			return err
		}
		if err := CanLeave(role); err != nil {
			return err
		}
		if err := s.teams.RemoveMember(ctx, teamID, userID); err != nil {
			return err
		}
		unassigned, err = s.tasks.UnassignUser(ctx, teamID, userID, userID)
		return err
	})
	if err != nil {
		return err
	}

	if unassigned > 0 {
		s.invalidateTasks(ctx, teamID)
	}
	return nil
}

// TransferOwnership makes another member the owner; the previous owner
// becomes an admin. Both rows are locked so the team always has exactly
// one owner.
func (s *teamService) TransferOwnership(ctx context.Context, actorID, teamID, newOwnerID string) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		actorRole, err := s.teams.GetMemberRoleForUpdate(ctx, teamID, actorID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden.WithMessage("you are not a member of this team")
		}
		if err != nil {
			return err
		}
		if err := CanTransferOwnership(actorRole, actorID == newOwnerID); err != nil {
			return err
		}
		if _, err := s.teams.GetMemberRoleForUpdate(ctx, teamID, newOwnerID); err != nil {
			return err
		}
		if err := s.teams.UpdateMemberRole(ctx, teamID, newOwnerID, domain.RoleOwner); err != nil {
			return err
		}
		return s.teams.UpdateMemberRole(ctx, teamID, actorID, domain.RoleAdmin)
	})
	if err != nil {
		return err
	}

	logger.FromContext(ctx).Info().
		Str("team_id", teamID).Str("from", actorID).Str("to", newOwnerID).Msg("team ownership transferred")
	return nil
}

func (s *teamService) ListMembers(ctx context.Context, userID, teamID string) ([]*domain.TeamMemberResponse, error) {
	if _, err := memberRole(ctx, s.teams, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.ListMembers(ctx, teamID)
}

func (s *teamService) ChangeMemberRole(ctx context.Context, actorID, teamID, targetID string, role domain.Role) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		actorRole, err := memberRole(ctx, s.teams, teamID, actorID)
		if err != nil {
			return err
		}
		// Lock the target's membership so two managers cannot race.
		targetRole, err := s.teams.GetMemberRoleForUpdate(ctx, teamID, targetID)
		if err != nil {
			return err
		}
		if err := CanChangeRole(actorRole, targetRole, role, actorID == targetID); err != nil {
			return err
		}
		return s.teams.UpdateMemberRole(ctx, teamID, targetID, role)
	})
}

// RemoveMember deletes the membership and, in the same transaction,
// unassigns the removed user's tasks (recording history), so a task is
// never left assigned to someone outside its team.
func (s *teamService) RemoveMember(ctx context.Context, actorID, teamID, targetID string) error {
	var unassigned int

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		actorRole, err := memberRole(ctx, s.teams, teamID, actorID)
		if err != nil {
			return err
		}
		targetRole, err := s.teams.GetMemberRoleForUpdate(ctx, teamID, targetID)
		if err != nil {
			return err
		}
		if err := CanRemoveMember(actorRole, targetRole, actorID == targetID); err != nil {
			return err
		}
		if err := s.teams.RemoveMember(ctx, teamID, targetID); err != nil {
			return err
		}
		unassigned, err = s.tasks.UnassignUser(ctx, teamID, targetID, actorID)
		return err
	})
	if err != nil {
		return err
	}

	if unassigned > 0 {
		s.invalidateTasks(ctx, teamID)
	}

	logger.FromContext(ctx).Info().
		Str("team_id", teamID).Str("actor_id", actorID).Str("removed_user_id", targetID).
		Int("unassigned_tasks", unassigned).
		Msg("member removed")
	return nil
}

// GetStats returns the analytics report; owner/admin only.
func (s *teamService) GetStats(ctx context.Context, userID, teamID string) (*domain.TeamStats, error) {
	role, err := memberRole(ctx, s.teams, teamID, userID)
	if err != nil {
		return nil, err
	}
	if err := CanViewAnalytics(role); err != nil {
		return nil, err
	}
	return s.teams.GetStats(ctx, teamID)
}

func (s *teamService) invalidateTasks(ctx context.Context, teamID string) {
	if err := s.taskCache.InvalidateTeam(ctx, teamID); err != nil && !errors.Is(err, context.Canceled) {
		logger.FromContext(ctx).Warn().Err(err).Str("team_id", teamID).Msg("task cache invalidation failed")
	}
}
