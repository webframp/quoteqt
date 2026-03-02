-- Fix moderator table to allow multiple Twitch-only moderators per channel
-- The original unique constraint on (channel_name, user_email) doesn't work
-- when user_email is empty for Twitch-based moderators.

-- SQLite doesn't support dropping constraints, so we need to recreate the table

-- Create new table with better constraints
CREATE TABLE nightbot_channel_moderators_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_name TEXT NOT NULL,
    user_email TEXT,  -- NULL for Twitch-only moderators
    twitch_id TEXT,
    twitch_username TEXT,
    added_by TEXT NOT NULL,
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Each user can only be added once per channel (by email OR twitch)
    UNIQUE(channel_name, user_email),
    UNIQUE(channel_name, twitch_username)
);

-- Copy existing data
INSERT INTO nightbot_channel_moderators_new 
    (id, channel_name, user_email, twitch_id, twitch_username, added_by, added_at)
SELECT 
    id, 
    channel_name, 
    CASE WHEN user_email = '' THEN NULL ELSE user_email END,
    twitch_id,
    twitch_username,
    added_by,
    added_at
FROM nightbot_channel_moderators;

-- Drop old table and rename new one
DROP TABLE nightbot_channel_moderators;
ALTER TABLE nightbot_channel_moderators_new RENAME TO nightbot_channel_moderators;

-- Recreate indexes
CREATE INDEX idx_nightbot_moderators_email ON nightbot_channel_moderators(user_email);
CREATE INDEX idx_nightbot_moderators_channel ON nightbot_channel_moderators(channel_name);
CREATE INDEX idx_nightbot_moderators_twitch ON nightbot_channel_moderators(twitch_username);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (22, '022-fix-moderator-constraint');
