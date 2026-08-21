//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/database"
)

func insertUser(t *testing.T, db *database.DB, id string) {
	t.Helper()

	user := &domain.User{
		ID:           id,
		Email:        id + "@example.com",
		PasswordHash: "hashed-secret",
		Name:         id,
	}

	if err := repository.NewUserRepository(db).Create(context.Background(), user); err != nil {
		t.Fatalf("insert user %q: %v", id, err)
	}
}

func newTeamWithOwner(t *testing.T, db *database.DB) (ownerID, teamID string) {
	t.Helper()

	ownerID = uuid.NewString()
	insertUser(t, db, ownerID)

	teamID = uuid.NewString()
	team := &domain.Team{
		ID:        teamID,
		Name:      "TEAM-1",
		CreatedBy: ownerID,
		CreatedAt: time.Now(),
	}

	if err := repository.NewTeamRepository(db).CreateWithOwner(context.Background(), team); err != nil {
		t.Fatalf("CreateWithOwner() error = %v, want nil", err)
	}

	return ownerID, teamID
}

func TestTeamRepository_CreateWithOwner(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	ownerID := uuid.NewString()
	insertUser(t, db, ownerID)

	team := &domain.Team{
		ID:        uuid.NewString(),
		Name:      "ENGINEERING",
		CreatedBy: ownerID,
		CreatedAt: time.Now(),
	}

	if err := repo.CreateWithOwner(ctx, team); err != nil {
		t.Fatalf("CreateWithOwner() error = %v, want nil", err)
	}

	role, err := repo.GetUserRoleInTeam(ctx, ownerID, team.ID)
	if err != nil {
		t.Fatalf("GetUserRoleInTeam() error = %v, want nil", err)
	}
	if role != domain.RoleOwner {
		t.Errorf("owner role = %q, want %q", role, domain.RoleOwner)
	}
}

func TestTeamRepository_AddMember_Duplicate(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	_, teamID := newTeamWithOwner(t, db)

	memberID := uuid.NewString()
	insertUser(t, db, memberID)

	member := &domain.TeamMember{
		TeamID: teamID,
		UserID: memberID,
		Role:   domain.RoleMember,
	}

	if err := repo.AddMember(ctx, member); err != nil {
		t.Fatalf("first AddMember() error = %v, want nil", err)
	}

	err := repo.AddMember(ctx, member)

	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("second AddMember() error = %v, want %v", err, domain.ErrAlreadyExists)
	}
}

func TestTeamRepository_ListTeamsByUser(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	ownerA, teamA := newTeamWithOwner(t, db)
	_, teamB := newTeamWithOwner(t, db)

	memberID := uuid.NewString()
	insertUser(t, db, memberID)
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamB, UserID: memberID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("AddMember() error = %v, want nil", err)
	}

	teams, err := repo.ListTeamsByUser(ctx, ownerA)
	if err != nil {
		t.Fatalf("ListTeamsByUser() error = %v, want nil", err)
	}
	if len(teams) != 1 || teams[0].ID != teamA {
		t.Errorf("ListTeamsByUser() = %v, want only team %q", teams, teamA)
	}

	memberTeams, err := repo.ListTeamsByUser(ctx, memberID)
	if err != nil {
		t.Fatalf("ListTeamsByUser() for member error = %v, want nil", err)
	}
	if len(memberTeams) != 1 || memberTeams[0].ID != teamB {
		t.Errorf("member ListTeamsByUser() = %v, want only team %q", memberTeams, teamB)
	}
}

func TestTeamRepository_UpdateMemberRole_UnauthorizedActor(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	_, teamID := newTeamWithOwner(t, db)

	actorID := uuid.NewString()
	targetID := uuid.NewString()
	insertUser(t, db, actorID)
	insertUser(t, db, targetID)
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: actorID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("add actor: %v", err)
	}
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: targetID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("add target: %v", err)
	}

	// A plain member tries to promote the target: the single conditional
	// UPDATE must leave the role untouched and report forbidden.
	err := repo.UpdateMemberRole(ctx, teamID, actorID, targetID, domain.RoleAdmin)

	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("UpdateMemberRole() by member error = %v, want %v", err, domain.ErrForbidden)
	}

	role, err := repo.GetUserRoleInTeam(ctx, targetID, teamID)
	if err != nil {
		t.Fatalf("GetUserRoleInTeam() error = %v, want nil", err)
	}
	if role != domain.RoleMember {
		t.Errorf("target role = %q, want unchanged %q", role, domain.RoleMember)
	}
}

func TestTeamRepository_UpdateMemberRole_AuthorizedActor(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	ownerID, teamID := newTeamWithOwner(t, db)

	targetID := uuid.NewString()
	insertUser(t, db, targetID)
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: targetID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("add target: %v", err)
	}

	if err := repo.UpdateMemberRole(ctx, teamID, ownerID, targetID, domain.RoleAdmin); err != nil {
		t.Fatalf("UpdateMemberRole() by owner error = %v, want nil", err)
	}

	role, err := repo.GetUserRoleInTeam(ctx, targetID, teamID)
	if err != nil {
		t.Fatalf("GetUserRoleInTeam() error = %v, want nil", err)
	}
	if role != domain.RoleAdmin {
		t.Errorf("target role = %q, want %q", role, domain.RoleAdmin)
	}
}

func TestTeamRepository_UpdateMemberRole_UnknownTarget(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	ownerID, teamID := newTeamWithOwner(t, db)

	ghostID := uuid.NewString()
	insertUser(t, db, ghostID)

	err := repo.UpdateMemberRole(ctx, teamID, ownerID, ghostID, domain.RoleAdmin)

	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("UpdateMemberRole() on non-member error = %v, want %v", err, domain.ErrForbidden)
	}
}

func TestTeamRepository_RemoveMember_UnauthorizedActor(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	_, teamID := newTeamWithOwner(t, db)

	actorID := uuid.NewString()
	targetID := uuid.NewString()
	insertUser(t, db, actorID)
	insertUser(t, db, targetID)
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: actorID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("add actor: %v", err)
	}
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: targetID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("add target: %v", err)
	}

	err := repo.RemoveMember(ctx, teamID, actorID, targetID)

	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("RemoveMember() by member error = %v, want %v", err, domain.ErrForbidden)
	}

	isMember, err := repo.IsTeamMember(ctx, teamID, targetID)
	if err != nil {
		t.Fatalf("IsTeamMember() error = %v, want nil", err)
	}
	if !isMember {
		t.Error("target was removed despite unauthorized actor")
	}
}

func TestTeamRepository_RemoveMember_AuthorizedActor(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	ownerID, teamID := newTeamWithOwner(t, db)

	targetID := uuid.NewString()
	insertUser(t, db, targetID)
	if err := repo.AddMember(ctx, &domain.TeamMember{TeamID: teamID, UserID: targetID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("add target: %v", err)
	}

	if err := repo.RemoveMember(ctx, teamID, ownerID, targetID); err != nil {
		t.Fatalf("RemoveMember() by owner error = %v, want nil", err)
	}

	isMember, err := repo.IsTeamMember(ctx, teamID, targetID)
	if err != nil {
		t.Fatalf("IsTeamMember() error = %v, want nil", err)
	}
	if isMember {
		t.Error("target still a member after owner removed them")
	}
}

func TestTeamRepository_AddMember_Concurrent(t *testing.T) {
	db := newDB(t)
	repo := repository.NewTeamRepository(db)
	ctx := context.Background()

	_, teamID := newTeamWithOwner(t, db)

	memberID := uuid.NewString()
	insertUser(t, db, memberID)

	member := &domain.TeamMember{
		TeamID: teamID,
		UserID: memberID,
		Role:   domain.RoleMember,
	}

	const workers = 8

	var wg sync.WaitGroup
	errs := make([]error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.AddMember(ctx, member)
		}(i)
	}
	wg.Wait()

	successes := 0
	duplicates := 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrAlreadyExists):
			duplicates++
		default:
			t.Errorf("unexpected AddMember() error: %v", err)
		}
	}

	if successes != 1 {
		t.Errorf("AddMember() succeeded %d time(s), want exactly 1", successes)
	}
	if duplicates != workers-1 {
		t.Errorf("AddMember() reported %d duplicate(s), want %d", duplicates, workers-1)
	}
}
