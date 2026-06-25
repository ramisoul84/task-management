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

// CanInviteMember checks if actor can invite new members to team.
//
// Permission rules:
//   - Owner can invite: anyone (as member or admin)
//   - Admin can invite: anyone (as member only, not admin)
//   - Member cannot invite anyone
func (s *rbacService) CanInviteMember(ctx context.Context, teamID string, actorID string) error {
	const op = "rbac.CanInviteMember"

	actorRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%s: actor is not a team member: %w", op, domain.ErrForbidden)
		}
		return fmt.Errorf("%s: get actor role: %w", op, err)
	}

	if !isOwnerOrAdmin(actorRole) {
		return domain.ErrForbidden
	}

	return nil
}

// CanChangeMemberRole checks if actor can change target user's role.
//
// Permission rules:
//   - Owner can change: member → admin, admin → member (not owner)
//   - Admin can change: member → admin, admin → member (not admin, not owner)
//   - Member cannot change anyone's role
//   - Cannot assign owner role
func (s *rbacService) CanChangeMemberRole(
	ctx context.Context,
	teamID string,
	actorID string,
	targetUserID string,
	newRole domain.Role,
) error {
	const op = "rbac.CanChangeMemberRole"

	// Input validation
	if teamID == "" {
		return fmt.Errorf("%s: team ID is required: %w", op, domain.ErrInvalidInput)
	}
	if actorID == "" {
		return fmt.Errorf("%s: actor ID is required: %w", op, domain.ErrInvalidInput)
	}
	if targetUserID == "" {
		return fmt.Errorf("%s: target user ID is required: %w", op, domain.ErrInvalidInput)
	}

	// Validate new role
	if newRole != domain.RoleAdmin && newRole != domain.RoleMember {
		return fmt.Errorf("%s: invalid role '%s': %w", op, newRole, domain.ErrInvalidInput)
	}

	// Prevent self-role-change
	if actorID == targetUserID {
		return fmt.Errorf("%s: cannot change your own role: %w", op, domain.ErrInvalidInput)
	}

	// Get actor's role
	actorRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%s: actor is not a team member: %w", op, domain.ErrForbidden)
		}
		return fmt.Errorf("%s: get actor role: %w", op, err)
	}

	// Only owner or admin can change roles
	if !isOwnerOrAdmin(actorRole) {
		return domain.ErrForbidden
	}

	// Get target user's role
	targetRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, targetUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%s: target user is not a team member: %w", op, domain.ErrNotFound)
		}
		return fmt.Errorf("%s: get target role: %w", op, err)
	}

	// Owner cannot be modified
	if targetRole == domain.RoleOwner {
		return domain.ErrForbidden
	}

	// Admin cannot modify another admin
	if actorRole == domain.RoleAdmin && targetRole == domain.RoleAdmin {
		return domain.ErrForbidden
	}

	// Check if role is actually changing
	if targetRole == newRole {
		return fmt.Errorf("%s: user already has role '%s': %w", op, newRole, domain.ErrInvalidInput)
	}

	return nil
}

// CanRemoveMember checks if actor can remove target user from team.
//
// Permission rules:
//   - Owner can remove: admin, member (not owner, not self)
//   - Admin can remove: member (not admin, not owner, not self)
//   - Member cannot remove anyone
func (s *rbacService) CanRemoveMember(
	ctx context.Context,
	teamID string,
	actorID string,
	targetUserID string,
) error {
	const op = "rbac.CanRemoveMember"

	// Input validation
	if teamID == "" {
		return fmt.Errorf("%s: team ID is required: %w", op, domain.ErrInvalidInput)
	}
	if actorID == "" {
		return fmt.Errorf("%s: actor ID is required: %w", op, domain.ErrInvalidInput)
	}
	if targetUserID == "" {
		return fmt.Errorf("%s: target user ID is required: %w", op, domain.ErrInvalidInput)
	}

	// Prevent self-removal
	if actorID == targetUserID {
		return fmt.Errorf("%s: cannot remove yourself: %w", op, domain.ErrInvalidInput)
	}

	// Get actor's role (CORRECT: teamID, actorID)
	actorRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%s: actor is not a team member: %w", op, domain.ErrForbidden)
		}
		return fmt.Errorf("%s: get actor role: %w", op, err)
	}

	// Only owner or admin can remove members
	if !isOwnerOrAdmin(actorRole) {
		return domain.ErrForbidden
	}

	// Get target user's role (CORRECT: teamID, targetUserID)
	targetRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, targetUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("%s: get target role: %w", op, err)
	}

	// Owner can never be removed
	if targetRole == domain.RoleOwner {
		return domain.ErrForbidden
	}

	// Admin cannot remove another admin (only owner can)
	if actorRole == domain.RoleAdmin && targetRole == domain.RoleAdmin {
		return domain.ErrForbidden
	}

	return nil
}

func isOwnerOrAdmin(role domain.Role) bool {
	return role == domain.RoleOwner || role == domain.RoleAdmin
}
