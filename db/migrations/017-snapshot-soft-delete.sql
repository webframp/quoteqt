-- Add soft delete support for snapshots
-- Deleted snapshots can be restored within 14 days

ALTER TABLE nightbot_snapshots ADD COLUMN deleted_at DATETIME;
ALTER TABLE nightbot_snapshots ADD COLUMN deleted_by TEXT;

CREATE INDEX idx_nightbot_snapshots_deleted ON nightbot_snapshots(deleted_at);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (017, '017-snapshot-soft-delete');
