-- Add channel column for multi-streamer support
-- NULL means global (available to all channels)
-- Non-null means channel-specific quote
ALTER TABLE quotes ADD COLUMN channel TEXT;

CREATE INDEX IF NOT EXISTS idx_quotes_channel ON quotes(channel);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (008, '008-channel');
