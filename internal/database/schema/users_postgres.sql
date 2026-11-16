-- Users table moved to base_tables_postgres.sql

CREATE TABLE IF NOT EXISTS user_credentials (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	credential_type TEXT NOT NULL, -- 'fido', 'totp', 'ssh'
	credential_name TEXT NOT NULL, -- User-friendly name for the credential
	credential_data TEXT NOT NULL, -- JSON data specific to credential type
	public_key_fingerprint TEXT, -- SHA256 fingerprint for SSH keys (indexed)
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	last_used_at TIMESTAMPTZ,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON user_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_user_credentials_type ON user_credentials(credential_type);
CREATE INDEX IF NOT EXISTS idx_user_credentials_fingerprint ON user_credentials(public_key_fingerprint);

CREATE TABLE IF NOT EXISTS user_sessions (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	session_token TEXT UNIQUE NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	ip_address TEXT,
	user_agent TEXT,
	is_active BOOLEAN DEFAULT true,
	enrollment_required BOOLEAN DEFAULT false,
	auth_pending_type TEXT,
	remember_me BOOLEAN NOT NULL DEFAULT false,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);

CREATE TABLE IF NOT EXISTS user_invitations (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	token TEXT UNIQUE NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_invitations_token ON user_invitations(token);
CREATE INDEX IF NOT EXISTS idx_user_invitations_user_id ON user_invitations(user_id);

-- DSAR completion evidence: one row per Article 17 erasure execution. The
-- user row itself is pseudonymized (never deleted), so records persist.
CREATE TABLE IF NOT EXISTS user_erasure_records (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	requested_by TEXT NOT NULL, -- DSAR intake reference: subject email/channel reference
	requested_at TIMESTAMPTZ NOT NULL, -- when the erasure request was received
	approved_by INTEGER NOT NULL, -- admin user who approved execution
	executed_at TIMESTAMPTZ NOT NULL, -- when erasure completed
	policy_version TEXT NOT NULL, -- erasure policy version applied
	notes TEXT,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_erasure_records_user_id ON user_erasure_records(user_id);
