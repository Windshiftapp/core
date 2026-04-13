-- Condition Sets: named bundles of conditions that restrict workflow transitions
CREATE TABLE IF NOT EXISTS condition_sets (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	workflow_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

-- Links a condition set to a specific transition with a logic mode
CREATE TABLE IF NOT EXISTS condition_set_transitions (
	id SERIAL PRIMARY KEY,
	condition_set_id INTEGER NOT NULL,
	transition_id INTEGER NOT NULL,
	logic_mode TEXT NOT NULL DEFAULT 'and', -- 'and' or 'or'
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (condition_set_id) REFERENCES condition_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (transition_id) REFERENCES workflow_transitions(id) ON DELETE CASCADE,
	UNIQUE(condition_set_id, transition_id)
);

-- Individual conditions within a condition set transition
CREATE TABLE IF NOT EXISTS conditions (
	id SERIAL PRIMARY KEY,
	condition_set_transition_id INTEGER NOT NULL,
	condition_type TEXT NOT NULL, -- 'user_in_role', 'user_in_group', 'field_value', 'script'
	config TEXT NOT NULL,         -- JSON configuration
	display_order INTEGER DEFAULT 0,
	mode TEXT NOT NULL DEFAULT 'condition', -- 'condition' (hides transition) or 'validator' (blocks with error)
	error_message TEXT,           -- optional message shown when condition fails (validator mode)
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (condition_set_transition_id) REFERENCES condition_set_transitions(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_condition_sets_workflow_id ON condition_sets(workflow_id);
CREATE INDEX IF NOT EXISTS idx_condition_set_transitions_set_id ON condition_set_transitions(condition_set_id);
CREATE INDEX IF NOT EXISTS idx_condition_set_transitions_transition_id ON condition_set_transitions(transition_id);
CREATE INDEX IF NOT EXISTS idx_conditions_set_transition_id ON conditions(condition_set_transition_id);
