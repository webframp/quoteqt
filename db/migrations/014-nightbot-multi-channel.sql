-- Allow multiple Nightbot channel connections per user
-- Drop the old unique constraint and add a composite one

-- Create new table with correct constraints
CREATE TABLE nightbot_tokens_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_email TEXT NOT NULL,
    channel_name TEXT NOT NULL,
    channel_display_name TEXT,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_email, channel_name)
);

-- Copy existing data
INSERT INTO nightbot_tokens_new (id, user_email, channel_name, channel_display_name, access_token, refresh_token, expires_at, created_at, updated_at)
SELECT id, user_email, COALESCE(channel_name, 'unknown'), channel_display_name, access_token, refresh_token, expires_at, created_at, updated_at
FROM nightbot_tokens;

-- Drop old table and rename
DROP TABLE nightbot_tokens;
ALTER TABLE nightbot_tokens_new RENAME TO nightbot_tokens;

-- Recreate index
CREATE INDEX idx_nightbot_tokens_email ON nightbot_tokens(user_email);
