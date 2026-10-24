-- User-authored OpenUI dashboard widgets (PostgreSQL)
-- migration: 20261107_custom_widgets

CREATE TABLE IF NOT EXISTS custom_widgets (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    program TEXT NOT NULL,
    lang_version TEXT NOT NULL DEFAULT '',
    library_version TEXT NOT NULL DEFAULT '',
    queries TEXT NOT NULL DEFAULT '{}',
    visibility TEXT NOT NULL DEFAULT 'private',
    created_by INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_custom_widgets_owner ON custom_widgets(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_custom_widgets_workspace ON custom_widgets(workspace_id);
