-- SCM (Source Control Management) provider integration tables (PostgreSQL)
-- Supports GitHub, GitLab, and Gitea/Forgejo

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
	oauth_token_expires_at TIMESTAMPTZ,
	-- Provider settings
	scopes TEXT DEFAULT 'repo',                   -- Space-separated scopes
	workspace_restriction_mode TEXT DEFAULT 'unrestricted', -- 'unrestricted' or 'restricted'
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
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
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMPTZ NOT NULL,                -- 5-minute expiry
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
	-- Smart-commit processing (#comment / #<transition>) on PR merge. Default
	-- off — acting-user resolution trusts the raw git committer email, which
	-- is not authenticated. Workspace admins must opt in knowingly.
	smart_commits_enabled BOOLEAN DEFAULT FALSE,
	-- Workspace-specific settings
	default_branch_pattern TEXT,                  -- e.g., "main", "develop"
	item_key_pattern TEXT,                        -- Regex for detecting item keys (default uses workspace key)
	-- Workspace-level credentials (for OAuth/PAT - GitHub Apps use provider-level)
	oauth_access_token_encrypted TEXT,
	oauth_refresh_token_encrypted TEXT,
	oauth_token_expires_at TIMESTAMPTZ,
	personal_access_token_encrypted TEXT,
	created_by INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
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
	last_synced_at TIMESTAMPTZ,
	milestone_tag_pattern TEXT NOT NULL DEFAULT 'v*',           -- Glob of tags that trigger the milestone-from-tag action
	milestone_branch_pattern TEXT NOT NULL DEFAULT 'release/*', -- Glob of branches that trigger the planning-milestone action
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE CASCADE,
	UNIQUE(workspace_scm_connection_id, repository_external_id)
);

CREATE INDEX IF NOT EXISTS idx_workspace_repos_connection ON workspace_repositories(workspace_scm_connection_id);
CREATE INDEX IF NOT EXISTS idx_workspace_repos_name ON workspace_repositories(repository_name);
CREATE INDEX IF NOT EXISTS idx_workspace_repos_active ON workspace_repositories(is_active);

CREATE TABLE IF NOT EXISTS scm_webhooks (
	id SERIAL PRIMARY KEY,
	workspace_repository_id INTEGER NOT NULL UNIQUE REFERENCES workspace_repositories(id) ON DELETE CASCADE,
	webhook_key TEXT NOT NULL UNIQUE,
	webhook_external_id TEXT,
	webhook_secret_encrypted TEXT NOT NULL,
	events JSONB NOT NULL DEFAULT '["push","tag_push","merge_request","note","release"]'::jsonb,
	is_active BOOLEAN NOT NULL DEFAULT true,
	last_delivery_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS scm_webhook_deliveries (
	id SERIAL PRIMARY KEY,
	scm_webhook_id INTEGER NOT NULL REFERENCES scm_webhooks(id) ON DELETE CASCADE,
	delivery_id TEXT NOT NULL,
	event_type TEXT NOT NULL,
	payload_summary JSONB,
	status TEXT NOT NULL DEFAULT 'pending',
	error_message TEXT,
	processing_time_ms INTEGER,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(scm_webhook_id, delivery_id)
);

CREATE INDEX IF NOT EXISTS idx_scm_webhook_deliveries_status ON scm_webhook_deliveries(status, created_at);

-- Durable health snapshots for scheduled SCM operations. One row per
-- connection and operation keeps diagnostics useful without retaining an
-- unbounded failure history.
CREATE TABLE IF NOT EXISTS scm_connection_health (
	workspace_scm_connection_id INTEGER NOT NULL,
	operation TEXT NOT NULL,
	last_attempt_at TIMESTAMPTZ NOT NULL,
	last_success_at TIMESTAMPTZ,
	last_failure_at TIMESTAMPTZ,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	checked_resources INTEGER NOT NULL DEFAULT 0,
	failed_resources INTEGER NOT NULL DEFAULT 0,
	last_error TEXT,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (workspace_scm_connection_id, operation),
	FOREIGN KEY (workspace_scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scm_connection_health_failures
	ON scm_connection_health(consecutive_failures, last_failure_at);

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
	smart_commits_applied_at TIMESTAMPTZ,           -- When smart-commit actions for a merged PR body were last applied (prevents re-runs)
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE,
	UNIQUE(item_id, workspace_repository_id, link_type, external_id)
);

CREATE INDEX IF NOT EXISTS idx_item_scm_links_item ON item_scm_links(item_id);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_repo ON item_scm_links(workspace_repository_id);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_type ON item_scm_links(link_type);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_external ON item_scm_links(external_id);
CREATE INDEX IF NOT EXISTS idx_item_scm_links_state ON item_scm_links(state);

-- PR comment cursors: per-PR high-water mark of the last SCM comment id the
-- "@agent" continuation poller has processed, so each sync tick only looks at
-- newer comments and never re-fires an old one (idempotency layer of the
-- comment loop guard, WI-426).
CREATE TABLE IF NOT EXISTS pr_comment_cursors (
	workspace_repository_id INTEGER NOT NULL,
	pr_number INTEGER NOT NULL,
	last_comment_id BIGINT NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (workspace_repository_id, pr_number),
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_pr_review_events (
	id SERIAL PRIMARY KEY,
	workspace_repository_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	pr_number INTEGER NOT NULL,
	event_kind TEXT NOT NULL,
	external_id BIGINT NOT NULL,
	author_id TEXT,
	author_login TEXT,
	author_association TEXT,
	body TEXT NOT NULL,
	context_json JSONB,
	status TEXT NOT NULL DEFAULT 'pending',
	agent_run_id INTEGER,
	ack_comment_id BIGINT,
	terminal_comment_id BIGINT,
	terminal_body TEXT,
	attempts INTEGER NOT NULL DEFAULT 0,
	last_error TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(workspace_repository_id, pr_number, event_kind, external_id),
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_agent_pr_review_events_status ON agent_pr_review_events(status, updated_at);

CREATE TABLE IF NOT EXISTS agent_pr_ownerships (
	workspace_repository_id INTEGER NOT NULL,
	pr_number INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	agent_run_id INTEGER NOT NULL,
	binding_id INTEGER NOT NULL,
	triggered_by_user_id INTEGER,
	head_repo TEXT NOT NULL,
	head_branch TEXT NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (workspace_repository_id, pr_number),
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (triggered_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);

-- SCM Processed Commits (idempotency ledger for smart-commit actions)
CREATE TABLE IF NOT EXISTS scm_processed_commits (
	commit_sha              TEXT NOT NULL,
	workspace_repository_id INTEGER NOT NULL,
	processed_at            TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	actions_applied         INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (commit_sha, workspace_repository_id),
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
);

-- Milestone commit attachment has its own object-scoped ledger. It must not
-- consume smart-commit records, and overlapping milestones must each scan the
-- same commit independently.
CREATE TABLE IF NOT EXISTS scm_milestone_processed_commits (
	milestone_id            INTEGER NOT NULL,
	workspace_repository_id INTEGER NOT NULL,
	commit_sha              TEXT NOT NULL,
	processed_at            TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (milestone_id, workspace_repository_id, commit_sha),
	FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
);

-- SCM Processed Refs (idempotency ledger for tag / release-branch sync events)
-- migration: cutover_postgres_timestamp_timezone
CREATE TABLE IF NOT EXISTS scm_processed_refs (
	workspace_repository_id INTEGER NOT NULL,
	ref_type                TEXT NOT NULL,  -- 'tag' | 'branch'
	ref_name                TEXT NOT NULL,
	sha                     TEXT,
	processed_at            TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (workspace_repository_id, ref_type, ref_name),
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
);

-- SCM Provider Workspace Allowlist (restricts which workspaces can use a provider)
CREATE TABLE IF NOT EXISTS scm_provider_workspace_allowlist (
	id SERIAL PRIMARY KEY,
	provider_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
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
	oauth_token_expires_at TIMESTAMPTZ,
	scm_username TEXT,                            -- Username from SCM provider (e.g., GitHub username)
	scm_user_id TEXT,                             -- External user ID from SCM
	scm_avatar_url TEXT,                          -- Avatar URL from SCM
	connected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	last_used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (scm_provider_id) REFERENCES scm_providers(id) ON DELETE CASCADE,
	UNIQUE(user_id, scm_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_user_scm_tokens_user ON user_scm_oauth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_scm_tokens_provider ON user_scm_oauth_tokens(scm_provider_id);

-- Issue Sync Configuration (per-workspace-repository)
-- Configures how GitHub Issues sync into Windshift items
CREATE TABLE IF NOT EXISTS issue_sync_configs (
	id SERIAL PRIMARY KEY,
	workspace_repository_id INTEGER NOT NULL UNIQUE,
	sync_enabled BOOLEAN DEFAULT FALSE,
	-- Status mapping: GitHub state → Windshift status ID
	status_mapping TEXT DEFAULT '{}',               -- JSON: {"open": <status_id>, "closed": <status_id>}
	reverse_status_mapping TEXT DEFAULT '{}',        -- JSON: {<status_id>: "open"|"closed", ...}
	-- Label sync configuration
	label_sync_mode TEXT DEFAULT 'none',             -- 'mirror', 'mapped', 'none'
	label_mappings TEXT DEFAULT '[]',                -- JSON: [{"github_label": "bug", "windshift_label_id": 1}, ...]
	filter_labels TEXT DEFAULT '[]',                 -- JSON: ["bug", "enhancement"] - only sync issues with these labels
	-- User/milestone mapping
	assignee_mappings TEXT DEFAULT '{}',             -- JSON: {"github_username": windshift_user_id, ...}
	milestone_mappings TEXT DEFAULT '{}',            -- JSON: {"github_milestone_number": windshift_milestone_id, ...}
	-- Defaults for imported issues
	default_item_type_id INTEGER,
	default_priority_id INTEGER,
	-- Comment sync
	sync_comments BOOLEAN DEFAULT FALSE,
	-- Sync state
	last_full_sync_at TIMESTAMPTZ,
	last_sync_error TEXT,
	-- Audit
	created_by INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_issue_sync_configs_repo ON issue_sync_configs(workspace_repository_id);
CREATE INDEX IF NOT EXISTS idx_issue_sync_configs_enabled ON issue_sync_configs(sync_enabled);

-- Issue Sync Items (GitHub Issue ↔ Windshift Item mapping)
CREATE TABLE IF NOT EXISTS issue_sync_items (
	id SERIAL PRIMARY KEY,
	issue_sync_config_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	github_issue_number INTEGER NOT NULL,
	github_issue_id BIGINT NOT NULL,                 -- GitHub's internal issue ID (int64)
	github_issue_url TEXT NOT NULL,
	last_synced_at TIMESTAMPTZ,
	last_github_updated_at TIMESTAMPTZ,                -- GitHub's updated_at at last sync
	sync_lock BOOLEAN DEFAULT FALSE,                 -- Prevents re-entrant sync during pushback
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (issue_sync_config_id) REFERENCES issue_sync_configs(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	UNIQUE(issue_sync_config_id, github_issue_number),
	UNIQUE(issue_sync_config_id, item_id)
);

CREATE INDEX IF NOT EXISTS idx_issue_sync_items_config ON issue_sync_items(issue_sync_config_id);
CREATE INDEX IF NOT EXISTS idx_issue_sync_items_item ON issue_sync_items(item_id);

-- Issue Sync Comments (GitHub Comment ↔ Windshift Comment mapping)
CREATE TABLE IF NOT EXISTS issue_sync_comments (
	id SERIAL PRIMARY KEY,
	issue_sync_item_id INTEGER NOT NULL,
	comment_id INTEGER,                              -- Windshift comment ID (NULL if comment deleted)
	github_comment_id BIGINT NOT NULL,               -- GitHub comment ID
	github_updated_at TIMESTAMPTZ,                     -- GitHub comment updated_at for change detection
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (issue_sync_item_id) REFERENCES issue_sync_items(id) ON DELETE CASCADE,
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE SET NULL,
	UNIQUE(issue_sync_item_id, github_comment_id)
);

CREATE INDEX IF NOT EXISTS idx_issue_sync_comments_item ON issue_sync_comments(issue_sync_item_id);
CREATE INDEX IF NOT EXISTS idx_issue_sync_comments_comment ON issue_sync_comments(comment_id);

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
