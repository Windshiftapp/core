-- Permission system tables

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

CREATE TABLE IF NOT EXISTS user_global_permissions (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	permission_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(user_id, permission_id)
);

-- Note: user_workspace_permissions table removed - use role-based permissions via user_workspace_roles

CREATE INDEX IF NOT EXISTS idx_permissions_key ON permissions(permission_key);
CREATE INDEX IF NOT EXISTS idx_permissions_scope ON permissions(scope);
CREATE INDEX IF NOT EXISTS idx_user_global_permissions_user_id ON user_global_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_global_permissions_permission_id ON user_global_permissions(permission_id);

-- Workspace roles tables
CREATE TABLE IF NOT EXISTS workspace_roles (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	display_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_workspace_roles (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	role_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES workspace_roles(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(user_id, workspace_id, role_id)
);

CREATE TABLE IF NOT EXISTS role_permissions (
	id SERIAL PRIMARY KEY,
	role_id INTEGER NOT NULL,
	permission_id INTEGER NOT NULL,
	FOREIGN KEY (role_id) REFERENCES workspace_roles(id) ON DELETE CASCADE,
	FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
	UNIQUE(role_id, permission_id)
);

-- Group management tables (must be before group_workspace_roles)
CREATE TABLE IF NOT EXISTS groups (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	ldap_distinguished_name TEXT,
	ldap_common_name TEXT,
	ldap_sync_enabled BOOLEAN DEFAULT false,
	ldap_last_sync_at TIMESTAMP,
	is_system_group BOOLEAN DEFAULT false,
	is_active BOOLEAN DEFAULT true,
	scim_external_id TEXT, -- SCIM externalId from identity provider
	scim_managed BOOLEAN DEFAULT false, -- If true, group is managed via SCIM
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS group_members (
	id SERIAL PRIMARY KEY,
	group_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	ldap_sync_enabled BOOLEAN DEFAULT false,
	ldap_last_sync_at TIMESTAMP,
	scim_managed BOOLEAN DEFAULT false, -- If true, membership is managed via SCIM
	added_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(group_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_groups_name ON groups(name);
CREATE INDEX IF NOT EXISTS idx_groups_is_active ON groups(is_active);
CREATE INDEX IF NOT EXISTS idx_groups_ldap_sync ON groups(ldap_sync_enabled);
CREATE INDEX IF NOT EXISTS idx_groups_created_by ON groups(created_by);
CREATE UNIQUE INDEX IF NOT EXISTS idx_groups_scim_external_id ON groups(scim_external_id) WHERE scim_external_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_groups_scim_managed ON groups(scim_managed);
CREATE INDEX IF NOT EXISTS idx_group_members_group_id ON group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_group_members_user_id ON group_members(user_id);
CREATE INDEX IF NOT EXISTS idx_group_members_added_by ON group_members(added_by);

CREATE TABLE IF NOT EXISTS group_workspace_roles (
	id SERIAL PRIMARY KEY,
	group_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	role_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES workspace_roles(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(group_id, workspace_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_workspace_roles_name ON workspace_roles(name);
CREATE INDEX IF NOT EXISTS idx_workspace_roles_display_order ON workspace_roles(display_order);
CREATE INDEX IF NOT EXISTS idx_user_workspace_roles_user_id ON user_workspace_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_workspace_roles_workspace_id ON user_workspace_roles(workspace_id);
CREATE INDEX IF NOT EXISTS idx_user_workspace_roles_role_id ON user_workspace_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_group_workspace_roles_group_id ON group_workspace_roles(group_id);
CREATE INDEX IF NOT EXISTS idx_group_workspace_roles_workspace_id ON group_workspace_roles(workspace_id);
CREATE INDEX IF NOT EXISTS idx_group_workspace_roles_role_id ON group_workspace_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission_id ON role_permissions(permission_id);

-- Permission sets tables
CREATE TABLE IF NOT EXISTS permission_sets (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS permission_set_permissions (
	id SERIAL PRIMARY KEY,
	permission_set_id INTEGER NOT NULL,
	permission_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (permission_set_id) REFERENCES permission_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(permission_set_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_permission_sets_name ON permission_sets(name);
CREATE INDEX IF NOT EXISTS idx_permission_sets_is_system ON permission_sets(is_system);
CREATE INDEX IF NOT EXISTS idx_permission_sets_created_by ON permission_sets(created_by);
CREATE INDEX IF NOT EXISTS idx_permission_set_permissions_set_id ON permission_set_permissions(permission_set_id);
CREATE INDEX IF NOT EXISTS idx_permission_set_permissions_permission_id ON permission_set_permissions(permission_id);

-- Group permissions tables
CREATE TABLE IF NOT EXISTS group_global_permissions (
	id SERIAL PRIMARY KEY,
	group_id INTEGER NOT NULL,
	permission_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
	FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(group_id, permission_id)
);

-- Note: group_workspace_permissions table removed - use role-based permissions via group_workspace_roles

CREATE INDEX IF NOT EXISTS idx_group_global_permissions_group_id ON group_global_permissions(group_id);
CREATE INDEX IF NOT EXISTS idx_group_global_permissions_permission_id ON group_global_permissions(permission_id);

-- Default permissions
INSERT INTO permissions (permission_key, permission_name, description, scope, is_system) VALUES
	('system.admin', 'System Administrator', 'Full system administration access', 'global', true),
	('workspace.create', 'Create Workspaces', 'Can create new workspaces', 'global', false),
	('milestone.create', 'Manage Global Milestones', 'Can create and manage global milestones visible across all workspaces', 'global', false),
	('iteration.manage', 'Manage Global Iterations', 'Can create and manage global iterations visible across all workspaces', 'global', false),
	('user.list', 'List Users', 'Can view the user directory', 'global', false),
	('asset.manage', 'Manage Asset Sets', 'Can create and manage asset management sets', 'global', false),
	('customers.manage', 'Manage Customers', 'Can manage customer organisations and portal customers', 'global', false),
	('project.manage', 'Manage Time Projects', 'Can manage all time projects and worklogs', 'global', false),
	('workspace.admin', 'Manage Workspace', 'Full administration access to a workspace including settings and configuration', 'workspace', false),
	('item.view', 'View Workspace & Items', 'Can view workspace and work items', 'workspace', false),
	('item.create', 'Create Items', 'Can create work items in a workspace', 'workspace', false),
	('item.edit', 'Edit Items', 'Can edit work items in a workspace', 'workspace', false),
	('item.delete', 'Delete Items', 'Can delete work items in a workspace', 'workspace', false),
	('item.comment', 'Add Comment & Edit Own', 'Can add comments and edit own comments', 'workspace', false),
	('comment.edit_others', 'Edit Other Comments', 'Can edit comments created by other users', 'workspace', false),
	('test.view', 'View Tests', 'Can view test cases, test runs, and test results', 'workspace', false),
	('test.execute', 'Execute Tests', 'Can execute test runs and record test results', 'workspace', false),
	('test.manage', 'Manage Tests', 'Can create, edit, and delete test cases, sets, and folders', 'workspace', false),
	('action.manage', 'Manage Actions', 'Can create, edit, delete, and execute workspace actions', 'workspace', false)
ON CONFLICT (permission_key) DO NOTHING;

-- Default workspace roles
INSERT INTO workspace_roles (name, description, is_system, display_order)
VALUES
	('Viewer', 'Can view workspace content and participate in discussions', true, 1),
	('Editor', 'Can create and edit work items within the workspace', true, 2),
	('Administrator', 'Full workspace administration including permissions', true, 3),
	('Tester', 'Can view items, execute tests, manage test cases, and create defects', true, 4)
ON CONFLICT (name) DO NOTHING;

-- Default role permission mappings
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.view'
WHERE r.name = 'Viewer'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.comment'
WHERE r.name = 'Viewer'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.view'
WHERE r.name = 'Editor'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.comment'
WHERE r.name = 'Editor'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.edit'
WHERE r.name = 'Editor'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'workspace.admin'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.view'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.comment'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.edit'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.delete'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'comment.edit_others'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Tester role permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.view'
WHERE r.name = 'Tester'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.create'
WHERE r.name = 'Tester'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.view'
WHERE r.name = 'Tester'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.execute'
WHERE r.name = 'Tester'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.manage'
WHERE r.name = 'Tester'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Add item.create to Editor role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.create'
WHERE r.name = 'Editor'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Add item.create to Administrator role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'item.create'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Administrator role test permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.view'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.execute'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.manage'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Administrator role action permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'action.manage'
WHERE r.name = 'Administrator'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Editor role can view tests (read-only)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM workspace_roles r
JOIN permissions p ON p.permission_key = 'test.view'
WHERE r.name = 'Editor'
ON CONFLICT (role_id, permission_id) DO NOTHING;
