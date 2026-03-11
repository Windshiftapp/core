-- ============================================
-- BASE TABLES (no foreign key dependencies)
-- ============================================
-- These tables must be created first as they have no foreign key dependencies
-- and are referenced by other tables in the schema.

-- From users_postgres.sql
CREATE TABLE IF NOT EXISTS users (
	id SERIAL PRIMARY KEY,
	email TEXT UNIQUE NOT NULL,
	username TEXT UNIQUE NOT NULL,
	first_name TEXT NOT NULL,
	last_name TEXT NOT NULL,
	is_active BOOLEAN DEFAULT true,
	avatar_url TEXT,
	password_hash TEXT, -- bcrypt hashed password
	requires_password_reset BOOLEAN DEFAULT false,
	timezone TEXT,
	language TEXT DEFAULT 'en',
	email_verified BOOLEAN DEFAULT true, -- Default true for backwards compatibility
	email_verification_token TEXT, -- Token for email verification flow
	email_verification_expires TIMESTAMP, -- Expiry time for verification token
	scim_external_id TEXT, -- SCIM externalId from identity provider
	scim_managed BOOLEAN DEFAULT false, -- If true, user is managed via SCIM
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_scim_external_id ON users(scim_external_id) WHERE scim_external_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_scim_managed ON users(scim_managed);

-- Channel Categories table for organizing channels
CREATE TABLE IF NOT EXISTS channel_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL DEFAULT '#3b82f6',
	description TEXT,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- From channels_postgres.sql
CREATE TABLE IF NOT EXISTS channels (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL, -- smtp, webhook, portal, imap, etc.
	direction TEXT NOT NULL, -- inbound, outbound
	description TEXT,
	status TEXT NOT NULL DEFAULT 'disabled', -- enabled, disabled
	is_default BOOLEAN DEFAULT false,
	config TEXT, -- JSON configuration data specific to channel type
	plugin_name TEXT, -- NULL for user-created, plugin name for plugin-managed
	plugin_webhook_id TEXT, -- Plugin's internal webhook identifier
	category_id INTEGER REFERENCES channel_categories(id) ON DELETE SET NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_activity TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_channels_type ON channels(type);
CREATE INDEX IF NOT EXISTS idx_channels_direction ON channels(direction);
CREATE INDEX IF NOT EXISTS idx_channels_status ON channels(status);
CREATE INDEX IF NOT EXISTS idx_channels_is_default ON channels(is_default);
CREATE INDEX IF NOT EXISTS idx_channels_plugin_name ON channels(plugin_name);
CREATE INDEX IF NOT EXISTS idx_channels_category_id ON channels(category_id);

-- From time_tracking_postgres.sql
CREATE TABLE IF NOT EXISTS customer_organisations (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT,
	description TEXT,
	active BOOLEAN DEFAULT true,
	avatar_url TEXT,
	custom_field_values JSONB,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_project_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	color TEXT DEFAULT '#3B82F6',
	display_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From core_postgres.sql
CREATE TABLE IF NOT EXISTS projects (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER,
	name TEXT NOT NULL,
	description TEXT,
	active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_projects_workspace_id ON projects(workspace_id);

CREATE TABLE IF NOT EXISTS custom_field_definitions (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	field_type TEXT NOT NULL,
	description TEXT,
	required BOOLEAN DEFAULT false,
	options TEXT,
	display_order INTEGER DEFAULT 0,
	system_default BOOLEAN DEFAULT false,
	applies_to_portal_customers BOOLEAN DEFAULT false,
	applies_to_customer_organisations BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS custom_field_indexes (
	id SERIAL PRIMARY KEY,
	custom_field_id INTEGER NOT NULL,
	target_table TEXT NOT NULL,
	index_name TEXT NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (custom_field_id) REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
	UNIQUE(custom_field_id, target_table)
);

-- From config_workflows_postgres.sql
CREATE TABLE IF NOT EXISTS workflows (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS screens (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS priorities (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	icon TEXT DEFAULT 'AlertCircle',
	color TEXT DEFAULT '#3b82f6',
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS item_types (
	id SERIAL PRIMARY KEY,
	configuration_set_id INTEGER, -- Made nullable for many-to-many relationship
	name TEXT NOT NULL UNIQUE, -- Changed to global unique (not per config set)
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	icon TEXT DEFAULT 'FileText',
	color TEXT DEFAULT '#3b82f6',
	hierarchy_level INTEGER DEFAULT 3,
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hierarchy_levels (
	id SERIAL PRIMARY KEY,
	level INTEGER NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS status_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	is_completed BOOLEAN NOT NULL DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From milestones_postgres.sql
CREATE TABLE IF NOT EXISTS milestone_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From permissions_postgres.sql
CREATE TABLE IF NOT EXISTS permissions (
	id SERIAL PRIMARY KEY,
	permission_key TEXT UNIQUE NOT NULL,
	permission_name TEXT NOT NULL,
	description TEXT,
	scope TEXT NOT NULL CHECK (scope IN ('global', 'workspace')),
	is_system BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS workspace_roles (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	display_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From system_postgres.sql
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

CREATE TABLE IF NOT EXISTS board_configurations (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER,
	collection_id INTEGER,
	backlog_status_ids TEXT,
	list_columns TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From content_postgres.sql
CREATE TABLE IF NOT EXISTS attachment_settings (
	id SERIAL PRIMARY KEY,
	max_file_size INTEGER NOT NULL DEFAULT 52428800, -- 50MB default
	allowed_mime_types TEXT, -- JSON array of allowed MIME types
	attachment_path TEXT NOT NULL,
	enabled BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS link_types (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	forward_label TEXT NOT NULL,
	reverse_label TEXT NOT NULL,
	color TEXT DEFAULT '#6b7280',
	is_system BOOLEAN DEFAULT false,
	active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From notifications_postgres.sql
CREATE TABLE IF NOT EXISTS notification_templates (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	template_type TEXT, -- header, footer, body, etc.
	event_type TEXT, -- item_created, item_updated, comment_added, etc.
	template_subject TEXT,
	template_body TEXT,
	content TEXT, -- Alternative content field for non-event templates
	template_variables TEXT, -- JSON array of available variables
	channel_type TEXT DEFAULT 'in_app', -- in_app, email, slack, webhook
	is_system BOOLEAN DEFAULT false,
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notification_templates_event_type ON notification_templates(event_type);
CREATE INDEX IF NOT EXISTS idx_notification_templates_channel_type ON notification_templates(channel_type);

-- From tests_postgres.sql (simplified for base tables - full definition with FKs in tests_postgres.sql)
CREATE TABLE IF NOT EXISTS test_folders (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	parent_id INTEGER REFERENCES test_folders(id) ON DELETE SET NULL,
	name TEXT NOT NULL,
	description TEXT,
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_test_folders_parent_id ON test_folders(parent_id);

CREATE TABLE IF NOT EXISTS test_labels (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '#3B82F6',
	description TEXT DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(workspace_id, name)
);

-- From iterations_postgres.sql
CREATE TABLE IF NOT EXISTS iteration_types (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT DEFAULT '#6b7280',
	duration_days INTEGER NOT NULL DEFAULT 14,
	description TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- From portal_postgres.sql
CREATE TABLE IF NOT EXISTS contact_roles (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- INDEXES
-- ============================================
-- Additional indexes for base tables

CREATE INDEX IF NOT EXISTS idx_item_types_hierarchy_level ON item_types(hierarchy_level);
CREATE INDEX IF NOT EXISTS idx_item_types_sort_order ON item_types(sort_order);
CREATE INDEX IF NOT EXISTS idx_hierarchy_levels_level ON hierarchy_levels(level);
