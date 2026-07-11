package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
)

func TestRBAC_CanInviteMember(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name  string
		roles map[string]domain.Role
		actor string
		want  error
	}{
		{"owner_can_invite", map[string]domain.Role{"alice": domain.RoleOwner}, "alice", nil},
		{"admin_can_invite", map[string]domain.Role{"alice": domain.RoleAdmin}, "alice", nil},
		{"member_cannot_invite", map[string]domain.Role{"alice": domain.RoleMember}, "alice", domain.ErrForbidden},
		{"non_member_cannot_invite", map[string]domain.Role{}, "ghost", domain.ErrForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rbac := service.NewRBACService(&fakeTeamRepo{roles: tc.roles})
			got := rbac.CanInviteMember(ctx, "team-1", tc.actor)
			if !errors.Is(got, tc.want) {
				t.Fatalf("CanInviteMember() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRBAC_CanChangeMemberRole(t *testing.T) {
	ctx := context.Background()

	const (
		teamID = "team-1"
		owner  = "owner"
		admin  = "admin"
		admin2 = "admin2"
		member = "member"
		ghost  = "ghost"
	)

	cases := []struct {
		name    string
		roles   map[string]domain.Role
		actor   string
		target  string
		newRole domain.Role
		want    error
	}{
		{"owner_promotes_member", map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember}, owner, member, domain.RoleAdmin, nil},
		{"owner_demotes_admin", map[string]domain.Role{owner: domain.RoleOwner, admin: domain.RoleAdmin}, owner, admin, domain.RoleMember, nil},
		{"admin_promotes_member", map[string]domain.Role{admin: domain.RoleAdmin, member: domain.RoleMember}, admin, member, domain.RoleAdmin, nil},
		{"member_cannot_change", map[string]domain.Role{member: domain.RoleMember, admin: domain.RoleAdmin}, member, admin, domain.RoleMember, domain.ErrForbidden},
		{"non_member_cannot_change", map[string]domain.Role{member: domain.RoleMember}, ghost, member, domain.RoleAdmin, domain.ErrForbidden},
		{"cannot_assign_owner", map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember}, owner, member, domain.RoleOwner, domain.ErrInvalidInput},
		{"invalid_role", map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember}, owner, member, domain.Role("superadmin"), domain.ErrInvalidInput},
		{"cannot_change_own_role", map[string]domain.Role{owner: domain.RoleOwner}, owner, owner, domain.RoleAdmin, domain.ErrInvalidInput},
		{"owner_target_protected", map[string]domain.Role{owner: domain.RoleOwner, admin: domain.RoleAdmin}, admin, owner, domain.RoleMember, domain.ErrForbidden},
		{"admin_cannot_modify_admin", map[string]domain.Role{admin: domain.RoleAdmin, admin2: domain.RoleAdmin}, admin, admin2, domain.RoleMember, domain.ErrForbidden},
		{"already_has_role", map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember}, owner, member, domain.RoleMember, domain.ErrInvalidInput},
		{"target_not_found", map[string]domain.Role{owner: domain.RoleOwner}, owner, ghost, domain.RoleAdmin, domain.ErrNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rbac := service.NewRBACService(&fakeTeamRepo{roles: tc.roles})
			got := rbac.CanChangeMemberRole(ctx, teamID, tc.actor, tc.target, tc.newRole)
			if !errors.Is(got, tc.want) {
				t.Fatalf("CanChangeMemberRole() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRBAC_CanRemoveMember(t *testing.T) {
	ctx := context.Background()

	const (
		teamID  = "team-1"
		owner   = "owner"
		admin   = "admin"
		admin2  = "admin2"
		member  = "member"
		member2 = "member2"
		ghost   = "ghost"
	)

	cases := []struct {
		name   string
		roles  map[string]domain.Role
		actor  string
		target string
		want   error
	}{
		{"owner_removes_member", map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember}, owner, member, nil},
		{"owner_removes_admin", map[string]domain.Role{owner: domain.RoleOwner, admin: domain.RoleAdmin}, owner, admin, nil},
		{"admin_removes_member", map[string]domain.Role{admin: domain.RoleAdmin, member: domain.RoleMember}, admin, member, nil},
		{"member_cannot_remove", map[string]domain.Role{member: domain.RoleMember, member2: domain.RoleMember}, member, member2, domain.ErrForbidden},
		{"non_member_cannot_remove", map[string]domain.Role{member: domain.RoleMember}, ghost, member, domain.ErrForbidden},
		{"cannot_remove_self", map[string]domain.Role{owner: domain.RoleOwner}, owner, owner, domain.ErrInvalidInput},
		{"owner_cannot_be_removed", map[string]domain.Role{owner: domain.RoleOwner, admin: domain.RoleAdmin}, admin, owner, domain.ErrForbidden},
		{"admin_cannot_remove_admin", map[string]domain.Role{admin: domain.RoleAdmin, admin2: domain.RoleAdmin}, admin, admin2, domain.ErrForbidden},
		{"target_not_found", map[string]domain.Role{owner: domain.RoleOwner}, owner, ghost, domain.ErrNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rbac := service.NewRBACService(&fakeTeamRepo{roles: tc.roles})
			got := rbac.CanRemoveMember(ctx, teamID, tc.actor, tc.target)
			if !errors.Is(got, tc.want) {
				t.Fatalf("CanRemoveMember() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRBAC_TaskPermissions(t *testing.T) {
	ctx := context.Background()

	const (
		teamID   = "team-1"
		owner    = "owner"
		admin    = "admin"
		creator  = "creator"
		assignee = "assignee"
		other    = "other"
		ghost    = "ghost"
	)

	baseRoles := map[string]domain.Role{
		owner:    domain.RoleOwner,
		admin:    domain.RoleAdmin,
		creator:  domain.RoleMember,
		assignee: domain.RoleMember,
		other:    domain.RoleMember,
	}

	assigneeID := assignee

	t.Run("CanCreateTask", func(t *testing.T) {
		cases := []struct {
			name  string
			actor string
			want  error
		}{
			{"owner", owner, nil},
			{"admin", admin, nil},
			{"member", creator, nil},
			{"non_member", ghost, domain.ErrForbidden},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rbac := service.NewRBACService(&fakeTeamRepo{roles: baseRoles})
				got := rbac.CanCreateTask(ctx, teamID, tc.actor)
				if !errors.Is(got, tc.want) {
					t.Fatalf("CanCreateTask() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("CanEditTask", func(t *testing.T) {
		cases := []struct {
			name  string
			actor string
			want  error
		}{
			{"owner", owner, nil},
			{"admin", admin, nil},
			{"creator", creator, nil},
			{"assignee", assignee, domain.ErrForbidden},
			{"other", other, domain.ErrForbidden},
			{"non_member", ghost, domain.ErrForbidden},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rbac := service.NewRBACService(&fakeTeamRepo{roles: baseRoles})
				got := rbac.CanEditTask(ctx, teamID, tc.actor, creator)
				if !errors.Is(got, tc.want) {
					t.Fatalf("CanEditTask() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("CanChangeTaskStatus", func(t *testing.T) {
		cases := []struct {
			name  string
			actor string
			want  error
		}{
			{"owner", owner, nil},
			{"admin", admin, nil},
			{"creator", creator, nil},
			{"assignee", assignee, nil},
			{"other", other, domain.ErrForbidden},
			{"non_member", ghost, domain.ErrForbidden},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rbac := service.NewRBACService(&fakeTeamRepo{roles: baseRoles})
				got := rbac.CanChangeTaskStatus(ctx, teamID, tc.actor, creator, &assigneeID)
				if !errors.Is(got, tc.want) {
					t.Fatalf("CanChangeTaskStatus() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("CanReassignTask", func(t *testing.T) {
		cases := []struct {
			name  string
			actor string
			want  error
		}{
			{"owner", owner, nil},
			{"admin", admin, nil},
			{"creator", creator, nil},
			{"assignee", assignee, domain.ErrForbidden},
			{"other", other, domain.ErrForbidden},
			{"non_member", ghost, domain.ErrForbidden},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rbac := service.NewRBACService(&fakeTeamRepo{roles: baseRoles})
				got := rbac.CanReassignTask(ctx, teamID, tc.actor, creator)
				if !errors.Is(got, tc.want) {
					t.Fatalf("CanReassignTask() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("CanCommentOnTask", func(t *testing.T) {
		cases := []struct {
			name  string
			actor string
			want  error
		}{
			{"owner", owner, nil},
			{"admin", admin, nil},
			{"member", creator, nil},
			{"non_member", ghost, domain.ErrForbidden},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rbac := service.NewRBACService(&fakeTeamRepo{roles: baseRoles})
				got := rbac.CanCommentOnTask(ctx, teamID, tc.actor)
				if !errors.Is(got, tc.want) {
					t.Fatalf("CanCommentOnTask() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("CanViewAnalytics", func(t *testing.T) {
		cases := []struct {
			name  string
			actor string
			want  error
		}{
			{"owner", owner, nil},
			{"admin", admin, nil},
			{"member", creator, domain.ErrForbidden},
			{"non_member", ghost, domain.ErrForbidden},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rbac := service.NewRBACService(&fakeTeamRepo{roles: baseRoles})
				got := rbac.CanViewAnalytics(ctx, teamID, tc.actor)
				if !errors.Is(got, tc.want) {
					t.Fatalf("CanViewAnalytics() = %v, want %v", got, tc.want)
				}
			})
		}
	})
}
