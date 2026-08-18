package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
)

var errRepoFailure = errors.New("repo failure")

func TestRBAC_CanInviteMember(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name  string
		roles map[string]domain.Role
		actor string
		want  error
	}{
		{
			name:  "owner_can_invite",
			roles: map[string]domain.Role{"alice": domain.RoleOwner},
			actor: "alice",
			want:  nil,
		},
		{
			name:  "admin_can_invite",
			roles: map[string]domain.Role{"alice": domain.RoleAdmin},
			actor: "alice",
			want:  nil,
		},
		{
			name:  "member_cannot_invite",
			roles: map[string]domain.Role{"alice": domain.RoleMember},
			actor: "alice",
			want:  domain.ErrForbidden,
		},
		{
			name:  "non_member_cannot_invite",
			roles: map[string]domain.Role{},
			actor: "ghost",
			want:  domain.ErrForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rbac := service.NewRBACService(&fakeTeamRepo{roles: tc.roles})

			got := rbac.CanInviteMember(ctx, "team-1", tc.actor)

			if !errors.Is(got, tc.want) {
				t.Fatalf("CanInviteMember() error = %v, want %v", got, tc.want)
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
		{
			name:    "owner_promotes_member_to_admin",
			roles:   map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:   owner,
			target:  member,
			newRole: domain.RoleAdmin,
			want:    nil,
		},
		{
			name:    "admin_promotes_member_to_admin",
			roles:   map[string]domain.Role{admin: domain.RoleAdmin, member: domain.RoleMember},
			actor:   admin,
			target:  member,
			newRole: domain.RoleAdmin,
			want:    nil,
		},
		{
			name:    "owner_demotes_admin_to_member",
			roles:   map[string]domain.Role{owner: domain.RoleOwner, admin: domain.RoleAdmin},
			actor:   owner,
			target:  admin,
			newRole: domain.RoleMember,
			want:    nil,
		},
		{
			name:    "owner_keeps_member_as_member",
			roles:   map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:   owner,
			target:  member,
			newRole: domain.RoleMember,
			want:    nil,
		},
		{
			name:    "member_cannot_change_roles",
			roles:   map[string]domain.Role{member: domain.RoleMember, owner: domain.RoleOwner},
			actor:   member,
			target:  owner,
			newRole: domain.RoleAdmin,
			want:    domain.ErrForbidden,
		},
		{
			name:    "non_member_cannot_change_roles",
			roles:   map[string]domain.Role{member: domain.RoleMember},
			actor:   ghost,
			target:  member,
			newRole: domain.RoleAdmin,
			want:    domain.ErrForbidden,
		},
		{
			name:    "granting_owner_is_invalid_input",
			roles:   map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:   owner,
			target:  member,
			newRole: domain.RoleOwner,
			want:    domain.ErrInvalidInput,
		},
		{
			name:    "unknown_role_is_invalid_input",
			roles:   map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:   owner,
			target:  member,
			newRole: domain.Role("superadmin"),
			want:    domain.ErrInvalidInput,
		},
		{
			name:    "owner_target_is_protected",
			roles:   map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:   owner,
			target:  owner,
			newRole: domain.RoleAdmin,
			want:    domain.ErrForbidden,
		},
		{
			name:    "admin_cannot_modify_another_admin",
			roles:   map[string]domain.Role{admin: domain.RoleAdmin},
			actor:   admin,
			target:  admin,
			newRole: domain.RoleMember,
			want:    domain.ErrForbidden,
		},
		{
			name:    "target_not_in_team_is_not_found",
			roles:   map[string]domain.Role{owner: domain.RoleOwner},
			actor:   owner,
			target:  ghost,
			newRole: domain.RoleAdmin,
			want:    domain.ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rbac := service.NewRBACService(&fakeTeamRepo{roles: tc.roles})

			got := rbac.CanChangeMemberRole(ctx, teamID, tc.actor, tc.target, tc.newRole)

			if !errors.Is(got, tc.want) {
				t.Fatalf("CanChangeMemberRole() error = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRBAC_CanChangeMemberRole_RepoError_IsWrapped(t *testing.T) {
	ctx := context.Background()

	rbac := service.NewRBACService(&fakeTeamRepo{
		roles: map[string]domain.Role{},
		err:   errRepoFailure,
	})

	got := rbac.CanChangeMemberRole(ctx, "team-1", "owner", "ghost", domain.RoleAdmin)

	if !errors.Is(got, errRepoFailure) {
		t.Fatalf("CanChangeMemberRole() error = %v, want it to wrap %v", got, errRepoFailure)
	}
}

func TestRBAC_CanRemoveMember(t *testing.T) {
	ctx := context.Background()

	const (
		teamID = "team-1"
		owner  = "owner"
		admin  = "admin"
		member = "member"
		ghost  = "ghost"
	)

	cases := []struct {
		name   string
		roles  map[string]domain.Role
		actor  string
		target string
		want   error
	}{
		{
			name:   "owner_removes_member",
			roles:  map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:  owner,
			target: member,
			want:   nil,
		},
		{
			name:   "admin_removes_member",
			roles:  map[string]domain.Role{admin: domain.RoleAdmin, member: domain.RoleMember},
			actor:  admin,
			target: member,
			want:   nil,
		},
		{
			name:   "owner_removes_admin",
			roles:  map[string]domain.Role{owner: domain.RoleOwner, admin: domain.RoleAdmin},
			actor:  owner,
			target: admin,
			want:   nil,
		},
		{
			name:   "member_cannot_remove_member",
			roles:  map[string]domain.Role{member: domain.RoleMember},
			actor:  member,
			target: member,
			want:   domain.ErrForbidden,
		},
		{
			name:   "non_member_cannot_remove_member",
			roles:  map[string]domain.Role{member: domain.RoleMember},
			actor:  ghost,
			target: member,
			want:   domain.ErrForbidden,
		},
		{
			name:   "owner_target_is_protected",
			roles:  map[string]domain.Role{owner: domain.RoleOwner, member: domain.RoleMember},
			actor:  owner,
			target: owner,
			want:   domain.ErrForbidden,
		},
		{
			name:   "admin_cannot_remove_another_admin",
			roles:  map[string]domain.Role{admin: domain.RoleAdmin},
			actor:  admin,
			target: admin,
			want:   domain.ErrForbidden,
		},
		{
			name:   "target_not_in_team_is_not_found",
			roles:  map[string]domain.Role{owner: domain.RoleOwner},
			actor:  owner,
			target: ghost,
			want:   domain.ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rbac := service.NewRBACService(&fakeTeamRepo{roles: tc.roles})

			got := rbac.CanRemoveMember(ctx, teamID, tc.actor, tc.target)

			if !errors.Is(got, tc.want) {
				t.Fatalf("CanRemoveMember() error = %v, want %v", got, tc.want)
			}
		})
	}
}
