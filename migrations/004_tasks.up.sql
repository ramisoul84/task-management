CREATE TABLE IF NOT EXISTS tasks (
    id CHAR(36) PRIMARY KEY,
    team_id CHAR(36) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status ENUM('todo', 'in_progress', 'review', 'done', 'cancelled') NOT NULL DEFAULT 'todo',
    created_by CHAR(36) NOT NULL,
    assignee_id CHAR(36),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    closed_at TIMESTAMP NULL,
    version INT NOT NULL DEFAULT 0,

    FOREIGN KEY (team_id)
        REFERENCES teams(id)
        ON DELETE CASCADE,
    FOREIGN KEY (created_by)
        REFERENCES users(id)
        ON DELETE RESTRICT,
    FOREIGN KEY (assignee_id)
        REFERENCES users(id)
        ON DELETE SET NULL
);