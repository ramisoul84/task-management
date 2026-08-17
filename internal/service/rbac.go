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
	CanChangeMemberRole(ctx context.Context, teamID string, actorID string, targetUserID string, newRole domain.Role) error
	CanRemoveMember(ctx context.Context, teamID string, actorID string, targetUserID string) error
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
		return err
	}

	if !isOwnerOrAdmin(role) {
		return domain.ErrForbidden
	}

	return nil
}

func (s *rbacService) CanChangeMemberRole(
	ctx context.Context,
	teamID string,
	actorID string,
	targetUserID string,
	newRole domain.Role,
) error {
	if newRole != domain.RoleAdmin &&
		newRole != domain.RoleMember {
		return domain.ErrInvalidInput
	}

	actorRole, err := s.getRole(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	if !isOwnerOrAdmin(actorRole) {
		return domain.ErrForbidden
	}

	targetRole, err := s.teamRepo.GetUserRoleInTeam(
		ctx,
		targetUserID,
		teamID,
	)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}

		return fmt.Errorf("get target role: %w", err)
	}

	// Owner cannot be modified.
	if targetRole == domain.RoleOwner {
		return domain.ErrForbidden
	}

	// Admin cannot modify another admin (or itself through this path).
	if actorRole == domain.RoleAdmin &&
		targetRole == domain.RoleAdmin {
		return domain.ErrForbidden
	}

	return nil
}

func (s *rbacService) CanRemoveMember(
	ctx context.Context,
	teamID string,
	actorID string,
	targetUserID string,
) error {
	actorRole, err := s.getRole(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	if !isOwnerOrAdmin(actorRole) {
		return domain.ErrForbidden
	}

	targetRole, err := s.teamRepo.GetUserRoleInTeam(
		ctx,
		targetUserID,
		teamID,
	)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}

		return fmt.Errorf("get target role: %w", err)
	}

	// Owner can never be removed.
	if targetRole == domain.RoleOwner {
		return domain.ErrForbidden
	}

	// Admin cannot remove another admin.
	if actorRole == domain.RoleAdmin &&
		targetRole == domain.RoleAdmin {
		return domain.ErrForbidden
	}

	return nil
}

// Helper functions
func (s *rbacService) getRole(ctx context.Context, teamID string, actorID string) (domain.Role, error) {
	role, err := s.teamRepo.GetUserRoleInTeam(
		ctx,
		actorID,
		teamID,
	)
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
