-- Portal WebAuthn tables (SQLite). Mirrors webauthn.sql but scoped to
-- portal_customers — keeps the customer-facing passkey stack fully separate
-- from the internal-user stack so the two cannot share or leak credentials.

CREATE TABLE IF NOT EXISTS portal_webauthn_credentials (
	id TEXT PRIMARY KEY, -- base64 credential ID from the authenticator
	portal_customer_id INTEGER NOT NULL,
	credential_name TEXT NOT NULL,
	public_key BLOB NOT NULL, -- COSE encoded public key
	attestation_type TEXT,
	aaguid BLOB,
	sign_count INTEGER DEFAULT 0,
	clone_warning BOOLEAN DEFAULT 0,
	transport TEXT, -- JSON array of transport types
	flags_user_present BOOLEAN DEFAULT 0,
	flags_user_verified BOOLEAN DEFAULT 0,
	flags_backup_eligible BOOLEAN DEFAULT 0,
	flags_backup_state BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_portal_webauthn_creds_customer_id ON portal_webauthn_credentials(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_webauthn_creds_aaguid      ON portal_webauthn_credentials(aaguid);
CREATE INDEX IF NOT EXISTS idx_portal_webauthn_creds_last_used   ON portal_webauthn_credentials(last_used_at);

-- portal_customer_id is NULL for discoverable (passwordless) auth sessions:
-- the subject is unknown until the authenticator presents a userHandle.
CREATE TABLE IF NOT EXISTS portal_webauthn_sessions (
	id TEXT PRIMARY KEY,
	portal_customer_id INTEGER,
	challenge TEXT NOT NULL,
	session_data TEXT NOT NULL,
	session_type TEXT NOT NULL, -- 'registration' | 'authentication'
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_portal_webauthn_sessions_customer_id ON portal_webauthn_sessions(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_webauthn_sessions_expires    ON portal_webauthn_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_portal_webauthn_sessions_type       ON portal_webauthn_sessions(session_type);

CREATE TRIGGER IF NOT EXISTS cleanup_expired_portal_webauthn_sessions
AFTER INSERT ON portal_webauthn_sessions
BEGIN
	DELETE FROM portal_webauthn_sessions
	WHERE expires_at < datetime('now')
	AND (ABS(RANDOM()) % 100) = 0;
END;

-- portal_customers.dismissed_passkey_prompt_at — see migrations slice in
-- database.go for the SQLite-side ALTER (SQLite has no IF NOT EXISTS for
-- ADD COLUMN, so re-running this file would fail on upgrades).
