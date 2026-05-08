-- Portal WebAuthn tables (PostgreSQL). Mirrors webauthn_postgres.sql but
-- scoped to portal_customers — keeps the customer-facing passkey stack fully
-- separate from the internal-user stack so the two cannot share or leak
-- credentials.

CREATE TABLE IF NOT EXISTS portal_webauthn_credentials (
	id TEXT PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	credential_name TEXT NOT NULL,
	public_key BYTEA NOT NULL,
	attestation_type TEXT,
	aaguid BYTEA,
	sign_count INTEGER DEFAULT 0,
	clone_warning BOOLEAN DEFAULT FALSE,
	transport TEXT,
	flags_user_present BOOLEAN DEFAULT FALSE,
	flags_user_verified BOOLEAN DEFAULT FALSE,
	flags_backup_eligible BOOLEAN DEFAULT FALSE,
	flags_backup_state BOOLEAN DEFAULT FALSE,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_used_at TIMESTAMP,
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
	session_type TEXT NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_portal_webauthn_sessions_customer_id ON portal_webauthn_sessions(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_webauthn_sessions_expires    ON portal_webauthn_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_portal_webauthn_sessions_type       ON portal_webauthn_sessions(session_type);

-- Banner-dismissal state for the post-magic-link "set up a passkey" prompt.
ALTER TABLE portal_customers ADD COLUMN IF NOT EXISTS dismissed_passkey_prompt_at TIMESTAMP NULL;
