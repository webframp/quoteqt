-- Channel owners: users who can manage quotes for specific channels
CREATE TABLE IF NOT EXISTS channel_owners (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    user_email TEXT NOT NULL,
    invited_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    invited_by TEXT NOT NULL,
    UNIQUE(channel, user_email)
);

-- Index for looking up channels by user
CREATE INDEX IF NOT EXISTS idx_channel_owners_email ON channel_owners(user_email);
