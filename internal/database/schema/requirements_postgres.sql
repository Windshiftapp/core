-- Workspace-scoped requirements backed by Knowledge Pages (PostgreSQL).
-- Mirror of requirements.sql with SERIAL keys and TIMESTAMPTZ.
-- migration: 20260915_requirements

CREATE TABLE IF NOT EXISTS requirement_sequences (
    workspace_id INTEGER PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    last_number INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS requirements (
    id SERIAL PRIMARY KEY,
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
    owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_by INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (page_id, workspace_id) REFERENCES pages(id, workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    UNIQUE (workspace_id, requirement_number)
);

CREATE INDEX IF NOT EXISTS idx_requirements_workspace_id ON requirements(workspace_id);
CREATE INDEX IF NOT EXISTS idx_requirements_workspace_status ON requirements(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_requirements_owner_id ON requirements(owner_id);

CREATE TABLE IF NOT EXISTS requirement_history (
    id SERIAL PRIMARY KEY,
    requirement_id INTEGER NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    field_name TEXT NOT NULL,
    old_value TEXT,
    new_value TEXT
);

CREATE INDEX IF NOT EXISTS idx_requirement_history_requirement_id_changed_at
    ON requirement_history(requirement_id, changed_at DESC);
