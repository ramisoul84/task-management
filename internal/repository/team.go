package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type TeamRepository interface {
	CreateWithOwner(ctx context.Context, team *domain.Team) error
	ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error)
	AddMember(ctx context.Context, member *domain.TeamMember) error
	GetUserRoleInTeam(ctx context.Context, userID string, teamID string) (domain.Role, error)
	UpdateMemberRole(ctx context.Context, teamID string, userID string, role domain.Role) error
	RemoveMember(ctx context.Context, teamID string, userID string) error
	IsTeamMember(ctx context.Context, teamID string, userID string) (bool, error)
}

type teamRepo struct {
	db  *database.DB
	log *logger.Logger
}

func NewTeamRepository(
	db *database.DB,
	log *logger.Logger,
) TeamRepository {
	return &teamRepo{
		db:  db,
		log: log,
	}
}

func (r *teamRepo) CreateWithOwner(ctx context.Context, team *domain.Team) error {
	const op = "teamRepo.CreateWithOwner"

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin transaction: %w", op, err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	const createTeamQuery = `
		INSERT INTO teams (
			id,
			name,
			created_by,
			created_at
		)
		VALUES (?, ?, ?, ?)
	`

	if _, err := tx.ExecContext(
		ctx,
		createTeamQuery,
		team.ID,
		team.Name,
		team.CreatedBy,
		team.CreatedAt,
	); err != nil {
		return fmt.Errorf("%s: insert team: %w", op, err)
	}

	const addOwnerQuery = `
		INSERT INTO team_members (
			team_id,
			user_id,
			role
		)
		VALUES (?, ?, ?)
	`

	if _, err := tx.ExecContext(
		ctx,
		addOwnerQuery,
		team.ID,
		team.CreatedBy,
		domain.RoleOwner,
	); err != nil {
		return fmt.Errorf("%s: insert owner: %w", op, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit transaction: %w", op, err)
	}

	return nil
}

func (r *teamRepo) ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error) {
	const op = "teamRepo.ListTeamsByUser"

	const query = `
		SELECT
			t.id,
			t.name,
			t.created_by,
			t.created_at
		FROM teams AS t
		INNER JOIN team_members AS tm
			ON tm.team_id = t.id
		WHERE tm.user_id = ?
		ORDER BY t.created_at DESC
	`

	var teams []*domain.Team

	if err := r.db.SelectContext(
		ctx,
		&teams,
		query,
		userID,
	); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return teams, nil
}

func (r *teamRepo) AddMember(ctx context.Context, member *domain.TeamMember) error {
	const op = "teamRepo.AddMember"

	const query = `
		INSERT INTO team_members (
			team_id,
			user_id,
			role
		)
		VALUES (?, ?, ?)
	`

	if _, err := r.db.ExecContext(
		ctx,
		query,
		member.TeamID,
		member.UserID,
		member.Role,
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *teamRepo) GetUserRoleInTeam(ctx context.Context, userID string, teamID string) (domain.Role, error) {
	const op = "teamRepo.GetUserRoleInTeam"

	const query = `
		SELECT role
		FROM team_members
		WHERE user_id = ?
		  AND team_id = ?
	`

	var role domain.Role

	err := r.db.GetContext(
		ctx,
		&role,
		query,
		userID,
		teamID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrNotFound
		}

		return "", fmt.Errorf("%s: %w", op, err)
	}

	return role, nil
}

func (r *teamRepo) IsTeamMember(ctx context.Context, teamID string, userID string) (bool, error) {
	const op = "teamRepo.IsTeamMember"

	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM team_members
			WHERE team_id = ?
			  AND user_id = ?
		)
	`

	var exists bool

	if err := r.db.GetContext(
		ctx,
		&exists,
		query,
		teamID,
		userID,
	); err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	return exists, nil
}

func (r *teamRepo) RemoveMember(ctx context.Context, teamID string, userID string) error {
	const query = `
		DELETE FROM team_members
		WHERE team_id = ?
		  AND user_id = ?
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		teamID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows: %w", err)
	}

	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *teamRepo) UpdateMemberRole(ctx context.Context, teamID string, userID string, role domain.Role) error {
	const op = "teamRepo.UpdateMemberRole"

	const query = `
		UPDATE team_members
		SET role = ?
		WHERE team_id = ?
		  AND user_id = ?
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		role,
		teamID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", op, err)
	}

	if rows == 0 {
		return domain.ErrNotFound
	}

	return nil
}
