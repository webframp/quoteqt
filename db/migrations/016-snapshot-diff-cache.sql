-- Cache diff results on snapshots for display in list view
ALTER TABLE nightbot_snapshots ADD COLUMN last_diff_added INTEGER;
ALTER TABLE nightbot_snapshots ADD COLUMN last_diff_removed INTEGER;
ALTER TABLE nightbot_snapshots ADD COLUMN last_diff_modified INTEGER;
ALTER TABLE nightbot_snapshots ADD COLUMN last_diff_at DATETIME;

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (016, '016-snapshot-diff-cache');
