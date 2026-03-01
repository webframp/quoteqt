-- Channel moderators for Nightbot backup viewing
-- Moderators can view snapshots/diffs for channels they're assigned to

CREATE TABLE nightbot_channel_moderators (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_name TEXT NOT NULL,
    user_email TEXT NOT NULL,
    added_by TEXT NOT NULL,
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(channel_name, user_email)
);

CREATE INDEX idx_nightbot_moderators_email ON nightbot_channel_moderators(user_email);
CREATE INDEX idx_nightbot_moderators_channel ON nightbot_channel_moderators(channel_name);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (19, '019-nightbot-moderators');
