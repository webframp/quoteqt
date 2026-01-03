-- Store Nightbot command snapshots for history/versioning
CREATE TABLE nightbot_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_name TEXT NOT NULL,
    snapshot_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    command_count INTEGER NOT NULL,
    commands_json TEXT NOT NULL,  -- JSON array of commands
    created_by TEXT NOT NULL,     -- user email who saved the snapshot
    note TEXT                     -- optional note about this snapshot
);

CREATE INDEX idx_nightbot_snapshots_channel ON nightbot_snapshots(channel_name);
CREATE INDEX idx_nightbot_snapshots_at ON nightbot_snapshots(snapshot_at DESC);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (015, '015-nightbot-snapshots');
