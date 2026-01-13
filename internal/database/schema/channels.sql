-- Channels system tables

-- Channel Categories table for organizing channels
CREATE TABLE IF NOT EXISTS channel_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL DEFAULT '#3b82f6',
	description TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Channels system table for inbound/outbound integrations
CREATE TABLE IF NOT EXISTS channels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	type TEXT NOT NULL, -- smtp, webhook, portal, imap, etc.
	direction TEXT NOT NULL, -- inbound, outbound
	description TEXT,
	status TEXT NOT NULL DEFAULT 'disabled', -- enabled, disabled
	is_default BOOLEAN DEFAULT false,
	config TEXT, -- JSON configuration data specific to channel type
	plugin_name TEXT, -- NULL for user-created, plugin name for plugin-managed
	plugin_webhook_id TEXT, -- Plugin's internal webhook identifier
	category_id INTEGER REFERENCES channel_categories(id) ON DELETE SET NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_activity DATETIME
);

CREATE INDEX IF NOT EXISTS idx_channels_type ON channels(type);
CREATE INDEX IF NOT EXISTS idx_channels_direction ON channels(direction);
CREATE INDEX IF NOT EXISTS idx_channels_status ON channels(status);
CREATE INDEX IF NOT EXISTS idx_channels_is_default ON channels(is_default);
CREATE INDEX IF NOT EXISTS idx_channels_plugin_name ON channels(plugin_name);
CREATE INDEX IF NOT EXISTS idx_channels_category_id ON channels(category_id);

-- Channel managers table for access control
CREATE TABLE IF NOT EXISTS channel_managers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id INTEGER NOT NULL,
	manager_type TEXT NOT NULL CHECK (manager_type IN ('user', 'group')),
	manager_id INTEGER NOT NULL,
	added_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(channel_id, manager_type, manager_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_managers_channel ON channel_managers(channel_id);
CREATE INDEX IF NOT EXISTS idx_channel_managers_manager ON channel_managers(manager_type, manager_id);
