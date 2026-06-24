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
		return domain.ErrInvalidInput
	}

	if actorID == userID {
		return fmt.Errorf("%s: cannot invite yourself: %w",
			op, domain.ErrInvalidInput)
	}

	if err := s.rbacSvc.CanInviteMember(ctx, teamID, actorID); err != nil {
		return err
	}

	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}

		s.log.Error().
			Err(err).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("invite: get user")
		return fmt.Errorf("%s: get user: %w", op, err)
	}

	isMember, err := s.teamRepo.IsTeamMember(ctx, teamID, userID)
	if err != nil {
		s.log.Error().
			Err(err).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("invite: check membership")
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
		s.log.Error().
			Err(err).
			Str("team_id", teamID).
			Str("user_id", userID).
			Msg("invite: add member")
		return fmt.Errorf("%s: add member: %w", op, err)
	}

	return nil
}
