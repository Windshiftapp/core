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