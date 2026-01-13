-- ============================================
-- CONFIG/WORKFLOWS TABLES
-- ============================================
-- Base tables (workflows, screens, priorities, item_types, hierarchy_levels, status_categories)
-- have been moved to base_tables_postgres.sql

-- ============================================
-- DEPENDENT TABLES (with foreign keys to base tables)
-- ============================================

CREATE TABLE IF NOT EXISTS statuses (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	category_id INTEGER NOT NULL,
	is_default BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (category_id) REFERENCES status_categories(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS configuration_sets (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	differentiate_by_item_type BOOLEAN DEFAULT false,
	workflow_id INTEGER REFERENCES workflows(id) ON DELETE SET NULL,
	default_item_type_id INTEGER REFERENCES item_types(id) ON DELETE SET NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS screen_fields (
	id SERIAL PRIMARY KEY,
	screen_id INTEGER NOT NULL,
	field_type TEXT NOT NULL,
	field_identifier TEXT NOT NULL,
	display_order INTEGER DEFAULT 0,
	is_required BOOLEAN DEFAULT false,
	field_width TEXT DEFAULT 'full',
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS screen_system_fields (
	id SERIAL PRIMARY KEY,
	screen_id INTEGER NOT NULL,
	field_name TEXT NOT NULL,
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE,
	UNIQUE(screen_id, field_name)
);

-- ============================================
-- JUNCTION/RELATIONSHIP TABLES
-- ============================================

-- Junction table for many-to-many relationship between configuration sets and item types
-- Includes optional overrides for workflow and screens per item type
CREATE TABLE IF NOT EXISTS configuration_set_item_types (
	id SERIAL PRIMARY KEY,
	configuration_set_id INTEGER NOT NULL,
	item_type_id INTEGER NOT NULL,
	-- Optional workflow override (NULL = use configuration set default)
	workflow_id INTEGER,
	-- Optional screen overrides (NULL = use configuration set defaults)
	create_screen_id INTEGER,
	edit_screen_id INTEGER,
	view_screen_id INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE CASCADE,
	FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE SET NULL,
	FOREIGN KEY (create_screen_id) REFERENCES screens(id) ON DELETE SET NULL,
	FOREIGN KEY (edit_screen_id) REFERENCES screens(id) ON DELETE SET NULL,
	FOREIGN KEY (view_screen_id) REFERENCES screens(id) ON DELETE SET NULL,
	UNIQUE(configuration_set_id, item_type_id)
);

-- Junction table for many-to-many relationship between configuration sets and priorities
CREATE TABLE IF NOT EXISTS configuration_set_priorities (
	id SERIAL PRIMARY KEY,
	configuration_set_id INTEGER NOT NULL,
	priority_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (priority_id) REFERENCES priorities(id) ON DELETE CASCADE,
	UNIQUE(configuration_set_id, priority_id)
);

-- Configuration Set Screen Assignments
CREATE TABLE IF NOT EXISTS configuration_set_screens (
	id SERIAL PRIMARY KEY,
	configuration_set_id INTEGER NOT NULL,
	screen_id INTEGER NOT NULL,
	context TEXT NOT NULL DEFAULT 'create', -- create, edit, view
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE,
	UNIQUE(configuration_set_id, context) -- One screen per context per configuration set
);

CREATE TABLE IF NOT EXISTS workspace_screens (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	screen_id INTEGER NOT NULL,
	context TEXT NOT NULL DEFAULT 'create',
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE,
	UNIQUE(workspace_id, screen_id, context)
);

-- One-to-many relationship: each workspace can only have one configuration set
CREATE TABLE IF NOT EXISTS workspace_configuration_sets (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL UNIQUE,
	configuration_set_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS workflow_transitions (
	id SERIAL PRIMARY KEY,
	workflow_id INTEGER NOT NULL,
	from_status_id INTEGER,  -- NULL means it's an initial status
	to_status_id INTEGER NOT NULL,
	display_order INTEGER DEFAULT 0,
	source_handle TEXT,  -- Connection point on source status (top, right, bottom, left)
	target_handle TEXT,  -- Connection point on target status (top, right, bottom, left)
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE,
	FOREIGN KEY (from_status_id) REFERENCES statuses(id) ON DELETE CASCADE,
	FOREIGN KEY (to_status_id) REFERENCES statuses(id) ON DELETE CASCADE,
	UNIQUE(workflow_id, from_status_id, to_status_id)
);

-- ============================================
-- INDEXES
-- ============================================

-- Configuration set indexes
CREATE INDEX IF NOT EXISTS idx_configuration_sets_workflow_id ON configuration_sets(workflow_id);

-- Item type indexes (moved to base_tables_postgres.sql)
CREATE INDEX IF NOT EXISTS idx_item_types_configuration_set_id ON item_types(configuration_set_id);

-- Screen field indexes
CREATE INDEX IF NOT EXISTS idx_screen_fields_screen_id ON screen_fields(screen_id);
CREATE INDEX IF NOT EXISTS idx_screen_system_fields_screen_id ON screen_system_fields(screen_id);

-- Workspace screen indexes
CREATE INDEX IF NOT EXISTS idx_workspace_screens_workspace_id ON workspace_screens(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_screens_screen_id ON workspace_screens(screen_id);

-- Status indexes
CREATE INDEX IF NOT EXISTS idx_statuses_category_id ON statuses(category_id);

-- Workflow transition indexes
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_workflow_id ON workflow_transitions(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_from_status ON workflow_transitions(from_status_id);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_to_status ON workflow_transitions(to_status_id);

-- Configuration set item type indexes
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_config_id ON configuration_set_item_types(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_item_type_id ON configuration_set_item_types(item_type_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_workflow_id ON configuration_set_item_types(workflow_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_create_screen_id ON configuration_set_item_types(create_screen_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_edit_screen_id ON configuration_set_item_types(edit_screen_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_view_screen_id ON configuration_set_item_types(view_screen_id);

-- Configuration set priority indexes
CREATE INDEX IF NOT EXISTS idx_config_set_priorities_config_id ON configuration_set_priorities(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_config_set_priorities_priority_id ON configuration_set_priorities(priority_id);

-- Configuration set screen indexes
CREATE INDEX IF NOT EXISTS idx_configuration_set_screens_config_id ON configuration_set_screens(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_configuration_set_screens_screen_id ON configuration_set_screens(screen_id);

-- Workspace configuration set indexes
CREATE INDEX IF NOT EXISTS idx_workspace_configuration_sets_workspace_id ON workspace_configuration_sets(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_configuration_sets_config_id ON workspace_configuration_sets(configuration_set_id);
