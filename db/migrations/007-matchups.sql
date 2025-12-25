-- Add opponent civilization for matchup-specific quotes
ALTER TABLE quotes ADD COLUMN opponent_civ TEXT;

CREATE INDEX IF NOT EXISTS idx_quotes_matchup ON quotes(civilization, opponent_civ);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (007, '007-matchups');
