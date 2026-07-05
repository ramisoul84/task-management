package service_test

import (
	"context"

	"github.com/ramisoul84/task-management/internal/domain"
)

// fakeTeamRepo implements repository.TeamRepository
type fakeTeamRepo struct {
	// For RBAC tests
	roles map[string]domain.Role
	err   error

	// For Team service tests
	created   []*domain.Team
	createErr error

	teamsList []*domain.Team
	listErr   error

	added  []*domain.TeamMember
	addErr error

	isMember    bool
	isMemberErr error

	updateCalls []updateCall
	updateErr   error

	removeCalls []removeCall
	removeErr   error
}

type updateCall struct {
	teamID  string
	actorID string
	userID  string
	role    domain.Role
}

type removeCall struct {
	teamID  string
	actorID string
	userID  string
}

func (f *fakeTeamRepo) CreateWithOwner(ctx context.Context, team *domain.Team) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, team)
	return nil
}

func (f *fakeTeamRepo) ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.teamsList, nil
}

func (f *fakeTeamRepo) GetUserRoleInTeam(ctx context.Context, teamID, userID string) (domain.Role, error) {
	if f.err != nil {
		return "", f.err
	}
	role, exists := f.roles[userID]
	if !exists {
		return "", domain.ErrNotFound
	}
	return role, nil
}

func (f *fakeTeamRepo) AddMember(ctx context.Context, member *domain.TeamMember) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, member)
	return nil
}

func (f *fakeTeamRepo) IsTeamMember(ctx context.Context, teamID, userID string) (bool, error) {
	if f.isMemberErr != nil {
		return false, f.isMemberErr
	}
	if f.roles != nil {
		_, exists := f.roles[userID]
		return exists, nil
	}
	return f.isMember, nil
}

func (f *fakeTeamRepo) ListMembers(ctx context.Context, teamID string) ([]*domain.TeamMemberResponse, error) {
	return nil, nil
}

func (f *fakeTeamRepo) UpdateMemberRole(ctx context.Context, teamID, actorID, userID string, role domain.Role) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updateCalls = append(f.updateCalls, updateCall{teamID, actorID, userID, role})
	return nil
}

func (f *fakeTeamRepo) RemoveMember(ctx context.Context, teamID, actorID, userID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removeCalls = append(f.removeCalls, removeCall{teamID, actorID, userID})
	return nil
}

func (f *fakeTeamRepo) GetTeamStats(ctx context.Context, teamID string) (*domain.TeamStats, error) {
	return nil, nil
}

// fakeUserRepo implements repository.UserRepository
type fakeUserRepo struct {
	users map[string]*domain.User
	err   error
}

func (f *fakeUserRepo) Create(ctx context.Context, user *domain.User) error {
	return f.err
}

func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUserRepo) GetByID(ctx context.Context, userID string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if u, exists := f.users[userID]; exists {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

// fakeRBAC implements service.RBACService
type fakeRBAC struct {
	inviteErr   error
	changeErr   error
	removeErr   error
	inviteCalls int
}

func (f *fakeRBAC) CanInviteMember(ctx context.Context, teamID, actorID string) error {
	f.inviteCalls++
	return f.inviteErr
}

func (f *fakeRBAC) CanChangeMemberRole(ctx context.Context, teamID, actorID, targetUserID string, newRole domain.Role) error {
	return f.changeErr
}

func (f *fakeRBAC) CanRemoveMember(ctx context.Context, teamID, actorID, targetUserID string) error {
	return f.removeErr
}

func (f *fakeRBAC) CanViewAnalytics(ctx context.Context, teamID, actorID string) error {
	return nil
}

func (f *fakeRBAC) CanCreateTask(ctx context.Context, teamID, actorID string) error {
	return nil
}

func (f *fakeRBAC) CanEditTask(ctx context.Context, teamID, actorID, taskCreatorID string) error {
	return nil
}

func (f *fakeRBAC) CanChangeTaskStatus(ctx context.Context, teamID, actorID, taskCreatorID string, assigneeID *string) error {
	return nil
}

func (f *fakeRBAC) CanReassignTask(ctx context.Context, teamID, actorID, taskCreatorID string) error {
	return nil
}

func (f *fakeRBAC) CanCommentOnTask(ctx context.Context, teamID, actorID string) error {
	return nil
}
