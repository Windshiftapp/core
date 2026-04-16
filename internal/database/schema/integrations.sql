-- Integration provider tables (Notion, Confluence, Google Docs, etc.)
-- Generalized system for linking external pages/documents to work items

-- Integration Providers (system-level configuration, admin sets up)
CREATE TABLE IF NOT EXISTS integration_providers (
	id TEXT PRIMARY KEY,
	slug TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	provider_type TEXT NOT NULL,                  -- 'notion', 'confluence', etc.
	enabled BOOLEAN DEFAULT 1,
	oauth_client_id TEXT,
	oauth_client_secret_encrypted TEXT,
	provider_config TEXT DEFAULT '{}',            -- JSON: provider-specific config (e.g., base_url for self-hosted)
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_integration_providers_slug ON integration_providers(slug);
CREATE INDEX IF NOT EXISTS idx_integration_providers_type ON integration_providers(provider_type);
CREATE INDEX IF NOT EXISTS idx_integration_providers_enabled ON integration_providers(enabled);

-- Integration OAuth State (temporary storage for OAuth flow, 5-min expiry)
CREATE TABLE IF NOT EXISTS integration_oauth_state (
	id TEXT PRIMARY KEY,
	provider_id TEXT NOT NULL,
	state TEXT UNIQUE NOT NULL,
	user_id TEXT NOT NULL,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_integration_oauth_state_state ON integration_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_integration_oauth_state_expires ON integration_oauth_state(expires_at);

-- Cleanup trigger for expired state tokens
CREATE TRIGGER IF NOT EXISTS cleanup_expired_integration_oauth_state
AFTER INSERT ON integration_oauth_state
BEGIN
	DELETE FROM integration_oauth_state
	WHERE expires_at < datetime('now')
	AND (ABS(RANDOM()) % 100) = 0;
END;

-- User Integration Tokens (per-user OAuth tokens for integration providers)
CREATE TABLE IF NOT EXISTS user_integration_tokens (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL,
	oauth_access_token_encrypted TEXT NOT NULL,
	provider_metadata TEXT DEFAULT '{}',          -- JSON: provider-specific data (e.g., workspace_id, workspace_name for Notion)
	connected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (integration_provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
	UNIQUE(user_id, integration_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_user_integration_tokens_user ON user_integration_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_integration_tokens_provider ON user_integration_tokens(integration_provider_id);

-- Item Integration Links (links between items and external pages/documents)
CREATE TABLE IF NOT EXISTS item_integration_links (
	id TEXT PRIMARY KEY,
	item_id TEXT NOT NULL,
	integration_provider_id TEXT NOT NULL,
	external_id TEXT NOT NULL,
	external_url TEXT NOT NULL,
	title TEXT NOT NULL,
	icon TEXT DEFAULT '',
	link_type TEXT NOT NULL,                      -- 'page', 'database', 'doc', etc.
	link_metadata TEXT DEFAULT '{}',              -- JSON: provider-specific extras
	linked_by TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (integration_provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
	UNIQUE(item_id, integration_provider_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_item_integration_links_item ON item_integration_links(item_id);
CREATE INDEX IF NOT EXISTS idx_item_integration_links_provider ON item_integration_links(integration_provider_id);
CREATE INDEX IF NOT EXISTS idx_item_integration_links_external ON item_integration_links(external_id);
