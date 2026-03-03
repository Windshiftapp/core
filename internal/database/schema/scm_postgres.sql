-- SCM (Source Control Management) provider integration tables (PostgreSQL)
-- Supports GitHub and Gitea/Forgejo

-- SCM Providers (system-level configuration)
-- Similar pattern to sso_providers table
CREATE TABLE IF NOT EXISTS scm_providers (
	id SERIAL PRIMARY KEY,
	slug TEXT UNIQUE NOT NULL,                    -- URL-safe identifier (e.g., "github-main")
	name TEXT NOT NULL,                           -- Display name (e.g., "GitHub - Main Org")
	provider_type TEXT NOT NULL,                  -- 'github', 'gitlab', 'gitea', 'bitbucket'
	auth_method TEXT NOT NULL,                    -- 'oauth', 'pat', 'github_app'
	enabled BOOLEAN DEFAULT FALSE,
	is_default BOOLEAN DEFAULT FALSE,
	-- Connection settings
	base_url TEXT,                                -- API base URL (null = use provider default)
	-- OAuth credentials
	oauth_client_id TEXT,
	oauth_client_secret_encrypted TEXT,           -- Encrypted using AES-256-GCM
	-- Personal Access Token (for PAT auth method)
	personal_access_token_encrypted TEXT,
	-- GitHub App specific
	github_app_id TEXT,
	github_app_private_key_encrypted TEXT,
	github_app_installation_id TEXT,
	github_org_id BIGINT,                         -- Stable org ID for GitHub App discovery (survives org renames)
	-- OAuth token storage (after OAuth flow completion - DEPRECATED, use workspace_scm_connections)
	oauth_access_token_encrypted TEXT,
	oauth_refresh_token_encrypted TEXT,
	oauth_token_expires_at TIMESTAMP,
	-- Provider settings
	scopes TEXT DEFAULT 'repo',                   -- Space-separated scopes
	workspace_restriction_mode TEXT DEFAULT 'unrestricted', -- 'unrestricted' or 'restricted'
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scm_providers_slug ON scm_providers(slug);
CREATE INDEX IF NOT EXISTS idx_scm_providers_type ON scm_providers(provider_type);
CREATE INDEX IF NOT EXISTS idx_scm_providers_enabled ON scm_providers(enabled);
CREATE INDEX IF NOT EXISTS idx_scm_providers_default ON scm_providers(is_default);

-- SCM OAuth State Tokens (temporary storage for OAuth flow)
-- Similar to sso_state_tokens
CREATE TABLE IF NOT EXISTS scm_oauth_state (
	id SERIAL PRIMARY KEY,
	provider_id INTEGER NOT NULL,
	state TEXT UNIQUE NOT NULL,                   -- Cryptographically random state parameter
	redirect_uri TEXT NOT NULL,                   -- Callback URL
	user_id INTEGER NOT NULL,                     -- User initiating the connection
	workspace_id INTEGER,                         -- If set, store credentials at workspace level
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMP NOT NULL,                -- 5-minute expiry
	FOREIGN KEY (provider_id) REFERENCES scm_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scm_oauth_state_state ON scm_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_scm_oauth_state_expires ON scm_oauth_state(expires_at);
CREATE INDEX IF NOT EXISTS idx_scm_oauth_state_provider ON scm_oauth_state(provider_id);
CREATE INDEX IF NOT EXISTS idx_scm_oauth_state_workspace ON scm_oauth_state(workspace_id);

-- Function to automatically clean up expired state tokens
CREATE OR REPLACE FUNCTION cleanup_expired_scm_oauth_state()
RETURNS void AS $$
BEGIN
	DELETE FROM scm_oauth_state
	WHERE expires_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- Workspace SCM Connections (which providers a workspace can use)
CREATE TABLE IF NOT EXISTS workspace_scm_connections (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	scm_provider_id INTEGER NOT NULL,
	enabled BOOLEAN DEFAULT TRUE,
	-- Workspace-specific settings
	default_branch_pattern TEXT,                  -- e.g., "main", "develop"
	item_key_pattern TEXT,                        -- Regex for detecting item keys (default uses workspace key)
	-- Workspace-level credentials (for OAuth/PAT - GitHub Apps use provider-level)
	oauth_access_token_encrypted TEXT,
	oauth_refresh_token_encrypted TEXT,
	oauth_token_expires_at TIMESTAMP,
	personal_access_token_encrypted TEXT,
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (scm_provider_id) REFERENCES scm_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(workspace_id, scm_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_workspace_scm_workspace ON workspace_scm_connections(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_scm_provider ON workspace_scm_connections(scm_provider_id);

-- Workspace Repositories (repos linked to workspaces)
CREATE TABLE IF NOT EXISTS workspace_repositories (
	id SERIAL PRIMARY KEY,
	workspace_scm_connection_id INTEGER NOT NULL,
	repository_external_id TEXT NOT NULL,         -- External repo ID from SCM
	repository_name TEXT NOT NULL,                -- e.g., "org/repo-name"
	repository_url TEXT NOT NULL,                 -- Clone/web URL
	default_branch TEXT DEFAULT 'main',
	is_active BOOLEAN DEFAULT TRUE,
	last_synced_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE CASCADE,
	UNIQUE(workspace_scm_connection_id, repository_external_id)
);

CREATE INDEX IF NOT EXISTS idx_workspace_repos_connection ON workspace_repositories(workspace_scm_connection_id);
CREATE INDEX IF NOT EXISTS idx_workspace_repos_name ON workspace_repositories(repository_name);
CREATE INDEX IF NOT EXISTS idx_workspace_repos_active ON workspace_repositories(is_active);

-- Item SCM Links (PRs, commits, branches linked to items)
CREATE TABLE IF NOT EXISTS item_scm_links (
	id SERIAL PRIMARY KEY,
	item_id INTEGER NOT NULL,
	workspace_repository_id INTEGER NOT NULL,
	link_type TEXT NOT NULL,                      -- 'pull_request', 'commit', 'branch'
	external_id TEXT NOT NULL,                    -- PR number, commit SHA, branch name
	external_url TEXT,                            -- Direct link to PR/commit/branch
	title TEXT,                                   -- PR title, commit message first line
	state TEXT,                                   -- PR state: 'open', 'closed', 'merged'
	author_external_id TEXT,                      -- Author's external ID from SCM
	author_name TEXT,                             -- Author display name
	detection_source TEXT,                        -- 'webhook', 'manual', 'branch_name', 'pr_title', 'pr_body', 'commit_message'
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE,
	UNIQUE(item_id, workspace_repository_id, link_type, external_id)
);

CREATE INDEX IF NOT EXISTS idx_item_scm_links_item ON item_scm_links(item_id);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_repo ON item_scm_links(workspace_repository_id);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_type ON item_scm_links(link_type);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_external ON item_scm_links(external_id);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_state ON item_scm_links(state);

-- SCM Webhook Registrations (track registered webhooks)
CREATE TABLE IF NOT EXISTS scm_webhooks (
	id SERIAL PRIMARY KEY,
	workspace_repository_id INTEGER NOT NULL,
	webhook_external_id TEXT,                     -- External webhook ID from SCM
	webhook_secret_encrypted TEXT,                -- HMAC secret for verification
	events TEXT NOT NULL,                         -- JSON array: ["pull_request", "push"]
	is_active BOOLEAN DEFAULT TRUE,
	last_delivery_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scm_webhooks_repo ON scm_webhooks(workspace_repository_id);
CREATE INDEX IF NOT EXISTS idx_scm_webhooks_active ON scm_webhooks(is_active);

-- SCM Webhook Deliveries (audit trail for debugging)
CREATE TABLE IF NOT EXISTS scm_webhook_deliveries (
	id SERIAL PRIMARY KEY,
	scm_webhook_id INTEGER NOT NULL,
	delivery_id TEXT,                             -- External delivery ID
	event_type TEXT NOT NULL,                     -- 'pull_request', 'push', etc.
	payload_summary TEXT,                         -- JSON summary (not full payload)
	status TEXT NOT NULL,                         -- 'success', 'failed', 'ignored'
	error_message TEXT,
	processing_time_ms INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (scm_webhook_id) REFERENCES scm_webhooks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scm_webhook_deliveries_webhook ON scm_webhook_deliveries(scm_webhook_id);
CREATE INDEX IF NOT EXISTS idx_scm_webhook_deliveries_created ON scm_webhook_deliveries(created_at);
CREATE INDEX IF NOT EXISTS idx_scm_webhook_deliveries_status ON scm_webhook_deliveries(status);

-- Function to clean up old webhook deliveries (keep last 30 days)
CREATE OR REPLACE FUNCTION cleanup_old_scm_webhook_deliveries()
RETURNS void AS $$
BEGIN
	DELETE FROM scm_webhook_deliveries
	WHERE created_at < NOW() - INTERVAL '30 days';
END;
$$ LANGUAGE plpgsql;

-- SCM Provider Workspace Allowlist (restricts which workspaces can use a provider)
CREATE TABLE IF NOT EXISTS scm_provider_workspace_allowlist (
	id SERIAL PRIMARY KEY,
	provider_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_by INTEGER,
	FOREIGN KEY (provider_id) REFERENCES scm_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(provider_id, workspace_id)
);

CREATE INDEX IF NOT EXISTS idx_scm_provider_allowlist_provider ON scm_provider_workspace_allowlist(provider_id);
CREATE INDEX IF NOT EXISTS idx_scm_provider_allowlist_workspace ON scm_provider_workspace_allowlist(workspace_id);

-- User SCM OAuth Tokens (per-user token storage)
-- Each user must connect their own SCM account for OAuth-based providers
-- This ensures PRs/branches are created under the correct user's identity
CREATE TABLE IF NOT EXISTS user_scm_oauth_tokens (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	scm_provider_id INTEGER NOT NULL,
	oauth_access_token_encrypted TEXT NOT NULL,
	oauth_refresh_token_encrypted TEXT,
	oauth_token_expires_at TIMESTAMP,
	scm_username TEXT,                            -- Username from SCM provider (e.g., GitHub username)
	scm_user_id TEXT,                             -- External user ID from SCM
	scm_avatar_url TEXT,                          -- Avatar URL from SCM
	connected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	last_used_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (scm_provider_id) REFERENCES scm_providers(id) ON DELETE CASCADE,
	UNIQUE(user_id, scm_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_user_scm_tokens_user ON user_scm_oauth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_scm_tokens_provider ON user_scm_oauth_tokens(scm_provider_id);

-- Add deferred FK from milestone_releases to workspace_scm_connections
-- (broken out of milestones_postgres.sql to avoid circular dep: items→milestones→scm→items)
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM information_schema.table_constraints
		WHERE constraint_name = 'fk_milestone_releases_scm_connection'
		AND table_name = 'milestone_releases'
	) THEN
		ALTER TABLE milestone_releases
			ADD CONSTRAINT fk_milestone_releases_scm_connection
			FOREIGN KEY (scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE SET NULL;
	END IF;
END $$;
