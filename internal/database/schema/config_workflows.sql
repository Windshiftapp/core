CREATE TABLE IF NOT EXISTS configuration_sets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	differentiate_by_item_type BOOLEAN DEFAULT false,
	workflow_id INTEGER REFERENCES workflows(id) ON DELETE SET NULL,
	default_item_type_id INTEGER REFERENCES item_types(id) ON DELETE SET NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS item_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	configuration_set_id INTEGER, -- Made nullable for many-to-many relationship
	name TEXT NOT NULL UNIQUE, -- Changed to global unique (not per config set)
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	icon TEXT DEFAULT 'FileText',
	color TEXT DEFAULT '#3b82f6',
	hierarchy_level INTEGER DEFAULT 3,
	sort_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Junction table for many-to-many relationship between configuration sets and item types
-- Includes optional overrides for workflow and screens per item type
CREATE TABLE IF NOT EXISTS configuration_set_item_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	configuration_set_id INTEGER NOT NULL,
	item_type_id INTEGER NOT NULL,
	-- Optional workflow override (NULL = use configuration set default)
	workflow_id INTEGER,
	-- Optional screen overrides (NULL = use configuration set defaults)
	create_screen_id INTEGER,
	edit_screen_id INTEGER,
	view_screen_id INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE CASCADE,
	FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE SET NULL,
	FOREIGN KEY (create_screen_id) REFERENCES screens(id) ON DELETE SET NULL,
	FOREIGN KEY (edit_screen_id) REFERENCES screens(id) ON DELETE SET NULL,
	FOREIGN KEY (view_screen_id) REFERENCES screens(id) ON DELETE SET NULL,
	UNIQUE(configuration_set_id, item_type_id)
);

CREATE INDEX IF NOT EXISTS idx_config_set_item_types_config_id ON configuration_set_item_types(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_item_type_id ON configuration_set_item_types(item_type_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_workflow_id ON configuration_set_item_types(workflow_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_create_screen_id ON configuration_set_item_types(create_screen_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_edit_screen_id ON configuration_set_item_types(edit_screen_id);
CREATE INDEX IF NOT EXISTS idx_config_set_item_types_view_screen_id ON configuration_set_item_types(view_screen_id);

CREATE TABLE IF NOT EXISTS priorities (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	icon TEXT DEFAULT 'AlertCircle',
	color TEXT DEFAULT '#3b82f6',
	sort_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Junction table for many-to-many relationship between configuration sets and priorities
CREATE TABLE IF NOT EXISTS configuration_set_priorities (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	configuration_set_id INTEGER NOT NULL,
	priority_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (priority_id) REFERENCES priorities(id) ON DELETE CASCADE,
	UNIQUE(configuration_set_id, priority_id)
);

CREATE INDEX IF NOT EXISTS idx_config_set_priorities_config_id ON configuration_set_priorities(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_config_set_priorities_priority_id ON configuration_set_priorities(priority_id);

CREATE TABLE IF NOT EXISTS hierarchy_levels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	level INTEGER NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS screens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS screen_fields (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	screen_id INTEGER NOT NULL,
	field_type TEXT NOT NULL,
	field_identifier TEXT NOT NULL,
	display_order INTEGER DEFAULT 0,
	is_required BOOLEAN DEFAULT false,
	field_width TEXT DEFAULT 'full',
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS screen_system_fields (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	screen_id INTEGER NOT NULL,
	field_name TEXT NOT NULL,
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE,
	UNIQUE(screen_id, field_name)
);

CREATE TABLE IF NOT EXISTS workspace_screens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
	screen_id INTEGER NOT NULL,
	context TEXT NOT NULL DEFAULT 'create',
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE,
	UNIQUE(workspace_id, screen_id, context)
);

CREATE INDEX IF NOT EXISTS idx_configuration_sets_workflow_id ON configuration_sets(workflow_id);
CREATE INDEX IF NOT EXISTS idx_item_types_configuration_set_id ON item_types(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_item_types_hierarchy_level ON item_types(hierarchy_level);
CREATE INDEX IF NOT EXISTS idx_item_types_sort_order ON item_types(sort_order);
CREATE INDEX IF NOT EXISTS idx_hierarchy_levels_level ON hierarchy_levels(level);
CREATE INDEX IF NOT EXISTS idx_screen_fields_screen_id ON screen_fields(screen_id);
CREATE INDEX IF NOT EXISTS idx_screen_system_fields_screen_id ON screen_system_fields(screen_id);
CREATE INDEX IF NOT EXISTS idx_workspace_screens_workspace_id ON workspace_screens(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_screens_screen_id ON workspace_screens(screen_id);

-- Workflow System Tables
CREATE TABLE IF NOT EXISTS status_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	is_completed BOOLEAN NOT NULL DEFAULT false,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS statuses (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	category_id INTEGER NOT NULL,
	is_default BOOLEAN DEFAULT false,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (category_id) REFERENCES status_categories(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS workflows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS workflow_transitions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workflow_id INTEGER NOT NULL,
	from_status_id INTEGER,  -- NULL means it's an initial status
	to_status_id INTEGER NOT NULL,
	display_order INTEGER DEFAULT 0,
	source_handle TEXT,  -- Connection point on source status (top, right, bottom, left)
	target_handle TEXT,  -- Connection point on target status (top, right, bottom, left)
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE,
	FOREIGN KEY (from_status_id) REFERENCES statuses(id) ON DELETE CASCADE,
	FOREIGN KEY (to_status_id) REFERENCES statuses(id) ON DELETE CASCADE,
	UNIQUE(workflow_id, from_status_id, to_status_id)
);

-- Indexes for workflow system
CREATE INDEX IF NOT EXISTS idx_statuses_category_id ON statuses(category_id);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_workflow_id ON workflow_transitions(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_from_status ON workflow_transitions(from_status_id);
CREATE INDEX IF NOT EXISTS idx_workflow_transitions_to_status ON workflow_transitions(to_status_id);

-- Configuration Set Screen Assignments
CREATE TABLE IF NOT EXISTS configuration_set_screens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	configuration_set_id INTEGER NOT NULL,
	screen_id INTEGER NOT NULL,
	context TEXT NOT NULL DEFAULT 'create', -- create, edit, view
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (screen_id) REFERENCES screens(id) ON DELETE CASCADE,
	UNIQUE(configuration_set_id, context) -- One screen per context per configuration set
);

CREATE INDEX IF NOT EXISTS idx_configuration_set_screens_config_id ON configuration_set_screens(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_configuration_set_screens_screen_id ON configuration_set_screens(screen_id);

-- One-to-many relationship: each workspace can only have one configuration set
CREATE TABLE IF NOT EXISTS workspace_configuration_sets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL UNIQUE,
	configuration_set_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_workspace_configuration_sets_workspace_id ON workspace_configuration_sets(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_configuration_sets_config_id ON workspace_configuration_sets(configuration_set_id);
