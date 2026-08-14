ALTER TABLE tasks
    DROP INDEX idx_tasks_team_priority_created,
    DROP INDEX idx_tasks_team_due,
    DROP COLUMN due_at,
    DROP COLUMN priority;
