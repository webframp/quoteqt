-- Add shortname column to civilizations
ALTER TABLE civilizations ADD COLUMN shortname TEXT;

-- Update shortnames based on aoe4world.com conventions
UPDATE civilizations SET shortname = 'abbasid' WHERE name = 'Abbasid Dynasty';
UPDATE civilizations SET shortname = 'ayyubids' WHERE name = 'Ayyubids';
UPDATE civilizations SET shortname = 'byzantines' WHERE name = 'Byzantines';
UPDATE civilizations SET shortname = 'chinese' WHERE name = 'Chinese';
UPDATE civilizations SET shortname = 'delhi' WHERE name = 'Delhi Sultanate';
UPDATE civilizations SET shortname = 'english' WHERE name = 'English';
UPDATE civilizations SET shortname = 'french' WHERE name = 'French';
UPDATE civilizations SET shortname = 'goldenhorde' WHERE name = 'Golden Horde';
UPDATE civilizations SET shortname = 'hre' WHERE name = 'Holy Roman Empire';
UPDATE civilizations SET shortname = 'japanese' WHERE name = 'Japanese';
UPDATE civilizations SET shortname = 'jeannedarc' WHERE name = 'Jeanne d''Arc';
UPDATE civilizations SET shortname = 'macedonian' WHERE name = 'Macedonian Dynasty';
UPDATE civilizations SET shortname = 'malians' WHERE name = 'Malians';
UPDATE civilizations SET shortname = 'mongols' WHERE name = 'Mongols';
UPDATE civilizations SET shortname = 'orderofthedragon' WHERE name = 'Order of the Dragon';
UPDATE civilizations SET shortname = 'ottomans' WHERE name = 'Ottomans';
UPDATE civilizations SET shortname = 'rus' WHERE name = 'Rus';
UPDATE civilizations SET shortname = 'sengoku' WHERE name = 'Sengoku Daimyo';
UPDATE civilizations SET shortname = 'tughlaq' WHERE name = 'Tughlaq Dynasty';
UPDATE civilizations SET shortname = 'zhuxi' WHERE name = 'Zhu Xi''s Legacy';

CREATE INDEX IF NOT EXISTS idx_civilizations_shortname ON civilizations(shortname);

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (005, '005-civ-shortnames');
