-- Jira Cloud Import tables (PostgreSQL version)
-- Tables for managing Jira import connections, jobs, and ID mappings

-- Stores encrypted Jira connection credentials
CREATE TABLE IF NOT EXISTS jira_import_connections (
    id TEXT PRIMARY KEY,
    instance_url TEXT NOT NULL,
    email TEXT NOT NULL,
    encrypted_credentials TEXT NOT NULL,
    instance_name TEXT,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jira_connections_created_by ON jira_import_connections(created_by);

-- Import job tracking
CREATE TABLE IF NOT EXISTS jira_import_jobs (
    id TEXT PRIMARY KEY,
    connection_id TEXT NOT NULL REFERENCES jira_import_connections(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued',
    phase TEXT,
    scope TEXT NOT NULL DEFAULT 'work_items',
    config_json TEXT NOT NULL,
    progress_json TEXT,
    result_json TEXT,
    error_message TEXT,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jira_jobs_connection_id ON jira_import_jobs(connection_id);
CREATE INDEX IF NOT EXISTS idx_jira_jobs_status ON jira_import_jobs(status);
CREATE INDEX IF NOT EXISTS idx_jira_jobs_created_by ON jira_import_jobs(created_by);

-- ID mappings for imported entities (preserves references)
CREATE TABLE IF NOT EXISTS jira_import_id_mappings (
    id SERIAL PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jira_import_jobs(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    jira_id TEXT NOT NULL,
    jira_key TEXT,
    windshift_id INTEGER NOT NULL,
    metadata_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(job_id, entity_type, jira_id)
);

CREATE INDEX IF NOT EXISTS idx_jira_mappings_lookup ON jira_import_id_mappings(job_id, entity_type, jira_id);
CREATE INDEX IF NOT EXISTS idx_jira_mappings_key ON jira_import_id_mappings(job_id, jira_key);
CREATE INDEX IF NOT EXISTS idx_jira_mappings_windshift ON jira_import_id_mappings(entity_type, windshift_id);

-- Asset import progress (for resumable large imports)
CREATE TABLE IF NOT EXISTS jira_import_asset_progress (
    id SERIAL PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jira_import_jobs(id) ON DELETE CASCADE,
    schema_id TEXT NOT NULL,
    object_type_id TEXT,
    last_page INTEGER DEFAULT 0,
    total_pages INTEGER,
    objects_imported INTEGER DEFAULT 0,
    total_objects INTEGER,
    status TEXT NOT NULL DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(job_id, schema_id, object_type_id)
);

CREATE INDEX IF NOT EXISTS idx_jira_asset_progress_job ON jira_import_asset_progress(job_id);

-- User mappings for imported Jira users
CREATE TABLE IF NOT EXISTS jira_import_user_mappings (
    id SERIAL PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jira_import_jobs(id) ON DELETE CASCADE,
    jira_account_id TEXT NOT NULL,
    jira_email TEXT,
    jira_display_name TEXT,
    windshift_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    was_created BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(job_id, jira_account_id)
);

CREATE INDEX IF NOT EXISTS idx_jira_user_mappings_job ON jira_import_user_mappings(job_id);
CREATE INDEX IF NOT EXISTS idx_jira_user_mappings_windshift ON jira_import_user_mappings(windshift_user_id);
