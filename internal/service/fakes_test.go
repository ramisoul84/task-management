package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ramisoul84/task-management/internal/cache"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/auth"
)

// These fakes are small in-memory implementations rather than stubs: they
// enforce the same invariants as MySQL (unique keys, version checks), so
// service tests exercise real behaviour, not scripted return values.

var errBoom = errors.New("boom")

// ---------- transactor ----------

type fakeTx struct{ calls int }

func (f *fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	return fn(ctx)
}

// ---------- store ----------

type store struct {
	mu       sync.Mutex
	users    map[string]*domain.User
	teams    map[string]*domain.Team
	members  map[string]map[string]domain.Role // team -> user -> role
	tasks    map[string]*domain.Task
	history  []*domain.TaskHistory
	comments []*domain.TaskComment

	failNext error // returned (once) by the next repository call
}

func newStore() *store {
	return &store{
		users:   map[string]*domain.User{},
		teams:   map[string]*domain.Team{},
		members: map[string]map[string]domain.Role{},
		tasks:   map[string]*domain.Task{},
	}
}

func (s *store) fail() error {
	err := s.failNext
	s.failNext = nil
	return err
}

func (s *store) addUser(id string) {
	s.users[id] = &domain.User{ID: id, Email: id + "@example.com", Name: strings.ToUpper(id[:1]) + id[1:]}
}

func (s *store) addTeam(teamID, ownerID string) {
	s.teams[teamID] = &domain.Team{ID: teamID, Name: teamID, CreatedBy: ownerID}
	s.members[teamID] = map[string]domain.Role{ownerID: domain.RoleOwner}
}

func (s *store) addMember(teamID, userID string, role domain.Role) {
	s.members[teamID][userID] = role
}

func (s *store) addTask(t domain.Task) *domain.Task {
	cp := t
	if cp.Version == 0 {
		cp.Version = 1
	}
	if cp.Status == "" {
		cp.Status = domain.TaskStatusTodo
	}
	if cp.Priority == "" {
		cp.Priority = domain.PriorityMedium
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		cp.UpdatedAt = cp.CreatedAt
	}
	s.tasks[cp.ID] = &cp
	return &cp
}

// ---------- users ----------

type userRepo struct{ s *store }

func (r userRepo) Create(_ context.Context, u *domain.User) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	for _, existing := range r.s.users {
		if existing.Email == u.Email {
			return domain.ErrAlreadyExists
		}
	}
	cp := *u
	r.s.users[u.ID] = &cp
	return nil
}

func (r userRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return nil, err
	}
	for _, u := range r.s.users {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r userRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return nil, err
	}
	if u, ok := r.s.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

// ---------- teams ----------

type teamRepo struct{ s *store }

func (r teamRepo) Create(_ context.Context, t *domain.Team) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	cp := *t
	r.s.teams[t.ID] = &cp
	r.s.members[t.ID] = map[string]domain.Role{}
	return nil
}

func (r teamRepo) ListByUser(_ context.Context, userID string) ([]*domain.Team, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return nil, err
	}
	out := []*domain.Team{}
	for id, m := range r.s.members {
		if role, ok := m[userID]; ok {
			cp := *r.s.teams[id]
			cp.Role = role
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r teamRepo) AddMember(_ context.Context, m *domain.TeamMember) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	team, ok := r.s.members[m.TeamID]
	if !ok {
		return domain.ErrNotFound
	}
	if _, dup := team[m.UserID]; dup {
		return domain.ErrAlreadyExists
	}
	team[m.UserID] = m.Role
	return nil
}

func (r teamRepo) GetMemberRole(_ context.Context, teamID, userID string) (domain.Role, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return "", err
	}
	if role, ok := r.s.members[teamID][userID]; ok {
		return role, nil
	}
	return "", domain.ErrNotFound
}

func (r teamRepo) GetMemberRoleForUpdate(ctx context.Context, teamID, userID string) (domain.Role, error) {
	return r.GetMemberRole(ctx, teamID, userID)
}

func (r teamRepo) IsMember(_ context.Context, teamID, userID string) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return false, err
	}
	_, ok := r.s.members[teamID][userID]
	return ok, nil
}

func (r teamRepo) UpdateMemberRole(_ context.Context, teamID, userID string, role domain.Role) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.members[teamID][userID]; !ok {
		return domain.ErrNotFound
	}
	r.s.members[teamID][userID] = role
	return nil
}

func (r teamRepo) RemoveMember(_ context.Context, teamID, userID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.members[teamID][userID]; !ok {
		return domain.ErrNotFound
	}
	delete(r.s.members[teamID], userID)
	return nil
}

func (r teamRepo) ListMembers(_ context.Context, teamID string) ([]*domain.TeamMemberResponse, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := []*domain.TeamMemberResponse{}
	for uid, role := range r.s.members[teamID] {
		out = append(out, &domain.TeamMemberResponse{UserID: uid, Role: role})
	}
	return out, nil
}

func (r teamRepo) GetStats(_ context.Context, _ string) (*domain.TeamStats, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return nil, err
	}
	return &domain.TeamStats{TasksByStatus: map[domain.TaskStatus]int{}}, nil
}

// ---------- tasks ----------

type taskRepo struct{ s *store }

func (r taskRepo) Create(_ context.Context, t *domain.Task) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	cp := *t
	r.s.tasks[t.ID] = &cp
	return nil
}

func (r taskRepo) GetByID(_ context.Context, id string) (*domain.Task, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return nil, err
	}
	if t, ok := r.s.tasks[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (r taskRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Task, error) {
	return r.GetByID(ctx, id)
}

func (r taskRepo) matching(q *domain.TaskListQuery) []*domain.Task {
	out := []*domain.Task{}
	for _, t := range r.s.tasks {
		if t.TeamID != q.TeamID ||
			(q.Status != "" && t.Status != q.Status) ||
			(q.Priority != "" && t.Priority != q.Priority) ||
			(q.Overdue && !t.IsOverdue(time.Now())) ||
			(q.AssigneeID != "" && !t.IsAssignee(q.AssigneeID)) {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	slices.SortFunc(out, func(a, b *domain.Task) int { return strings.Compare(b.ID, a.ID) })
	return out
}

// List orders by id descending (fixtures use ordered IDs) and treats the
// cursor ID as the keyset position.
func (r taskRepo) List(_ context.Context, q *domain.TaskListQuery, after *domain.TaskCursor) ([]*domain.Task, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return nil, err
	}
	all := r.matching(q)
	start := q.Offset
	if after != nil {
		start = len(all)
		for i, t := range all {
			if t.ID < after.ID {
				start = i
				break
			}
		}
	}
	if start >= len(all) {
		return []*domain.Task{}, nil
	}
	return all[start:min(start+q.Limit, len(all))], nil
}

func (r taskRepo) Count(_ context.Context, q *domain.TaskListQuery) (int, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return len(r.matching(q)), nil
}

func (r taskRepo) Update(_ context.Context, t *domain.Task, expected int) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	cur, ok := r.s.tasks[t.ID]
	if !ok || cur.Version != expected {
		return domain.ErrVersionConflict
	}
	cp := *t
	cp.Version = expected + 1
	r.s.tasks[t.ID] = &cp
	t.Version = cp.Version
	return nil
}

func (r taskRepo) UnassignUser(_ context.Context, teamID, userID, changedBy string) (int, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	n := 0
	for _, t := range r.s.tasks {
		if t.TeamID == teamID && t.IsAssignee(userID) {
			raw, _ := json.Marshal(domain.TaskChanges{"assignee_id": {Old: userID, New: nil}})
			r.s.history = append(r.s.history, &domain.TaskHistory{TaskID: t.ID, ChangedBy: changedBy, Changes: raw})
			t.AssigneeID = nil
			t.Version++
			n++
		}
	}
	return n, nil
}

func (r taskRepo) Delete(_ context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.tasks[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.s.tasks, id)
	return nil
}

func (r taskRepo) AddHistory(_ context.Context, h *domain.TaskHistory) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	cp := *h
	cp.ID = int64(len(r.s.history) + 1)
	r.s.history = append(r.s.history, &cp)
	return nil
}

func (r taskRepo) ListHistory(_ context.Context, taskID string) ([]*domain.TaskHistory, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := []*domain.TaskHistory{}
	for _, h := range r.s.history {
		if h.TaskID == taskID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (r taskRepo) AddComment(_ context.Context, c *domain.TaskComment) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.s.fail(); err != nil {
		return err
	}
	cp := *c
	r.s.comments = append(r.s.comments, &cp)
	return nil
}

func (r taskRepo) ListComments(_ context.Context, taskID string) ([]*domain.TaskComment, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := []*domain.TaskComment{}
	for _, c := range r.s.comments {
		if c.TaskID == taskID {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---------- task list cache ----------

type fakeTaskCache struct {
	mu          sync.Mutex
	entries     map[string]*domain.TaskListResponse
	invalidated []string
	getErr      error
}

func newFakeTaskCache() *fakeTaskCache {
	return &fakeTaskCache{entries: map[string]*domain.TaskListResponse{}}
}

func cacheKey(q *domain.TaskListQuery) string {
	b, _ := json.Marshal(q)
	return string(b)
}

func (c *fakeTaskCache) Get(_ context.Context, q *domain.TaskListQuery) (*domain.TaskListResponse, cache.Stamp, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return nil, "", c.getErr
	}
	return c.entries[cacheKey(q)], "0", nil
}

func (c *fakeTaskCache) Set(_ context.Context, q *domain.TaskListQuery, _ cache.Stamp, r *domain.TaskListResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(q)] = r
	return nil
}

func (c *fakeTaskCache) InvalidateTeam(_ context.Context, teamID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, teamID)
	for k := range c.entries {
		if strings.Contains(k, teamID) {
			delete(c.entries, k)
		}
	}
	return nil
}

// ---------- auth ----------

type fakeTokenCache struct {
	mu          sync.Mutex
	refresh     map[string]string
	blacklisted map[string]time.Duration
}

func newFakeTokenCache() *fakeTokenCache {
	return &fakeTokenCache{refresh: map[string]string{}, blacklisted: map[string]time.Duration{}}
}

func (f *fakeTokenCache) SetRefreshToken(_ context.Context, hash, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refresh[hash] = userID
	return nil
}

func (f *fakeTokenCache) GetRefreshToken(_ context.Context, hash string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if uid, ok := f.refresh[hash]; ok {
		return uid, nil
	}
	return "", domain.ErrNotFound
}

func (f *fakeTokenCache) RotateRefreshToken(_ context.Context, oldHash, newHash, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refresh[oldHash] != userID {
		return domain.ErrInvalidCredentials
	}
	delete(f.refresh, oldHash)
	f.refresh[newHash] = userID
	return nil
}

func (f *fakeTokenCache) DeleteRefreshToken(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.refresh, hash)
	return nil
}

func (f *fakeTokenCache) BlacklistAccessToken(_ context.Context, jti string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blacklisted[jti] = ttl
	return nil
}

func (f *fakeTokenCache) IsAccessTokenBlacklisted(_ context.Context, jti string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.blacklisted[jti]
	return ok, nil
}

// plainHasher "hashes" by prefixing, so tests stay fast and deterministic.
type plainHasher struct {
	compares int
	hashErr  error
}

func (h *plainHasher) Hash(p string) (string, error) {
	if h.hashErr != nil {
		return "", h.hashErr
	}
	return "hashed:" + p, nil
}

func (h *plainHasher) Compare(hash, p string) error {
	h.compares++
	if hash != "hashed:"+p {
		return errors.New("mismatch")
	}
	return nil
}

var _ auth.Hasher = (*plainHasher)(nil)
