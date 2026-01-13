-- User preferences table for storing user-specific settings as JSON
CREATE TABLE IF NOT EXISTS user_preferences (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL UNIQUE,
	preferences JSONB NOT NULL DEFAULT '{}',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id);
