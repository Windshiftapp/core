package database

const boardPublicationMigration = `
ALTER TABLE collections ADD COLUMN board_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE collections ADD COLUMN board_standalone_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE collections ADD COLUMN board_ever_published BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE collections ADD COLUMN sharing_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE collections ADD COLUMN public_revoked_at TEXT;
ALTER TABLE collections ADD COLUMN public_revoked_by INTEGER REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE collections ADD COLUMN public_revoke_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE collections ADD COLUMN sharing_changed_at TEXT;
ALTER TABLE collections ADD COLUMN sharing_changed_by INTEGER REFERENCES users(id) ON DELETE SET NULL;
UPDATE collections SET board_enabled = is_public, board_ever_published = is_public;
INSERT INTO system_settings (key, value, value_type, description, category)
VALUES ('public_boards_enabled', 'true', 'boolean', 'Allow external board publications', 'security')
ON CONFLICT(key) DO NOTHING;
`

const boardPlacementSchema = `
CREATE TABLE IF NOT EXISTS board_placements (
 id TEXT PRIMARY KEY,
 collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
 channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
 label TEXT NOT NULL,
 display_order INTEGER NOT NULL DEFAULT 0,
 enabled BOOLEAN NOT NULL DEFAULT true,
 access_mode TEXT NOT NULL DEFAULT 'public' CHECK(access_mode IN ('public', 'organisations')),
 organisation_ids TEXT NOT NULL DEFAULT '[]',
 revision INTEGER NOT NULL DEFAULT 1,
 UNIQUE(collection_id, channel_id)
);
CREATE INDEX IF NOT EXISTS idx_board_placements_channel ON board_placements(channel_id, display_order);
`
