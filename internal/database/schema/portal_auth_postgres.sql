-- Portal customer authentication tables (PostgreSQL)

-- Magic link tokens for portal customer authentication
CREATE TABLE IF NOT EXISTS portal_customer_magic_links (
	id SERIAL PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	token TEXT NOT NULL UNIQUE,
	channel_id INTEGER,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL
);

-- Portal customer sessions (separate from internal user sessions).
-- channel_id binds the session to the portal it was authenticated through;
-- middleware/resolvePortalBySlug rejects sessions presented on a different
-- portal slug so a cookie minted on portal A cannot authenticate on portal B.
-- Nullable for legacy rows minted before the column existed; those fail the
-- binding check on next request and force re-auth.
CREATE TABLE IF NOT EXISTS portal_customer_sessions (
	id SERIAL PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	session_token TEXT UNIQUE NOT NULL,
	channel_id INTEGER,
	expires_at TIMESTAMPTZ NOT NULL,
	ip_address TEXT,
	user_agent TEXT,
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_portal_magic_links_token ON portal_customer_magic_links(token);
CREATE INDEX IF NOT EXISTS idx_portal_magic_links_customer_id ON portal_customer_magic_links(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_magic_links_expires_at ON portal_customer_magic_links(expires_at);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_token ON portal_customer_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_customer_id ON portal_customer_sessions(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_expires_at ON portal_customer_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_channel_id ON portal_customer_sessions(channel_id);

-- Customer identity and resource admission. Loaded after portal customers and SSO providers.
CREATE TABLE IF NOT EXISTS customer_resource_access (
	resource_kind TEXT NOT NULL CHECK(resource_kind IN ('portal', 'board')),
	resource_id INTEGER NOT NULL,
	mode TEXT NOT NULL CHECK(mode IN ('public', 'organisations')),
	organisation_ids TEXT NOT NULL DEFAULT '[]',
	PRIMARY KEY(resource_kind, resource_id)
);
CREATE TABLE IF NOT EXISTS customer_external_accounts (
	provider_id INTEGER NOT NULL REFERENCES sso_providers(id) ON DELETE CASCADE,
	external_id TEXT NOT NULL,
	customer_id INTEGER NOT NULL REFERENCES portal_customers(id) ON DELETE CASCADE,
	PRIMARY KEY(provider_id, external_id)
);
CREATE TABLE IF NOT EXISTS customer_resource_sessions (
	token TEXT PRIMARY KEY,
	resource_kind TEXT NOT NULL,
	resource_id INTEGER NOT NULL,
	customer_id INTEGER NOT NULL REFERENCES portal_customers(id) ON DELETE CASCADE,
	expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_customer_resource_sessions_expires ON customer_resource_sessions(expires_at);
CREATE TABLE IF NOT EXISTS customer_resource_tokens (
	token TEXT PRIMARY KEY,
	purpose TEXT NOT NULL,
	payload TEXT NOT NULL DEFAULT '',
	resource_kind TEXT NOT NULL,
	resource_id INTEGER NOT NULL,
	customer_id INTEGER REFERENCES portal_customers(id) ON DELETE CASCADE,
	expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_customer_resource_tokens_expires ON customer_resource_tokens(expires_at);
