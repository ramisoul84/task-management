package service_test

import (
	"errors"
	"testing"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
)

const (
	owner  = domain.RoleOwner
	admin  = domain.RoleAdmin
	member = domain.RoleMember
)

func wantErr(t *testing.T, got, want error) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("unexpected error: %v", got)
		}
		return
	}
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

func TestCanInvite(t *testing.T) {
	tests := []struct {
		actor, invitee domain.Role
		want           error
	}{
		{owner, member, nil},
		{owner, admin, nil},
		{admin, member, nil},
		{admin, admin, nil},
		{member, member, domain.ErrForbidden},
		{owner, owner, domain.ErrInvalidInput}, // ownership is never granted by invite
		{admin, owner, domain.ErrInvalidInput},
	}
	for _, tc := range tests {
		t.Run(string(tc.actor)+"_invites_"+string(tc.invitee), func(t *testing.T) {
			wantErr(t, service.CanInvite(tc.actor, tc.invitee), tc.want)
		})
	}
}

func TestCanChangeRole(t *testing.T) {
	tests := []struct {
		name                  string
		actor, target, toRole domain.Role
		self                  bool
		want                  error
	}{
		{"owner promotes member", owner, member, admin, false, nil},
		{"owner demotes admin", owner, admin, member, false, nil},
		{"admin promotes member", admin, member, admin, false, nil},
		{"admin cannot change admin", admin, admin, member, false, domain.ErrForbidden},
		{"admin cannot change owner", admin, owner, member, false, domain.ErrForbidden},
		{"owner role is immutable", owner, owner, admin, false, domain.ErrForbidden},
		{"member cannot change roles", member, member, admin, false, domain.ErrForbidden},
		{"cannot grant owner", owner, member, owner, false, domain.ErrInvalidInput},
		{"cannot change own role", admin, admin, member, true, domain.ErrInvalidInput},
		{"no-op change rejected", owner, member, member, false, domain.ErrInvalidInput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantErr(t, service.CanChangeRole(tc.actor, tc.target, tc.toRole, tc.self), tc.want)
		})
	}
}

func TestCanRemoveMember(t *testing.T) {
	tests := []struct {
		name          string
		actor, target domain.Role
		self          bool
		want          error
	}{
		{"owner removes admin", owner, admin, false, nil},
		{"owner removes member", owner, member, false, nil},
		{"admin removes member", admin, member, false, nil},
		{"admin cannot remove admin", admin, admin, false, domain.ErrForbidden},
		{"nobody removes owner", admin, owner, false, domain.ErrForbidden},
		{"member cannot remove", member, member, false, domain.ErrForbidden},
		{"cannot remove self", admin, admin, true, domain.ErrInvalidInput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantErr(t, service.CanRemoveMember(tc.actor, tc.target, tc.self), tc.want)
		})
	}
}

func TestCanViewAnalytics(t *testing.T) {
	wantErr(t, service.CanViewAnalytics(owner), nil)
	wantErr(t, service.CanViewAnalytics(admin), nil)
	wantErr(t, service.CanViewAnalytics(member), domain.ErrForbidden)
}

func TestTaskPermissionsFor(t *testing.T) {
	assignee := "assignee"
	task := &domain.Task{CreatedBy: "creator", AssigneeID: &assignee}

	all := service.TaskPermissions{EditContent: true, ChangeStatus: true, Reassign: true}

	tests := []struct {
		name   string
		role   domain.Role
		userID string
		want   service.TaskPermissions
	}{
		{"owner edits any task", owner, "someone", all},
		{"admin edits any task", admin, "someone", all},
		{"creator edits own task", member, "creator", all},
		{"assignee may only change status", member, "assignee", service.TaskPermissions{ChangeStatus: true}},
		{"unrelated member may do nothing", member, "stranger", service.TaskPermissions{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := service.TaskPermissionsFor(tc.role, tc.userID, task); got != tc.want {
				t.Fatalf("permissions = %+v, want %+v", got, tc.want)
			}
		})
	}

	t.Run("unassigned task grants nothing to members", func(t *testing.T) {
		got := service.TaskPermissionsFor(member, "assignee", &domain.Task{CreatedBy: "creator"})
		if got.Any() {
			t.Fatalf("permissions = %+v, want none", got)
		}
	})
}
