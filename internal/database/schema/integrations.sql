-- Integration provider tables (Notion, Confluence, Google Docs, etc.)
-- Generalized system for linking external pages/documents to work items

-- Integration Providers (system-level configuration, admin sets up)
CREATE TABLE IF NOT EXISTS integration_providers (
	id TEXT PRIMARY KEY,
	slug TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	provider_type TEXT NOT NULL,                  -- 'notion', 'confluence', etc.
	enabled BOOLEAN DEFAULT TRUE,
	oauth_client_id TEXT,
	oauth_client_secret_encrypted TEXT,
	provider_config TEXT DEFAULT '{}',            -- JSON: provider-specific config (e.g., base_url for self-hosted)
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_integration_providers_slug ON integration_providers(slug);
CREATE INDEX IF NOT EXISTS idx_integration_providers_type ON integration_providers(provider_type);
CREATE INDEX IF NOT EXISTS idx_integration_providers_enabled ON integration_providers(enabled);

-- Integration OAuth State (temporary storage for OAuth flow, 5-min expiry)
CREATE TABLE IF NOT EXISTS integration_oauth_state (
	id TEXT PRIMARY KEY,
	provider_id TEXT NOT NULL,
	state TEXT UNIQUE NOT NULL,
	user_id TEXT NOT NULL,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_integration_oauth_state_state ON integration_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_integration_oauth_state_expires ON integration_oauth_state(expires_at);

-- Cleanup trigger for expired state tokens
CREATE TRIGGER IF NOT EXISTS cleanup_expired_integration_oauth_state
AFTER INSERT ON integration_oauth_state
BEGIN
	DELETE FROM integration_oauth_state
	WHERE expires_at < datetime('now')
	AND (ABS(RANDOM()) % 100) = 0;
END;

-- User Integration Tokens (per-user OAuth tokens for integration providers)
CREATE TABLE IF NOT EXISTS user_integration_tokens (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL,
	oauth_access_token_encrypted TEXT NOT NULL,
	provider_metadata TEXT DEFAULT '{}',          -- JSON: provider-specific data (e.g., workspace_id, workspace_name for Notion)
	connected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (integration_provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
	UNIQUE(user_id, integration_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_user_integration_tokens_user ON user_integration_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_integration_tokens_provider ON user_integration_tokens(integration_provider_id);

-- Item Integration Links (links between items and external pages/documents)
CREATE TABLE IF NOT EXISTS item_integration_links (
	id TEXT PRIMARY KEY,
	item_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL,
	external_id TEXT NOT NULL,
	external_url TEXT NOT NULL,
	title TEXT NOT NULL,
	icon TEXT DEFAULT '',
	link_type TEXT NOT NULL,                      -- 'page', 'database', 'doc', etc.
	link_metadata TEXT DEFAULT '{}',              -- JSON: provider-specific extras
	linked_by TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (integration_provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
	UNIQUE(item_id, integration_provider_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_item_integration_links_item ON item_integration_links(item_id);
CREATE INDEX IF NOT EXISTS idx_item_integration_links_provider ON item_integration_links(integration_provider_id);
CREATE INDEX IF NOT EXISTS idx_item_integration_links_external ON item_integration_links(external_id);

-- ============================================================================
-- Todoist personal-task sync (WI-402)
-- Two-way 1:1 mirror between a user's personal workspace and their Todoist
-- account. Reuses integration_providers + user_integration_tokens for the
-- connection; these tables hold the per-user sync configuration and the
-- item <-> Todoist-task id mapping.
-- ============================================================================

-- Per-user sync configuration. One row per (user, provider).
CREATE TABLE IF NOT EXISTS todoist_sync_config (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL,
	personal_workspace_id INTEGER NOT NULL,        -- workspace synced tasks live in
	enabled BOOLEAN DEFAULT FALSE,
	scope_mode TEXT NOT NULL DEFAULT 'all',        -- 'all' (every project, flattened) | 'project'
	todoist_project_id TEXT DEFAULT '',            -- set when scope_mode = 'project'
	sync_token TEXT DEFAULT '*',                   -- Todoist incremental sync cursor
	last_synced_at DATETIME,
	last_error TEXT DEFAULT '',                    -- last sync error, surfaced in settings UI
	sync_lock_until DATETIME,                      -- per-config run lock: set to a future lease while a sync runs; NULL/past = free
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (integration_provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
	UNIQUE(user_id, integration_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_todoist_sync_config_user ON todoist_sync_config(user_id);
CREATE INDEX IF NOT EXISTS idx_todoist_sync_config_enabled ON todoist_sync_config(enabled);

-- Item <-> Todoist-task id map. One row per synced task pair. The last_*
-- columns snapshot the agreed state at the previous sync so the engine can
-- detect which side changed which field (field-level last-write-wins).
CREATE TABLE IF NOT EXISTS todoist_task_links (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	item_id INTEGER NOT NULL,                      -- personal-workspace item id
	todoist_task_id TEXT NOT NULL,
	todoist_project_id TEXT DEFAULT '',
	last_title TEXT DEFAULT '',
	last_description TEXT DEFAULT '',
	last_due TEXT DEFAULT '',                      -- 'YYYY-MM-DD' or RFC3339 or ''
	last_priority INTEGER DEFAULT 1,              -- Todoist scale: 1 (normal) .. 4 (urgent)
	last_completed BOOLEAN DEFAULT FALSE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, todoist_task_id),
	UNIQUE(item_id)
);

CREATE INDEX IF NOT EXISTS idx_todoist_task_links_user ON todoist_task_links(user_id);
CREATE INDEX IF NOT EXISTS idx_todoist_task_links_item ON todoist_task_links(item_id);
CREATE INDEX IF NOT EXISTS idx_todoist_task_links_todoist ON todoist_task_links(todoist_task_id);

-- Zammad provider extension. Provider identity and visible item links remain
-- in the generic integration tables; these tables hold only Zammad-specific
-- connection policy and durable synchronization state.
CREATE TABLE IF NOT EXISTS zammad_connections (
	provider_id TEXT PRIMARY KEY,
	credential_id INTEGER NOT NULL,
	auth_method TEXT NOT NULL DEFAULT 'api_token',
	oauth_generation INTEGER NOT NULL DEFAULT 1,
	config_revision INTEGER NOT NULL DEFAULT 1,
	oauth_attempt_id TEXT,
	base_url TEXT NOT NULL,
	default_group_id INTEGER,
	default_group_name TEXT DEFAULT '',
	allowed_groups TEXT NOT NULL DEFAULT '[]',
	default_customer TEXT NOT NULL,
	correlation_field TEXT NOT NULL DEFAULT 'windshift_item_key',
	closed_state_ids TEXT NOT NULL DEFAULT '[]',
	completion_status_id INTEGER,
	last_tested_at DATETIME,
	last_test_error TEXT DEFAULT '',
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (credential_id) REFERENCES action_credentials(id) ON DELETE RESTRICT,
	FOREIGN KEY (completion_status_id) REFERENCES statuses(id) ON DELETE SET NULL,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_zammad_connections_credential ON zammad_connections(credential_id);

-- Connection-scoped OAuth material. Zammad is a system/workspace integration,
-- so these tokens intentionally never use user_integration_tokens.
CREATE TABLE IF NOT EXISTS zammad_oauth_tokens (
	provider_id TEXT PRIMARY KEY,
	oauth_generation INTEGER NOT NULL,
	expires_at DATETIME NOT NULL,
	reauthorization_required BOOLEAN NOT NULL DEFAULT false,
	refresh_lock_until DATETIME,
	refresh_claim_owner TEXT,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES zammad_connections(provider_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS zammad_oauth_state (
	state TEXT PRIMARY KEY,
	provider_id TEXT NOT NULL,
	initiated_by INTEGER NOT NULL,
	oauth_generation INTEGER NOT NULL,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
	FOREIGN KEY (initiated_by) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_zammad_oauth_state_expires ON zammad_oauth_state(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_zammad_oauth_state_provider ON zammad_oauth_state(provider_id);

CREATE TABLE IF NOT EXISTS zammad_ticket_links (
	id TEXT PRIMARY KEY,
	item_id INTEGER NOT NULL,
	provider_id TEXT NOT NULL,
	item_integration_link_id TEXT,
	ticket_id INTEGER,
	ticket_number TEXT DEFAULT '',
	ticket_url TEXT DEFAULT '',
	group_id INTEGER,
	group_name TEXT DEFAULT '',
	owner_id INTEGER,
	owner_name TEXT DEFAULT '',
	correlation_key TEXT NOT NULL,
	sync_state TEXT NOT NULL DEFAULT 'pending',
	creating_started_at DATETIME,
	last_status_id INTEGER,
	last_status_name TEXT DEFAULT '',
	last_synced_at DATETIME,
	last_attempt_at DATETIME,
	next_attempt_at DATETIME,
	last_error TEXT DEFAULT '',
	completion_applied BOOLEAN NOT NULL DEFAULT false,
	sync_lock_until DATETIME,
	sync_lock_owner TEXT,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE RESTRICT,
	FOREIGN KEY (provider_id) REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
	FOREIGN KEY (item_integration_link_id) REFERENCES item_integration_links(id) ON DELETE SET NULL,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(item_id, provider_id),
	UNIQUE(provider_id, ticket_id),
	UNIQUE(provider_id, correlation_key)
);

CREATE INDEX IF NOT EXISTS idx_zammad_ticket_links_item ON zammad_ticket_links(item_id);
CREATE INDEX IF NOT EXISTS idx_zammad_ticket_links_sync ON zammad_ticket_links(sync_state, last_synced_at);

-- migration: 20260615_todoist_sync_tables
-- migration: 20260829_zammad_integration
