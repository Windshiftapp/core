-- SSO (Single Sign-On) tables for OIDC and future SAML support (PostgreSQL)
-- Uses github.com/zitadel/oidc library for OIDC implementation

-- SSO Identity Providers configuration
-- Supports multiple providers (MVP limits to one OIDC)
CREATE TABLE IF NOT EXISTS sso_providers (
	id SERIAL PRIMARY KEY,
	slug TEXT UNIQUE NOT NULL, -- URL-safe identifier for routing
	name TEXT NOT NULL, -- Display name (e.g., "Company SSO", "Keycloak")
	provider_type TEXT NOT NULL DEFAULT 'oidc', -- 'oidc' or 'saml' (future)
	enabled BOOLEAN DEFAULT FALSE,
	is_default BOOLEAN DEFAULT FALSE, -- Default provider for SSO login
	-- OIDC-specific fields
	issuer_url TEXT, -- OIDC issuer URL for discovery
	client_id TEXT,
	client_secret_encrypted TEXT, -- Encrypted client secret
	scopes TEXT DEFAULT 'openid email profile', -- Space-separated scopes
	-- Common settings
	auto_provision_users BOOLEAN DEFAULT FALSE, -- Create users on first SSO login
	allow_password_login BOOLEAN DEFAULT TRUE, -- Allow password login when SSO enabled
	require_verified_email BOOLEAN DEFAULT TRUE, -- Require email_verified=true from IdP (security)
	-- Claim/attribute mappings (JSON for flexibility)
	attribute_mapping TEXT DEFAULT '{"email":"email","name":"name","given_name":"given_name","family_name":"family_name","username":"preferred_username"}',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for SSO providers
CREATE INDEX IF NOT EXISTS idx_sso_providers_slug ON sso_providers(slug);
CREATE INDEX IF NOT EXISTS idx_sso_providers_enabled ON sso_providers(enabled);
CREATE INDEX IF NOT EXISTS idx_sso_providers_default ON sso_providers(is_default);

-- SSO State Tokens (temporary storage for login flow)
-- Used for CSRF protection and session correlation
CREATE TABLE IF NOT EXISTS sso_state_tokens (
	id SERIAL PRIMARY KEY,
	provider_id INTEGER NOT NULL,
	state TEXT UNIQUE NOT NULL, -- Cryptographically random state parameter
	nonce TEXT, -- OIDC nonce (NULL for SAML)
	redirect_uri TEXT NOT NULL, -- Callback URL
	remember_me BOOLEAN DEFAULT FALSE, -- Extended session flag
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMP NOT NULL, -- 5-minute expiry
	FOREIGN KEY (provider_id) REFERENCES sso_providers(id) ON DELETE CASCADE
);

-- Indexes for SSO state tokens
CREATE INDEX IF NOT EXISTS idx_sso_state_tokens_state ON sso_state_tokens(state);
CREATE INDEX IF NOT EXISTS idx_sso_state_tokens_expires ON sso_state_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_sso_state_tokens_provider ON sso_state_tokens(provider_id);

-- Function to automatically clean up expired state tokens
CREATE OR REPLACE FUNCTION cleanup_expired_sso_state_tokens()
RETURNS void AS $$
BEGIN
	DELETE FROM sso_state_tokens
	WHERE expires_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- User External Accounts (link users to SSO identities)
-- Allows users to have multiple SSO identities linked
CREATE TABLE IF NOT EXISTS user_external_accounts (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	provider_id INTEGER NOT NULL,
	external_id TEXT NOT NULL, -- 'sub' claim for OIDC, NameID for SAML
	email TEXT, -- Email from SSO provider (may differ from user email)
	profile_data TEXT, -- JSON blob of raw claims/attributes for debugging
	linked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_login_at TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (provider_id) REFERENCES sso_providers(id) ON DELETE CASCADE,
	UNIQUE(provider_id, external_id)
);

-- Indexes for user external accounts
CREATE INDEX IF NOT EXISTS idx_user_external_accounts_user_id ON user_external_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_user_external_accounts_provider ON user_external_accounts(provider_id, external_id);
CREATE INDEX IF NOT EXISTS idx_user_external_accounts_email ON user_external_accounts(email);
