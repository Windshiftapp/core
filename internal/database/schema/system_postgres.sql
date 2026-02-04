-- System tables (settings, labels, reviews, plugins, audit, etc.)

-- System settings table for module configuration
CREATE TABLE IF NOT EXISTS system_settings (
	id SERIAL PRIMARY KEY,
	key TEXT NOT NULL UNIQUE,
	value TEXT,
	value_type TEXT DEFAULT 'string', -- string, boolean, integer, json
	description TEXT,
	category TEXT DEFAULT 'general',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_system_settings_key ON system_settings(key);
CREATE INDEX IF NOT EXISTS idx_system_settings_category ON system_settings(category);

-- Personal labels table
CREATE TABLE IF NOT EXISTS personal_labels (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT DEFAULT '#3B82F6',
	user_id INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(user_id, review_date, review_type) -- One review per user per date per type
);

CREATE INDEX idx_reviews_user_date ON reviews(user_id, review_date);

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
	installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plugin_registry_name ON plugin_registry(name);
CREATE INDEX IF NOT EXISTS idx_plugin_registry_enabled ON plugin_registry(enabled);

-- API tokens table
CREATE TABLE api_tokens (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	token_prefix TEXT NOT NULL,
	permissions TEXT DEFAULT '["read"]',
	expires_at TIMESTAMP NULL,
	last_used_at TIMESTAMP NULL,
	is_temporary BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_expires_at ON api_tokens(expires_at);

-- Collections table
CREATE TABLE IF NOT EXISTS collections (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	ql_query TEXT,
	is_public BOOLEAN DEFAULT false,
	workspace_id INTEGER,
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_collections_name ON collections(name);
CREATE INDEX IF NOT EXISTS idx_collections_workspace_id ON collections(workspace_id);
CREATE INDEX IF NOT EXISTS idx_collections_created_by ON collections(created_by);
CREATE INDEX IF NOT EXISTS idx_collections_is_public ON collections(is_public);

-- Active timers table
CREATE TABLE active_timers (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	item_id INTEGER,
	project_id INTEGER NOT NULL,
	description TEXT NOT NULL,
	start_time_utc INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_active_timers_workspace_id ON active_timers(workspace_id);
CREATE INDEX IF NOT EXISTS idx_active_timers_item_id ON active_timers(item_id);
CREATE INDEX IF NOT EXISTS idx_active_timers_project_id ON active_timers(project_id);

-- Themes table
CREATE TABLE IF NOT EXISTS themes (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	is_active BOOLEAN DEFAULT false,
	nav_background_color_light TEXT NOT NULL DEFAULT '#ffffff',
	nav_text_color_light TEXT NOT NULL DEFAULT '#374151',
	nav_background_color_dark TEXT NOT NULL DEFAULT '#1f2937',
	nav_text_color_dark TEXT NOT NULL DEFAULT '#f3f4f6',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Board configuration tables
CREATE TABLE IF NOT EXISTS board_configurations (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER,
	collection_id INTEGER,
	backlog_status_ids TEXT, -- JSON array of status IDs for backlog
	list_columns TEXT, -- JSON array of list column configurations
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (board_configuration_id) REFERENCES board_configurations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS board_column_statuses (
	id SERIAL PRIMARY KEY,
	board_column_id INTEGER NOT NULL,
	status_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_test_coverage_config_workspace_id ON test_coverage_configurations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_test_coverage_config_collection_id ON test_coverage_configurations(collection_id);

-- Audit logging table
CREATE TABLE IF NOT EXISTS audit_logs (
	id SERIAL PRIMARY KEY,
	timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
	last_visited_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	visit_count INTEGER DEFAULT 1,
	expires_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	UNIQUE(user_id, workspace_id)
);

CREATE TABLE IF NOT EXISTS user_item_activities (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	activity_type TEXT NOT NULL,
	last_activity_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	activity_count INTEGER DEFAULT 1,
	expires_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(plugin_name, key)
);

CREATE INDEX IF NOT EXISTS idx_plugin_kv_plugin_name ON plugin_kv_store(plugin_name);

-- Calendar feed tokens for ICS subscription URLs
CREATE TABLE IF NOT EXISTS calendar_feed_tokens (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL UNIQUE,
	token TEXT NOT NULL UNIQUE,
	is_active BOOLEAN DEFAULT true,
	last_accessed_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	expires_at TIMESTAMP,
	last_used_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scim_tokens_token_prefix ON scim_tokens(token_prefix);
CREATE INDEX IF NOT EXISTS idx_scim_tokens_is_active ON scim_tokens(is_active);
