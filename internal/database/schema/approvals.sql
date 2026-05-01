-- Approvals: asynchronous, multi-step gates that fire when an item enters a designated status.
-- Sibling of condition_sets — see condition_sets.sql for the synchronous-gate counterpart.

-- ============================================================================
-- Templates (config)
-- ============================================================================

CREATE TABLE IF NOT EXISTS approval_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    workflow_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

-- One row per (set, status) — the approval that fires on entry to that status.
-- The approve_transition_id and deny_transition_id are the two transitions out
-- of the status that the approval engine drives. Users cannot invoke them directly.
CREATE TABLE IF NOT EXISTS approval_set_statuses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    approval_set_id INTEGER NOT NULL,
    status_id INTEGER NOT NULL,
    approve_transition_id INTEGER NOT NULL,
    deny_transition_id INTEGER NOT NULL,
    step_mode TEXT NOT NULL DEFAULT 'sequential', -- 'sequential' | 'parallel'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_set_id) REFERENCES approval_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (status_id) REFERENCES statuses(id) ON DELETE CASCADE,
    FOREIGN KEY (approve_transition_id) REFERENCES workflow_transitions(id) ON DELETE CASCADE,
    FOREIGN KEY (deny_transition_id) REFERENCES workflow_transitions(id) ON DELETE CASCADE,
    UNIQUE(approval_set_id, status_id)
);

-- Individual steps within an approval-set-status.
-- approver_source mirrors ConditionUserInRoleConfig.UserSource semantics, extended
-- with 'regular_field' / 'custom_field' to disambiguate the field type.
CREATE TABLE IF NOT EXISTS approval_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    approval_set_status_id INTEGER NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,

    -- Quorum
    quorum_mode TEXT NOT NULL DEFAULT 'any', -- 'any' | 'all' | 'count' | 'percent'
    quorum_count INTEGER,                    -- meaningful when quorum_mode = 'count'
    quorum_percent INTEGER,                  -- 1..100, round up; meaningful when quorum_mode = 'percent'
    rejection_policy TEXT NOT NULL DEFAULT 'any_rejection_fails', -- 'any_rejection_fails' | 'requires_quorum_to_fail'

    -- Approver source (resolved at request time; on-leave handling is unconditional)
    approver_source TEXT NOT NULL,           -- 'creator'|'assignee'|'regular_field'|'custom_field'|'role'|'group'|'user'
    approver_field_identifier TEXT,          -- whitelisted regular-field column when source='regular_field'
    approver_field_id INTEGER,               -- custom field id when source='custom_field'
    approver_role_id INTEGER,
    approver_group_id INTEGER,
    approver_user_id INTEGER,
    allow_self_approval INTEGER NOT NULL DEFAULT 0,

    -- On-leave override (engine ALWAYS reads UserLeavePeriod; this only changes WHAT to do).
    -- Default 'use_substitute' uses UserLeavePeriod.SubstituteUserID, falling back to escalation.
    on_leave_strategy TEXT NOT NULL DEFAULT 'use_substitute', -- 'use_substitute' | 'skip' | 'keep'

    -- Time-based escalation (NULL escalation_after_hours disables it for this step).
    escalation_after_hours INTEGER,
    escalation_action TEXT,                  -- 'reassign' | 'skip_step' | 'auto_reject'
    escalation_target_source TEXT,           -- same vocabulary as approver_source
    escalation_target_field_identifier TEXT,
    escalation_target_field_id INTEGER,
    escalation_target_role_id INTEGER,
    escalation_target_group_id INTEGER,
    escalation_target_user_id INTEGER,
    max_escalations INTEGER,                 -- NULL = unlimited chained escalations

    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_set_status_id) REFERENCES approval_set_statuses(id) ON DELETE CASCADE
);

-- ============================================================================
-- Instances (runtime)
-- ============================================================================

CREATE TABLE IF NOT EXISTS approval_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id INTEGER NOT NULL,
    approval_set_status_id INTEGER NOT NULL,
    status_id INTEGER NOT NULL,              -- snapshot of the status the item entered
    triggered_by_user_id INTEGER NOT NULL,   -- actor of the inbound transition
    status TEXT NOT NULL DEFAULT 'pending',  -- 'pending' | 'approved' | 'rejected' | 'cancelled'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
    FOREIGN KEY (approval_set_status_id) REFERENCES approval_set_statuses(id) ON DELETE RESTRICT,
    FOREIGN KEY (status_id) REFERENCES statuses(id) ON DELETE RESTRICT,
    FOREIGN KEY (triggered_by_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

-- At most one open request per item.
CREATE UNIQUE INDEX IF NOT EXISTS uq_approval_requests_one_open_per_item
    ON approval_requests(item_id) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS approval_step_instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    approval_request_id INTEGER NOT NULL,
    approval_step_id INTEGER NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',  -- 'pending' | 'approved' | 'rejected' | 'skipped' | 'escalated'
    escalation_due_at DATETIME,              -- re-armed after each chained escalation; NULL after completion / cap
    escalation_count INTEGER NOT NULL DEFAULT 0,
    last_escalated_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    FOREIGN KEY (approval_request_id) REFERENCES approval_requests(id) ON DELETE CASCADE,
    FOREIGN KEY (approval_step_id) REFERENCES approval_steps(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_approval_step_instances_request_id
    ON approval_step_instances(approval_request_id, display_order);

-- Sweeper-friendly: only pending steps with a due time.
CREATE INDEX IF NOT EXISTS idx_approval_step_instances_due
    ON approval_step_instances(escalation_due_at)
    WHERE status = 'pending' AND escalation_due_at IS NOT NULL;

-- Snapshot of resolved approver pool for a step instance. Each row points at
-- EITHER an internal user OR a portal customer — never both, never neither.
-- Approval routing for a customer-created item flows through portal_customer_id;
-- internal users (the historical case) flow through user_id.
CREATE TABLE IF NOT EXISTS approval_step_approvers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    approval_step_instance_id INTEGER NOT NULL,
    user_id INTEGER,                         -- internal-user approver
    portal_customer_id INTEGER,              -- portal-customer approver (mutually exclusive with user_id)
    source_role_id INTEGER,                  -- provenance: resolved via this role
    source_group_id INTEGER,                 -- provenance: resolved via this group
    substituted_for_user_id INTEGER,         -- set when this approver replaces an on-leave / delegated user
    is_active INTEGER NOT NULL DEFAULT 1,    -- flipped to 0 on reassign; preserves history
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_step_instance_id) REFERENCES approval_step_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
    FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE RESTRICT,
    FOREIGN KEY (substituted_for_user_id) REFERENCES users(id) ON DELETE SET NULL,
    -- Polymorphic invariant: exactly one identity is set. SQLite supports CHECK
    -- on CREATE TABLE; the migration block enforces this at the application
    -- layer for existing dbs since SQLite can't add CHECK to an existing table.
    CHECK ((user_id IS NOT NULL AND portal_customer_id IS NULL) OR (user_id IS NULL AND portal_customer_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_approval_step_approvers_step_active
    ON approval_step_approvers(approval_step_instance_id, is_active);
CREATE INDEX IF NOT EXISTS idx_approval_step_approvers_user_active
    ON approval_step_approvers(user_id, is_active) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_approval_step_approvers_customer_active
    ON approval_step_approvers(portal_customer_id, is_active) WHERE portal_customer_id IS NOT NULL;

-- Append-only audit log. Every state-changing event lands here.
CREATE TABLE IF NOT EXISTS approval_decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    approval_request_id INTEGER NOT NULL,
    approval_step_instance_id INTEGER,       -- NULL for request-level events (cancel, requested)
    actor_user_id INTEGER,                   -- internal-user actor; NULL for system or portal-customer actor
    actor_portal_customer_id INTEGER,        -- portal-customer actor; NULL for system or user actor
    decision TEXT NOT NULL,                  -- 'approve'|'reject'|'comment'|'delegate'|'reassign'|'cancel'|'escalate'|'substitute'|'requested'|'completed'
    comment TEXT,
    delegated_to_user_id INTEGER,
    metadata TEXT,                           -- JSON: prior_pool, new_pool, escalation_count, source_field_value, ...
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_request_id) REFERENCES approval_requests(id) ON DELETE CASCADE,
    FOREIGN KEY (approval_step_instance_id) REFERENCES approval_step_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
    FOREIGN KEY (actor_portal_customer_id) REFERENCES portal_customers(id) ON DELETE SET NULL,
    FOREIGN KEY (delegated_to_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_approval_decisions_request_created
    ON approval_decisions(approval_request_id, created_at);
CREATE INDEX IF NOT EXISTS idx_approval_decisions_actor
    ON approval_decisions(actor_user_id);

-- One vote (approve/reject) per actor per step. Defense in depth vs double-voting.
CREATE UNIQUE INDEX IF NOT EXISTS uq_approval_decisions_one_vote_per_actor
    ON approval_decisions(approval_step_instance_id, actor_user_id)
    WHERE decision IN ('approve', 'reject');
