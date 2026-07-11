package service_test

import (
	"context"
	"errors"
	"time"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/auth"
)

var (
	errRepoFailure     = errors.New("repo failure")
	errTaskRepoFailure = errors.New("task repo failure")
)

// ==================== TEAM FAKES ====================

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
	if f.roles == nil {
		return "", domain.ErrNotFound
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

// ==================== USER FAKES ====================

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

// ==================== RBAC FAKES ====================

// fakeRBAC implements service.RBACService
type fakeRBAC struct {
	inviteErr   error
	changeErr   error
	removeErr   error
	inviteCalls int

	createTaskErr error
	editTaskErr   error
	statusErr     error
	reassignErr   error
	commentErr    error
	analyticsErr  error
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
	return f.analyticsErr
}

func (f *fakeRBAC) CanCreateTask(ctx context.Context, teamID, actorID string) error {
	return f.createTaskErr
}

func (f *fakeRBAC) CanEditTask(ctx context.Context, teamID, actorID, taskCreatorID string) error {
	return f.editTaskErr
}

func (f *fakeRBAC) CanChangeTaskStatus(ctx context.Context, teamID, actorID, taskCreatorID string, assigneeID *string) error {
	return f.statusErr
}

func (f *fakeRBAC) CanReassignTask(ctx context.Context, teamID, actorID, taskCreatorID string) error {
	return f.reassignErr
}

func (f *fakeRBAC) CanCommentOnTask(ctx context.Context, teamID, actorID string) error {
	return f.commentErr
}

// ==================== TASK FAKES ====================

// fakeTaskRepo implements repository.TaskRepository
type fakeTaskRepo struct {
	created   []*domain.Task
	createErr error

	task   *domain.Task
	getErr error

	tasksList []*domain.Task
	listErr   error

	count    int
	countErr error

	updatedTask *domain.Task
	updateErr   error
	updateCalls []updateTaskCall

	history    []*domain.TaskHistory
	historyErr error

	comments     []*domain.TaskComment
	commentErr   error
	addedComment *domain.TaskComment
}

type updateTaskCall struct {
	taskID          string
	updates         map[string]interface{}
	changedBy       string
	expectedVersion int
}

func (f *fakeTaskRepo) Create(ctx context.Context, task *domain.Task) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, task)
	return nil
}

func (f *fakeTaskRepo) GetByID(ctx context.Context, taskID string) (*domain.Task, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.task == nil {
		return nil, domain.ErrNotFound
	}
	return f.task, nil
}

func (f *fakeTaskRepo) ListByTeam(ctx context.Context, filter *domain.TaskListQuery) ([]*domain.Task, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tasksList, nil
}

func (f *fakeTaskRepo) CountByTeam(ctx context.Context, filter *domain.TaskListQuery) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.count, nil
}

func (f *fakeTaskRepo) UpdateWithHistory(
	ctx context.Context,
	taskID string,
	updates map[string]interface{},
	changedBy string,
	expectedVersion int,
) (*domain.Task, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updateCalls = append(f.updateCalls, updateTaskCall{
		taskID:          taskID,
		updates:         updates,
		changedBy:       changedBy,
		expectedVersion: expectedVersion,
	})
	return f.updatedTask, nil
}

func (f *fakeTaskRepo) GetHistory(ctx context.Context, taskID string) ([]*domain.TaskHistory, error) {
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history, nil
}

func (f *fakeTaskRepo) AddComment(ctx context.Context, comment *domain.TaskComment) error {
	if f.commentErr != nil {
		return f.commentErr
	}
	f.addedComment = comment
	return nil
}

func (f *fakeTaskRepo) ListComments(ctx context.Context, taskID string) ([]*domain.TaskComment, error) {
	if f.commentErr != nil {
		return nil, f.commentErr
	}
	return f.comments, nil
}

// ==================== CACHE FAKES ====================

// fakeTaskCache implements cache.TaskCache
type fakeTaskCache struct {
	cached    *domain.TaskListResponse
	found     bool
	getErr    error
	setErr    error
	invErr    error
	invCalled int
}

func (f *fakeTaskCache) GetTasks(ctx context.Context, query *domain.TaskListQuery) (*domain.TaskListResponse, bool, error) {
	if f.getErr != nil {
		return nil, false, f.getErr
	}
	return f.cached, f.found, nil
}

func (f *fakeTaskCache) SetTasks(ctx context.Context, query *domain.TaskListQuery, response *domain.TaskListResponse) error {
	return f.setErr
}

func (f *fakeTaskCache) InvalidateTeamTasks(ctx context.Context, teamID string) error {
	f.invCalled++
	return f.invErr
}

// ==================== AUTH FAKES ====================

// fakeTokenCache implements cache.TokenCache
type fakeTokenCache struct {
	refreshToken string
	getErr       error
	setErr       error
	rotateErr    error
	deleteErr    error
	blacklistErr error
}

func (f *fakeTokenCache) SetRefreshToken(ctx context.Context, hash, userID string) error {
	return f.setErr
}

func (f *fakeTokenCache) GetRefreshToken(ctx context.Context, hash string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	if f.refreshToken == "" {
		return "", domain.ErrNotFound
	}
	return f.refreshToken, nil
}

func (f *fakeTokenCache) RotateRefreshToken(ctx context.Context, oldHash, newHash, userID string) error {
	return f.rotateErr
}

func (f *fakeTokenCache) DeleteRefreshToken(ctx context.Context, hash string) error {
	return f.deleteErr
}

func (f *fakeTokenCache) BlacklistAccessToken(ctx context.Context, jti string, ttl time.Duration) error {
	return f.blacklistErr
}

func (f *fakeTokenCache) IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return false, nil
}

// fakeTokenService implements auth.TokenService
type fakeTokenService struct {
	accessToken  string
	refreshToken string
	claims       *auth.TokenClaims
	err          error
}

func (f *fakeTokenService) GenerateAccessToken(userID, email string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.accessToken == "" {
		return "fake-access-token", nil
	}
	return f.accessToken, nil
}

func (f *fakeTokenService) GenerateRefreshToken() (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.refreshToken == "" {
		return "fake-refresh-token", nil
	}
	return f.refreshToken, nil
}

func (f *fakeTokenService) HashRefreshToken(token string) string {
	return "hashed_" + token
}

func (f *fakeTokenService) ValidateAccessToken(token string) (*auth.TokenClaims, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.claims == nil {
		return &auth.TokenClaims{
			UserID:    "user-1",
			Email:     "test@example.com",
			JTI:       "token-id-1",
			ExpiresAt: time.Now().Add(15 * time.Minute),
		}, nil
	}
	return f.claims, nil
}

// fakeHasher implements auth.Hasher
type fakeHasher struct {
	hash       string
	hashErr    error
	compareErr error
}

func (f *fakeHasher) Hash(password string) (string, error) {
	if f.hashErr != nil {
		return "", f.hashErr
	}
	if f.hash == "" {
		return "hashed_" + password, nil
	}
	return f.hash, nil
}

func (f *fakeHasher) Compare(hashedPassword, password string) error {
	return f.compareErr
}
