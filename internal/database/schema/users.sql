	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		username TEXT UNIQUE NOT NULL,
		first_name TEXT NOT NULL,
		last_name TEXT NOT NULL,
		is_active BOOLEAN DEFAULT 1,
		avatar_url TEXT,
		password_hash TEXT, -- bcrypt hashed password
		requires_password_reset BOOLEAN DEFAULT 0,
		timezone TEXT,
		language TEXT DEFAULT 'en',
		email_verified BOOLEAN DEFAULT 1, -- Default true for backwards compatibility
		email_verification_token TEXT, -- Token for email verification flow
		email_verification_expires DATETIME, -- Expiry time for verification token
		scim_external_id TEXT, -- SCIM externalId from identity provider
		scim_managed BOOLEAN DEFAULT false, -- If true, user is managed via SCIM
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_scim_external_id ON users(scim_external_id) WHERE scim_external_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_users_scim_managed ON users(scim_managed);

	CREATE TABLE IF NOT EXISTS user_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		credential_type TEXT NOT NULL, -- 'fido', 'totp', 'ssh'
		credential_name TEXT NOT NULL, -- User-friendly name for the credential
		credential_data TEXT NOT NULL, -- JSON data specific to credential type
		is_active BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON user_credentials(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_credentials_type ON user_credentials(credential_type);

	CREATE TABLE IF NOT EXISTS user_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		session_token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		ip_address TEXT,
		user_agent TEXT,
		is_active BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions(session_token);
	CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);

	CREATE TABLE IF NOT EXISTS user_app_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_name TEXT NOT NULL, -- User-friendly name for the token
		token_hash TEXT NOT NULL, -- Hash of the actual token for security
		token_prefix TEXT NOT NULL, -- First few characters for display
		scopes TEXT, -- JSON array of granted scopes/permissions
		expires_at DATETIME, -- NULL for never expires
		is_active BOOLEAN DEFAULT 1,
		last_used_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_user_app_tokens_user_id ON user_app_tokens(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_app_tokens_hash ON user_app_tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_user_app_tokens_prefix ON user_app_tokens(token_prefix);
	CREATE INDEX IF NOT EXISTS idx_user_app_tokens_expires ON user_app_tokens(expires_at);

