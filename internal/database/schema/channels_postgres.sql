-- Channels system tables
-- Channels table moved to base_tables_postgres.sql

-- Channel managers table for access control
CREATE TABLE IF NOT EXISTS channel_managers (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	manager_type TEXT NOT NULL CHECK (manager_type IN ('user', 'group')),
	manager_id INTEGER NOT NULL,
	added_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(channel_id, manager_type, manager_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_managers_channel ON channel_managers(channel_id);
CREATE INDEX IF NOT EXISTS idx_channel_managers_manager ON channel_managers(manager_type, manager_id);
