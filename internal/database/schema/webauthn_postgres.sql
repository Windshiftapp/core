-- WebAuthn tables for proper FIDO2/WebAuthn implementation (PostgreSQL)
-- Uses github.com/go-webauthn/webauthn library

-- Table for storing WebAuthn credentials
-- Separate from user_credentials to maintain proper structure
CREATE TABLE IF NOT EXISTS webauthn_credentials (
	id TEXT PRIMARY KEY, -- base64 credential ID from the authenticator
	user_id INTEGER NOT NULL,
	credential_name TEXT NOT NULL, -- User-friendly name for the credential
	public_key BYTEA NOT NULL, -- COSE encoded public key
	attestation_type TEXT, -- 'none', 'indirect', 'direct', etc.
	aaguid BYTEA, -- Authenticator Attestation GUID
	sign_count INTEGER DEFAULT 0, -- Counter for clone detection
	clone_warning BOOLEAN DEFAULT FALSE, -- Flag if clone detected
	transport TEXT, -- JSON array of transport types ['usb', 'nfc', 'ble', 'internal']
	flags_user_present BOOLEAN DEFAULT FALSE,
	flags_user_verified BOOLEAN DEFAULT FALSE,
	flags_backup_eligible BOOLEAN DEFAULT FALSE, -- Passkey sync capability
	flags_backup_state BOOLEAN DEFAULT FALSE, -- Currently backed up
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_used_at TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for WebAuthn credentials
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user_id ON webauthn_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_aaguid ON webauthn_credentials(aaguid);
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_last_used ON webauthn_credentials(last_used_at);

-- Table for storing WebAuthn session data (challenges)
-- Required for proper challenge validation
CREATE TABLE IF NOT EXISTS webauthn_sessions (
	id TEXT PRIMARY KEY, -- Random session ID
	user_id INTEGER, -- NULL for passwordless/discoverable credentials
	challenge TEXT NOT NULL, -- Base64 encoded challenge
	session_data TEXT NOT NULL, -- JSON serialized SessionData from go-webauthn
	session_type TEXT NOT NULL, -- 'registration' or 'authentication'
	expires_at TIMESTAMP NOT NULL, -- Challenge expiration
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for WebAuthn sessions
CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_user_id ON webauthn_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_expires ON webauthn_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_type ON webauthn_sessions(session_type);

-- Function to automatically clean up expired sessions
CREATE OR REPLACE FUNCTION cleanup_expired_webauthn_sessions()
RETURNS void AS $$
BEGIN
	DELETE FROM webauthn_sessions
	WHERE expires_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- Optional: Create a scheduled job to clean expired sessions (requires pg_cron extension)
-- CREATE EXTENSION IF NOT EXISTS pg_cron;
-- SELECT cron.schedule('cleanup-webauthn-sessions', '*/15 * * * *', 'SELECT cleanup_expired_webauthn_sessions();');