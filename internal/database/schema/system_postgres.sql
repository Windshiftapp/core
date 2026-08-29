-- System tables (settings, labels, reviews, plugins, audit, etc.)

-- Schema migrations registry. Source of truth for which migrations have
-- been applied. Paired with internal/database/migrations.go (the Catalog).
-- The bootstrap CREATE in Initialize() makes this table available before
-- any other migration logic runs, so existing installs can be retroactively
-- stamped on first upgrade.
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	checksum   TEXT NOT NULL DEFAULT '',
	applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- System settings table for module configuration
CREATE TABLE IF NOT EXISTS system_settings (
	id SERIAL PRIMARY KEY,
	key TEXT NOT NULL UNIQUE,
	value TEXT,
	value_type TEXT DEFAULT 'string', -- string, boolean, integer, json
	description TEXT,
	category TEXT DEFAULT 'general',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_system_settings_key ON system_settings(key);
CREATE INDEX IF NOT EXISTS idx_system_settings_category ON system_settings(category);

-- Personal labels table
CREATE TABLE IF NOT EXISTS personal_labels (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT DEFAULT '#3B82F6',
	user_id INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_personal_labels_user_id ON personal_labels(user_id);

-- Reviews table
CREATE TABLE IF NOT EXISTS reviews (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	review_date DATE NOT NULL,
	review_type TEXT NOT NULL CHECK (review_type IN ('daily', 'weekly')),
	review_data TEXT NOT NULL, -- JSON data for unstructured storage
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(user_id, review_date, review_type) -- One review per user per date per type
);

CREATE INDEX IF NOT EXISTS idx_reviews_user_date ON reviews(user_id, review_date);

-- Plugin registry table
CREATE TABLE IF NOT EXISTS plugin_registry (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	version TEXT NOT NULL,
	description TEXT,
	author TEXT,
	path TEXT NOT NULL,
	routes TEXT,
	extensions TEXT,
	enabled BOOLEAN DEFAULT true,
	installed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plugin_registry_name ON plugin_registry(name);
CREATE INDEX IF NOT EXISTS idx_plugin_registry_enabled ON plugin_registry(enabled);

-- API tokens table
CREATE TABLE IF NOT EXISTS api_tokens (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	token_prefix TEXT NOT NULL,
	permissions TEXT DEFAULT '["read"]',
	expires_at TIMESTAMPTZ NULL,
	last_used_at TIMESTAMPTZ NULL,
	is_temporary BOOLEAN DEFAULT false,
	oauth_client_id TEXT,
	oauth_resource TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_expires_at ON api_tokens(expires_at);

-- OAuth 2.0 server tables. Pair with internal/handlers/oauth.go and
-- internal/handlers/admin_oauth_clients.go.
--
-- oauth_clients holds the registered third-party clients an admin manages
-- via the OAuth Client Manager UI. Confidential clients store a bcrypt of
-- the secret; public clients leave it null and authenticate via PKCE.
CREATE TABLE IF NOT EXISTS oauth_clients (
	id SERIAL PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	client_id TEXT NOT NULL UNIQUE,
	client_type TEXT NOT NULL,           -- 'public' | 'confidential'
	client_secret_hash TEXT,             -- bcrypt; null for public clients
	redirect_uris TEXT NOT NULL DEFAULT '[]',
	allowed_scopes TEXT NOT NULL DEFAULT '[]',
	resource_uri TEXT,
	enabled BOOLEAN NOT NULL DEFAULT true,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_oauth_clients_client_id ON oauth_clients(client_id);
CREATE INDEX IF NOT EXISTS idx_oauth_clients_enabled ON oauth_clients(enabled);

-- Wire up the FK from users.oauth_client_id → oauth_clients(id) now that
-- oauth_clients exists. Declared here (not in base_tables_postgres.sql)
-- because base_tables runs before this file, and Postgres requires the FK
-- target table to exist at CREATE TABLE time.
DO $$ BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'users_oauth_client_id_fkey'
	) THEN
		ALTER TABLE users
			ADD CONSTRAINT users_oauth_client_id_fkey
			FOREIGN KEY (oauth_client_id) REFERENCES oauth_clients(id) ON DELETE CASCADE;
	END IF;
END $$;

-- oauth_authorization_codes: short-lived authorization codes minted on
-- consent-screen approve, exchanged once for an access+refresh token pair
-- at POST /api/oauth/token. consumed_at marks one-shot use; the PKCE
-- challenge is echoed back at exchange time for verifier matching.
CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
	id SERIAL PRIMARY KEY,
	code TEXT NOT NULL UNIQUE,
	client_id TEXT NOT NULL,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	agent_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	redirect_uri TEXT NOT NULL,
	scopes TEXT NOT NULL DEFAULT '[]',
	code_challenge TEXT,
	code_challenge_method TEXT,          -- 'S256' | 'plain'
	state TEXT,
	resource_uri TEXT,
	expires_at TIMESTAMPTZ NOT NULL,
	consumed_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_oauth_authorization_codes_code ON oauth_authorization_codes(code);
CREATE INDEX IF NOT EXISTS idx_oauth_authorization_codes_expires_at ON oauth_authorization_codes(expires_at);

-- oauth_refresh_tokens: 30-day refresh tokens with rotation. token_hash is
-- SHA-256 of the plaintext (NOT bcrypt; we need O(1) lookup at refresh
-- time). api_token_id points at the matching access token in api_tokens
-- so revoking rotates both. rotated_to_id chains revoked → replacement for
-- audit visibility on a leaked-token replay.
CREATE TABLE IF NOT EXISTS oauth_refresh_tokens (
	id SERIAL PRIMARY KEY,
	token_hash TEXT NOT NULL UNIQUE,
	api_token_id INTEGER NOT NULL REFERENCES api_tokens(id) ON DELETE CASCADE,
	client_id TEXT NOT NULL,
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	agent_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	scopes TEXT NOT NULL DEFAULT '[]',
	resource_uri TEXT,
	expires_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ,
	rotated_to_id INTEGER REFERENCES oauth_refresh_tokens(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_token_hash ON oauth_refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_api_token_id ON oauth_refresh_tokens(api_token_id);
CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_expires_at ON oauth_refresh_tokens(expires_at);

-- Collection categories table (for organizing global collections)
CREATE TABLE IF NOT EXISTS collection_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL DEFAULT '#3b82f6',
	description TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_collection_categories_name ON collection_categories(name);

-- Collections table
CREATE TABLE IF NOT EXISTS collections (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	ql_query TEXT,
	filter_state TEXT,
	is_public BOOLEAN DEFAULT false,
	workspace_id INTEGER,
	category_id INTEGER,
	created_by INTEGER,
	public_slug TEXT UNIQUE,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (category_id) REFERENCES collection_categories(id) ON DELETE SET NULL,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_collections_name ON collections(name);
CREATE INDEX IF NOT EXISTS idx_collections_workspace_id ON collections(workspace_id);
CREATE INDEX IF NOT EXISTS idx_collections_created_by ON collections(created_by);
CREATE INDEX IF NOT EXISTS idx_collections_is_public ON collections(is_public);
CREATE INDEX IF NOT EXISTS idx_collections_category_id ON collections(category_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_collections_public_slug ON collections(public_slug);

CREATE OR REPLACE FUNCTION log_collection_change() RETURNS trigger AS $$
BEGIN
	INSERT INTO item_change_log(item_id, workspace_id, change_type) VALUES (0, COALESCE(NEW.workspace_id, OLD.workspace_id, 0), 'upsert');
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_collections_change_update ON collections;
CREATE TRIGGER trg_collections_change_update AFTER UPDATE OF ql_query, filter_state, workspace_id ON collections FOR EACH ROW EXECUTE FUNCTION log_collection_change();

-- Active timers table
CREATE TABLE IF NOT EXISTS active_timers (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	item_id INTEGER,
	project_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	description TEXT NOT NULL,
	start_time_utc INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_active_timers_workspace_id ON active_timers(workspace_id);
CREATE INDEX IF NOT EXISTS idx_active_timers_item_id ON active_timers(item_id);
CREATE INDEX IF NOT EXISTS idx_active_timers_project_id ON active_timers(project_id);
-- A user can have at most one active timer at a time; the UNIQUE index is the
-- DB-level backstop for concurrent timer starts.
-- migration: 20260610_active_timers_unique_user
CREATE UNIQUE INDEX IF NOT EXISTS idx_active_timers_user_id ON active_timers(user_id);

-- Themes table
CREATE TABLE IF NOT EXISTS themes (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	is_active BOOLEAN DEFAULT false,
	nav_background_color_light TEXT NOT NULL DEFAULT '#ffffff',
	nav_text_color_light TEXT NOT NULL DEFAULT '#374151',
	nav_background_color_dark TEXT NOT NULL DEFAULT '#1f2937',
	nav_text_color_dark TEXT NOT NULL DEFAULT '#f3f4f6',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_themes_builtin_key ON themes(builtin_key) WHERE builtin_key IS NOT NULL;

-- Board configuration tables
CREATE TABLE IF NOT EXISTS board_configurations (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER,
	collection_id INTEGER,
	backlog_status_ids TEXT, -- JSON array of status IDs for backlog
	list_columns TEXT, -- JSON array of list column configurations
	roadmap_config TEXT, -- JSON object with roadmap view settings
	card_fields TEXT, -- JSON array of card field configurations
	show_rightmost_column_last_50 BOOLEAN DEFAULT false,
	completed_item_retention_days INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS board_columns (
	id SERIAL PRIMARY KEY,
	board_configuration_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	display_order INTEGER NOT NULL,
	wip_limit INTEGER,
	color TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (board_configuration_id) REFERENCES board_configurations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS board_column_statuses (
	id SERIAL PRIMARY KEY,
	board_column_id INTEGER NOT NULL,
	status_id INTEGER NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (board_column_id) REFERENCES board_columns(id) ON DELETE CASCADE,
	FOREIGN KEY (status_id) REFERENCES statuses(id) ON DELETE CASCADE,
	UNIQUE(board_column_id, status_id)
);

CREATE INDEX IF NOT EXISTS idx_board_configurations_workspace_id ON board_configurations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_board_configurations_collection_id ON board_configurations(collection_id);
CREATE INDEX IF NOT EXISTS idx_board_columns_board_configuration_id ON board_columns(board_configuration_id);
CREATE INDEX IF NOT EXISTS idx_board_columns_display_order ON board_columns(display_order);
CREATE INDEX IF NOT EXISTS idx_board_column_statuses_board_column_id ON board_column_statuses(board_column_id);
CREATE INDEX IF NOT EXISTS idx_board_column_statuses_status_id ON board_column_statuses(status_id);

-- Test coverage configuration table
CREATE TABLE IF NOT EXISTS test_coverage_configurations (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER,
	collection_id INTEGER,
	requirement_item_type_ids TEXT, -- JSON array of item type IDs
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_test_coverage_config_workspace_id ON test_coverage_configurations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_test_coverage_config_collection_id ON test_coverage_configurations(collection_id);

-- Audit logging table
CREATE TABLE IF NOT EXISTS audit_logs (
	id SERIAL PRIMARY KEY,
	timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	user_id INTEGER,
	username TEXT NOT NULL,
	ip_address TEXT,
	user_agent TEXT,
	action_type TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	resource_id INTEGER,
	resource_name TEXT,
	details TEXT,
	success BOOLEAN NOT NULL DEFAULT TRUE,
	error_message TEXT,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_type ON audit_logs(action_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_type ON audit_logs(resource_type);

-- Activity tracking tables
CREATE TABLE IF NOT EXISTS user_workspace_visits (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	last_visited_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	visit_count INTEGER DEFAULT 1,
	expires_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	UNIQUE(user_id, workspace_id)
);

CREATE TABLE IF NOT EXISTS user_item_activities (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	activity_type TEXT NOT NULL,
	last_activity_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	activity_count INTEGER DEFAULT 1,
	expires_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	UNIQUE(user_id, item_id, activity_type)
);

CREATE TABLE IF NOT EXISTS item_watches (
	id SERIAL PRIMARY KEY,
	item_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	is_active BOOLEAN DEFAULT true,
	watch_reason TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(user_id, item_id)
);

CREATE INDEX IF NOT EXISTS idx_user_workspace_visits_user_id ON user_workspace_visits(user_id);
CREATE INDEX IF NOT EXISTS idx_user_workspace_visits_workspace_id ON user_workspace_visits(workspace_id);
CREATE INDEX IF NOT EXISTS idx_user_workspace_visits_last_visited ON user_workspace_visits(last_visited_at);
CREATE INDEX IF NOT EXISTS idx_user_item_activities_user_id ON user_item_activities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_item_activities_item_id ON user_item_activities(item_id);
CREATE INDEX IF NOT EXISTS idx_user_item_activities_last_viewed ON user_item_activities(last_activity_at);
CREATE INDEX IF NOT EXISTS idx_item_watches_item_id ON item_watches(item_id);
CREATE INDEX IF NOT EXISTS idx_item_watches_user_id ON item_watches(user_id);
-- migration: idx_item_watches_user_active
CREATE INDEX IF NOT EXISTS idx_item_watches_user_active ON item_watches(user_id, is_active);

-- Request types and fields for portal/channel routing
-- Note: request_types table is defined in request_types_postgres.sql

CREATE TABLE IF NOT EXISTS request_type_fields (
	id SERIAL PRIMARY KEY,
	request_type_id INTEGER NOT NULL,
	field_identifier TEXT NOT NULL,
	field_type TEXT NOT NULL,
	is_required BOOLEAN DEFAULT false,
	display_order INTEGER DEFAULT 0,
	options TEXT,
	-- Display customization for portal
	display_name TEXT,
	description TEXT,
	-- Multi-step form support
	step_number INTEGER DEFAULT 1,
	-- Virtual field support (field_type = 'virtual')
	virtual_field_type TEXT,
	virtual_field_options TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (request_type_id) REFERENCES request_types(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_request_types_name ON request_types(name);
CREATE INDEX IF NOT EXISTS idx_request_types_display_order ON request_types(display_order);
CREATE INDEX IF NOT EXISTS idx_request_type_fields_request_type_id ON request_type_fields(request_type_id);

-- Plugin key-value store for plugin-scoped settings/data
CREATE TABLE IF NOT EXISTS plugin_kv_store (
	id SERIAL PRIMARY KEY,
	plugin_name TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(plugin_name, key)
);

CREATE INDEX IF NOT EXISTS idx_plugin_kv_plugin_name ON plugin_kv_store(plugin_name);

-- Calendar feed tokens for ICS subscription URLs
CREATE TABLE IF NOT EXISTS calendar_feed_tokens (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL UNIQUE,
	token TEXT NOT NULL UNIQUE,
	is_active BOOLEAN DEFAULT true,
	last_accessed_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_calendar_feed_tokens_token ON calendar_feed_tokens(token);
CREATE INDEX IF NOT EXISTS idx_calendar_feed_tokens_user_id ON calendar_feed_tokens(user_id);

-- SCIM tokens table for dedicated SCIM authentication
CREATE TABLE IF NOT EXISTS scim_tokens (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	token_prefix TEXT NOT NULL,
	is_active BOOLEAN DEFAULT true,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	expires_at TIMESTAMPTZ,
	last_used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scim_tokens_token_prefix ON scim_tokens(token_prefix);
CREATE INDEX IF NOT EXISTS idx_scim_tokens_is_active ON scim_tokens(is_active);

-- CLI auth codes (short-lived, single-use). Backs the `ws init` automatic
-- onboarding flow: the CLI starts a flow, the user approves in the browser,
-- and the CLI redeems the code at POST /api/cli/auth/exchange.
CREATE TABLE IF NOT EXISTS cli_auth_codes (
	id SERIAL PRIMARY KEY,
	code TEXT NOT NULL UNIQUE,
	state TEXT NOT NULL,
	callback_url TEXT NOT NULL,
	hostname TEXT NOT NULL,
	agent_name TEXT NOT NULL,
	requested_scopes TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending', -- pending | approved | denied | consumed | expired
	approved_by_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	agent_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	token_id INTEGER REFERENCES api_tokens(id) ON DELETE SET NULL,
	token_plaintext TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMPTZ NOT NULL,
	consumed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cli_auth_codes_code ON cli_auth_codes(code);
CREATE INDEX IF NOT EXISTS idx_cli_auth_codes_expires_at ON cli_auth_codes(expires_at);

-- Native auth codes (short-lived, single-use). Bridge a system-browser SSO
-- login back into a native client (desktop/iOS, WI-446): each code is bound
-- to a session and redeemed once at /api/auth/native/exchange for the encoded
-- session cookie.
CREATE TABLE IF NOT EXISTS native_auth_codes (
	id SERIAL PRIMARY KEY,
	code TEXT NOT NULL UNIQUE,
	session_token TEXT NOT NULL,
	session_expires_at TIMESTAMPTZ NOT NULL,
	status TEXT NOT NULL DEFAULT 'valid',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMPTZ NOT NULL,
	consumed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_native_auth_codes_code ON native_auth_codes(code);
CREATE INDEX IF NOT EXISTS idx_native_auth_codes_expires_at ON native_auth_codes(expires_at);

-- Background scheduler runs: one row per tick of each in-process scheduler
-- (briefing, email, recurrence, notification). Surfaced on the admin
-- Diagnostics page so admins can see whether jobs ran on time, how long they
-- took, and whether they failed.
CREATE TABLE IF NOT EXISTS scheduler_runs (
	id SERIAL PRIMARY KEY,
	scheduler_name TEXT NOT NULL,
	started_at TIMESTAMPTZ NOT NULL,
	completed_at TIMESTAMPTZ,
	duration_ms INTEGER,
	items_processed INTEGER,
	success BOOLEAN NOT NULL DEFAULT FALSE,
	error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_scheduler_runs_name_started ON scheduler_runs(scheduler_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_scheduler_runs_started_at ON scheduler_runs(started_at);
CREATE INDEX IF NOT EXISTS idx_scheduler_runs_success ON scheduler_runs(success);

-- Pending custom-field maintenance jobs drained by the CFVCleanupScheduler.
-- Doing this work inline on the admin request would block it for as long as
-- the workspace has items/assets (can be millions), so we enqueue a row here
-- and process it off-thread in batches. job_type selects the work:
--   field_scrub     - a deleted field's key is removed from cfv JSON
--   option_removal   - removed select/multiselect option ids are stripped
--                      (payload carries field_id, field_type, removed_ids)
--   index_build      - a Postgres custom-field index is built CONCURRENTLY
--                      (payload carries field_id, field_type, target_table,
--                      index_name) off the request thread
--
-- status transitions: pending -> running -> done (or failed).
CREATE TABLE IF NOT EXISTS pending_custom_field_cleanups (
	id SERIAL PRIMARY KEY,
	field_id INTEGER NOT NULL,
	job_type TEXT NOT NULL DEFAULT 'field_scrub',
	payload TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at TIMESTAMPTZ,
	completed_at TIMESTAMPTZ,
	items_processed INTEGER NOT NULL DEFAULT 0,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	next_attempt_at TIMESTAMPTZ,
	error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_pending_cfv_cleanups_status ON pending_custom_field_cleanups(status, created_at);
