-- Distinguish task creation from later edits in the audit log.
ALTER TABLE task_history
    ADD COLUMN action ENUM('created', 'updated') NOT NULL DEFAULT 'updated' AFTER changed_by;
