-- Actions automation system tables (PostgreSQL)

-- Actions: workspace-scoped automation definitions
CREATE TABLE IF NOT EXISTS actions (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	is_enabled BOOLEAN DEFAULT true,
	trigger_type TEXT NOT NULL,    -- status_transition, item_created, item_updated, item_linked, manual, scm_tag_created, scm_release_branch_created, scm_pr_linked, scm_pr_merged
	trigger_config TEXT,           -- JSON with trigger-specific conditions
	created_by INTEGER,
	actor_user_id INTEGER,         -- NULL = run as triggering user; set = impersonate (requires action.set_actor)
	template_key TEXT,             -- Lineage stamp set when the action was created from a template (registry key)
	current_revision_id INTEGER,   -- Live immutable definition revision (see action_definition_revisions)
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_actions_workspace_id ON actions(workspace_id);
CREATE INDEX IF NOT EXISTS idx_actions_is_enabled ON actions(is_enabled);
CREATE INDEX IF NOT EXISTS idx_actions_trigger_type ON actions(trigger_type);
CREATE INDEX IF NOT EXISTS idx_actions_created_by ON actions(created_by);

-- Immutable, attributable snapshots of an action's executable definition.
-- Rows are retained after the action is deleted (no ON DELETE CASCADE) so an
-- execution log can always be traced back to the definition that ran. The
-- snapshot captures trigger/config, nodes/config (including future script
-- source), edges, actor override, enablement, and execution-role restrictions.
CREATE TABLE IF NOT EXISTS action_definition_revisions (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	revision_number INTEGER NOT NULL,
	source TEXT NOT NULL,              -- create, update, template_apply, baseline, import, restore
	editor_user_id INTEGER,            -- NULL for baseline/system revisions
	editor_username TEXT NOT NULL DEFAULT '',
	editor_source TEXT NOT NULL DEFAULT '',  -- USER, SCIM, SYSTEM
	change_summary TEXT NOT NULL DEFAULT '',
	changed_fields TEXT NOT NULL DEFAULT '[]',
	snapshot TEXT NOT NULL,            -- JSON ActionDefinitionSnapshot
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (action_id, revision_number),
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (editor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_action_definition_revisions_action_id
	ON action_definition_revisions(action_id, revision_number DESC);
CREATE INDEX IF NOT EXISTS idx_action_definition_revisions_workspace_id
	ON action_definition_revisions(workspace_id, created_at DESC);

-- Optional role allowlist for manual actions. No rows means the action is
-- available to every workspace editor; one or more rows restrict visibility
-- and execution to members of those roles (workspace admins retain override).
CREATE TABLE IF NOT EXISTS action_allowed_roles (
	action_id INTEGER NOT NULL,
	role_id INTEGER NOT NULL,
	PRIMARY KEY (action_id, role_id),
	FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE,
	FOREIGN KEY (role_id) REFERENCES workspace_roles(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_action_allowed_roles_role_id ON action_allowed_roles(role_id);

-- Action nodes: steps in the flow (set_field, add_comment, condition, etc.)
CREATE TABLE IF NOT EXISTS action_nodes (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	node_type TEXT NOT NULL,       -- trigger, set_field, set_status, add_comment, notify_user, condition
	node_config TEXT NOT NULL,     -- JSON configuration for the node
	position_x REAL DEFAULT 0,
	position_y REAL DEFAULT 0,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_action_nodes_action_id ON action_nodes(action_id);
CREATE INDEX IF NOT EXISTS idx_action_nodes_node_type ON action_nodes(node_type);

-- Action edges: connections between nodes
CREATE TABLE IF NOT EXISTS action_edges (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	source_node_id INTEGER NOT NULL,
	target_node_id INTEGER NOT NULL,
	edge_type TEXT DEFAULT 'default',  -- default, true, false (for conditions)
	source_handle TEXT,
	target_handle TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE,
	FOREIGN KEY (source_node_id) REFERENCES action_nodes(id) ON DELETE CASCADE,
	FOREIGN KEY (target_node_id) REFERENCES action_nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_action_edges_action_id ON action_edges(action_id);
CREATE INDEX IF NOT EXISTS idx_action_edges_source_node_id ON action_edges(source_node_id);
CREATE INDEX IF NOT EXISTS idx_action_edges_target_node_id ON action_edges(target_node_id);

-- Action execution logs: audit trail
CREATE TABLE IF NOT EXISTS action_execution_logs (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	item_id INTEGER,
	trigger_event TEXT NOT NULL,
	status TEXT NOT NULL,          -- running, completed, failed, skipped
	trigger_user_id INTEGER,            -- user whose event triggered the action
	effective_actor_user_id INTEGER,    -- user whose perms governed execution (= actor override or trigger user)
	started_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMPTZ,
	error_message TEXT,
	execution_trace TEXT,          -- JSON step log
	definition_revision_id INTEGER, -- Immutable definition revision that ran
	durable_event_key TEXT,
	FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (trigger_user_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (effective_actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_action_execution_logs_action_id ON action_execution_logs(action_id);
CREATE INDEX IF NOT EXISTS idx_action_execution_logs_item_id ON action_execution_logs(item_id);
CREATE INDEX IF NOT EXISTS idx_action_execution_logs_status ON action_execution_logs(status);
CREATE INDEX IF NOT EXISTS idx_action_execution_logs_started_at ON action_execution_logs(started_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_action_execution_logs_durable_target
	ON action_execution_logs(durable_event_key, action_id)
	WHERE durable_event_key IS NOT NULL;

-- Action capabilities: admin-provisioned resources that action nodes can reference
CREATE TABLE IF NOT EXISTS action_capabilities (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	capability_type TEXT NOT NULL,  -- 'docker_environment', 'http_client', 'llm_connection'
	config TEXT NOT NULL,           -- JSON, type-specific configuration
	is_enabled BOOLEAN DEFAULT true,
	-- applies_to_all_workspaces: when true, every workspace's action editor can
	-- reference this capability. When false, restricted to workspaces in the
	-- action_capability_workspaces join table. Default true preserves the
	-- pre-scoping behavior for legacy capabilities.
	applies_to_all_workspaces BOOLEAN DEFAULT true,
	created_by INTEGER,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_action_capabilities_type ON action_capabilities(capability_type);
CREATE INDEX IF NOT EXISTS idx_action_capabilities_enabled ON action_capabilities(is_enabled);

-- Per-workspace scope for capabilities that don't apply to all workspaces.
-- Only consulted when action_capabilities.applies_to_all_workspaces = false.
CREATE TABLE IF NOT EXISTS action_capability_workspaces (
	capability_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	PRIMARY KEY (capability_id, workspace_id),
	FOREIGN KEY (capability_id) REFERENCES action_capabilities(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_action_capability_workspaces_workspace ON action_capability_workspaces(workspace_id);

-- Action credentials: encrypted API tokens / API keys / basic-auth pairs that
-- HTTP capabilities reference instead of embedding plaintext in JSON config.
-- Scope mirrors action_capabilities: applies_to_all_workspaces=true makes the
-- credential usable everywhere; false restricts it to workspaces listed in the
-- action_credential_workspaces join table. The secret is never returned to
-- clients; the API exposes only metadata (name, type, prefix, has_secret).
CREATE TABLE IF NOT EXISTS action_credentials (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	credential_type TEXT NOT NULL,   -- bearer_token, api_key, basic_auth, custom_header
	applies_to_all_workspaces BOOLEAN NOT NULL DEFAULT true,
	created_by INTEGER,
	encrypted_secret TEXT NOT NULL,  -- AES-GCM ciphertext (label-bound HKDF key)
	secret_prefix TEXT,              -- non-sensitive fingerprint (first 4 chars + "…")
	secret_metadata TEXT,            -- JSON: provider, scopes, expires_at, etc. Must not contain secrets.
	is_enabled BOOLEAN DEFAULT true,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_action_credentials_enabled ON action_credentials(is_enabled);

-- Per-workspace scope for credentials that don't apply to all workspaces.
-- Only consulted when action_credentials.applies_to_all_workspaces = false.
CREATE TABLE IF NOT EXISTS action_credential_workspaces (
	credential_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	PRIMARY KEY (credential_id, workspace_id),
	FOREIGN KEY (credential_id) REFERENCES action_credentials(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_action_credential_workspaces_workspace ON action_credential_workspaces(workspace_id);
