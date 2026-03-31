-- Teams and on-call management tables

-- Teams table
CREATE TABLE IF NOT EXISTS teams (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_active BOOLEAN DEFAULT true,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_teams_is_active ON teams(is_active);
CREATE INDEX IF NOT EXISTS idx_teams_created_by ON teams(created_by);

-- Team members: users belonging to a team with a role
CREATE TABLE IF NOT EXISTS team_members (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	role TEXT DEFAULT 'member',
	added_by INTEGER,
	added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_members_team_id ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_user_id ON team_members(user_id);
CREATE INDEX IF NOT EXISTS idx_team_members_added_by ON team_members(added_by);

-- Team groups: groups associated with a team
CREATE TABLE IF NOT EXISTS team_groups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	group_id INTEGER NOT NULL,
	added_by INTEGER,
	added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
	FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(team_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_team_groups_team_id ON team_groups(team_id);
CREATE INDEX IF NOT EXISTS idx_team_groups_group_id ON team_groups(group_id);
CREATE INDEX IF NOT EXISTS idx_team_groups_added_by ON team_groups(added_by);

-- User leave periods: tracks when users are on leave with optional substitute
CREATE TABLE IF NOT EXISTS user_leave_periods (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	substitute_user_id INTEGER,
	start_date DATE NOT NULL,
	end_date DATE NOT NULL,
	reason TEXT,
	is_active BOOLEAN DEFAULT true,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (substitute_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CHECK (end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS idx_user_leave_periods_user_id ON user_leave_periods(user_id);
CREATE INDEX IF NOT EXISTS idx_user_leave_periods_substitute_user_id ON user_leave_periods(substitute_user_id);
CREATE INDEX IF NOT EXISTS idx_user_leave_periods_dates ON user_leave_periods(start_date, end_date);
CREATE INDEX IF NOT EXISTS idx_user_leave_periods_is_active ON user_leave_periods(is_active);

-- Team round robin state: tracks assignment rotation for action nodes
CREATE TABLE IF NOT EXISTS team_round_robin_state (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	action_node_id INTEGER NOT NULL,
	team_id INTEGER NOT NULL,
	last_assigned_user_id INTEGER,
	last_assigned_at DATETIME,
	assignment_count INTEGER DEFAULT 0,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
	FOREIGN KEY (last_assigned_user_id) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(action_node_id, team_id)
);

CREATE INDEX IF NOT EXISTS idx_team_round_robin_state_team_id ON team_round_robin_state(team_id);
CREATE INDEX IF NOT EXISTS idx_team_round_robin_state_action_node_id ON team_round_robin_state(action_node_id);
CREATE INDEX IF NOT EXISTS idx_team_round_robin_state_last_assigned_user_id ON team_round_robin_state(last_assigned_user_id);

-- On-call schedules: defines on-call rotations for a team
CREATE TABLE IF NOT EXISTS on_call_schedules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	timezone TEXT DEFAULT 'UTC',
	is_active BOOLEAN DEFAULT true,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_on_call_schedules_team_id ON on_call_schedules(team_id);
CREATE INDEX IF NOT EXISTS idx_on_call_schedules_is_active ON on_call_schedules(is_active);
CREATE INDEX IF NOT EXISTS idx_on_call_schedules_created_by ON on_call_schedules(created_by);

-- On-call schedule layers: rotation layers within a schedule
CREATE TABLE IF NOT EXISTS on_call_schedule_layers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	schedule_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	priority INTEGER DEFAULT 1,
	rotation_type TEXT DEFAULT 'daily',
	rotation_interval_days INTEGER DEFAULT 1,
	handoff_time TEXT DEFAULT '09:00',
	start_date DATE NOT NULL,
	end_date DATE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (schedule_id) REFERENCES on_call_schedules(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_on_call_schedule_layers_schedule_id ON on_call_schedule_layers(schedule_id);
CREATE INDEX IF NOT EXISTS idx_on_call_schedule_layers_priority ON on_call_schedule_layers(priority);

-- On-call schedule layer members: users in a rotation layer with ordering
CREATE TABLE IF NOT EXISTS on_call_schedule_layer_members (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	layer_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	position INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (layer_id) REFERENCES on_call_schedule_layers(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(layer_id, user_id),
	UNIQUE(layer_id, position)
);

CREATE INDEX IF NOT EXISTS idx_on_call_schedule_layer_members_layer_id ON on_call_schedule_layer_members(layer_id);
CREATE INDEX IF NOT EXISTS idx_on_call_schedule_layer_members_user_id ON on_call_schedule_layer_members(user_id);

-- On-call schedule overrides: temporary overrides for on-call shifts
CREATE TABLE IF NOT EXISTS on_call_schedule_overrides (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	schedule_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	override_user_id INTEGER NOT NULL,
	start_time DATETIME NOT NULL,
	end_time DATETIME NOT NULL,
	reason TEXT,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (schedule_id) REFERENCES on_call_schedules(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (override_user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_on_call_schedule_overrides_schedule_id ON on_call_schedule_overrides(schedule_id);
CREATE INDEX IF NOT EXISTS idx_on_call_schedule_overrides_user_id ON on_call_schedule_overrides(user_id);
CREATE INDEX IF NOT EXISTS idx_on_call_schedule_overrides_override_user_id ON on_call_schedule_overrides(override_user_id);
CREATE INDEX IF NOT EXISTS idx_on_call_schedule_overrides_times ON on_call_schedule_overrides(start_time, end_time);
CREATE INDEX IF NOT EXISTS idx_on_call_schedule_overrides_created_by ON on_call_schedule_overrides(created_by);

-- On-call escalation policies: defines escalation chains for a team
CREATE TABLE IF NOT EXISTS on_call_escalation_policies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	repeat_count INTEGER DEFAULT 1,
	is_active BOOLEAN DEFAULT true,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_on_call_escalation_policies_team_id ON on_call_escalation_policies(team_id);
CREATE INDEX IF NOT EXISTS idx_on_call_escalation_policies_is_active ON on_call_escalation_policies(is_active);
CREATE INDEX IF NOT EXISTS idx_on_call_escalation_policies_created_by ON on_call_escalation_policies(created_by);

-- On-call escalation rules: individual steps in an escalation policy
CREATE TABLE IF NOT EXISTS on_call_escalation_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	policy_id INTEGER NOT NULL,
	step_order INTEGER NOT NULL,
	escalation_delay_minutes INTEGER DEFAULT 5,
	target_type TEXT NOT NULL,
	target_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (policy_id) REFERENCES on_call_escalation_policies(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_on_call_escalation_rules_policy_id ON on_call_escalation_rules(policy_id);
CREATE INDEX IF NOT EXISTS idx_on_call_escalation_rules_target ON on_call_escalation_rules(target_type, target_id);

-- On-call notification rules: how to notify for each escalation step
CREATE TABLE IF NOT EXISTS on_call_notification_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	escalation_rule_id INTEGER NOT NULL,
	notification_type TEXT NOT NULL,
	delay_minutes INTEGER DEFAULT 0,
	repeat_interval_minutes INTEGER,
	repeat_count INTEGER DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (escalation_rule_id) REFERENCES on_call_escalation_rules(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_on_call_notification_rules_escalation_rule_id ON on_call_notification_rules(escalation_rule_id);

-- On-call swap requests: requests to swap on-call shifts between users
CREATE TABLE IF NOT EXISTS on_call_swap_requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	schedule_id INTEGER NOT NULL,
	requester_user_id INTEGER NOT NULL,
	target_user_id INTEGER NOT NULL,
	swap_start DATETIME NOT NULL,
	swap_end DATETIME NOT NULL,
	status TEXT DEFAULT 'pending',
	responded_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (schedule_id) REFERENCES on_call_schedules(id) ON DELETE CASCADE,
	FOREIGN KEY (requester_user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (target_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_on_call_swap_requests_schedule_id ON on_call_swap_requests(schedule_id);
CREATE INDEX IF NOT EXISTS idx_on_call_swap_requests_requester_user_id ON on_call_swap_requests(requester_user_id);
CREATE INDEX IF NOT EXISTS idx_on_call_swap_requests_target_user_id ON on_call_swap_requests(target_user_id);
CREATE INDEX IF NOT EXISTS idx_on_call_swap_requests_status ON on_call_swap_requests(status);

-- On-call incidents: tracks triggered incidents and their escalation state
CREATE TABLE IF NOT EXISTS on_call_incidents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	escalation_policy_id INTEGER NOT NULL,
	item_id INTEGER,
	status TEXT DEFAULT 'triggered',
	triggered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	acknowledged_at DATETIME,
	acknowledged_by INTEGER,
	resolved_at DATETIME,
	resolved_by INTEGER,
	current_escalation_step INTEGER DEFAULT 0,
	escalation_repeat_count INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (escalation_policy_id) REFERENCES on_call_escalation_policies(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (resolved_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_on_call_incidents_escalation_policy_id ON on_call_incidents(escalation_policy_id);
CREATE INDEX IF NOT EXISTS idx_on_call_incidents_item_id ON on_call_incidents(item_id);
CREATE INDEX IF NOT EXISTS idx_on_call_incidents_status ON on_call_incidents(status);
CREATE INDEX IF NOT EXISTS idx_on_call_incidents_acknowledged_by ON on_call_incidents(acknowledged_by);
CREATE INDEX IF NOT EXISTS idx_on_call_incidents_resolved_by ON on_call_incidents(resolved_by);
CREATE INDEX IF NOT EXISTS idx_on_call_incidents_triggered_at ON on_call_incidents(triggered_at);
