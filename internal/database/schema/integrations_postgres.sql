-- Integration provider tables (Notion, Confluence, Google Docs, etc.)
-- Generalized system for linking external pages/documents to work items

-- Integration Providers (system-level configuration, admin sets up)
CREATE TABLE IF NOT EXISTS integration_providers (
	id TEXT PRIMARY KEY,
	slug TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	provider_type TEXT NOT NULL,
	enabled BOOLEAN DEFAULT true,
	oauth_client_id TEXT,
	oauth_client_secret_encrypted TEXT,
	provider_config TEXT DEFAULT '{}',
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integration_providers_slug ON integration_providers(slug);
CREATE INDEX IF NOT EXISTS idx_integration_providers_type ON integration_providers(provider_type);
CREATE INDEX IF NOT EXISTS idx_integration_providers_enabled ON integration_providers(enabled);

-- Integration OAuth State (temporary storage for OAuth flow, 5-min expiry)
CREATE TABLE IF NOT EXISTS integration_oauth_state (
	id TEXT PRIMARY KEY,
	provider_id TEXT NOT NULL REFERENCES integration_providers(id) ON DELETE CASCADE,
	state TEXT UNIQUE NOT NULL,
	user_id TEXT NOT NULL,
	expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integration_oauth_state_state ON integration_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_integration_oauth_state_expires ON integration_oauth_state(expires_at);

-- User Integration Tokens (per-user OAuth tokens for integration providers)
CREATE TABLE IF NOT EXISTS user_integration_tokens (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL REFERENCES integration_providers(id) ON DELETE CASCADE,
	oauth_access_token_encrypted TEXT NOT NULL,
	provider_metadata TEXT DEFAULT '{}',
	connected_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	UNIQUE(user_id, integration_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_user_integration_tokens_user ON user_integration_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_integration_tokens_provider ON user_integration_tokens(integration_provider_id);

-- Item Integration Links (links between items and external pages/documents)
CREATE TABLE IF NOT EXISTS item_integration_links (
	id TEXT PRIMARY KEY,
	item_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL REFERENCES integration_providers(id) ON DELETE CASCADE,
	external_id TEXT NOT NULL,
	external_url TEXT NOT NULL,
	title TEXT NOT NULL,
	icon TEXT DEFAULT '',
	link_type TEXT NOT NULL,
	link_metadata TEXT DEFAULT '{}',
	linked_by TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	UNIQUE(item_id, integration_provider_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_item_integration_links_item ON item_integration_links(item_id);
CREATE INDEX IF NOT EXISTS idx_item_integration_links_provider ON item_integration_links(integration_provider_id);
CREATE INDEX IF NOT EXISTS idx_item_integration_links_external ON item_integration_links(external_id);
