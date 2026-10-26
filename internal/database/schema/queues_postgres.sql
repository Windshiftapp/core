-- Support queues (WI-1603): user-defined work-item filters scoped to a
-- collection or the workspace default view (collection_id NULL). Built-in
-- presets are virtual and never seeded; a row with builtin_key set and
-- is_hidden = true dismisses that preset for the scope. A collection scope is
-- identified by collection_id alone; workspace_id is NULL for a collection
-- that spans multiple workspaces.

CREATE TABLE IF NOT EXISTS queues (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT,
    collection_id BIGINT,
    name TEXT NOT NULL,
    ql_query TEXT NOT NULL,
    filter_state TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_by BIGINT,
    builtin_key TEXT,
    is_hidden BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_queues_workspace_scope ON queues(workspace_id, collection_id, position);
CREATE INDEX IF NOT EXISTS idx_queues_collection_scope ON queues(collection_id, position);
CREATE UNIQUE INDEX IF NOT EXISTS uq_queues_ws_builtin ON queues(workspace_id, builtin_key) WHERE collection_id IS NULL AND builtin_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_queues_coll_builtin ON queues(collection_id, builtin_key) WHERE collection_id IS NOT NULL AND builtin_key IS NOT NULL;
