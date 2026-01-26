-- Users table moved to base_tables_postgres.sql

CREATE TABLE IF NOT EXISTS user_credentials (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	credential_type TEXT NOT NULL, -- 'fido', 'totp', 'ssh'
	credential_name TEXT NOT NULL, -- User-friendly name for the credential
	credential_data TEXT NOT NULL, -- JSON data specific to credential type
	public_key_fingerprint TEXT, -- SHA256 fingerprint for SSH keys (indexed)
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_used_at TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON user_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_user_credentials_type ON user_credentials(credential_type);
CREATE INDEX IF NOT EXISTS idx_user_credentials_fingerprint ON user_credentials(public_key_fingerprint);

CREATE TABLE IF NOT EXISTS user_sessions (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	session_token TEXT UNIQUE NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	ip_address TEXT,
	user_agent TEXT,
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);

CREATE TABLE IF NOT EXISTS user_app_tokens (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	token_name TEXT NOT NULL, -- User-friendly name for the token
	token_hash TEXT NOT NULL, -- Hash of the actual token for security
	token_prefix TEXT NOT NULL, -- First few characters for display
	scopes TEXT, -- JSON array of granted scopes/permissions
	expires_at TIMESTAMP, -- NULL for never expires
	is_active BOOLEAN DEFAULT true,
	last_used_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_app_tokens_user_id ON user_app_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_app_tokens_hash ON user_app_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_user_app_tokens_prefix ON user_app_tokens(token_prefix);
CREATE INDEX IF NOT EXISTS idx_user_app_tokens_expires ON user_app_tokens(expires_at);
