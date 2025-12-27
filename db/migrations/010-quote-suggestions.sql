-- Quote suggestions from anonymous users
CREATE TABLE IF NOT EXISTS quote_suggestions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    text TEXT NOT NULL,
    author TEXT,
    civilization TEXT,
    opponent_civ TEXT,
    channel TEXT NOT NULL,
    submitted_by_ip TEXT NOT NULL,
    submitted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_by TEXT,
    reviewed_at DATETIME
);

-- Index for listing pending suggestions by channel
CREATE INDEX IF NOT EXISTS idx_suggestions_channel_status ON quote_suggestions(channel, status);
