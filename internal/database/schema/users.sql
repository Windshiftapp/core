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
		is_agent BOOLEAN DEFAULT false, -- If true, user is a non-human agent (API-only; cannot log in)
		agent_owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE, -- NULL = service user (admin-provisioned); non-NULL = owned agent (inherits owner permissions)
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_scim_external_id ON users(scim_external_id) WHERE scim_external_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_users_scim_managed ON users(scim_managed);
	CREATE INDEX IF NOT EXISTS idx_users_is_agent ON users(is_agent);
	CREATE INDEX IF NOT EXISTS idx_users_agent_owner ON users(agent_owner_user_id) WHERE agent_owner_user_id IS NOT NULL;

	-- is_agent is immutable once set at creation: allowing toggles would let
	-- an admin flip a human user into an agent and mint a token for them.
	CREATE TRIGGER IF NOT EXISTS users_is_agent_immutable
	BEFORE UPDATE OF is_agent ON users
	FOR EACH ROW
	WHEN IFNULL(NEW.is_agent, 0) IS NOT IFNULL(OLD.is_agent, 0)
	BEGIN
		SELECT RAISE(ABORT, 'is_agent is immutable');
	END;

	-- agent_owner_user_id is immutable for the same reason: flipping ownership
	-- would silently transfer an agent's inherited permissions to a new user.
	CREATE TRIGGER IF NOT EXISTS users_agent_owner_immutable
	BEFORE UPDATE OF agent_owner_user_id ON users
	FOR EACH ROW
	WHEN NEW.agent_owner_user_id IS NOT OLD.agent_owner_user_id
	BEGIN
		SELECT RAISE(ABORT, 'agent_owner_user_id is immutable');
	END;

	-- An owner link only makes sense on an agent. Reject on insert or update
	-- attempts that would create a non-agent user with an owner.
	CREATE TRIGGER IF NOT EXISTS users_agent_owner_requires_agent_insert
	BEFORE INSERT ON users
	FOR EACH ROW
	WHEN NEW.agent_owner_user_id IS NOT NULL AND IFNULL(NEW.is_agent, 0) = 0
	BEGIN
		SELECT RAISE(ABORT, 'agent_owner_user_id requires is_agent');
	END;

	CREATE TABLE IF NOT EXISTS user_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		credential_type TEXT NOT NULL, -- 'fido', 'totp', 'ssh'
		credential_name TEXT NOT NULL, -- User-friendly name for the credential
		credential_data TEXT NOT NULL, -- JSON data specific to credential type
		public_key_fingerprint TEXT, -- SHA256 fingerprint for SSH keys (indexed)
		is_active BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON user_credentials(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_credentials_type ON user_credentials(credential_type);
	CREATE INDEX IF NOT EXISTS idx_user_credentials_fingerprint ON user_credentials(public_key_fingerprint);

	CREATE TABLE IF NOT EXISTS user_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		session_token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		ip_address TEXT,
		user_agent TEXT,
		is_active BOOLEAN DEFAULT 1,
		enrollment_required BOOLEAN DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS user_invitations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	token TEXT UNIQUE NOT NULL,
	expires_at DATETIME NOT NULL,
	used_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_invitations_token ON user_invitations(token);
CREATE INDEX IF NOT EXISTS idx_user_invitations_user_id ON user_invitations(user_id);

