-- Workspace-scoped requirements backed by Knowledge Pages.
-- Ordinary wiki pages have no row here. Promotion allocates an immutable
-- workspace-local number; sequences only increase so archived or deleted
-- numbers are never reused.
-- migration: 20260915_requirements

CREATE TABLE IF NOT EXISTS requirement_sequences (
    workspace_id INTEGER PRIMARY KEY,
    last_number INTEGER NOT NULL,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS requirements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id INTEGER NOT NULL UNIQUE,
    workspace_id INTEGER NOT NULL,
    requirement_number INTEGER NOT NULL,
    requirement_type TEXT NOT NULL CHECK (requirement_type IN (
        'business_requirement',
        'functional_requirement',
        'non_functional_requirement',
        'business_rule',
        'use_case',
        'business_process',
        'system_specification',
        'api_specification',
        'data_model',
        'architecture_decision',
        'glossary_entry'
    )),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
        'draft',
        'in_review',
        'approved',
        'deprecated'
    )),
    owner_id INTEGER,
    created_by INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    updated_by INTEGER,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (page_id, workspace_id) REFERENCES pages(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE SET NULL,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT,
    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (workspace_id, requirement_number)
);

CREATE INDEX IF NOT EXISTS idx_requirements_workspace_id ON requirements(workspace_id);
CREATE INDEX IF NOT EXISTS idx_requirements_workspace_status ON requirements(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_requirements_owner_id ON requirements(owner_id);

CREATE TABLE IF NOT EXISTS requirement_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    requirement_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    field_name TEXT NOT NULL,
    old_value TEXT,
    new_value TEXT,
    FOREIGN KEY (requirement_id) REFERENCES requirements(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_requirement_history_requirement_id_changed_at
    ON requirement_history(requirement_id, changed_at DESC);
