-- Add Knights of Cross and Rose DLC civilizations
INSERT OR IGNORE INTO civilizations (name, shortname, variant_of, dlc) VALUES
    ('House of Lancaster', 'lancaster', 'English', 'Knights of Cross and Rose'),
    ('Knights Templar', 'templar', NULL, 'Knights of Cross and Rose');

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (006, '006-knights-cross-rose');
