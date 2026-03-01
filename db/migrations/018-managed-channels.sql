-- Managed channels for automated Nightbot backup via session tokens
-- These are channels where a user has manager access but not owner access,
-- so OAuth won't work. Instead, we store their browser session token.

CREATE TABLE nightbot_managed_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_email TEXT NOT NULL,              -- admin who configured this
    channel_id TEXT NOT NULL UNIQUE,       -- Nightbot-Channel header value
    channel_name TEXT NOT NULL,            -- display name for UI
    session_token_encrypted TEXT NOT NULL, -- AES-GCM encrypted session token
    sync_enabled INTEGER NOT NULL DEFAULT 1,
    sync_interval_minutes INTEGER NOT NULL DEFAULT 60,
    last_sync_at DATETIME,
    last_sync_status TEXT,                 -- 'success', 'auth_failed', 'api_error', 'disabled'
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_managed_channels_user ON nightbot_managed_channels(user_email);
CREATE INDEX idx_managed_channels_sync ON nightbot_managed_channels(sync_enabled, last_sync_at);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (18, '018-managed-channels');
