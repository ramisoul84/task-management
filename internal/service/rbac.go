package service

import (
	"github.com/ramisoul84/task-management/internal/domain"
)

// This file is the single source of truth for authorisation rules. Every
// function is pure (role/task in, decision out) so the whole policy can be
// unit-tested exhaustively without fakes. Services resolve the caller's
// role once per request and then consult these functions.
//
// Roles:
//   owner  - full control of the team; cannot be removed or demoted.
//   admin  - invites members, manages members' roles, edits any task,
//            sees analytics; cannot touch the owner or other admins.
//   member - sees the team, its tasks, history and comments; creates tasks;
//            edits only tasks they created or are assigned to.

// CanInvite reports whether role may add people to the team, and with
// which role. Nobody can grant ownership through an invite; only owners
// can create other admins directly. (Admins can still promote a member
// later via role change, matching the original behaviour.)
func CanInvite(actor, invitee domain.Role) error {
	if !actor.IsManager() {
		return domain.ErrForbidden.WithMessage("only the team owner or an admin can invite members")
	}
	switch invitee {
	case domain.RoleMember, domain.RoleAdmin:
		return nil
	default:
		return domain.ErrInvalidInput.WithMessage("invited members can only be given the admin or member role")
	}
}

// CanChangeRole decides whether actor may set target's role to newRole.
func CanChangeRole(actor, target, newRole domain.Role, self bool) error {
	switch {
	case newRole != domain.RoleAdmin && newRole != domain.RoleMember:
		return domain.ErrInvalidInput.WithMessage("role must be admin or member")
	case self:
		return domain.ErrInvalidInput.WithMessage("you cannot change your own role")
	case !actor.IsManager():
		return domain.ErrForbidden.WithMessage("only the team owner or an admin can change roles")
	case target == domain.RoleOwner:
		return domain.ErrForbidden.WithMessage("the team owner's role cannot be changed")
	case actor == domain.RoleAdmin && target == domain.RoleAdmin:
		return domain.ErrForbidden.WithMessage("admins cannot change another admin's role")
	case target == newRole:
		return domain.ErrInvalidInput.WithMessage("member already has this role")
	}
	return nil
}

// CanRemoveMember decides whether actor may remove target from the team.
func CanRemoveMember(actor, target domain.Role, self bool) error {
	switch {
	case self:
		return domain.ErrInvalidInput.WithMessage("you cannot remove yourself")
	case !actor.IsManager():
		return domain.ErrForbidden.WithMessage("only the team owner or an admin can remove members")
	case target == domain.RoleOwner:
		return domain.ErrForbidden.WithMessage("the team owner cannot be removed")
	case actor == domain.RoleAdmin && target == domain.RoleAdmin:
		return domain.ErrForbidden.WithMessage("admins cannot remove another admin")
	}
	return nil
}

// CanViewAnalytics reports whether role may read team statistics.
func CanViewAnalytics(role domain.Role) error {
	if !role.IsManager() {
		return domain.ErrForbidden.WithMessage("only the team owner or an admin can view analytics")
	}
	return nil
}

// TaskPermissions is what a user may do to one particular task.
type TaskPermissions struct {
	EditContent  bool // title, description
	ChangeStatus bool
	Reassign     bool
	Delete       bool
}

// Any reports whether the user may change the task at all.
func (p TaskPermissions) Any() bool { return p.EditContent || p.ChangeStatus || p.Reassign }

// TaskPermissionsFor resolves what userID (with role in the task's team)
// may change on task:
//   - owner/admin: everything
//   - task creator: everything (new assignee must still be in the team)
//   - assignee: status only — they cannot reassign or delete the task
//   - anyone else: nothing
func TaskPermissionsFor(role domain.Role, userID string, task *domain.Task) TaskPermissions {
	switch {
	case role.IsManager(), task.CreatedBy == userID:
		return TaskPermissions{EditContent: true, ChangeStatus: true, Reassign: true, Delete: true}
	case task.IsAssignee(userID):
		return TaskPermissions{ChangeStatus: true}
	default:
		return TaskPermissions{}
	}
}

// CanEditComment: only the author may rewrite their words.
func CanEditComment(userID string, c *domain.TaskComment) bool {
	return c.UserID == userID
}

// CanDeleteComment: the author, or a team manager moderating.
func CanDeleteComment(role domain.Role, userID string, c *domain.TaskComment) bool {
	return c.UserID == userID || role.IsManager()
}

// CanRenameTeam: owner or admin.
func CanRenameTeam(role domain.Role) error {
	if !role.IsManager() {
		return domain.ErrForbidden.WithMessage("only the team owner or an admin can rename the team")
	}
	return nil
}

// CanDeleteTeam: owner only. Deleting a team destroys all of its tasks.
func CanDeleteTeam(role domain.Role) error {
	if role != domain.RoleOwner {
		return domain.ErrForbidden.WithMessage("only the team owner can delete the team")
	}
	return nil
}

// CanLeave: anyone but the owner, who must transfer ownership first so the
// team is never left without one.
func CanLeave(role domain.Role) error {
	if role == domain.RoleOwner {
		return domain.ErrForbidden.WithMessage("the owner cannot leave; transfer ownership first")
	}
	return nil
}

// CanTransferOwnership: only the owner, to another existing member.
func CanTransferOwnership(actor domain.Role, self bool) error {
	switch {
	case actor != domain.RoleOwner:
		return domain.ErrForbidden.WithMessage("only the team owner can transfer ownership")
	case self:
		return domain.ErrInvalidInput.WithMessage("you already own this team")
	}
	return nil
}
