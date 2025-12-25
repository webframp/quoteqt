-- Add civilization column to quotes
ALTER TABLE quotes ADD COLUMN civilization TEXT;

CREATE INDEX IF NOT EXISTS idx_quotes_civilization ON quotes(civilization);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (003, '003-civilization');
