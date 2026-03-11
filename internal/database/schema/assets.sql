-- Asset Management System Tables
-- This schema provides system-wide asset management with sets, types, categories, and assets

-- Asset Management Sets (system-wide containers)
CREATE TABLE IF NOT EXISTS asset_management_sets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_asset_management_sets_name ON asset_management_sets(name);
CREATE INDEX IF NOT EXISTS idx_asset_management_sets_is_default ON asset_management_sets(is_default);

-- Asset Roles (similar to workspace_roles)
CREATE TABLE IF NOT EXISTS asset_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT 0,
	display_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_asset_roles_name ON asset_roles(name);
CREATE INDEX IF NOT EXISTS idx_asset_roles_display_order ON asset_roles(display_order);

-- Asset Permissions
CREATE TABLE IF NOT EXISTS asset_permissions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	permission_key TEXT NOT NULL UNIQUE,
	permission_name TEXT NOT NULL,
	description TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_asset_permissions_key ON asset_permissions(permission_key);

-- Asset Role to Permission mapping
CREATE TABLE IF NOT EXISTS asset_role_permissions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	role_id INTEGER NOT NULL,
	permission_id INTEGER NOT NULL,
	FOREIGN KEY (role_id) REFERENCES asset_roles(id) ON DELETE CASCADE,
	FOREIGN KEY (permission_id) REFERENCES asset_permissions(id) ON DELETE CASCADE,
	UNIQUE(role_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_role_permissions_role_id ON asset_role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_asset_role_permissions_permission_id ON asset_role_permissions(permission_id);

-- User role assignments per set (new role-based system)
CREATE TABLE IF NOT EXISTS user_asset_set_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	set_id INTEGER NOT NULL,
	role_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES asset_roles(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(user_id, set_id)
);

CREATE INDEX IF NOT EXISTS idx_user_asset_set_roles_user_id ON user_asset_set_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_asset_set_roles_set_id ON user_asset_set_roles(set_id);
CREATE INDEX IF NOT EXISTS idx_user_asset_set_roles_role_id ON user_asset_set_roles(role_id);

-- Group role assignments per set (new role-based system)
CREATE TABLE IF NOT EXISTS group_asset_set_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	group_id INTEGER NOT NULL,
	set_id INTEGER NOT NULL,
	role_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES asset_roles(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(group_id, set_id)
);

CREATE INDEX IF NOT EXISTS idx_group_asset_set_roles_group_id ON group_asset_set_roles(group_id);
CREATE INDEX IF NOT EXISTS idx_group_asset_set_roles_set_id ON group_asset_set_roles(set_id);
CREATE INDEX IF NOT EXISTS idx_group_asset_set_roles_role_id ON group_asset_set_roles(role_id);

-- Everyone default role per set (similar to workspace_everyone_roles)
CREATE TABLE IF NOT EXISTS asset_set_everyone_roles (
	set_id INTEGER PRIMARY KEY,
	role_id INTEGER,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES asset_roles(id) ON DELETE SET NULL,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Default asset roles
INSERT OR IGNORE INTO asset_roles (name, description, is_system, display_order) VALUES
	('Viewer', 'Can view assets and categories', 1, 1),
	('Editor', 'Can create and edit assets', 1, 2),
	('Administrator', 'Full access including set configuration', 1, 3);

-- Default asset permissions
INSERT OR IGNORE INTO asset_permissions (permission_key, permission_name, description) VALUES
	('asset.view', 'View Assets', 'Can view assets, types, and categories'),
	('asset.create', 'Create Assets', 'Can create new assets'),
	('asset.edit', 'Edit Assets', 'Can edit existing assets'),
	('asset.delete', 'Delete Assets', 'Can delete assets'),
	('asset.admin', 'Administer Set', 'Can manage set configuration and permissions');

-- Viewer role permissions: asset.view
INSERT OR IGNORE INTO asset_role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM asset_roles r, asset_permissions p
WHERE r.name = 'Viewer' AND p.permission_key = 'asset.view';

-- Editor role permissions: asset.view, asset.create, asset.edit
INSERT OR IGNORE INTO asset_role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM asset_roles r, asset_permissions p
WHERE r.name = 'Editor' AND p.permission_key IN ('asset.view', 'asset.create', 'asset.edit');

-- Administrator role permissions: all
INSERT OR IGNORE INTO asset_role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM asset_roles r, asset_permissions p
WHERE r.name = 'Administrator';

-- Asset Types (define structure of assets within a set)
CREATE TABLE IF NOT EXISTS asset_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	set_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	icon TEXT DEFAULT 'Box',
	color TEXT DEFAULT '#6b7280',
	display_order INTEGER DEFAULT 0,
	is_active BOOLEAN DEFAULT true,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	UNIQUE(set_id, name)
);

CREATE INDEX IF NOT EXISTS idx_asset_types_set_id ON asset_types(set_id);
CREATE INDEX IF NOT EXISTS idx_asset_types_name ON asset_types(name);
CREATE INDEX IF NOT EXISTS idx_asset_types_display_order ON asset_types(display_order);
CREATE INDEX IF NOT EXISTS idx_asset_types_is_active ON asset_types(is_active);

-- Asset Type Custom Fields (junction: which custom fields apply to which asset type)
CREATE TABLE IF NOT EXISTS asset_type_fields (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	asset_type_id INTEGER NOT NULL,
	custom_field_id INTEGER NOT NULL,
	is_required BOOLEAN DEFAULT false,
	display_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (asset_type_id) REFERENCES asset_types(id) ON DELETE CASCADE,
	FOREIGN KEY (custom_field_id) REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
	UNIQUE(asset_type_id, custom_field_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_type_fields_asset_type_id ON asset_type_fields(asset_type_id);
CREATE INDEX IF NOT EXISTS idx_asset_type_fields_custom_field_id ON asset_type_fields(custom_field_id);
CREATE INDEX IF NOT EXISTS idx_asset_type_fields_display_order ON asset_type_fields(display_order);

-- Asset Categories (hierarchical organization within a set)
CREATE TABLE IF NOT EXISTS asset_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	set_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	parent_id INTEGER REFERENCES asset_categories(id) ON DELETE CASCADE,
	path TEXT DEFAULT '/',
	has_children BOOLEAN DEFAULT false,
	children_count INTEGER DEFAULT 0,
	descendants_count INTEGER DEFAULT 0,
	frac_index TEXT COLLATE BINARY,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_asset_categories_set_id ON asset_categories(set_id);
CREATE INDEX IF NOT EXISTS idx_asset_categories_parent_id ON asset_categories(parent_id);
CREATE INDEX IF NOT EXISTS idx_asset_categories_path ON asset_categories(path);
CREATE INDEX IF NOT EXISTS idx_asset_categories_set_parent ON asset_categories(set_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_asset_categories_frac_index ON asset_categories(frac_index) WHERE frac_index IS NOT NULL;

-- Asset Statuses (configurable statuses per set)
CREATE TABLE IF NOT EXISTS asset_statuses (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	set_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	color TEXT DEFAULT '#6b7280',
	description TEXT,
	is_default BOOLEAN DEFAULT false,
	display_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	UNIQUE(set_id, name)
);

CREATE INDEX IF NOT EXISTS idx_asset_statuses_set_id ON asset_statuses(set_id);
CREATE INDEX IF NOT EXISTS idx_asset_statuses_is_default ON asset_statuses(set_id, is_default);
CREATE INDEX IF NOT EXISTS idx_asset_statuses_display_order ON asset_statuses(display_order);

-- Assets (individual asset instances)
CREATE TABLE IF NOT EXISTS assets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	set_id INTEGER NOT NULL,
	asset_type_id INTEGER NOT NULL,
	category_id INTEGER REFERENCES asset_categories(id) ON DELETE SET NULL,
	status_id INTEGER REFERENCES asset_statuses(id) ON DELETE SET NULL,
	title TEXT NOT NULL,
	description TEXT,
	asset_tag TEXT,
	custom_field_values TEXT,
	frac_index TEXT COLLATE BINARY,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (asset_type_id) REFERENCES asset_types(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_assets_set_id ON assets(set_id);
CREATE INDEX IF NOT EXISTS idx_assets_asset_type_id ON assets(asset_type_id);
CREATE INDEX IF NOT EXISTS idx_assets_category_id ON assets(category_id);
CREATE INDEX IF NOT EXISTS idx_assets_title ON assets(title);
CREATE INDEX IF NOT EXISTS idx_assets_asset_tag ON assets(asset_tag) WHERE asset_tag IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assets_status_id ON assets(status_id);
CREATE INDEX IF NOT EXISTS idx_assets_set_category ON assets(set_id, category_id);
CREATE INDEX IF NOT EXISTS idx_assets_set_type ON assets(set_id, asset_type_id);
CREATE INDEX IF NOT EXISTS idx_assets_frac_index ON assets(frac_index) WHERE frac_index IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assets_created_by ON assets(created_by);

-- User's default/primary asset management set preference
CREATE TABLE IF NOT EXISTS user_asset_set_preferences (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL UNIQUE,
	primary_set_id INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (primary_set_id) REFERENCES asset_management_sets(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_user_asset_set_preferences_user_id ON user_asset_set_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_user_asset_set_preferences_primary_set_id ON user_asset_set_preferences(primary_set_id);

-- Asset Import Jobs (CSV import tracking)
CREATE TABLE IF NOT EXISTS asset_import_jobs (
	id TEXT PRIMARY KEY,
	set_id INTEGER NOT NULL REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	status TEXT NOT NULL DEFAULT 'queued',
	phase TEXT DEFAULT 'initializing',
	file_path TEXT NOT NULL,
	config_json TEXT,
	progress_json TEXT,
	error_message TEXT,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	completed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_asset_import_jobs_set_id ON asset_import_jobs(set_id);
CREATE INDEX IF NOT EXISTS idx_asset_import_jobs_status ON asset_import_jobs(status);
CREATE INDEX IF NOT EXISTS idx_asset_import_jobs_created_by ON asset_import_jobs(created_by);
