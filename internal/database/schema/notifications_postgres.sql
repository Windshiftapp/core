-- Notifications system tables

-- Notifications system table
CREATE TABLE IF NOT EXISTS notifications (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'info', -- info, warning, error, success, assignment, comment, status_change, reminder, milestone
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	read BOOLEAN DEFAULT false,
	sent_at TIMESTAMP, -- When notification was sent via email (NULL if not sent)
	avatar TEXT, -- Initials or avatar identifier
	action_url TEXT, -- URL to navigate to when clicked
	metadata TEXT, -- JSON for additional data
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_timestamp ON notifications(timestamp);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read);
CREATE INDEX IF NOT EXISTS idx_notifications_sent_at ON notifications(sent_at);
CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);

-- Notification templates table is defined in base_tables_postgres.sql
-- Default templates are inserted via initializePostgresDefaultData() to avoid semicolon parsing issues

-- Notification settings system for configuration sets
CREATE TABLE IF NOT EXISTS notification_settings (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	is_active BOOLEAN DEFAULT true,
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_settings_active ON notification_settings(is_active);
CREATE INDEX IF NOT EXISTS idx_notification_settings_created_by ON notification_settings(created_by);

-- Notification event rules for each setting
CREATE TABLE IF NOT EXISTS notification_event_rules (
	id SERIAL PRIMARY KEY,
	notification_setting_id INTEGER NOT NULL,
	event_type TEXT NOT NULL,
	is_enabled BOOLEAN DEFAULT true,
	notify_assignee BOOLEAN DEFAULT false,
	notify_creator BOOLEAN DEFAULT false,
	notify_watchers BOOLEAN DEFAULT false,
	notify_workspace_admins BOOLEAN DEFAULT false,
	custom_recipients TEXT, -- JSON array of user IDs or email addresses
	message_template TEXT, -- Custom message template (optional)
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (notification_setting_id) REFERENCES notification_settings(id) ON DELETE CASCADE,
	UNIQUE(notification_setting_id, event_type)
);

CREATE INDEX IF NOT EXISTS idx_notification_event_rules_setting_id ON notification_event_rules(notification_setting_id);
CREATE INDEX IF NOT EXISTS idx_notification_event_rules_event_type ON notification_event_rules(event_type);
CREATE INDEX IF NOT EXISTS idx_notification_event_rules_enabled ON notification_event_rules(is_enabled);

-- Link notification settings to configuration sets
CREATE TABLE IF NOT EXISTS configuration_set_notification_settings (
	id SERIAL PRIMARY KEY,
	configuration_set_id INTEGER NOT NULL,
	notification_setting_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (notification_setting_id) REFERENCES notification_settings(id) ON DELETE CASCADE,
	UNIQUE(configuration_set_id, notification_setting_id)
);

CREATE INDEX IF NOT EXISTS idx_config_set_notification_settings_config_set ON configuration_set_notification_settings(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_config_set_notification_settings_notification ON configuration_set_notification_settings(notification_setting_id);
