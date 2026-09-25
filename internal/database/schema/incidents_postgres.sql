-- Incidents are pager state attached to a work item, not a separate work
-- entity. The incidents table owns the history; items.incident_id points at the
-- current/latest incident so rendering an item needs no subquery.

CREATE TABLE IF NOT EXISTS incidents (
	id SERIAL PRIMARY KEY,
	item_id INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'triggered',   -- triggered | acknowledged | resolved
	urgency TEXT NOT NULL DEFAULT 'high',        -- high | low
	source TEXT NOT NULL DEFAULT 'manual',       -- manual | automation | webhook | item
	escalation_policy_id INTEGER,
	triggered_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	acknowledged_at TIMESTAMPTZ,
	acknowledged_by INTEGER,
	resolved_at TIMESTAMPTZ,
	resolved_by INTEGER,
	escalation_step INTEGER NOT NULL DEFAULT 0,
	escalation_repeat_count INTEGER NOT NULL DEFAULT 0,
	next_escalation_at TIMESTAMPTZ,
	dedup_key TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (escalation_policy_id) REFERENCES on_call_escalation_policies(id) ON DELETE SET NULL,
	FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (resolved_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_incidents_item_id ON incidents(item_id);
CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);
CREATE INDEX IF NOT EXISTS idx_incidents_escalation_policy_id ON incidents(escalation_policy_id);
CREATE INDEX IF NOT EXISTS idx_incidents_next_escalation ON incidents(next_escalation_at)
	WHERE status = 'triggered';
-- One open incident per item; history rows accumulate once resolved.
CREATE UNIQUE INDEX IF NOT EXISTS uq_incidents_open_item ON incidents(item_id)
	WHERE status = 'triggered';

ALTER TABLE items ADD COLUMN IF NOT EXISTS incident_id INTEGER REFERENCES incidents(id) ON DELETE SET NULL;
-- One incident row can never be linked to two items.
CREATE UNIQUE INDEX IF NOT EXISTS uq_items_incident ON items(incident_id)
	WHERE incident_id IS NOT NULL;

-- Delayed/repeated notifications scheduled within an escalation step.
CREATE TABLE IF NOT EXISTS incident_notification_state (
	id SERIAL PRIMARY KEY,
	incident_id INTEGER NOT NULL,
	escalation_rule_id INTEGER NOT NULL,
	notification_rule_id INTEGER NOT NULL,
	repeat_index INTEGER NOT NULL DEFAULT 0,
	next_notification_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE CASCADE,
	FOREIGN KEY (notification_rule_id) REFERENCES on_call_notification_rules(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_incident_notification_state
	ON incident_notification_state(incident_id, notification_rule_id, repeat_index);
CREATE INDEX IF NOT EXISTS idx_incident_notification_state_due
	ON incident_notification_state(next_notification_at);
