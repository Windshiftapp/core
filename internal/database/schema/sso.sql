-- SSO (Single Sign-On) tables for OIDC and future SAML support
-- Uses github.com/zitadel/oidc library for OIDC implementation

-- SSO Identity Providers configuration
-- Supports multiple providers (MVP limits to one OIDC)
CREATE TABLE IF NOT EXISTS sso_providers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT UNIQUE NOT NULL, -- URL-safe identifier for routing
	name TEXT NOT NULL, -- Display name (e.g., "Company SSO", "Keycloak")
	provider_type TEXT NOT NULL DEFAULT 'oidc', -- 'oidc' or 'saml' (future)
	enabled BOOLEAN DEFAULT 0,
	is_default BOOLEAN DEFAULT 0, -- Default provider for SSO login
	-- OIDC-specific fields
	issuer_url TEXT, -- OIDC issuer URL for discovery
	client_id TEXT,
	client_secret_encrypted TEXT, -- Encrypted client secret
	scopes TEXT DEFAULT 'openid email profile', -- Space-separated scopes
	-- Common settings
	auto_provision_users BOOLEAN DEFAULT 0, -- Create users on first SSO login
	allow_password_login BOOLEAN DEFAULT 1, -- Allow password login when SSO enabled
	require_verified_email BOOLEAN DEFAULT 1, -- Require email_verified=true from IdP (security)
	-- Claim/attribute mappings (JSON for flexibility)
	attribute_mapping TEXT DEFAULT '{"email":"email","name":"name","given_name":"given_name","family_name":"family_name","username":"preferred_username"}',
	-- SAML-specific fields
	saml_idp_metadata_url TEXT,  -- IdP metadata URL for auto-configuration
	saml_idp_sso_url TEXT,       -- IdP Single Sign-On URL
	saml_idp_certificate TEXT,   -- IdP X.509 certificate (PEM)
	saml_sp_entity_id TEXT,      -- SP Entity ID (defaults to base URL)
	saml_sign_requests BOOLEAN DEFAULT 0, -- Whether to sign AuthnRequests
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for SSO providers
CREATE INDEX IF NOT EXISTS idx_sso_providers_slug ON sso_providers(slug);
CREATE INDEX IF NOT EXISTS idx_sso_providers_enabled ON sso_providers(enabled);
CREATE INDEX IF NOT EXISTS idx_sso_providers_default ON sso_providers(is_default);

-- SSO State Tokens (temporary storage for login flow)
-- Used for CSRF protection and session correlation
CREATE TABLE IF NOT EXISTS sso_state_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	provider_id INTEGER NOT NULL,
	state TEXT UNIQUE NOT NULL, -- Cryptographically random state parameter
	nonce TEXT, -- OIDC nonce (NULL for SAML)
	redirect_uri TEXT NOT NULL, -- Callback URL
	remember_me BOOLEAN DEFAULT 0, -- Extended session flag
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL, -- 5-minute expiry
	FOREIGN KEY (provider_id) REFERENCES sso_providers(id) ON DELETE CASCADE
);

-- Indexes for SSO state tokens
CREATE INDEX IF NOT EXISTS idx_sso_state_tokens_state ON sso_state_tokens(state);
CREATE INDEX IF NOT EXISTS idx_sso_state_tokens_expires ON sso_state_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_sso_state_tokens_provider ON sso_state_tokens(provider_id);

-- Cleanup trigger for expired state tokens
CREATE TRIGGER IF NOT EXISTS cleanup_expired_sso_state_tokens
AFTER INSERT ON sso_state_tokens
BEGIN
	-- Only cleanup 1% of the time to avoid performance impact
	DELETE FROM sso_state_tokens
	WHERE expires_at < datetime('now')
	AND (ABS(RANDOM()) % 100) = 0;
END;

-- User External Accounts (link users to SSO identities)
-- Allows users to have multiple SSO identities linked
CREATE TABLE IF NOT EXISTS user_external_accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	provider_id INTEGER NOT NULL,
	external_id TEXT NOT NULL, -- 'sub' claim for OIDC, NameID for SAML
	email TEXT, -- Email from SSO provider (may differ from user email)
	profile_data TEXT, -- JSON blob of raw claims/attributes for debugging
	linked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_login_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (provider_id) REFERENCES sso_providers(id) ON DELETE CASCADE,
	UNIQUE(provider_id, external_id)
);

-- Indexes for user external accounts
CREATE INDEX IF NOT EXISTS idx_user_external_accounts_user_id ON user_external_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_user_external_accounts_provider ON user_external_accounts(provider_id, external_id);
CREATE INDEX IF NOT EXISTS idx_user_external_accounts_email ON user_external_accounts(email);
