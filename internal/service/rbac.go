package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
)

type RBACService interface {
	// Team Permissions
	CanInviteMember(ctx context.Context, teamID, actorID string) error
	CanChangeMemberRole(ctx context.Context, teamID, actorID, targetUserID string, newRole domain.Role) error
	CanRemoveMember(ctx context.Context, teamID, actorID, targetUserID string) error
	CanViewAnalytics(ctx context.Context, teamID, actorID string) error

	// Task Permissions
	CanCreateTask(ctx context.Context, teamID, actorID string) error
	CanEditTask(ctx context.Context, teamID, actorID, taskCreatorID string) error
	CanChangeTaskStatus(ctx context.Context, teamID, actorID, taskCreatorID string, assigneeID *string) error
	CanReassignTask(ctx context.Context, teamID, actorID, taskCreatorID string) error
	CanCommentOnTask(ctx context.Context, teamID, actorID string) error
}

type rbacService struct {
	teamRepo repository.TeamRepository
}

func NewRBACService(teamRepo repository.TeamRepository) RBACService {
	return &rbacService{teamRepo: teamRepo}
}

// CanInviteMember - Only owner or admin can invite members
func (s *rbacService) CanInviteMember(ctx context.Context, teamID, actorID string) error {
	_, err := s.requireOwnerOrAdmin(ctx, teamID, actorID)
	return err
}

// CanChangeMemberRole - Owner can change admin/member roles; Admin can change member roles only
func (s *rbacService) CanChangeMemberRole(
	ctx context.Context,
	teamID, actorID, targetUserID string,
	newRole domain.Role,
) error {
	// Cannot assign owner role
	if newRole != domain.RoleAdmin && newRole != domain.RoleMember {
		return domain.ErrInvalidInput
	}

	// Cannot change your own role
	if actorID == targetUserID {
		return domain.ErrInvalidInput
	}

	// Only owner or admin can change roles
	actorRole, err := s.requireOwnerOrAdmin(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	// Get target user's role
	targetRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, targetUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("get target role: %w", err)
	}

	// Owner cannot be modified
	if targetRole == domain.RoleOwner {
		return domain.ErrForbidden
	}

	// Admin cannot modify another admin
	if actorRole == domain.RoleAdmin && targetRole == domain.RoleAdmin {
		return domain.ErrForbidden
	}

	// Already has this role
	if targetRole == newRole {
		return domain.ErrInvalidInput
	}

	return nil
}

// CanRemoveMember - Owner can remove admin/member; Admin can remove member only
func (s *rbacService) CanRemoveMember(ctx context.Context, teamID, actorID, targetUserID string) error {
	// Cannot remove yourself
	if actorID == targetUserID {
		return domain.ErrInvalidInput
	}

	// Only owner or admin can remove members
	actorRole, err := s.requireOwnerOrAdmin(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	// Get target user's role
	targetRole, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, targetUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("get target role: %w", err)
	}

	// Owner can never be removed
	if targetRole == domain.RoleOwner {
		return domain.ErrForbidden
	}

	// Admin cannot remove another admin
	if actorRole == domain.RoleAdmin && targetRole == domain.RoleAdmin {
		return domain.ErrForbidden
	}

	return nil
}

// CanViewAnalytics - Only owner or admin can view analytics
func (s *rbacService) CanViewAnalytics(ctx context.Context, teamID, actorID string) error {
	_, err := s.requireOwnerOrAdmin(ctx, teamID, actorID)
	return err
}

// CanCreateTask - Any team member can create tasks
func (s *rbacService) CanCreateTask(ctx context.Context, teamID, actorID string) error {
	isMember, err := s.teamRepo.IsTeamMember(ctx, teamID, actorID)
	if err != nil {
		return fmt.Errorf("check membership: %w", err)
	}

	if !isMember {
		return domain.ErrForbidden
	}

	return nil
}

// CanEditTask - Owner/Admin can edit any task; Creator can edit own task
func (s *rbacService) CanEditTask(ctx context.Context, teamID, actorID, taskCreatorID string) error {
	role, err := s.getRole(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	if isOwnerOrAdmin(role) || actorID == taskCreatorID {
		return nil
	}

	return domain.ErrForbidden
}

// CanChangeTaskStatus - Owner/Admin/Creator/Assignee can change status
func (s *rbacService) CanChangeTaskStatus(
	ctx context.Context,
	teamID, actorID, taskCreatorID string,
	assigneeID *string,
) error {
	role, err := s.getRole(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	if isOwnerOrAdmin(role) || actorID == taskCreatorID {
		return nil
	}

	if assigneeID != nil && actorID == *assigneeID {
		return nil
	}

	return domain.ErrForbidden
}

// CanReassignTask - Owner/Admin/Creator can reassign; Assignee cannot
func (s *rbacService) CanReassignTask(ctx context.Context, teamID, actorID, taskCreatorID string) error {
	role, err := s.getRole(ctx, teamID, actorID)
	if err != nil {
		return err
	}

	if isOwnerOrAdmin(role) || actorID == taskCreatorID {
		return nil
	}

	return domain.ErrForbidden
}

// CanCommentOnTask - Any team member can comment
func (s *rbacService) CanCommentOnTask(ctx context.Context, teamID, actorID string) error {
	_, err := s.getRole(ctx, teamID, actorID)
	return err
}

// Helpers

// getRole retrieves user's role. Returns ErrForbidden if not a team member.
func (s *rbacService) getRole(ctx context.Context, teamID, userID string) (domain.Role, error) {
	role, err := s.teamRepo.GetUserRoleInTeam(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrForbidden
		}
		return "", fmt.Errorf("get role: %w", err)
	}

	return role, nil
}

// requireOwnerOrAdmin checks if user is owner or admin.
// Returns ErrForbidden if not owner/admin or not a team member.
func (s *rbacService) requireOwnerOrAdmin(ctx context.Context, teamID, userID string) (domain.Role, error) {
	role, err := s.getRole(ctx, teamID, userID)
	if err != nil {
		return "", err
	}

	if !isOwnerOrAdmin(role) {
		return "", domain.ErrForbidden
	}

	return role, nil
}

func isOwnerOrAdmin(role domain.Role) bool {
	return role == domain.RoleOwner || role == domain.RoleAdmin
}
