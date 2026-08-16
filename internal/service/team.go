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
	ListTeams(ctx context.Context, userID string) ([]*domain.Team, error)
	InviteMember(ctx context.Context, actorID string, teamID string, userID string) error
	ChangeMemberRole(ctx context.Context, actorID string, teamID string, targetUserID string, role domain.Role) error
	RemoveMember(ctx context.Context, actorID string, teamID string, targetUserID string) error
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
	const op = "team.CreateTeam"

	teamName = strings.ToUpper(strings.TrimSpace(teamName))

	if ownerID == "" {
		return nil, fmt.Errorf(
			"%s: user ID is required: %w",
			op,
			domain.ErrInvalidInput,
		)
	}

	if teamName == "" {
		return nil, fmt.Errorf(
			"%s: team name is required: %w",
			op,
			domain.ErrInvalidInput,
		)
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

func (s *teamService) ListTeams(ctx context.Context, userID string) ([]*domain.Team, error) {
	const op = "team.ListTeams"

	if userID == "" {
		return nil, fmt.Errorf(
			"%s: user ID is required: %w",
			op,
			domain.ErrInvalidInput,
		)
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

	if actorID == "" {
		return fmt.Errorf("%s: actor ID is required: %w",
			op, domain.ErrInvalidInput)
	}

	if teamID == "" {
		return fmt.Errorf("%s: team ID is required: %w",
			op, domain.ErrInvalidInput)
	}

	if userID == "" {
		return fmt.Errorf("%s: user ID is required: %w",
			op, domain.ErrInvalidInput)
	}

	if actorID == userID {
		return fmt.Errorf("%s: cannot invite yourself: %w",
			op, domain.ErrInvalidInput)
	}

	if err := s.rbacSvc.CanInviteMember(
		ctx,
		teamID,
		actorID,
	); err != nil {
		return err
	}

	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}

		return fmt.Errorf("%s: get user: %w", op, err)
	}

	isMember, err := s.teamRepo.IsTeamMember(
		ctx,
		teamID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("%s: check membership: %w", op, err)
	}

	if isMember {
		return domain.ErrAlreadyExists
	}

	member := &domain.TeamMember{
		TeamID: teamID,
		UserID: userID,
		Role:   domain.RoleMember,
	}

	if err := s.teamRepo.AddMember(ctx, member); err != nil {
		return fmt.Errorf("%s: add member: %w", op, err)
	}

	return nil
}

func (s *teamService) ChangeMemberRole(ctx context.Context, actorID string, teamID string, targetUserID string, role domain.Role) error {
	if err := s.rbacSvc.CanChangeMemberRole(
		ctx,
		teamID,
		actorID,
		targetUserID,
		role,
	); err != nil {
		return err
	}

	if err := s.teamRepo.UpdateMemberRole(
		ctx,
		teamID,
		targetUserID,
		role,
	); err != nil {
		return fmt.Errorf("update member role: %w", err)
	}

	return nil
}

func (s *teamService) RemoveMember(
	ctx context.Context,
	actorID string,
	teamID string,
	targetUserID string,
) error {
	if err := s.rbacSvc.CanRemoveMember(
		ctx,
		teamID,
		actorID,
		targetUserID,
	); err != nil {
		return err
	}

	if err := s.teamRepo.RemoveMember(
		ctx,
		teamID,
		targetUserID,
	); err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}

	return nil
}
