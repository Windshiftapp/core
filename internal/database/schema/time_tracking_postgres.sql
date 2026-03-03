-- Time tracking tables
-- Base tables (customer_organisations, time_project_categories) moved to base_tables_postgres.sql

CREATE TABLE IF NOT EXISTS time_projects (
	id SERIAL PRIMARY KEY,
	customer_id INTEGER,
	category_id INTEGER,
	name TEXT NOT NULL,
	description TEXT,
	status TEXT DEFAULT 'Active',
	color TEXT,
	hourly_rate REAL DEFAULT 0,
	active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (customer_id) REFERENCES customer_organisations(id) ON DELETE SET NULL,
	FOREIGN KEY (category_id) REFERENCES time_project_categories(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_time_projects_customer_id ON time_projects(customer_id);
CREATE INDEX IF NOT EXISTS idx_time_projects_category_id ON time_projects(category_id);
CREATE INDEX IF NOT EXISTS idx_time_projects_status ON time_projects(status);

-- workspace_time_project_categories is in workspace_postgres.sql (depends on workspaces table)
-- time_worklogs is in time_worklogs_postgres.sql (depends on items table)

-- Project managers: can edit project, manage members, view all worklogs
CREATE TABLE IF NOT EXISTS time_project_managers (
	id SERIAL PRIMARY KEY,
	project_id INTEGER NOT NULL,
	manager_type TEXT NOT NULL CHECK (manager_type IN ('user', 'group')),
	manager_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(project_id, manager_type, manager_id)
);

-- Project members: can book time on project
CREATE TABLE IF NOT EXISTS time_project_members (
	id SERIAL PRIMARY KEY,
	project_id INTEGER NOT NULL,
	member_type TEXT NOT NULL CHECK (member_type IN ('user', 'group')),
	member_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(project_id, member_type, member_id)
);

CREATE INDEX IF NOT EXISTS idx_time_project_managers_project ON time_project_managers(project_id);
CREATE INDEX IF NOT EXISTS idx_time_project_members_project ON time_project_members(project_id);