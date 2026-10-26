-- Delta schema for running plane-import against a Windshift deployment that
-- predates the current migration catalog (measured on the deployed DB: 42
-- migrations applied, max 20260918).
--
-- Every statement below is copied from the upstream migration that owns the
-- object, so a later upgrade finds it already present (all are IF NOT EXISTS)
-- and stamps the catalog itself. Nothing here touches schema_migrations, so
-- the running application's checksums stay exactly as they were.
--
-- items.team_id            <- 20260925_items_team (teams table already exists)
-- items.intake_id          <- 20261110 intake split (plain INTEGER, no FK)
-- items.portal_org_shared  <- 20261010_portal_org_sharing
-- plane_import_mappings    <- 20261111_plane_import_mappings (this importer)

BEGIN;
ALTER TABLE items ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_items_team_id ON items(team_id);
ALTER TABLE items ADD COLUMN IF NOT EXISTS intake_id INTEGER;
ALTER TABLE items ADD COLUMN IF NOT EXISTS portal_org_shared BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS plane_import_mappings (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
	entity_type TEXT NOT NULL,
	external_id TEXT NOT NULL,
	windshift_id INTEGER NOT NULL,
	metadata_json TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(workspace_id, entity_type, external_id)
);
CREATE INDEX IF NOT EXISTS idx_plane_import_mappings_windshift ON plane_import_mappings(workspace_id, entity_type, windshift_id);
COMMIT;
