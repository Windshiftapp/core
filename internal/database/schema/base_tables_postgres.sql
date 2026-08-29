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
	email_verification_expires TIMESTAMPTZ, -- Expiry time for verification token
	scim_external_id TEXT, -- SCIM externalId from identity provider
	scim_managed BOOLEAN DEFAULT false, -- If true, user is managed via SCIM
	is_agent BOOLEAN DEFAULT false, -- If true, user is a non-human agent (API-only; cannot log in)
	agent_owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE, -- NULL = service user (admin-provisioned); non-NULL = owned agent
	-- Distinguishes how an agent row got created. 'user' covers both the
	-- profile-page agent UI and CLI onboarding (both are gated by
	-- allow_user_managed_agents). 'oauth' is set ONLY by a successful
	-- OAuth code-exchange against an enabled oauth_clients row — the
	-- CHECK below prevents anyone from forging this label without a
	-- backing client.
	agent_provenance TEXT NOT NULL DEFAULT 'user',
	-- FK to oauth_clients(id) is added in system_postgres.sql once oauth_clients
	-- has been created; declaring it inline here would fail because schema files
	-- run base_tables before system, and Postgres validates FK target tables
	-- exist at CREATE TABLE time (unlike SQLite, which is lazy).
	oauth_client_id INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT users_agent_owner_requires_agent CHECK (agent_owner_user_id IS NULL OR is_agent = true),
	CONSTRAINT users_oauth_provenance_requires_client CHECK (agent_provenance != 'oauth' OR oauth_client_id IS NOT NULL),
	CONSTRAINT users_oauth_client_requires_oauth_agent CHECK (oauth_client_id IS NULL OR (is_agent = true AND agent_provenance = 'oauth'))
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_scim_external_id ON users(scim_external_id) WHERE scim_external_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_scim_managed ON users(scim_managed);
CREATE INDEX IF NOT EXISTS idx_users_is_agent ON users(is_agent);
CREATE INDEX IF NOT EXISTS idx_users_agent_owner ON users(agent_owner_user_id) WHERE agent_owner_user_id IS NOT NULL;
-- Audit query: "show me OAuth-spawned agents" / "agents per OAuth client"
CREATE INDEX IF NOT EXISTS idx_users_agent_provenance ON users(agent_provenance) WHERE is_agent = true;
CREATE INDEX IF NOT EXISTS idx_users_oauth_client_id ON users(oauth_client_id) WHERE oauth_client_id IS NOT NULL;

-- is_agent is immutable once set at creation: allowing toggles would let
-- an admin flip a human user into an agent and mint a token for them.
-- agent_owner_user_id is immutable for the same reason: flipping ownership
-- would silently transfer an agent's inherited permissions to a new user.
CREATE OR REPLACE FUNCTION users_is_agent_immutable() RETURNS TRIGGER AS $$
BEGIN
	IF COALESCE(NEW.is_agent, false) IS DISTINCT FROM COALESCE(OLD.is_agent, false) THEN
		RAISE EXCEPTION 'is_agent is immutable';
	END IF;
	IF NEW.agent_owner_user_id IS DISTINCT FROM OLD.agent_owner_user_id THEN
		RAISE EXCEPTION 'agent_owner_user_id is immutable';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS users_is_agent_immutable_trigger ON users;
CREATE TRIGGER users_is_agent_immutable_trigger
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION users_is_agent_immutable();

-- OAuth agent provenance is fixed at account creation. Allowing either field
-- to change later would break the audit trail tying the agent to its client.
-- migration: trig_users_oauth_provenance_immutable_postgres
CREATE OR REPLACE FUNCTION users_oauth_provenance_immutable() RETURNS TRIGGER AS $$
BEGIN
	IF COALESCE(NEW.agent_provenance, '') IS DISTINCT FROM COALESCE(OLD.agent_provenance, '') THEN
		RAISE EXCEPTION 'agent_provenance is immutable';
	END IF;
	IF NEW.oauth_client_id IS DISTINCT FROM OLD.oauth_client_id THEN
		RAISE EXCEPTION 'oauth_client_id is immutable';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS users_oauth_provenance_immutable_trigger ON users;
CREATE TRIGGER users_oauth_provenance_immutable_trigger
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION users_oauth_provenance_immutable();

-- Channel Categories table for organizing channels
CREATE TABLE IF NOT EXISTS channel_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL DEFAULT '#3b82f6',
	description TEXT,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- From channels_postgres.sql
CREATE TABLE IF NOT EXISTS channels (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL, -- smtp, webhook, portal, imap, etc.
	direction TEXT NOT NULL, -- inbound, outbound
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'disabled', -- enabled, disabled
	is_default BOOLEAN NOT NULL DEFAULT false,
	config TEXT NOT NULL DEFAULT '{}', -- JSON configuration data specific to channel type
	public_slug TEXT, -- normalized portal/form slug used for race-safe uniqueness
	plugin_name TEXT, -- NULL for user-created, plugin name for plugin-managed
	plugin_webhook_id TEXT, -- Plugin's internal webhook identifier
	category_id INTEGER REFERENCES channel_categories(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	last_activity TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_channels_type ON channels(type);
CREATE INDEX IF NOT EXISTS idx_channels_direction ON channels(direction);
CREATE INDEX IF NOT EXISTS idx_channels_status ON channels(status);
CREATE INDEX IF NOT EXISTS idx_channels_is_default ON channels(is_default);
CREATE INDEX IF NOT EXISTS idx_channels_plugin_name ON channels(plugin_name);
CREATE INDEX IF NOT EXISTS idx_channels_category_id ON channels(category_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_channels_public_slug
	ON channels(type, public_slug)
	WHERE direction = 'inbound' AND public_slug IS NOT NULL;

-- From time_tracking_postgres.sql
CREATE TABLE IF NOT EXISTS customer_organisations (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT,
	description TEXT,
	active BOOLEAN DEFAULT true,
	avatar_url TEXT,
	custom_field_values JSONB,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_project_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	color TEXT DEFAULT '#3B82F6',
	display_order INTEGER DEFAULT 0,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- The legacy `projects` table previously defined here was removed; see
-- core.sql for context. "Project" in Windshift refers exclusively to
-- `time_projects` (see time_tracking_postgres.sql).

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
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS custom_field_indexes (
	id SERIAL PRIMARY KEY,
	custom_field_id INTEGER NOT NULL,
	target_table TEXT NOT NULL,
	index_name TEXT NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (custom_field_id) REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
	UNIQUE(custom_field_id, target_table)
);

-- From config_workflows_postgres.sql
CREATE TABLE IF NOT EXISTS workflows (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS screens (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL,
	description TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS priorities (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	icon TEXT DEFAULT 'AlertCircle',
	color TEXT DEFAULT '#3b82f6',
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS item_types (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	configuration_set_id INTEGER, -- Made nullable for many-to-many relationship
	name TEXT NOT NULL UNIQUE, -- Changed to global unique (not per config set)
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	icon TEXT DEFAULT 'FileText',
	color TEXT DEFAULT '#3b82f6',
	hierarchy_level INTEGER DEFAULT 3,
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hierarchy_levels (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	level INTEGER NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT DEFAULT '',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS status_categories (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	is_completed BOOLEAN NOT NULL DEFAULT false,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- From milestones_postgres.sql
CREATE TABLE IF NOT EXISTS milestone_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- From permissions_postgres.sql
CREATE TABLE IF NOT EXISTS permissions (
	id SERIAL PRIMARY KEY,
	permission_key TEXT UNIQUE NOT NULL,
	permission_name TEXT NOT NULL,
	description TEXT,
	scope TEXT NOT NULL CHECK (scope IN ('global', 'workspace')),
	is_system BOOLEAN DEFAULT false,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- workspace_roles is fully defined in permissions_postgres.sql (which runs
-- after this file). The previous duplicate definition here was missing the
-- permissions_enabled column, and CREATE TABLE IF NOT EXISTS would silently
-- skip the proper definition once base_tables had already created the stub.

-- From system_postgres.sql
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

CREATE TABLE IF NOT EXISTS board_configurations (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER,
	collection_id INTEGER,
	backlog_status_ids TEXT,
	list_columns TEXT,
	roadmap_config TEXT,
	card_fields TEXT,
	show_rightmost_column_last_50 BOOLEAN DEFAULT false,
	completed_item_retention_days INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- From content_postgres.sql
CREATE TABLE IF NOT EXISTS attachment_settings (
	id SERIAL PRIMARY KEY,
	max_file_size INTEGER NOT NULL DEFAULT 52428800, -- 50MB default
	allowed_mime_types TEXT, -- JSON array of allowed MIME types
	attachment_path TEXT NOT NULL,
	enabled BOOLEAN DEFAULT true,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS link_types (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	forward_label TEXT NOT NULL,
	reverse_label TEXT NOT NULL,
	color TEXT DEFAULT '#6b7280',
	is_system BOOLEAN DEFAULT false,
	active BOOLEAN DEFAULT true,
	allowed_entity_types TEXT DEFAULT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_link_types_builtin_key ON link_types(builtin_key) WHERE builtin_key IS NOT NULL;

-- Email templates: per-sender HTML/text/subject Go templates editable from the
-- admin UI. `name` is the lookup key consumed by senders (e.g. "magic_link",
-- "notification_batch"). The legacy template_type/event_type/template_body/
-- template_variables/channel_type columns are kept for backwards compatibility
-- but are no longer read or written by application code.
CREATE TABLE IF NOT EXISTS notification_templates (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	subject TEXT,
	content TEXT, -- HTML body template
	text_body TEXT, -- Plain-text body template
	is_system BOOLEAN DEFAULT false,
	is_active BOOLEAN DEFAULT true,
	template_type TEXT, -- legacy, unused
	event_type TEXT, -- legacy, unused
	template_subject TEXT, -- legacy, unused
	template_body TEXT, -- legacy, unused
	template_variables TEXT, -- legacy, unused
	channel_type TEXT DEFAULT 'in_app', -- legacy, unused
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- From tests_postgres.sql (simplified for base tables - full definition with FKs in tests_postgres.sql)
CREATE TABLE IF NOT EXISTS test_folders (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	parent_id INTEGER REFERENCES test_folders(id) ON DELETE SET NULL,
	name TEXT NOT NULL,
	description TEXT,
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_test_folders_parent_id ON test_folders(parent_id);

CREATE TABLE IF NOT EXISTS test_labels (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '#3B82F6',
	description TEXT DEFAULT '',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(workspace_id, name)
);

-- From iterations_postgres.sql
CREATE TABLE IF NOT EXISTS iteration_types (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT DEFAULT '#6b7280',
	duration_days INTEGER NOT NULL DEFAULT 14,
	description TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- From portal_postgres.sql
CREATE TABLE IF NOT EXISTS contact_roles (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- INDEXES
-- ============================================
-- Additional indexes for base tables

CREATE INDEX IF NOT EXISTS idx_item_types_hierarchy_level ON item_types(hierarchy_level);
CREATE INDEX IF NOT EXISTS idx_item_types_sort_order ON item_types(sort_order);
CREATE INDEX IF NOT EXISTS idx_hierarchy_levels_level ON hierarchy_levels(level);
