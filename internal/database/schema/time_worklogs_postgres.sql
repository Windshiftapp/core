-- Time worklogs table (separated from time_tracking due to dependency on items table)
CREATE TABLE IF NOT EXISTS time_worklogs (
	id SERIAL PRIMARY KEY,
	project_id INTEGER NOT NULL,
	customer_id INTEGER NOT NULL,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
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

CREATE INDEX IF NOT EXISTS idx_time_worklogs_project_id ON time_worklogs(project_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_customer_id ON time_worklogs(customer_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_date ON time_worklogs(date);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_item_id ON time_worklogs(item_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_user_id ON time_worklogs(user_id);