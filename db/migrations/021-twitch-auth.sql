-- Add Twitch authentication support for moderators
-- Moderators can now be identified by Twitch username instead of email

-- Add twitch columns to moderators table
ALTER TABLE nightbot_channel_moderators ADD COLUMN twitch_id TEXT;
ALTER TABLE nightbot_channel_moderators ADD COLUMN twitch_username TEXT;

-- Make user_email nullable (moderators can be added by twitch username only)
-- SQLite doesn't support ALTER COLUMN, but existing rows already have emails
-- New rows can have NULL email if twitch_username is set

-- Index for Twitch username lookups
CREATE INDEX IF NOT EXISTS idx_nightbot_moderators_twitch ON nightbot_channel_moderators(twitch_username);

-- Sessions table for Twitch-authenticated users
CREATE TABLE IF NOT EXISTS twitch_sessions (
    id TEXT PRIMARY KEY,  -- session token
    twitch_id TEXT NOT NULL,
    twitch_username TEXT NOT NULL,
    display_name TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_twitch_sessions_expires ON twitch_sessions(expires_at);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (21, '021-twitch-auth');
