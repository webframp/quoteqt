-- Civilizations reference table
CREATE TABLE IF NOT EXISTS civilizations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    variant_of TEXT,
    dlc TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Insert all civilizations
INSERT OR IGNORE INTO civilizations (name, variant_of, dlc) VALUES
    ('Abbasid Dynasty', NULL, NULL),
    ('Chinese', NULL, NULL),
    ('Delhi Sultanate', NULL, NULL),
    ('English', NULL, NULL),
    ('French', NULL, NULL),
    ('Holy Roman Empire', NULL, NULL),
    ('Mongols', NULL, NULL),
    ('Rus', NULL, NULL),
    ('Malians', NULL, 'Anniversary Edition'),
    ('Ottomans', NULL, 'Anniversary Edition'),
    ('Ayyubids', NULL, 'The Sultans Ascend'),
    ('Byzantines', NULL, 'The Sultans Ascend'),
    ('Japanese', NULL, 'The Sultans Ascend'),
    ('Jeanne d''Arc', 'French', 'The Sultans Ascend'),
    ('Order of the Dragon', 'Holy Roman Empire', 'The Sultans Ascend'),
    ('Zhu Xi''s Legacy', 'Chinese', 'The Sultans Ascend'),
    ('Golden Horde', 'Mongols', 'Dynasties of the East'),
    ('Macedonian Dynasty', 'Byzantines', 'Dynasties of the East'),
    ('Sengoku Daimyo', 'Japanese', 'Dynasties of the East'),
    ('Tughlaq Dynasty', 'Delhi Sultanate', 'Dynasties of the East');

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (004, '004-civilizations');
