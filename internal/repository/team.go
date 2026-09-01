package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/pkg/database"
)

type TeamRepository interface {
	Create(ctx context.Context, team *domain.Team) error
	ListByUser(ctx context.Context, userID string) ([]*domain.Team, error)
	GetByID(ctx context.Context, id string) (*domain.Team, error)
	Rename(ctx context.Context, id, name string) error
	// Delete removes the team; memberships, tasks, history and comments
	// cascade.
	Delete(ctx context.Context, id string) error

	AddMember(ctx context.Context, member *domain.TeamMember) error
	// GetMemberRole returns ErrNotFound when userID is not in the team.
	GetMemberRole(ctx context.Context, teamID, userID string) (domain.Role, error)
	// GetMemberRoleForUpdate is GetMemberRole with a row lock; call it
	// inside a transaction.
	GetMemberRoleForUpdate(ctx context.Context, teamID, userID string) (domain.Role, error)
	IsMember(ctx context.Context, teamID, userID string) (bool, error)
	UpdateMemberRole(ctx context.Context, teamID, userID string, role domain.Role) error
	RemoveMember(ctx context.Context, teamID, userID string) error
	ListMembers(ctx context.Context, teamID string) ([]*domain.TeamMemberResponse, error)

	GetStats(ctx context.Context, teamID string) (*domain.TeamStats, error)
}

type teamRepo struct {
	db *database.DB
}

func NewTeamRepository(db *database.DB) TeamRepository {
	return &teamRepo{db: db}
}

func (r *teamRepo) Create(ctx context.Context, t *domain.Team) error {
	const q = `INSERT INTO teams (id, name, created_by, created_at) VALUES (?, ?, ?, ?)`

	if _, err := r.db.Q(ctx).ExecContext(ctx, q, t.ID, t.Name, t.CreatedBy, t.CreatedAt); err != nil {
		return fmt.Errorf("teamRepo.Create: %w", err)
	}
	return nil
}

// ListByUser returns the teams userID belongs to, with the caller's role.
func (r *teamRepo) ListByUser(ctx context.Context, userID string) ([]*domain.Team, error) {
	const q = `
		SELECT t.id, t.name, t.created_by, t.created_at, tm.role
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id
		WHERE tm.user_id = ?
		ORDER BY t.created_at DESC, t.id DESC`

	teams := make([]*domain.Team, 0)
	if err := r.db.Q(ctx).SelectContext(ctx, &teams, q, userID); err != nil {
		return nil, fmt.Errorf("teamRepo.ListByUser: %w", err)
	}
	return teams, nil
}

func (r *teamRepo) GetByID(ctx context.Context, id string) (*domain.Team, error) {
	var t domain.Team
	err := r.db.Q(ctx).GetContext(ctx, &t, `SELECT id, name, created_by, created_at FROM teams WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound.WithMessage("team not found")
	}
	if err != nil {
		return nil, fmt.Errorf("teamRepo.GetByID: %w", err)
	}
	return &t, nil
}

func (r *teamRepo) Rename(ctx context.Context, id, name string) error {
	res, err := r.db.Q(ctx).ExecContext(ctx, `UPDATE teams SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("teamRepo.Rename: %w", err)
	}
	if _, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("teamRepo.Rename: rows affected: %w", err)
	}
	return nil
}

func (r *teamRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.Q(ctx).ExecContext(ctx, `DELETE FROM teams WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("teamRepo.Delete: %w", err)
	}
	return requireAffected(res, domain.ErrNotFound.WithMessage("team not found"))
}

func (r *teamRepo) AddMember(ctx context.Context, m *domain.TeamMember) error {
	const q = `INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`

	if _, err := r.db.Q(ctx).ExecContext(ctx, q, m.TeamID, m.UserID, m.Role); err != nil {
		switch {
		case database.IsDuplicateKey(err):
			return domain.ErrAlreadyExists.WithMessage("user is already a member of this team")
		case database.IsForeignKeyViolation(err):
			return domain.ErrNotFound.WithMessage("team or user not found")
		}
		return fmt.Errorf("teamRepo.AddMember: %w", err)
	}
	return nil
}

func (r *teamRepo) GetMemberRole(ctx context.Context, teamID, userID string) (domain.Role, error) {
	return r.getRole(ctx, `SELECT role FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID)
}

func (r *teamRepo) GetMemberRoleForUpdate(ctx context.Context, teamID, userID string) (domain.Role, error) {
	return r.getRole(ctx, `SELECT role FROM team_members WHERE team_id = ? AND user_id = ? FOR UPDATE`, teamID, userID)
}

func (r *teamRepo) getRole(ctx context.Context, q, teamID, userID string) (domain.Role, error) {
	var role domain.Role
	if err := r.db.Q(ctx).GetContext(ctx, &role, q, teamID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrNotFound.WithMessage("user is not a member of this team")
		}
		return "", fmt.Errorf("teamRepo.getRole: %w", err)
	}
	return role, nil
}

func (r *teamRepo) IsMember(ctx context.Context, teamID, userID string) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM team_members WHERE team_id = ? AND user_id = ?)`

	var ok bool
	if err := r.db.Q(ctx).GetContext(ctx, &ok, q, teamID, userID); err != nil {
		return false, fmt.Errorf("teamRepo.IsMember: %w", err)
	}
	return ok, nil
}

func (r *teamRepo) UpdateMemberRole(ctx context.Context, teamID, userID string, role domain.Role) error {
	const q = `UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?`

	res, err := r.db.Q(ctx).ExecContext(ctx, q, role, teamID, userID)
	if err != nil {
		return fmt.Errorf("teamRepo.UpdateMemberRole: %w", err)
	}
	return requireAffected(res, domain.ErrNotFound.WithMessage("member not found"))
}

func (r *teamRepo) RemoveMember(ctx context.Context, teamID, userID string) error {
	const q = `DELETE FROM team_members WHERE team_id = ? AND user_id = ?`

	res, err := r.db.Q(ctx).ExecContext(ctx, q, teamID, userID)
	if err != nil {
		return fmt.Errorf("teamRepo.RemoveMember: %w", err)
	}
	return requireAffected(res, domain.ErrNotFound.WithMessage("member not found"))
}

func (r *teamRepo) ListMembers(ctx context.Context, teamID string) ([]*domain.TeamMemberResponse, error) {
	const q = `
		SELECT u.id AS user_id, u.email, u.name, tm.role
		FROM team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ?
		ORDER BY FIELD(tm.role, 'owner', 'admin', 'member'), u.name`

	members := make([]*domain.TeamMemberResponse, 0)
	if err := r.db.Q(ctx).SelectContext(ctx, &members, q, teamID); err != nil {
		return nil, fmt.Errorf("teamRepo.ListMembers: %w", err)
	}
	return members, nil
}

// GetStats builds the team report in a single round-trip: one statement
// with CTEs, JOINs, GROUP BY, aggregates and date arithmetic (no N+1).
//
// The result has one row per top performer (or a single row with NULL
// performer columns when nobody closed a task in the last 30 days); the
// aggregate columns repeat on every row.
func (r *teamRepo) GetStats(ctx context.Context, teamID string) (*domain.TeamStats, error) {
	const q = `
		WITH team_tasks AS (
			SELECT id, status, assignee_id, created_at, closed_at, due_at
			FROM tasks
			WHERE team_id = ?
		),
		status_counts AS (
			SELECT
				COUNT(*)                                     AS total_tasks,
				COALESCE(SUM(status = 'todo'), 0)            AS todo_count,
				COALESCE(SUM(status = 'in_progress'), 0)     AS in_progress_count,
				COALESCE(SUM(status = 'review'), 0)          AS review_count,
				COALESCE(SUM(status = 'done'), 0)            AS done_count,
				COALESCE(SUM(status = 'cancelled'), 0)       AS cancelled_count,
				COALESCE(SUM(due_at < UTC_TIMESTAMP()
				             AND status NOT IN ('done', 'cancelled')), 0)
				                                             AS overdue_count,
				AVG(CASE WHEN status = 'done' AND closed_at IS NOT NULL
				         THEN TIMESTAMPDIFF(SECOND, created_at, closed_at) END) / 3600.0
				                                             AS avg_close_hours
			FROM team_tasks
		),
		comment_counts AS (
			SELECT COUNT(c.id) AS total_comments
			FROM task_comments c
			JOIN team_tasks t ON t.id = c.task_id
		),
		top_performers AS (
			SELECT u.id AS user_id, u.name AS user_name, COUNT(*) AS closed_tasks
			FROM team_tasks t
			JOIN users u ON u.id = t.assignee_id
			WHERE t.status = 'done'
			  AND t.closed_at >= UTC_TIMESTAMP() - INTERVAL 30 DAY
			GROUP BY u.id, u.name
			ORDER BY closed_tasks DESC, u.name, u.id
			LIMIT 3
		)
		SELECT
			s.total_tasks, s.todo_count, s.in_progress_count, s.review_count,
			s.done_count, s.cancelled_count, s.overdue_count, s.avg_close_hours,
			cc.total_comments,
			tp.user_id, tp.user_name, tp.closed_tasks
		FROM status_counts s
		CROSS JOIN comment_counts cc
		LEFT JOIN top_performers tp ON TRUE
		ORDER BY tp.closed_tasks DESC, tp.user_name, tp.user_id`

	rows, err := r.db.Q(ctx).QueryContext(ctx, q, teamID)
	if err != nil {
		return nil, fmt.Errorf("teamRepo.GetStats: query: %w", err)
	}
	defer rows.Close()

	stats := &domain.TeamStats{
		TasksByStatus: make(map[domain.TaskStatus]int, len(domain.TaskStatuses)),
		TopPerformers: make([]domain.TopPerformer, 0, 3),
	}

	for rows.Next() {
		var (
			todo, inProgress, review, done, cancelled int
			avgHours                                  sql.NullFloat64
			userID, userName                          sql.NullString
			closed                                    sql.NullInt64
		)
		if err := rows.Scan(
			&stats.TotalTasks, &todo, &inProgress, &review, &done, &cancelled, &stats.OverdueTasks, &avgHours,
			&stats.TotalComments,
			&userID, &userName, &closed,
		); err != nil {
			return nil, fmt.Errorf("teamRepo.GetStats: scan: %w", err)
		}

		stats.TasksByStatus[domain.TaskStatusTodo] = todo
		stats.TasksByStatus[domain.TaskStatusInProgress] = inProgress
		stats.TasksByStatus[domain.TaskStatusReview] = review
		stats.TasksByStatus[domain.TaskStatusDone] = done
		stats.TasksByStatus[domain.TaskStatusCancelled] = cancelled
		stats.AvgCloseTime = avgHours.Float64

		if userID.Valid {
			stats.TopPerformers = append(stats.TopPerformers, domain.TopPerformer{
				UserID:      userID.String,
				UserName:    userName.String,
				ClosedTasks: int(closed.Int64),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("teamRepo.GetStats: rows: %w", err)
	}

	return stats, nil
}

// requireAffected turns "0 rows affected" into notFound.
func requireAffected(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}
