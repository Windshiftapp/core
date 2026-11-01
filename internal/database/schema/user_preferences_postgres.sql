-- User preferences table for storing user-specific settings as JSON
CREATE TABLE IF NOT EXISTS user_preferences (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL UNIQUE,
	preferences JSONB NOT NULL DEFAULT '{}',
	-- Per-user due-date reminder opt-in. Queryable columns (not JSON) so the
	-- reminder sweep can filter opted-in recipients in one query.
	due_date_reminders_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	due_date_reminder_days INTEGER NOT NULL DEFAULT 3,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id);
