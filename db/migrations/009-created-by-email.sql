-- Add created_by_email to store the email of who created the quote
ALTER TABLE quotes ADD COLUMN created_by_email TEXT;

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (009, '009-created-by-email');
