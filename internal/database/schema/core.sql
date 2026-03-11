-- Core tables (projects, custom field definitions)

CREATE TABLE IF NOT EXISTS projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER,
	name TEXT NOT NULL,
	description TEXT,
	active BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_projects_workspace_id ON projects(workspace_id);

CREATE TABLE IF NOT EXISTS custom_field_definitions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	field_type TEXT NOT NULL,
	description TEXT,
	required BOOLEAN DEFAULT false,
	options TEXT,
	display_order INTEGER DEFAULT 0,
	system_default BOOLEAN DEFAULT false,
	applies_to_portal_customers BOOLEAN DEFAULT false,
	applies_to_customer_organisations BOOLEAN DEFAULT false,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS custom_field_indexes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	custom_field_id INTEGER NOT NULL,
	target_table TEXT NOT NULL,
	index_name TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (custom_field_id) REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
	UNIQUE(custom_field_id, target_table)
);
