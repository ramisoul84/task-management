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
	CreateTeam(ctx context.Context, userID, name string) (*domain.Team, error)
	ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error)
	InviteMember(ctx context.Context, actorID string, teamID string, userID string) error
	ListMembers(ctx context.Context, userID string, teamID string) ([]*domain.TeamMemberResponse, error)
	ChangeMemberRole(ctx context.Context, teamID string, actorID string, userID string, role domain.Role) error
	RemoveMember(ctx context.Context, actorID string, teamID string, userID string) error
	GetTeamStats(ctx context.Context, userID, teamID string) (*domain.TeamStats, error)
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
	requestID := logger.RequestIDFromContext(ctx)

	teamName = strings.ToUpper(strings.TrimSpace(teamName))

	if ownerID == "" || teamName == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("owner_id", ownerID).
			Str("team_name", teamName).
			Msg("create team failed: invalid input")
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
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", team.ID).
			Str("owner_id", ownerID).
			Str("team_name", teamName).
			Msg("create team failed")
		return nil, fmt.Errorf("%s: create team: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("team_id", team.ID).
		Str("owner_id", ownerID).
		Str("team_name", teamName).
		Msg("team created successfully")

	return team, nil
}

func (s *teamService) ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error) {
	const op = "teamService.ListTeamsByUser"
	requestID := logger.RequestIDFromContext(ctx)

	if userID == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Msg("list teams failed: empty user ID")
		return nil, domain.ErrInvalidInput
	}

	teams, err := s.teamRepo.ListTeamsByUser(ctx, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", userID).
			Msg("list teams failed")
		return nil, fmt.Errorf("%s: list teams: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("user_id", userID).
		Int("teams_count", len(teams)).
		Msg("teams listed successfully")

	return teams, nil
}

func (s *teamService) InviteMember(
	ctx context.Context,
	actorID string,
	teamID string,
	userID string,
) error {
	const op = "teamService.InviteMember"
	requestID := logger.RequestIDFromContext(ctx)

	if actorID == "" || teamID == "" || userID == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("actor_id", actorID).
			Str("team_id", teamID).
			Str("invited_user_id", userID).
			Msg("invite failed: invalid input")
		return fmt.Errorf("%s: actor, team and user IDs are required: %w", op, domain.ErrInvalidInput)
	}

	if actorID == userID {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("actor_id", actorID).
			Msg("invite failed: cannot invite yourself")
		return fmt.Errorf("%s: cannot invite yourself: %w", op, domain.ErrInvalidInput)
	}

	// Check permission via RBAC
	if err := s.rbacSvc.CanInviteMember(ctx, teamID, actorID); err != nil {
		s.log.Warn().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Msg("invite failed: permission denied")
		return err
	}

	// Check if user exists
	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Warn().
				Str("request_id", requestID).
				Str("op", op).
				Str("team_id", teamID).
				Str("invited_user_id", userID).
				Msg("invite failed: user not found")
			return domain.ErrNotFound
		}
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("invited_user_id", userID).
			Msg("get user failed")
		return fmt.Errorf("%s: get user: %w", op, err)
	}

	// Check if already a member
	isMember, err := s.teamRepo.IsTeamMember(ctx, teamID, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("invited_user_id", userID).
			Msg("check membership failed")
		return fmt.Errorf("%s: check membership: %w", op, err)
	}

	if isMember {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("invited_user_id", userID).
			Msg("invite failed: user already a member")
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
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("invited_user_id", userID).
			Msg("add member failed")
		return fmt.Errorf("%s: add member: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("team_id", teamID).
		Str("actor_id", actorID).
		Str("invited_user_id", userID).
		Msg("member invited successfully")

	return nil
}

func (s *teamService) ListMembers(ctx context.Context, userID string, teamID string) ([]*domain.TeamMemberResponse, error) {
	const op = "teamService.ListMembers"
	requestID := logger.RequestIDFromContext(ctx)

	if userID == "" || teamID == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", userID).
			Str("team_id", teamID).
			Msg("list members failed: invalid input")
		return nil, fmt.Errorf("%s: user ID and team ID are required: %w", op, domain.ErrInvalidInput)
	}

	isMember, err := s.teamRepo.IsTeamMember(ctx, teamID, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("check membership failed")
		return nil, fmt.Errorf("%s: check membership: %w", op, err)
	}

	if !isMember {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("list members failed: not team member")
		return nil, domain.ErrForbidden
	}

	members, err := s.teamRepo.ListMembers(ctx, teamID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Msg("list members failed")
		return nil, fmt.Errorf("%s: list members: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("team_id", teamID).
		Str("user_id", userID).
		Int("members_count", len(members)).
		Msg("members listed successfully")

	return members, nil
}

func (s *teamService) ChangeMemberRole(
	ctx context.Context,
	actorID string,
	teamID string,
	targetUserID string,
	role domain.Role,
) error {
	const op = "teamService.ChangeMemberRole"
	requestID := logger.RequestIDFromContext(ctx)

	if actorID == "" || teamID == "" || targetUserID == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("actor_id", actorID).
			Str("team_id", teamID).
			Str("target_user_id", targetUserID).
			Msg("change role failed: invalid input")
		return fmt.Errorf("%s: actor, team and target user IDs are required: %w", op, domain.ErrInvalidInput)
	}

	if err := s.rbacSvc.CanChangeMemberRole(ctx, teamID, actorID, targetUserID, role); err != nil {
		s.log.Warn().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Str("new_role", string(role)).
			Msg("change role failed: permission denied")
		return err
	}

	// Update member role
	if err := s.teamRepo.UpdateMemberRole(ctx, teamID, actorID, targetUserID, role); err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Str("new_role", string(role)).
			Msg("change role failed: update member role")
		return fmt.Errorf("%s: update member role: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("team_id", teamID).
		Str("actor_id", actorID).
		Str("target_user_id", targetUserID).
		Str("new_role", string(role)).
		Msg("member role updated successfully")

	return nil
}

func (s *teamService) RemoveMember(
	ctx context.Context,
	actorID string,
	teamID string,
	targetUserID string,
) error {
	const op = "teamService.RemoveMember"
	requestID := logger.RequestIDFromContext(ctx)

	// Input validation
	if actorID == "" || teamID == "" || targetUserID == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("actor_id", actorID).
			Str("team_id", teamID).
			Str("target_user_id", targetUserID).
			Msg("remove member failed: invalid input")
		return fmt.Errorf("%s: actor, team and target user IDs are required: %w", op, domain.ErrInvalidInput)
	}

	// Check permission via RBAC
	if err := s.rbacSvc.CanRemoveMember(ctx, teamID, actorID, targetUserID); err != nil {
		s.log.Warn().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Msg("remove member failed: permission denied")
		return err
	}

	// Remove member from team
	if err := s.teamRepo.RemoveMember(ctx, teamID, actorID, targetUserID); err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("actor_id", actorID).
			Str("target_user_id", targetUserID).
			Msg("remove member failed")
		return fmt.Errorf("%s: remove member: %w", op, err)
	}

	// Success log
	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("team_id", teamID).
		Str("actor_id", actorID).
		Str("removed_user_id", targetUserID).
		Msg("member removed successfully")

	return nil
}

// GetTeamStats retrieves team analytics (owner/admin only)
func (s *teamService) GetTeamStats(ctx context.Context, userID string, teamID string) (*domain.TeamStats, error) {
	const op = "teamService.GetTeamStats"
	requestID := logger.RequestIDFromContext(ctx)

	// Input validation
	if userID == "" || teamID == "" {
		s.log.Warn().
			Str("request_id", requestID).
			Str("op", op).
			Str("user_id", userID).
			Str("team_id", teamID).
			Msg("get team stats failed: invalid input")
		return nil, fmt.Errorf("%s: user ID and team ID are required: %w", op, domain.ErrInvalidInput)
	}

	// Check permission (owner/admin only)
	if err := s.rbacSvc.CanViewAnalytics(ctx, teamID, userID); err != nil {
		s.log.Warn().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("get team stats failed: permission denied")
		return nil, err
	}

	// Get stats from repository
	stats, err := s.teamRepo.GetTeamStats(ctx, teamID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("op", op).
			Str("team_id", teamID).
			Msg("get team stats failed")
		return nil, fmt.Errorf("%s: get team stats: %w", op, err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("op", op).
		Str("team_id", teamID).
		Str("user_id", userID).
		Int("total_tasks", stats.TotalTasks).
		Int("total_comments", stats.TotalComments).
		Msg("team stats retrieved successfully")

	return stats, nil
}
