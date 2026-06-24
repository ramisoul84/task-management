package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
)

type RBACService interface {
	CanInviteMember(ctx context.Context, teamID string, actorID string) error
}

type rbacService struct {
	teamRepo repository.TeamRepository
}

func NewRBACService(teamRepo repository.TeamRepository) RBACService {
	return &rbacService{teamRepo: teamRepo}
}

func (s *rbacService) CanInviteMember(ctx context.Context, teamID string, actorID string) error {
	role, err := s.getRole(ctx, teamID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden
		}
		return err
	}

	if !isOwnerOrAdmin(role) {
		return domain.ErrForbidden
	}

	return nil
}

func (s *rbacService) getRole(ctx context.Context, teamID string, actorID string) (domain.Role, error) {
	role, err := s.teamRepo.GetUserRoleInTeam(ctx, actorID, teamID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrForbidden
		}

		return "", fmt.Errorf("get actor role: %w", err)
	}

	return role, nil
}

func isOwnerOrAdmin(role domain.Role) bool {
	return role == domain.RoleOwner || role == domain.RoleAdmin
}
