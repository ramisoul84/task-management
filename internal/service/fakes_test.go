package service_test

import (
	"context"

	"github.com/ramisoul84/task-management/internal/domain"
)

// fakeTeamRepo is a hand-rolled in-memory repository.
// roles maps a user ID to its role inside the (single) team under test.
// A missing user behaves like "not a member of this team".
// The remaining fields control return values and record the calls made,
// so orchestration tests can assert on what the service actually did.
type fakeTeamRepo struct {
	roles map[string]domain.Role
	err   error

	created   []*domain.Team
	createErr error

	teamsList []*domain.Team
	listErr   error

	added       []*domain.TeamMember
	addErr      error
	isMember    bool
	isMemberErr error

	updateCalls []updateRoleCall
	updateErr   error

	removeCalls []removeMemberCall
	removeErr   error
}

type updateRoleCall struct {
	teamID  string
	actorID string
	userID  string
	role    domain.Role
}

type removeMemberCall struct {
	teamID  string
	actorID string
	userID  string
}

func (f *fakeTeamRepo) GetUserRoleInTeam(_ context.Context, userID string, _ string) (domain.Role, error) {
	if f.err != nil {
		return "", f.err
	}

	role, ok := f.roles[userID]
	if !ok {
		return "", domain.ErrNotFound
	}

	return role, nil
}

func (f *fakeTeamRepo) CreateWithOwner(_ context.Context, team *domain.Team) error {
	if f.createErr != nil {
		return f.createErr
	}

	f.created = append(f.created, team)
	return nil
}

func (f *fakeTeamRepo) ListTeamsByUser(_ context.Context, _ string) ([]*domain.Team, error) {
	return f.teamsList, f.listErr
}

func (f *fakeTeamRepo) AddMember(_ context.Context, member *domain.TeamMember) error {
	if f.addErr != nil {
		return f.addErr
	}

	f.added = append(f.added, member)
	return nil
}

func (f *fakeTeamRepo) UpdateMemberRole(_ context.Context, teamID, actorID, userID string, role domain.Role) error {
	f.updateCalls = append(f.updateCalls, updateRoleCall{
		teamID:  teamID,
		actorID: actorID,
		userID:  userID,
		role:    role,
	})
	return f.updateErr
}

func (f *fakeTeamRepo) RemoveMember(_ context.Context, teamID, actorID, userID string) error {
	f.removeCalls = append(f.removeCalls, removeMemberCall{
		teamID:  teamID,
		actorID: actorID,
		userID:  userID,
	})
	return f.removeErr
}

func (f *fakeTeamRepo) IsTeamMember(_ context.Context, _ string, _ string) (bool, error) {
	return f.isMember, f.isMemberErr
}

// fakeUserRepo is a minimal in-memory user repository.
// A user missing from users behaves like "unknown user" (ErrNotFound).
type fakeUserRepo struct {
	users map[string]*domain.User
	err   error
}

func (f *fakeUserRepo) Create(context.Context, *domain.User) error {
	return nil
}

func (f *fakeUserRepo) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeUserRepo) GetByID(_ context.Context, userID string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}

	user, ok := f.users[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}

	return user, nil
}

// fakeRBAC is a controllable stand-in for the RBAC service.
// Each method records that it was consulted and returns its injected error.
type fakeRBAC struct {
	inviteErr error
	changeErr error
	removeErr error

	inviteCalls int
	changeCalls int
	removeCalls int
}

func (f *fakeRBAC) CanInviteMember(context.Context, string, string) error {
	f.inviteCalls++
	return f.inviteErr
}

func (f *fakeRBAC) CanChangeMemberRole(context.Context, string, string, string, domain.Role) error {
	f.changeCalls++
	return f.changeErr
}

func (f *fakeRBAC) CanRemoveMember(context.Context, string, string, string) error {
	f.removeCalls++
	return f.removeErr
}
