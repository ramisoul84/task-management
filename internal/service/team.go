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
	InviteMember(ctx context.Context, actorID, teamID string, req *domain.InviteMemberRequest) error
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

func (s *teamService) InviteMember(ctx context.Context, actorID, teamID string, req *domain.InviteMemberRequest) error {
	actorRole, err := memberRole(ctx, s.teams, teamID, actorID)
	if err != nil {
		return err
	}

	inviteRole := domain.RoleMember
	if err := CanInvite(actorRole, inviteRole); err != nil {
		return err
	}

	if req.UserID == actorID {
		return domain.ErrInvalidInput.WithMessage("you are already a member of this team")
	}
	if _, err := s.users.GetByID(ctx, req.UserID); err != nil {
		return err
	}

	// The (team_id, user_id) primary key makes this race-free: a
	// concurrent duplicate invite fails with ErrAlreadyExists.
	if err := s.teams.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: req.UserID, Role: inviteRole}); err != nil {
		return err
	}

	logger.FromContext(ctx).Info().
		Str("team_id", teamID).Str("actor_id", actorID).Str("invited_user_id", req.UserID).
		Msg("member invited")
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
