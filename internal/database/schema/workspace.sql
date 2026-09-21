	CREATE TABLE IF NOT EXISTS workspaces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		key TEXT UNIQUE NOT NULL, -- Workspace key for issue prefixes (e.g., TEST, PROJ)
		description TEXT,
		time_project_id INTEGER REFERENCES time_projects(id) ON DELETE SET NULL,
		active BOOLEAN DEFAULT TRUE,
		is_personal BOOLEAN DEFAULT FALSE, -- Flag for personal workspaces
		owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL, -- Owner for personal workspaces
		icon TEXT,
		color TEXT,
		avatar_url TEXT,
		homepage_layout TEXT, -- JSON array of gadget configurations
		default_view TEXT DEFAULT 'board', -- Default view when entering workspace (board, backlog, list, tree, map)
		display_mode TEXT DEFAULT 'default', -- Display mode for workspace layout (default, board)
		internal_comments_enabled BOOLEAN DEFAULT FALSE, -- Allow internal comments on all items (not just portal requests)
		is_template BOOLEAN DEFAULT FALSE, -- Workspace can be used as a creation template
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_workspaces_is_personal ON workspaces(is_personal);
	CREATE INDEX IF NOT EXISTS idx_workspaces_owner_id ON workspaces(owner_id);
	-- One personal workspace per owner, enforced at the schema level so the
	-- get-or-create path can never race into duplicates.
	CREATE UNIQUE INDEX IF NOT EXISTS uq_workspaces_personal_owner ON workspaces(owner_id) WHERE is_personal = TRUE;
	-- migration: 20260918_workspaces_personal_owner_uniqueness
	CREATE INDEX IF NOT EXISTS idx_workspaces_template_active ON workspaces(is_template, active) WHERE is_template = TRUE;

	-- migration: 20260815_workspaces_is_template

	-- Junction table for workspace and time project categories relationship
	CREATE TABLE IF NOT EXISTS workspace_time_project_categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id INTEGER NOT NULL,
		time_project_category_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
		FOREIGN KEY (time_project_category_id) REFERENCES time_project_categories(id) ON DELETE CASCADE,
		UNIQUE(workspace_id, time_project_category_id)
	);

	CREATE INDEX IF NOT EXISTS idx_workspace_time_project_categories_workspace_id ON workspace_time_project_categories(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_workspace_time_project_categories_category_id ON workspace_time_project_categories(time_project_category_id);

-- migration: 0000_baseline
