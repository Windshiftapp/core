-- User-authored OpenUI dashboard widgets
-- migration: 20261107_custom_widgets

CREATE TABLE IF NOT EXISTS custom_widgets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER,
    owner_user_id INTEGER,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    program TEXT NOT NULL,
    lang_version TEXT NOT NULL DEFAULT '',
    library_version TEXT NOT NULL DEFAULT '',
    queries TEXT NOT NULL DEFAULT '{}',
    visibility TEXT NOT NULL DEFAULT 'private',
    created_by INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_custom_widgets_owner ON custom_widgets(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_custom_widgets_workspace ON custom_widgets(workspace_id);
