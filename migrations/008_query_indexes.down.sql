-- Re-add plain FK indexes first: an index backing a foreign key cannot be
-- dropped unless another index can take over.
ALTER TABLE tasks
    ADD INDEX team_id (team_id),
    DROP INDEX idx_tasks_team_created,
    DROP INDEX idx_tasks_team_status_created,
    DROP INDEX idx_tasks_team_assignee_created,
    DROP INDEX idx_tasks_team_status_closed;

ALTER TABLE task_history
    ADD INDEX task_id (task_id),
    DROP INDEX idx_task_history_task_created;

ALTER TABLE task_comments
    ADD INDEX task_id (task_id),
    DROP INDEX idx_task_comments_task_created;
