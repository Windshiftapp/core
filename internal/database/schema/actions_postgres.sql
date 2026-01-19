-- Actions automation system tables (PostgreSQL)

-- Actions: workspace-scoped automation definitions
CREATE TABLE IF NOT EXISTS actions (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	is_enabled BOOLEAN DEFAULT true,
	trigger_type TEXT NOT NULL,    -- status_transition, item_created, item_updated, item_linked
	trigger_config TEXT,           -- JSON with trigger-specific conditions
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_actions_workspace_id ON actions(workspace_id);
CREATE INDEX IF NOT EXISTS idx_actions_is_enabled ON actions(is_enabled);
CREATE INDEX IF NOT EXISTS idx_actions_trigger_type ON actions(trigger_type);
CREATE INDEX IF NOT EXISTS idx_actions_created_by ON actions(created_by);

-- Action nodes: steps in the flow (set_field, add_comment, condition, etc.)
CREATE TABLE IF NOT EXISTS action_nodes (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	node_type TEXT NOT NULL,       -- trigger, set_field, set_status, add_comment, notify_user, condition
	node_config TEXT NOT NULL,     -- JSON configuration for the node
	position_x REAL DEFAULT 0,
	position_y REAL DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP,
	error_message TEXT,
	execution_trace TEXT,          -- JSON step log
	FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_action_execution_logs_action_id ON action_execution_logs(action_id);
CREATE INDEX IF NOT EXISTS idx_action_execution_logs_item_id ON action_execution_logs(item_id);
CREATE INDEX IF NOT EXISTS idx_action_execution_logs_status ON action_execution_logs(status);
CREATE INDEX IF NOT EXISTS idx_action_execution_logs_started_at ON action_execution_logs(started_at);
