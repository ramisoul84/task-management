package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type TeamRepository interface {
	CreateWithOwner(ctx context.Context, team *domain.Team) error
	ListTeamsByUser(ctx context.Context, userID string) ([]*domain.Team, error)
	GetUserRoleInTeam(ctx context.Context, teamID string, userID string) (domain.Role, error)
	AddMember(ctx context.Context, member *domain.TeamMember) error
	IsTeamMember(ctx context.Context, teamID string, userID string) (bool, error)
	UpdateMemberRole(ctx context.Context, teamID string, actorID string, userID string, role domain.Role) error
	RemoveMember(ctx context.Context, teamID string, actorID string, targetUserID string) error
	ListMembers(ctx context.Context, teamID string) ([]*domain.TeamMemberResponse, error)
	GetTeamStats(ctx context.Context, teamID string) (*domain.TeamStats, error)
}

type teamRepo struct {
	db *database.DB
}

func NewTeamRepository(db *database.DB) TeamRepository {
	return &teamRepo{db: db}
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

	teams := make([]*domain.Team, 0)

	if err := r.db.SelectContext(ctx, &teams, query, userID); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return teams, nil
}

func (r *teamRepo) GetUserRoleInTeam(ctx context.Context, teamID string, userID string) (domain.Role, error) {
	const op = "teamRepo.GetUserRoleInTeam"

	const query = `
		SELECT role
		FROM team_members
		WHERE team_id = ?
		  AND user_id = ?
	`

	var role domain.Role

	err := r.db.GetContext(
		ctx,
		&role,
		query,
		teamID, // team_id first
		userID, // user_id second
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return role, nil
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
		var mysqlErr *mysql.MySQLError

		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return domain.ErrAlreadyExists
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
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

func (r *teamRepo) UpdateMemberRole(ctx context.Context, teamID string, actorID string, userID string, role domain.Role) error {
	const op = "teamRepo.UpdateMemberRole"

	const query = `
		UPDATE team_members AS target
		INNER JOIN team_members AS actor
			ON actor.team_id = target.team_id
		   AND actor.user_id = ?
		   AND actor.role IN ('owner', 'admin')
		SET target.role = ?
		WHERE target.team_id = ?
		  AND target.user_id = ?
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		actorID,
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
		return domain.ErrForbidden
	}

	return nil
}

func (r *teamRepo) RemoveMember(
	ctx context.Context,
	teamID string,
	actorID string,
	targetUserID string,
) error {
	const op = "teamRepo.RemoveMember"

	const query = `
		DELETE target
		FROM team_members AS target
		INNER JOIN team_members AS actor
			ON actor.team_id = target.team_id
		   AND actor.user_id = ?
		   AND actor.role IN ('owner', 'admin')
		WHERE target.team_id = ?
		  AND target.user_id = ?
		  AND target.role != 'owner'  -- Extra safety: never remove owner
		  AND NOT (
			actor.role = 'admin' 
			AND target.role = 'admin'
		  )  -- Admin cannot remove another admin
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		actorID,
		teamID,
		targetUserID,
	)
	if err != nil {
		return fmt.Errorf("%s: delete member: %w", op, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", op, err)
	}

	if rowsAffected == 0 {
		return domain.ErrForbidden
	}

	return nil
}

func (r *teamRepo) ListMembers(ctx context.Context, teamID string) ([]*domain.TeamMemberResponse, error) {
	const op = "teamRepo.ListMembers"

	const query = `
		SELECT
			u.id AS user_id,
			u.email,
			u.name,
			tm.role
		FROM team_members tm
		INNER JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ?
		ORDER BY tm.role, u.name
	`

	members := make([]*domain.TeamMemberResponse, 0)
	if err := r.db.SelectContext(ctx, &members, query, teamID); err != nil {
		return nil, fmt.Errorf("%s: select members: %w", op, err)
	}

	return members, nil
}

// GetTeamStats retrieves team analytics with a single CTE query
func (r *teamRepo) GetTeamStats(ctx context.Context, teamID string) (*domain.TeamStats, error) {
	const op = "teamRepo.GetTeamStats"

	const query = `
		WITH task_stats AS (
			SELECT 
				COUNT(*) AS total_tasks,
				COALESCE(SUM(CASE WHEN status = 'todo' THEN 1 ELSE 0 END), 0) AS todo_count,
			COALESCE(SUM(CASE WHEN status = 'in_progress' THEN 1 ELSE 0 END), 0) AS in_progress_count,
			COALESCE(SUM(CASE WHEN status = 'review' THEN 1 ELSE 0 END), 0) AS review_count,
			COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0) AS done_count,
			COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) AS cancelled_count,
				COALESCE(AVG(CASE 
					WHEN status = 'done' AND closed_at IS NOT NULL 
					THEN TIMESTAMPDIFF(HOUR, created_at, closed_at) 
				END), 0) AS avg_close_time
			FROM tasks
			WHERE team_id = ?
		),
		top_performers AS (
			SELECT 
				u.id AS user_id,
				u.name AS user_name,
				COUNT(t.id) AS closed_tasks
			FROM tasks t
			INNER JOIN users u ON u.id = t.assignee_id
			WHERE t.team_id = ?
			  AND t.status = 'done'
			  AND t.closed_at IS NOT NULL
			  AND t.closed_at >= DATE_SUB(NOW(), INTERVAL 30 DAY)
			GROUP BY u.id, u.name
			ORDER BY closed_tasks DESC, u.name ASC
			LIMIT 3
		),
		comment_stats AS (
			SELECT COUNT(tc.id) AS total_comments
			FROM task_comments tc
			INNER JOIN tasks t ON t.id = tc.task_id
			WHERE t.team_id = ?
		)
		SELECT 
			ts.total_tasks,
			ts.todo_count,
			ts.in_progress_count,
			ts.review_count,
			ts.done_count,
			ts.cancelled_count,
			ts.avg_close_time,
			cs.total_comments,
			tp.user_id,
			tp.user_name,
			tp.closed_tasks
		FROM task_stats ts
		CROSS JOIN comment_stats cs
		LEFT JOIN top_performers tp ON TRUE
	`

	rows, err := r.db.QueryContext(ctx, query, teamID, teamID, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: query stats: %w", op, err)
	}
	defer rows.Close()

	stats := &domain.TeamStats{
		TasksByStatus: make(map[domain.TaskStatus]int),
		TopPerformers: make([]domain.TopPerformer, 0, 3),
	}

	for rows.Next() {
		var (
			totalTasks    int
			todoCount     int
			inProgressCnt int
			reviewCount   int
			doneCount     int
			cancelledCnt  int
			avgCloseTime  sql.NullFloat64
			totalComments int
			userID        sql.NullString
			userName      sql.NullString
			closedTasks   sql.NullInt64
		)

		if err := rows.Scan(
			&totalTasks,
			&todoCount,
			&inProgressCnt,
			&reviewCount,
			&doneCount,
			&cancelledCnt,
			&avgCloseTime,
			&totalComments,
			&userID,
			&userName,
			&closedTasks,
		); err != nil {
			return nil, fmt.Errorf("%s: scan stats: %w", op, err)
		}

		// Set totals (same for every row)
		stats.TotalTasks = totalTasks
		stats.TasksByStatus[domain.TaskStatusTodo] = todoCount
		stats.TasksByStatus[domain.TaskStatusInProgress] = inProgressCnt
		stats.TasksByStatus[domain.TaskStatusReview] = reviewCount
		stats.TasksByStatus[domain.TaskStatusDone] = doneCount
		stats.TasksByStatus[domain.TaskStatusCancelled] = cancelledCnt
		stats.TotalComments = totalComments

		if avgCloseTime.Valid {
			stats.AvgCloseTime = avgCloseTime.Float64
		}

		// Add top performer if exists
		if userID.Valid && userName.Valid && closedTasks.Valid {
			stats.TopPerformers = append(stats.TopPerformers, domain.TopPerformer{
				UserID:      userID.String,
				UserName:    userName.String,
				ClosedTasks: int(closedTasks.Int64),
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: rows iteration: %w", op, err)
	}

	return stats, nil
}
