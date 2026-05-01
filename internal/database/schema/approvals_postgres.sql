-- Approvals: PostgreSQL companion of approvals.sql.
-- See approvals.sql for SQLite version + design notes.

-- ============================================================================
-- Templates (config)
-- ============================================================================

CREATE TABLE IF NOT EXISTS approval_sets (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    workflow_id INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS approval_set_statuses (
    id SERIAL PRIMARY KEY,
    approval_set_id INTEGER NOT NULL,
    status_id INTEGER NOT NULL,
    approve_transition_id INTEGER NOT NULL,
    deny_transition_id INTEGER NOT NULL,
    step_mode TEXT NOT NULL DEFAULT 'sequential',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_set_id) REFERENCES approval_sets(id) ON DELETE CASCADE,
    FOREIGN KEY (status_id) REFERENCES statuses(id) ON DELETE CASCADE,
    FOREIGN KEY (approve_transition_id) REFERENCES workflow_transitions(id) ON DELETE CASCADE,
    FOREIGN KEY (deny_transition_id) REFERENCES workflow_transitions(id) ON DELETE CASCADE,
    UNIQUE(approval_set_id, status_id)
);

CREATE TABLE IF NOT EXISTS approval_steps (
    id SERIAL PRIMARY KEY,
    approval_set_status_id INTEGER NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,

    quorum_mode TEXT NOT NULL DEFAULT 'any',
    quorum_count INTEGER,
    quorum_percent INTEGER,
    rejection_policy TEXT NOT NULL DEFAULT 'any_rejection_fails',

    approver_source TEXT NOT NULL,
    approver_field_identifier TEXT,
    approver_field_id INTEGER,
    approver_role_id INTEGER,
    approver_group_id INTEGER,
    approver_user_id INTEGER,
    allow_self_approval BOOLEAN NOT NULL DEFAULT FALSE,

    on_leave_strategy TEXT NOT NULL DEFAULT 'use_substitute',

    escalation_after_hours INTEGER,
    escalation_action TEXT,
    escalation_target_source TEXT,
    escalation_target_field_identifier TEXT,
    escalation_target_field_id INTEGER,
    escalation_target_role_id INTEGER,
    escalation_target_group_id INTEGER,
    escalation_target_user_id INTEGER,
    max_escalations INTEGER,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_set_status_id) REFERENCES approval_set_statuses(id) ON DELETE CASCADE
);

-- ============================================================================
-- Instances (runtime)
-- ============================================================================

CREATE TABLE IF NOT EXISTS approval_requests (
    id SERIAL PRIMARY KEY,
    item_id INTEGER NOT NULL,
    approval_set_status_id INTEGER NOT NULL,
    status_id INTEGER NOT NULL,
    triggered_by_user_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
    FOREIGN KEY (approval_set_status_id) REFERENCES approval_set_statuses(id) ON DELETE RESTRICT,
    FOREIGN KEY (status_id) REFERENCES statuses(id) ON DELETE RESTRICT,
    FOREIGN KEY (triggered_by_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_approval_requests_one_open_per_item
    ON approval_requests(item_id) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS approval_step_instances (
    id SERIAL PRIMARY KEY,
    approval_request_id INTEGER NOT NULL,
    approval_step_id INTEGER NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    escalation_due_at TIMESTAMP,
    escalation_count INTEGER NOT NULL DEFAULT 0,
    last_escalated_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    FOREIGN KEY (approval_request_id) REFERENCES approval_requests(id) ON DELETE CASCADE,
    FOREIGN KEY (approval_step_id) REFERENCES approval_steps(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_approval_step_instances_request_id
    ON approval_step_instances(approval_request_id, display_order);

CREATE INDEX IF NOT EXISTS idx_approval_step_instances_due
    ON approval_step_instances(escalation_due_at)
    WHERE status = 'pending' AND escalation_due_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS approval_step_approvers (
    id SERIAL PRIMARY KEY,
    approval_step_instance_id INTEGER NOT NULL,
    user_id INTEGER,
    portal_customer_id INTEGER,
    source_role_id INTEGER,
    source_group_id INTEGER,
    substituted_for_user_id INTEGER,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (approval_step_instance_id) REFERENCES approval_step_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
    FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE RESTRICT,
    FOREIGN KEY (substituted_for_user_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT chk_approver_one_identity CHECK ((user_id IS NOT NULL AND portal_customer_id IS NULL) OR (user_id IS NULL AND portal_customer_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_approval_step_approvers_step_active
    ON approval_step_approvers(approval_step_instance_id, is_active);
CREATE INDEX IF NOT EXISTS idx_approval_step_approvers_user_active
    ON approval_step_approvers(user_id, is_active) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_approval_step_approvers_customer_active
    ON approval_step_approvers(portal_customer_id, is_active) WHERE portal_customer_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS approval_decisions (
    id SERIAL PRIMARY KEY,
    approval_request_id INTEGER NOT NULL,
    approval_step_instance_id INTEGER,
    actor_user_id INTEGER,
    actor_portal_customer_id INTEGER,
    decision TEXT NOT NULL,
    comment TEXT,
    delegated_to_user_id INTEGER,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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

CREATE UNIQUE INDEX IF NOT EXISTS uq_approval_decisions_one_vote_per_actor
    ON approval_decisions(approval_step_instance_id, actor_user_id)
    WHERE decision IN ('approve', 'reject');
