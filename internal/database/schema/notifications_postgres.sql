-- Notifications system tables

-- Notifications system table
CREATE TABLE IF NOT EXISTS notifications (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'info', -- info, warning, error, success, assignment, comment, status_change, reminder, milestone
	timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	read BOOLEAN DEFAULT false,
	-- seen_at marks "user looked at this in the tray" without acknowledging it.
	-- Distinct from `read` so passive tray views do not suppress email batching.
	-- migration: notifications_seen_at
	seen_at TIMESTAMPTZ,
	sent_at TIMESTAMPTZ, -- When notification was sent via email (NULL if not sent)
	email_delivery_state TEXT NOT NULL DEFAULT 'pending',
	email_claim_owner TEXT,
	email_claim_token TEXT,
	email_claim_expires_at TIMESTAMPTZ,
	email_delivery_attempts INTEGER NOT NULL DEFAULT 0,
	email_last_error TEXT,
	-- Set to TRUE only when the scheduler tried to roll back sent_at after an SMTP
	-- failure and the rollback ITSELF failed. Without this flag the row is wedged:
	-- sent_at is set so the next tick's `WHERE sent_at IS NULL` skips it, and the
	-- user never gets the email. Surfacing makes the wedge visible to operators.
	last_send_failed BOOLEAN DEFAULT FALSE,
	avatar TEXT, -- Initials or avatar identifier
	action_url TEXT, -- URL to navigate to when clicked
	metadata TEXT, -- JSON for additional data
	authorization_scope TEXT NOT NULL DEFAULT 'legacy', -- legacy rows fail closed at delivery
	workspace_id INTEGER,
	item_id INTEGER,
	source_type TEXT,
	source_id INTEGER,
	referenced_entity_type TEXT,
	referenced_entity_id INTEGER,
	referenced_workspace_id INTEGER,
	referenced_workspace_permission TEXT,
	delivery_key TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_timestamp ON notifications(timestamp);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read);
CREATE INDEX IF NOT EXISTS idx_notifications_sent_at ON notifications(sent_at);
CREATE INDEX IF NOT EXISTS idx_notifications_email_delivery ON notifications(email_delivery_state, email_claim_expires_at);
CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
CREATE INDEX IF NOT EXISTS idx_notifications_workspace_id ON notifications(workspace_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_notifications_delivery_key ON notifications(delivery_key) WHERE delivery_key IS NOT NULL;

-- Notification templates table is defined in base_tables_postgres.sql
-- Default templates are inserted via initializePostgresDefaultData() to avoid semicolon parsing issues

-- Notification settings system for configuration sets
CREATE TABLE IF NOT EXISTS notification_settings (
	id SERIAL PRIMARY KEY,
	builtin_key TEXT,
	name TEXT NOT NULL,
	description TEXT,
	is_active BOOLEAN DEFAULT true,
	created_by INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_settings_active ON notification_settings(is_active);
CREATE INDEX IF NOT EXISTS idx_notification_settings_created_by ON notification_settings(created_by);
CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_settings_builtin_key ON notification_settings(builtin_key) WHERE builtin_key IS NOT NULL;

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
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (configuration_set_id) REFERENCES configuration_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (notification_setting_id) REFERENCES notification_settings(id) ON DELETE CASCADE,
	UNIQUE(configuration_set_id, notification_setting_id)
);

CREATE INDEX IF NOT EXISTS idx_config_set_notification_settings_config_set ON configuration_set_notification_settings(configuration_set_id);
CREATE INDEX IF NOT EXISTS idx_config_set_notification_settings_notification ON configuration_set_notification_settings(notification_setting_id);
-- One notification setting per configuration set (matches schema/notifications.sql).
CREATE UNIQUE INDEX IF NOT EXISTS uq_config_set_notification_setting_one_per_set
    ON configuration_set_notification_settings(configuration_set_id);

-- Web Push subscriptions (matches schema/notifications.sql). Mirrored by the
-- 20260618_push_subscriptions catalog migration for existing installs.
CREATE TABLE IF NOT EXISTS push_subscriptions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    endpoint TEXT NOT NULL,
    auth_key TEXT NOT NULL,
    p256dh_key TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_push_subscriptions_endpoint ON push_subscriptions(endpoint);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user_id ON push_subscriptions(user_id);
