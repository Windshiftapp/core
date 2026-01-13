-- Recurring Tasks support (PostgreSQL)
-- Tables for recurring task patterns with iCalendar RRULE support.
--
-- Features:
-- - Define recurrence patterns on any task (template)
-- - Background scheduler generates instances ahead of time
-- - Full iCalendar RRULE support (daily, weekly, monthly, custom patterns)
-- - Configurable lead time and copy settings

-- Recurrence Rules (defines the recurrence pattern for a template item)
CREATE TABLE IF NOT EXISTS recurrence_rules (
	id SERIAL PRIMARY KEY,
	template_item_id INTEGER NOT NULL UNIQUE,
	workspace_id INTEGER NOT NULL,

	-- iCalendar RRULE string (RFC 5545 compliant)
	-- Examples: "FREQ=DAILY;INTERVAL=1", "FREQ=WEEKLY;BYDAY=MO,WE,FR", "FREQ=MONTHLY;BYMONTHDAY=15"
	rrule TEXT NOT NULL,

	-- Recurrence timing configuration
	dtstart TIMESTAMP NOT NULL,
	dtend TIMESTAMP,
	timezone TEXT DEFAULT 'UTC',

	-- Generation settings
	lead_time_days INTEGER DEFAULT 14,
	last_generated_until TIMESTAMP,
	next_generation_check TIMESTAMP,

	-- Instance configuration (what to copy from template)
	copy_assignee BOOLEAN DEFAULT TRUE,
	copy_priority BOOLEAN DEFAULT TRUE,
	copy_custom_fields BOOLEAN DEFAULT TRUE,
	copy_description BOOLEAN DEFAULT TRUE,
	status_on_create INTEGER,

	-- Lifecycle
	is_active BOOLEAN DEFAULT TRUE,
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

	FOREIGN KEY (template_item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (status_on_create) REFERENCES statuses(id) ON DELETE SET NULL
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_recurrence_rules_next_check ON recurrence_rules(next_generation_check);
CREATE INDEX IF NOT EXISTS idx_recurrence_rules_workspace ON recurrence_rules(workspace_id);
CREATE INDEX IF NOT EXISTS idx_recurrence_rules_template ON recurrence_rules(template_item_id);
CREATE INDEX IF NOT EXISTS idx_recurrence_rules_active ON recurrence_rules(is_active);

-- Partial index for active rules needing check (PostgreSQL supports this)
CREATE INDEX IF NOT EXISTS idx_recurrence_rules_active_check ON recurrence_rules(next_generation_check) WHERE is_active = TRUE;

-- Recurrence Instances (junction table linking templates to generated instances)
CREATE TABLE IF NOT EXISTS recurrence_instances (
	id SERIAL PRIMARY KEY,
	recurrence_rule_id INTEGER NOT NULL,
	instance_item_id INTEGER NOT NULL UNIQUE,

	-- Instance-specific metadata
	scheduled_date DATE NOT NULL,
	sequence_number INTEGER NOT NULL,

	-- Tracking
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

	FOREIGN KEY (recurrence_rule_id) REFERENCES recurrence_rules(id) ON DELETE CASCADE,
	FOREIGN KEY (instance_item_id) REFERENCES items(id) ON DELETE CASCADE,
	UNIQUE(recurrence_rule_id, scheduled_date)
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_recurrence_instances_rule ON recurrence_instances(recurrence_rule_id);
CREATE INDEX IF NOT EXISTS idx_recurrence_instances_item ON recurrence_instances(instance_item_id);
CREATE INDEX IF NOT EXISTS idx_recurrence_instances_date ON recurrence_instances(scheduled_date);
CREATE INDEX IF NOT EXISTS idx_recurrence_instances_rule_date ON recurrence_instances(recurrence_rule_id, scheduled_date);

