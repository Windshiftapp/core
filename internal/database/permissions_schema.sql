-- Permission System Schema Design
-- Supports both global and workspace-specific permissions
-- Extensible and future-ready for groups

-- Core permission definitions
CREATE TABLE permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    permission_key TEXT UNIQUE NOT NULL, -- e.g., 'system.admin', 'workspace.create', 'milestone.create'
    permission_name TEXT NOT NULL, -- Human-readable name
    description TEXT, -- Description of what this permission allows
    scope TEXT NOT NULL CHECK (scope IN ('global', 'workspace')), -- Permission scope
    is_system BOOLEAN DEFAULT 0, -- System permissions (like admin) that shouldn't be deleted
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- User global permissions (system-wide permissions)
CREATE TABLE user_global_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    granted_by INTEGER, -- User ID who granted this permission
    granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(user_id, permission_id)
);

-- User workspace permissions (workspace-specific permissions)
CREATE TABLE user_workspace_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    workspace_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    granted_by INTEGER, -- User ID who granted this permission
    granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(user_id, workspace_id, permission_id)
);

-- Future: Group definitions (for later implementation)
CREATE TABLE groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_name TEXT UNIQUE NOT NULL,
    description TEXT,
    workspace_id INTEGER, -- NULL for global groups
    created_by INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
);

-- Future: Group memberships
CREATE TABLE user_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    added_by INTEGER,
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(user_id, group_id)
);

-- Future: Group global permissions
CREATE TABLE group_global_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    granted_by INTEGER,
    granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(group_id, permission_id)
);

-- Future: Group workspace permissions
CREATE TABLE group_workspace_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    workspace_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    granted_by INTEGER,
    granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(group_id, workspace_id, permission_id)
);

-- Indexes for performance
CREATE INDEX idx_user_global_permissions_user_id ON user_global_permissions(user_id);
CREATE INDEX idx_user_global_permissions_permission_id ON user_global_permissions(permission_id);

CREATE INDEX idx_user_workspace_permissions_user_id ON user_workspace_permissions(user_id);
CREATE INDEX idx_user_workspace_permissions_workspace_id ON user_workspace_permissions(workspace_id);
CREATE INDEX idx_user_workspace_permissions_permission_id ON user_workspace_permissions(permission_id);

CREATE INDEX idx_permissions_scope ON permissions(scope);
CREATE INDEX idx_permissions_key ON permissions(permission_key);

-- Initial permission data
INSERT INTO permissions (permission_key, permission_name, description, scope, is_system) VALUES
('system.admin', 'System Administrator', 'Full system administration access', 'global', 1),
('workspace.create', 'Create Workspaces', 'Can create new workspaces', 'global', 0),
('milestone.create', 'Create Milestones', 'Can create milestones', 'global', 0),
('user.list', 'List Users', 'Can view the user directory', 'global', 0),
('workspace.admin', 'Manage Workspace', 'Full administration access to a workspace including settings and configuration', 'workspace', 0),
('item.view', 'View Workspace & Items', 'Can view workspace and work items', 'workspace', 0),
('item.edit', 'Edit Items', 'Can edit work items in a workspace', 'workspace', 0),
('item.delete', 'Delete Items', 'Can delete work items in a workspace', 'workspace', 0),
('item.comment', 'Add Comment & Edit Own', 'Can add comments and edit own comments', 'workspace', 0),
('comment.edit_others', 'Edit Other Comments', 'Can edit comments created by other users', 'workspace', 0);