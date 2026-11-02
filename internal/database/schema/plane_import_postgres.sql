-- Plane import idempotency and provenance map (PostgreSQL version).
-- One row per imported Plane entity, keyed on the source uuid, so a rerun
-- skips what a previous run already wrote and can reconcile provenance.
-- migration: 20261119_plane_import_mappings
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
