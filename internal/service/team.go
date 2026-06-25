package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type TeamService interface {
	CreateTeam(ctx context.Context, userID string, name string) (*domain.Team, error)
	ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error)
	InviteMember(ctx context.Context, actorID string, teamID string, userID string) error
	ListMembers(ctx context.Context, userID string, teamID string) ([]*domain.TeamMemberResponse, error)
	ChangeMemberRole(ctx context.Context, actorID string, teamID string, userID string, role domain.Role) error
	RemoveMember(ctx context.Context, actorID string, teamID string, userID string) error
}

type teamService struct {
	teamRepo repository.TeamRepository
	userRepo repository.UserRepository
	rbacSvc  RBACService
	log      *logger.Logger
}

func NewTeamService(
	teamRepo repository.TeamRepository,
	userRepo repository.UserRepository,
	rbacSvc RBACService,
	log *logger.Logger,
) TeamService {
	return &teamService{
		teamRepo: teamRepo,
		userRepo: userRepo,
		rbacSvc:  rbacSvc,
		log:      log,
	}
}

func (s *teamService) CreateTeam(ctx context.Context, ownerID, teamName string) (*domain.Team, error) {
	const op = "teamService.CreateTeam"

	teamName = strings.ToUpper(strings.TrimSpace(teamName))

	if ownerID == "" || teamName == "" {
		return nil, domain.ErrInvalidInput
	}

	team := &domain.Team{
		ID:        uuid.NewString(),
		Name:      teamName,
		CreatedBy: ownerID,
		CreatedAt: time.Now(),
	}

	if err := s.teamRepo.CreateWithOwner(ctx, team); err != nil {
		s.log.Error().
			Err(err).
			Str("team_id", team.ID).
			Str("user_id", ownerID).
			Msg("create team")

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	s.log.Info().
		Str("team_id", team.ID).
		Str("user_id", ownerID).
		Msg("team created")

	return team, nil
}

func (s *teamService) ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error) {
	const op = "teamService.ListTeamsByUser"

	if userID == "" {
		return nil, domain.ErrInvalidInput
	}

	teams, err := s.teamRepo.ListTeamsByUser(ctx, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("user_id", userID).
			Msg("list teams")

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return teams, nil
}

func (s *teamService) InviteMember(
	ctx context.Context,
	actorID string,
	teamID string,
	userID string,
) error {
	const op = "team.InviteMember"

	if actorID == "" || teamID == "" || userID == "" {
		return fmt.Errorf("%s: actor, team and user IDs are required: %w",
			op, domain.ErrInvalidInput)
	}

	if actorID == userID {
		return fmt.Errorf("%s: cannot invite yourself: %w", op, domain.ErrInvalidInput)
	}

	// Check permission via RBAC
	if err := s.rbacSvc.CanInviteMember(ctx, teamID, actorID); err != nil {
		s.log.Warn().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Msg("RBAC permission check failed")
		return err
	}

	// Check if user exists
	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("user_id", userID).
			Msg("get user")
		return fmt.Errorf("%s: get user: %w", op, err)
	}

	// Check if already a member
	isMember, err := s.teamRepo.IsTeamMember(ctx, teamID, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("check membership")
		return fmt.Errorf("%s: check membership: %w", op, err)
	}

	if isMember {
		return domain.ErrAlreadyExists
	}

	// Add member
	member := &domain.TeamMember{
		TeamID: teamID,
		UserID: userID,
		Role:   domain.RoleMember,
	}

	if err := s.teamRepo.AddMember(ctx, member); err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("add member")
		return fmt.Errorf("%s: add member: %w", op, err)
	}

	s.log.Info().
		Str("op", op).
		Str("team_id", teamID).
		Str("actor_id", actorID).
		Str("new_member_id", userID).
		Msg("member invited")

	return nil
}

func (s *teamService) ListMembers(ctx context.Context, userID string, teamID string) ([]*domain.TeamMemberResponse, error) {
	const op = "team.ListMembers"

	if userID == "" || teamID == "" {
		return nil, fmt.Errorf("%s: user ID and team ID are required: %w", op, domain.ErrInvalidInput)
	}

	isMember, err := s.teamRepo.IsTeamMember(ctx, teamID, userID)
	if err != nil {
		s.log.Error().Err(err).Str("op", op).Str("team_id", teamID).Str("user_id", userID).Msg("check membership")
		return nil, fmt.Errorf("%s: check membership: %w", op, err)
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	members, err := s.teamRepo.ListMembers(ctx, teamID)
	if err != nil {
		s.log.Error().Err(err).Str("op", op).Str("team_id", teamID).Msg("list members")
		return nil, fmt.Errorf("%s: list members: %w", op, err)
	}

	return members, nil
}

func (s *teamService) ChangeMemberRole(
	ctx context.Context,
	actorID string,
	teamID string,
	targetUserID string,
	role domain.Role,
) error {
	const op = "team.ChangeMemberRole"

	if actorID == "" || teamID == "" || targetUserID == "" {
		return fmt.Errorf("%s: actor, team and target user IDs are required: %w",
			op, domain.ErrInvalidInput)
	}

	if err := s.rbacSvc.CanChangeMemberRole(ctx, teamID, actorID, targetUserID, role); err != nil {
		s.log.Warn().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Str("new_role", string(role)).
			Msg("RBAC permission check failed")
		return err
	}

	// Update member role
	if err := s.teamRepo.UpdateMemberRole(ctx, teamID, actorID, targetUserID, role); err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Str("new_role", string(role)).
			Msg("update member role")
		return fmt.Errorf("%s: update member role: %w", op, err)
	}

	s.log.Info().
		Str("op", op).
		Str("team_id", teamID).
		Str("actor_id", actorID).
		Str("target_user_id", targetUserID).
		Str("new_role", string(role)).
		Msg("member role updated")

	return nil
}

func (s *teamService) RemoveMember(
	ctx context.Context,
	actorID string,
	teamID string,
	targetUserID string,
) error {
	const op = "team.RemoveMember"

	// Input validation
	if actorID == "" || teamID == "" || targetUserID == "" {
		return fmt.Errorf("%s: actor, team and target user IDs are required: %w",
			op, domain.ErrInvalidInput)
	}

	// Check permission via RBAC
	if err := s.rbacSvc.CanRemoveMember(ctx, teamID, actorID, targetUserID); err != nil {
		// Log RBAC failure
		s.log.Warn().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Msg("RBAC permission check failed")
		return err
	}

	// Remove member from team
	if err := s.teamRepo.RemoveMember(ctx, teamID, actorID, targetUserID); err != nil {
		s.log.Error().
			Err(err).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Msg("remove member")
		return fmt.Errorf("%s: remove member: %w", op, err)
	}

	// Success log
	s.log.Info().
		Str("op", op).
		Str("team_id", teamID).
		Str("actor_id", actorID).
		Str("removed_user_id", targetUserID).
		Msg("member removed")

	return nil
}
