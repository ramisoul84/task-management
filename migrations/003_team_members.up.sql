CREATE TABLE IF NOT EXISTS team_members (
    team_id CHAR(36) NOT NULL,
    user_id CHAR(36) NOT NULL,
    role ENUM('owner', 'admin', 'member') NOT NULL DEFAULT 'member',

    PRIMARY KEY (team_id, user_id),

    FOREIGN KEY (team_id)
        REFERENCES teams(id)
        ON DELETE CASCADE,

    FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE RESTRICT
);