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

-- Outbound webhook delivery audit log: one row per send attempt (success or failure),
-- including plugin-dispatched webhooks. Surfaced on the admin Diagnostics page.
CREATE TABLE IF NOT EXISTS webhook_deliveries (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	item_id INTEGER,
	event_type TEXT NOT NULL,
	attempt_type TEXT NOT NULL,
	transport TEXT NOT NULL DEFAULT 'http',
	request_url TEXT,
	requested_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	response_status_code INTEGER,
	response_time_ms INTEGER,
	success BOOLEAN NOT NULL DEFAULT FALSE,
	error_message TEXT,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
	-- FK on item_id is added in items_postgres.sql once items has been created
	-- (channels runs before items in the schema order; Postgres validates FK
	-- target tables exist at CREATE TABLE time, unlike SQLite).
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_channel_id ON webhook_deliveries(channel_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_requested_at ON webhook_deliveries(requested_at);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_success ON webhook_deliveries(success);
