-- Time tracking tables
-- Customer organisations (B2B entities for time tracking)
CREATE TABLE IF NOT EXISTS customer_organisations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT,
	description TEXT,
	active BOOLEAN DEFAULT 1,
	avatar_url TEXT,
	custom_field_values TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_project_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	color TEXT DEFAULT '#3B82F6',
	display_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id INTEGER,
	category_id INTEGER,
	name TEXT NOT NULL,
	description TEXT,
	status TEXT DEFAULT 'Active',
	color TEXT,
	hourly_rate REAL DEFAULT 0,
	active BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (customer_id) REFERENCES customer_organisations(id) ON DELETE SET NULL,
	FOREIGN KEY (category_id) REFERENCES time_project_categories(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS workspace_time_project_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
	time_project_category_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (time_project_category_id) REFERENCES time_project_categories(id) ON DELETE CASCADE,
	UNIQUE(workspace_id, time_project_category_id)
);

CREATE TABLE IF NOT EXISTS time_worklogs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	customer_id INTEGER NOT NULL,
	item_id INTEGER REFERENCES items(id) ON DELETE SET NULL,
	description TEXT NOT NULL,
	date INTEGER NOT NULL,
	start_time INTEGER NOT NULL,
	end_time INTEGER NOT NULL,
	duration_minutes INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE,
	FOREIGN KEY (customer_id) REFERENCES customer_organisations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_time_projects_customer_id ON time_projects(customer_id);
CREATE INDEX IF NOT EXISTS idx_time_projects_category_id ON time_projects(category_id);
CREATE INDEX IF NOT EXISTS idx_time_projects_status ON time_projects(status);
CREATE INDEX IF NOT EXISTS idx_workspace_time_project_categories_workspace_id ON workspace_time_project_categories(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_time_project_categories_category_id ON workspace_time_project_categories(time_project_category_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_project_id ON time_worklogs(project_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_customer_id ON time_worklogs(customer_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_date ON time_worklogs(date);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_item_id ON time_worklogs(item_id);
