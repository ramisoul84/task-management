-- Composite indexes matching the hot query shapes. Each one leads with the
-- column the matching foreign key needs, so InnoDB reuses them for the FK
-- and drops its implicit single-column index.

-- GET /tasks: WHERE team_id [AND status | AND assignee_id] ORDER BY created_at DESC, id DESC
ALTER TABLE tasks
    ADD INDEX idx_tasks_team_created (team_id, created_at, id),
    ADD INDEX idx_tasks_team_status_created (team_id, status, created_at, id),
    ADD INDEX idx_tasks_team_assignee_created (team_id, assignee_id, created_at, id),
    -- stats: done tasks closed in the last 30 days
    ADD INDEX idx_tasks_team_status_closed (team_id, status, closed_at);

-- GET /tasks/{id}/history and /comments, ordered by time
ALTER TABLE task_history
    ADD INDEX idx_task_history_task_created (task_id, created_at, id);

ALTER TABLE task_comments
    ADD INDEX idx_task_comments_task_created (task_id, created_at, id);
