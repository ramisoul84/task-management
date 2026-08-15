-- due_at is DATETIME, not TIMESTAMP: TIMESTAMP stops at 2038-01-19 and due
-- dates are routinely set years ahead. Values are stored in UTC (the
-- application pins the session time zone).
ALTER TABLE tasks
    ADD COLUMN priority ENUM('low', 'medium', 'high', 'urgent') NOT NULL DEFAULT 'medium' AFTER status,
    ADD COLUMN due_at DATETIME NULL AFTER priority,
    ADD INDEX idx_tasks_team_due (team_id, due_at),
    ADD INDEX idx_tasks_team_priority_created (team_id, priority, created_at, id);
