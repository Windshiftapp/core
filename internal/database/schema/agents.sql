-- Coding-agent harness state.
--
-- Folded into the fresh-install schema concat so a brand-new DB has these
-- tables without needing the catalog migrations to run. The matching catalog
-- entries (20260529_agent_runs, _agent_security_allowlist,
-- _workspace_agent_bindings, _workspace_agent_bindings_scm_connection) keep
-- their CheckSQLite predicates so they stamp without re-running on a fresh
-- install and still upgrade older DBs that predate this file.
--
-- See migrations.go for the WI references behind each table:
--   - agent_runs / agent_run_events: run-lifecycle + SSE event stream (WI-134)
--   - global_agent_acting_user_allowlist: acting-identity gate (WI-87)
--   - workspace_agent_bindings: workspace-admin-managed binding (WI-88)
--   - workspace_agent_bindings.scm_connection_id: SCM connection FK (WI-90)

CREATE TABLE IF NOT EXISTS agent_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL,
    item_id INTEGER,
    binding_id INTEGER, -- soft ref to workspace_agent_bindings; agent_runs must outlive bindings for audit
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','running','succeeded','failed','canceled','killed')),
    queued_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    ended_at DATETIME,
    container_id TEXT,
    runner_id INTEGER, -- soft ref to runner_instances; NULL for the in-process local runner (WI-141)
    target_pool_id INTEGER, -- soft ref to action_capabilities (runner_pool); NULL = local in-process pool (WI-141)
    cancel_requested_at DATETIME, -- set when a running remote run should abort; the runner learns via heartbeat (WI-141)
    grants_json TEXT, -- RunGrants snapshot the access-layer brokers authorize against (WI-144)
    run_token_id INTEGER, -- api_tokens row that binds a presented credential to this run's grants (WI-144)
    triggered_by_user_id INTEGER, -- soft ref to users: who fired the trigger; credential principal for OAuth SCM connections (WI-275)
    acting_user_id INTEGER, -- Standard profile identity used for permission checks and authorship
    root_initiator_user_id INTEGER, -- human that initiated an agent-to-agent chain
    immediate_trigger_user_id INTEGER, -- human or agent that directly queued this run
    parent_run_id INTEGER, -- soft self-ref retained when parent runs are pruned independently
    chain_depth INTEGER NOT NULL DEFAULT 0,
    session_id TEXT,
    profile_version INTEGER,
    profile_snapshot_json TEXT,
    job_kind TEXT NOT NULL DEFAULT 'coding_agent', -- coding_agent | action_container | ci_task (WI-146)
    job_image TEXT, -- admin image for action_container/ci_task jobs; NULL for coding_agent (fixed runner image)
    is_ephemeral BOOLEAN NOT NULL DEFAULT FALSE, -- private verification: no push or post-run mutation
    trigger_json TEXT, -- run trigger context + free-form instruction (e.g. the @mentioning comment) as a JSON blob; keeps new instruction shapes migration-free
    error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_runs_workspace_queued ON agent_runs(workspace_id, queued_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status);
CREATE INDEX IF NOT EXISTS idx_agent_runs_item_id ON agent_runs(item_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_binding_created ON agent_runs(binding_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_runner ON agent_runs(runner_id);
-- Supports the remote DB-as-queue claim: next queued run for a pool, oldest first.
CREATE INDEX IF NOT EXISTS idx_agent_runs_pool_claim ON agent_runs(target_pool_id, status, queued_at);
CREATE INDEX IF NOT EXISTS idx_agent_runs_standard_serial
    ON agent_runs(binding_id, item_id, status, queued_at, id)
    WHERE job_kind = 'standard_agent';
CREATE INDEX IF NOT EXISTS idx_agent_runs_standard_actor
    ON agent_runs(acting_user_id, item_id, status)
    WHERE job_kind = 'standard_agent';
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_runs_active_session
    ON agent_runs(session_id)
    WHERE session_id IS NOT NULL AND status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS agent_run_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    ts DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    type TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_agent_run_events_run ON agent_run_events(run_id, id);

-- Durable General and Standard conversations. Message bodies live only in
-- agent_messages; audit rows and run events carry correlation IDs/summaries.
CREATE TABLE IF NOT EXISTS agent_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_type TEXT NOT NULL CHECK (session_type IN ('general','standard')),
    owner_user_id INTEGER NOT NULL,
    workspace_id INTEGER,
    agent_profile_id INTEGER, -- soft ref: archived profiles remain transcript-attributable
    title TEXT NOT NULL DEFAULT '',
    archived_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    CHECK (
        (session_type = 'general' AND workspace_id IS NULL AND agent_profile_id IS NULL)
        OR
        (session_type = 'standard' AND workspace_id IS NOT NULL AND agent_profile_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_sessions_general_owner
    ON agent_sessions(owner_user_id) WHERE session_type = 'general';
CREATE INDEX IF NOT EXISTS idx_agent_sessions_owner_updated
    ON agent_sessions(owner_user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_workspace
    ON agent_sessions(workspace_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_session_participants (
    session_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    participant_role TEXT NOT NULL CHECK (participant_role IN ('human','agent')),
    joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (session_id, user_id),
    FOREIGN KEY (session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_agent_session_participants_user
    ON agent_session_participants(user_id, session_id);

CREATE TABLE IF NOT EXISTS agent_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user','assistant')),
    author_user_id INTEGER,
    agent_run_id INTEGER, -- soft ref: transcript remains if run retention changes
    content TEXT NOT NULL,
    context_json TEXT,
    metadata_json TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_messages_session
    ON agent_messages(session_id, id);
CREATE INDEX IF NOT EXISTS idx_agent_messages_run
    ON agent_messages(agent_run_id);

CREATE TABLE IF NOT EXISTS global_agent_acting_user_allowlist (
    user_id INTEGER NOT NULL,
    workspace_id INTEGER,
    reason TEXT NOT NULL DEFAULT '',
    created_by_user_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_global_agent_acting_user_allowlist_unique
    ON global_agent_acting_user_allowlist(user_id, COALESCE(workspace_id, 0));
CREATE INDEX IF NOT EXISTS idx_global_agent_acting_user_allowlist_workspace
    ON global_agent_acting_user_allowlist(workspace_id);

INSERT OR IGNORE INTO system_settings(key, value, value_type, description, category)
VALUES (
    'agents.allow_centralized_service_users',
    'false',
    'boolean',
    'Allow workspace admins to bind coding-agent runs to centralized service users (impersonation gate, WI-87).',
    'security'
);

INSERT OR IGNORE INTO system_settings(key, value, value_type, description, category)
VALUES (
    'allow_user_managed_agents',
    'false',
    'boolean',
    'Allow non-admin users to create and manage their own agent users from their profile',
    'security'
);

INSERT OR IGNORE INTO system_settings(key, value, value_type, description, category)
VALUES (
    'max_agents_per_user',
    '5',
    'integer',
    'Maximum number of owned agents a single non-admin user may create',
    'security'
);

INSERT OR IGNORE INTO system_settings(key, value, value_type, description, category)
VALUES (
    'workspace_managed_agents',
    'true',
    'boolean',
    'Allow workspace admins to create agent identities owned by their workspace',
    'security'
);

CREATE TABLE IF NOT EXISTS workspace_agent_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL,
    acting_user_id INTEGER NOT NULL,
    acting_user_kind TEXT NOT NULL
        CHECK (acting_user_kind IN ('agent','centralized_service')),
    profile_type TEXT NOT NULL DEFAULT 'legacy'
        CHECK (profile_type IN ('standard','coding','legacy')),
    lifecycle TEXT NOT NULL DEFAULT 'ready'
        CHECK (lifecycle IN ('draft','ready','paused','archived')),
    profile_version INTEGER NOT NULL DEFAULT 1 CHECK (profile_version > 0),
    identity_class TEXT NOT NULL DEFAULT 'centralized_service'
        CHECK (identity_class IN ('user_owned','centralized_service','workspace_managed')),
    purpose TEXT NOT NULL DEFAULT '',
    capability_groups_json TEXT NOT NULL DEFAULT '[]',
    archived_at DATETIME,
    archived_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    last_known_name TEXT NOT NULL DEFAULT '',
    last_known_handle TEXT NOT NULL DEFAULT '',
    last_known_avatar TEXT NOT NULL DEFAULT '',
    repo_slug TEXT,
    repo_base_ref TEXT,
    llm_connection_id INTEGER,
    token_scopes_json TEXT NOT NULL DEFAULT '[]',
    token_ttl_minutes INTEGER NOT NULL DEFAULT 60,
    max_runs_per_day INTEGER NOT NULL DEFAULT 0,
    scm_connection_id INTEGER REFERENCES workspace_scm_connections(id) ON DELETE SET NULL,
    target_pool_id INTEGER, -- soft ref to action_capabilities (runner_pool); NULL = local in-process run (WI-195)
    instructions TEXT NOT NULL DEFAULT '', -- appended to the run's initial prompt as the agent's role/persona (WI-258)
    runner_image TEXT, -- custom coding-agent container image for remote (pool) runs; NULL = the runner's default windshift-agent image (WI-450)
    created_by_user_id INTEGER, -- soft audit ref; NULL once the creator is deleted
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (acting_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_agent_bindings_workspace_acting
    ON workspace_agent_bindings(workspace_id, acting_user_id);
CREATE INDEX IF NOT EXISTS idx_workspace_agent_bindings_workspace
    ON workspace_agent_bindings(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_agent_bindings_workspace_lifecycle
    ON workspace_agent_bindings(workspace_id, lifecycle);
CREATE INDEX IF NOT EXISTS idx_workspace_agent_bindings_scm_connection
    ON workspace_agent_bindings(scm_connection_id);

-- Workspace agent skills (WI-258): a per-workspace library of markdown
-- "knowledge packs" attachable to agent bindings. Delivery is progressive
-- disclosure through the ws CLI: the run's initial prompt lists attached
-- skills (name + description) and the agent fetches a body with
-- `ws skill get <id>` only when relevant. Modeled after the Anthropic
-- Agent Skills standard (SKILL.md), markdown-only in v1.
CREATE TABLE IF NOT EXISTS workspace_agent_skills (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_user_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_agent_skills_workspace_name
    ON workspace_agent_skills(workspace_id, name);

CREATE TABLE IF NOT EXISTS workspace_agent_binding_skills (
    binding_id INTEGER NOT NULL,
    skill_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (binding_id, skill_id),
    FOREIGN KEY (binding_id) REFERENCES workspace_agent_bindings(id) ON DELETE CASCADE,
    FOREIGN KEY (skill_id) REFERENCES workspace_agent_skills(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_workspace_agent_binding_skills_skill
    ON workspace_agent_binding_skills(skill_id);

-- Workspace agent skill pages store a reviewed snapshot. Editing a source page
-- never changes what an agent receives until an admin saves the skill again.
CREATE TABLE IF NOT EXISTS workspace_agent_skill_pages (
    skill_id INTEGER NOT NULL,
    page_id INTEGER NOT NULL,
    title_snapshot TEXT NOT NULL DEFAULT '',
    content_snapshot TEXT NOT NULL DEFAULT '',
    page_updated_at_snapshot DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    snapshot_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (skill_id, page_id),
    FOREIGN KEY (skill_id) REFERENCES workspace_agent_skills(id) ON DELETE CASCADE,
    FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_workspace_agent_skill_pages_page
    ON workspace_agent_skill_pages(page_id);

-- Workspace agent binding repos (WI-449): a binding may bind N repositories so
-- the agent gets all of them checked out (e.g. core + core-tests) and opens one
-- PR per changed repo. Exactly one row per binding is is_primary=TRUE (the repo
-- whose PR links to the work item, and the single-repo backward-compat repo).
-- Like the binding itself, no clone URL is stored: it is derived server-side
-- from the trusted scm_connection_id + repo_slug (anti-SSRF). Supersedes the
-- legacy scalar workspace_agent_bindings.repo_slug/repo_base_ref/scm_connection_id
-- columns, which are kept one release as a dormant rollback net.
CREATE TABLE IF NOT EXISTS workspace_agent_binding_repos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id INTEGER NOT NULL,
    scm_connection_id INTEGER,
    repo_slug TEXT NOT NULL,
    repo_base_ref TEXT NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (binding_id) REFERENCES workspace_agent_bindings(id) ON DELETE CASCADE,
    FOREIGN KEY (scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wab_repos_binding_slug
    ON workspace_agent_binding_repos(binding_id, repo_slug);
-- one primary per binding (partial unique index; SQLite 3.8+ and Postgres)
CREATE UNIQUE INDEX IF NOT EXISTS idx_wab_repos_one_primary
    ON workspace_agent_binding_repos(binding_id) WHERE is_primary;
CREATE INDEX IF NOT EXISTS idx_wab_repos_binding
    ON workspace_agent_binding_repos(binding_id);

-- Remote runner pools (Initiative WI-141). A pool is an action_capabilities
-- row of type 'runner_pool'; these tables hang off it by soft ref (no FK,
-- mirroring the agent-table convention) so pool deletion is handled in the
-- service layer and runs/instances can outlive a pool for audit.

-- runner_registration_tokens: single-use, pool-scoped, revocable tokens. A
-- runner presents one once to register and exchanges it for a per-instance
-- credential (runner_instances).
CREATE TABLE IF NOT EXISTS runner_registration_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pool_capability_id INTEGER NOT NULL, -- soft ref to action_capabilities (runner_pool)
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by_user_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME, -- NULL = no expiry
    revoked_at DATETIME  -- NULL = active
);
CREATE INDEX IF NOT EXISTS idx_runner_registration_tokens_pool
    ON runner_registration_tokens(pool_capability_id);

-- runner_instances: one registered runner. credential_hash is the
-- per-instance control-plane credential it received at registration;
-- last_heartbeat_at drives lease reaping.
CREATE TABLE IF NOT EXISTS runner_instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pool_capability_id INTEGER NOT NULL, -- soft ref to action_capabilities (runner_pool)
    name TEXT NOT NULL DEFAULT '',
    credential_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','revoked')),
    registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat_at DATETIME,
    revoked_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_runner_instances_pool ON runner_instances(pool_capability_id);
CREATE INDEX IF NOT EXISTS idx_runner_instances_status ON runner_instances(status);

-- agent_template_catalog: system-admin overrides for the Agent Studio
-- creation catalog (WI-922). Enabled rows win over the embedded defaults;
-- the embedded defaults remain the fallback seed.
CREATE TABLE IF NOT EXISTS agent_template_catalog (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT '',
    default_type TEXT NOT NULL DEFAULT 'standard'
        CHECK (default_type IN ('standard','coding')),
    instructions TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- migration: 20260620_workspace_agent_binding_repos
