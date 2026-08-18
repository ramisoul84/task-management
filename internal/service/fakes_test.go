package service_test

import (
	"context"

	"github.com/ramisoul84/task-management/internal/domain"
)

// fakeTeamRepo is a hand-rolled in-memory repository.
// roles maps a user ID to its role inside the (single) team under test.
// A missing user behaves like "not a member of this team".
type fakeTeamRepo struct {
	roles map[string]domain.Role
	err   error
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

func (f *fakeTeamRepo) CreateWithOwner(context.Context, *domain.Team) error {
	return nil
}

func (f *fakeTeamRepo) ListTeamsByUser(context.Context, string) ([]*domain.Team, error) {
	return nil, nil
}

func (f *fakeTeamRepo) AddMember(context.Context, *domain.TeamMember) error {
	return nil
}

func (f *fakeTeamRepo) UpdateMemberRole(context.Context, string, string, string, domain.Role) error {
	return nil
}

func (f *fakeTeamRepo) RemoveMember(context.Context, string, string, string) error {
	return nil
}

func (f *fakeTeamRepo) IsTeamMember(context.Context, string, string) (bool, error) {
	return false, nil
}
