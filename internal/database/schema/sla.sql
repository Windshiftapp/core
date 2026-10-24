-- Service-level-agreement engine: calendars, metrics, goals, cycles, jobs.

-- A team may publish service hours for bound workspaces. The binding is the
-- consent boundary for referencing a team calendar; referencing it does not
-- move ownership of the SLA.
CREATE TABLE IF NOT EXISTS team_workspace_bindings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(team_id, workspace_id)
);

CREATE INDEX IF NOT EXISTS idx_team_workspace_bindings_workspace ON team_workspace_bindings(workspace_id);

-- Working calendars. Exactly one of workspace_id or team_id is set. A
-- workspace calendar is owned by its workspace; a team calendar is shared with
-- bound workspaces.
CREATE TABLE IF NOT EXISTS working_calendars (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
	team_id INTEGER REFERENCES teams(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	description TEXT,
	timezone TEXT NOT NULL,
	weekly_intervals TEXT NOT NULL,
	holidays TEXT NOT NULL DEFAULT '[]',
	is_default BOOLEAN NOT NULL DEFAULT FALSE,
	source TEXT,
	source_id TEXT,
	source_payload TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	CHECK ((workspace_id IS NOT NULL) != (team_id IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_working_calendars_workspace_name
	ON working_calendars(workspace_id, name) WHERE workspace_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_working_calendars_team_name
	ON working_calendars(team_id, name) WHERE team_id IS NOT NULL;

-- One row per workspace that has at least one active metric. config_generation
-- is bumped in the same transaction as any SLA configuration write, so inline
-- evaluation can use an in-transaction cache key.
CREATE TABLE IF NOT EXISTS sla_workspace_state (
	workspace_id INTEGER PRIMARY KEY,
	config_generation INTEGER NOT NULL DEFAULT 1,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sla_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	display_format TEXT NOT NULL DEFAULT 'time',
	position INTEGER NOT NULL DEFAULT 0,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	import_status TEXT NOT NULL DEFAULT 'native',
	source TEXT,
	source_id TEXT,
	source_payload TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	UNIQUE(workspace_id, name)
);

CREATE INDEX IF NOT EXISTS idx_sla_metrics_workspace ON sla_metrics(workspace_id, position);

CREATE TABLE IF NOT EXISTS sla_metric_conditions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	metric_id INTEGER NOT NULL,
	phase TEXT NOT NULL,
	position INTEGER NOT NULL DEFAULT 0,
	condition_type TEXT NOT NULL,
	config TEXT NOT NULL DEFAULT '{}',
	source_payload TEXT,
	FOREIGN KEY (metric_id) REFERENCES sla_metrics(id) ON DELETE CASCADE,
	UNIQUE(metric_id, phase, position)
);

CREATE INDEX IF NOT EXISTS idx_sla_metric_conditions_metric ON sla_metric_conditions(metric_id, phase, position);

CREATE TABLE IF NOT EXISTS sla_goals (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	metric_id INTEGER NOT NULL,
	position INTEGER NOT NULL,
	ql_query TEXT NOT NULL,
	original_jql TEXT,
	import_status TEXT NOT NULL DEFAULT 'native',
	source_id TEXT,
	source_payload TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (metric_id) REFERENCES sla_metrics(id) ON DELETE CASCADE,
	UNIQUE(metric_id, position)
);

CREATE INDEX IF NOT EXISTS idx_sla_goals_metric ON sla_goals(metric_id, position);

CREATE TABLE IF NOT EXISTS sla_goal_targets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	goal_id INTEGER NOT NULL,
	position INTEGER NOT NULL DEFAULT 0,
	target_ms INTEGER NOT NULL,
	calendar_id INTEGER NOT NULL,
	source_id TEXT,
	source_payload TEXT,
	FOREIGN KEY (goal_id) REFERENCES sla_goals(id) ON DELETE CASCADE,
	FOREIGN KEY (calendar_id) REFERENCES working_calendars(id) ON DELETE RESTRICT,
	UNIQUE(goal_id, position)
);

CREATE INDEX IF NOT EXISTS idx_sla_goal_targets_goal ON sla_goal_targets(goal_id, position);
CREATE INDEX IF NOT EXISTS idx_sla_goal_targets_calendar ON sla_goal_targets(calendar_id);

CREATE TABLE IF NOT EXISTS item_sla_cycles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id INTEGER NOT NULL,
	metric_id INTEGER NOT NULL,
	goal_id INTEGER,
	calendar_id INTEGER,
	cycle_no INTEGER NOT NULL,
	status TEXT NOT NULL,
	started_at DATETIME NOT NULL,
	stopped_at DATETIME,
	breach_time DATETIME,
	goal_duration_ms INTEGER NOT NULL,
	elapsed_ms INTEGER NOT NULL DEFAULT 0,
	remaining_ms INTEGER NOT NULL,
	paused BOOLEAN NOT NULL DEFAULT FALSE,
	within_calendar_hours BOOLEAN NOT NULL DEFAULT FALSE,
	breached BOOLEAN NOT NULL DEFAULT FALSE,
	pause_started_at DATETIME,
	next_deadline_at DATETIME,
	last_calculated_at DATETIME NOT NULL,
	remaining_at_pause_ms INTEGER,
	breached_at DATETIME,
	origin TEXT NOT NULL DEFAULT 'native',
	abandon_reason TEXT,
	calendar_snapshot TEXT NOT NULL,
	goal_query_snapshot TEXT NOT NULL,
	source_payload TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (metric_id) REFERENCES sla_metrics(id) ON DELETE CASCADE,
	FOREIGN KEY (goal_id) REFERENCES sla_goals(id) ON DELETE SET NULL,
	FOREIGN KEY (calendar_id) REFERENCES working_calendars(id) ON DELETE SET NULL,
	UNIQUE(item_id, metric_id, cycle_no)
);

-- At most one ongoing cycle per item and metric.
CREATE UNIQUE INDEX IF NOT EXISTS uq_item_sla_cycles_ongoing
	ON item_sla_cycles(item_id, metric_id) WHERE status = 'ongoing';
-- Inline evaluation locks the ongoing cycle for an item set.
CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_item_ongoing
	ON item_sla_cycles(item_id) WHERE status = 'ongoing';
-- Reports scan ongoing deadlines by metric.
CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_metric_deadline
	ON item_sla_cycles(metric_id, next_deadline_at) WHERE status = 'ongoing';
CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_item ON item_sla_cycles(item_id);
CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_metric ON item_sla_cycles(metric_id);
CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_goal ON item_sla_cycles(goal_id);
CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_calendar ON item_sla_cycles(calendar_id);

-- Warning thresholds that have already fired, keyed per cycle.
CREATE TABLE IF NOT EXISTS item_sla_cycle_thresholds (
	cycle_id INTEGER NOT NULL,
	threshold_key TEXT NOT NULL,
	fired_at DATETIME NOT NULL,
	PRIMARY KEY (cycle_id, threshold_key),
	FOREIGN KEY (cycle_id) REFERENCES item_sla_cycles(id) ON DELETE CASCADE
);

-- Pending SLA work. Every row is durable; finished jobs are deleted so
-- MIN(due_at) always points at real work.
CREATE TABLE IF NOT EXISTS sla_jobs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	threshold_key TEXT NOT NULL DEFAULT '',
	cycle_id INTEGER,
	item_id INTEGER,
	metric_id INTEGER,
	due_at DATETIME NOT NULL,
	deadline_at DATETIME,
	cursor TEXT,
	attempts INTEGER NOT NULL DEFAULT 0,
	lease_owner TEXT,
	last_error TEXT,
	state TEXT NOT NULL DEFAULT 'pending',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CHECK ((cycle_id IS NOT NULL) + (item_id IS NOT NULL) + (metric_id IS NOT NULL) = 1),
	FOREIGN KEY (cycle_id) REFERENCES item_sla_cycles(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (metric_id) REFERENCES sla_metrics(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sla_jobs_subject
	ON sla_jobs(kind, threshold_key, COALESCE(cycle_id, 0), COALESCE(item_id, 0), COALESCE(metric_id, 0));
CREATE INDEX IF NOT EXISTS idx_sla_jobs_due ON sla_jobs(due_at) WHERE state = 'pending';
CREATE INDEX IF NOT EXISTS idx_sla_jobs_cycle ON sla_jobs(cycle_id);
CREATE INDEX IF NOT EXISTS idx_sla_jobs_item ON sla_jobs(item_id);
CREATE INDEX IF NOT EXISTS idx_sla_jobs_metric ON sla_jobs(metric_id);
