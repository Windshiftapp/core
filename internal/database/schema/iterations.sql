-- Iteration System Tables (SQLite)

-- Iteration types/categories (Sprint, PI, Release, Quarter, etc.)
CREATE TABLE IF NOT EXISTS iteration_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Iterations - can be global (shared) or local (workspace-specific)
CREATE TABLE IF NOT EXISTS iterations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	start_date DATE NOT NULL,  -- YYYY-MM-DD when iteration starts
	end_date DATE NOT NULL,    -- YYYY-MM-DD when iteration ends
	status TEXT NOT NULL DEFAULT 'planned', -- planned, active, completed, cancelled
	type_id INTEGER,  -- Reference to iteration type (Sprint, PI, etc.)
	is_global INTEGER NOT NULL DEFAULT 0,  -- 0=local to workspace, 1=global (shared across workspaces)
	workspace_id INTEGER,  -- NULL if global, set if local to specific workspace
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (type_id) REFERENCES iteration_types(id) ON DELETE SET NULL,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	CHECK (end_date >= start_date),
	CHECK (
		(is_global = 1 AND workspace_id IS NULL) OR
		(is_global = 0 AND workspace_id IS NOT NULL)
	)
);

-- Indexes for iteration system
CREATE INDEX IF NOT EXISTS idx_iterations_type_id ON iterations(type_id);
CREATE INDEX IF NOT EXISTS idx_iterations_status ON iterations(status);
CREATE INDEX IF NOT EXISTS idx_iterations_workspace_id ON iterations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_iterations_is_global ON iterations(is_global);
CREATE INDEX IF NOT EXISTS idx_iterations_dates ON iterations(start_date, end_date);

-- Seed default iteration types
INSERT OR IGNORE INTO iteration_types (name, color, description) VALUES
('Sprint', '#3b82f6', 'Short-term development cycle (typically 1-4 weeks)'),
('PI / Quarter', '#8b5cf6', 'Program Increment or Quarterly cycle (typically 8-12 weeks)'),
('Release', '#f59e0b', 'Product release cycle');
