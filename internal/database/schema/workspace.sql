	CREATE TABLE IF NOT EXISTS workspace_field_requirements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id INTEGER NOT NULL,
		custom_field_id INTEGER NOT NULL,
		is_required BOOLEAN DEFAULT false,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
		FOREIGN KEY (custom_field_id) REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
		UNIQUE(workspace_id, custom_field_id)
	);

	CREATE INDEX IF NOT EXISTS idx_workspace_field_requirements_workspace_id ON workspace_field_requirements(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_workspace_field_requirements_field_id ON workspace_field_requirements(custom_field_id);

	CREATE TABLE IF NOT EXISTS workspaces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		key TEXT UNIQUE NOT NULL, -- Workspace key for issue prefixes (e.g., TEST, PROJ)
		description TEXT,
		time_project_id INTEGER REFERENCES time_projects(id) ON DELETE SET NULL,
		active BOOLEAN DEFAULT 1,
		is_personal BOOLEAN DEFAULT FALSE, -- Flag for personal workspaces
		owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL, -- Owner for personal workspaces
		icon TEXT,
		color TEXT,
		avatar_url TEXT,
		homepage_layout TEXT, -- JSON array of gadget configurations
		default_view TEXT DEFAULT 'board', -- Default view when entering workspace (board, backlog, list, tree, map)
		display_mode TEXT DEFAULT 'default', -- Display mode for workspace layout (default, board)
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_workspaces_is_personal ON workspaces(is_personal);
	CREATE INDEX IF NOT EXISTS idx_workspaces_owner_id ON workspaces(owner_id);

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
