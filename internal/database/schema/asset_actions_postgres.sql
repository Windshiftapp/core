-- Asset actions automation system tables (PostgreSQL)

-- Asset actions: asset-set-scoped automation definitions
CREATE TABLE IF NOT EXISTS asset_actions (
	id SERIAL PRIMARY KEY,
	set_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	is_enabled BOOLEAN DEFAULT true,
	trigger_type TEXT NOT NULL,    -- asset_created, asset_updated, asset_status_changed, manual
	trigger_config TEXT,           -- JSON with trigger-specific conditions
	created_by INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_actions_set_id ON asset_actions(set_id);
CREATE INDEX IF NOT EXISTS idx_asset_actions_is_enabled ON asset_actions(is_enabled);
CREATE INDEX IF NOT EXISTS idx_asset_actions_trigger_type ON asset_actions(trigger_type);

-- Asset action nodes: steps in the flow
CREATE TABLE IF NOT EXISTS asset_action_nodes (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	node_type TEXT NOT NULL,       -- trigger, create_item, set_field, set_status, condition, notify_user
	node_config TEXT NOT NULL,     -- JSON configuration for the node
	position_x REAL DEFAULT 0,
	position_y REAL DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (action_id) REFERENCES asset_actions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_asset_action_nodes_action_id ON asset_action_nodes(action_id);

-- Asset action edges: connections between nodes
CREATE TABLE IF NOT EXISTS asset_action_edges (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	source_node_id INTEGER NOT NULL,
	target_node_id INTEGER NOT NULL,
	edge_type TEXT DEFAULT 'default',  -- default, true, false (for conditions)
	source_handle TEXT,
	target_handle TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (action_id) REFERENCES asset_actions(id) ON DELETE CASCADE,
	FOREIGN KEY (source_node_id) REFERENCES asset_action_nodes(id) ON DELETE CASCADE,
	FOREIGN KEY (target_node_id) REFERENCES asset_action_nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_asset_action_edges_action_id ON asset_action_edges(action_id);

-- Asset action execution logs: audit trail
CREATE TABLE IF NOT EXISTS asset_action_execution_logs (
	id SERIAL PRIMARY KEY,
	action_id INTEGER NOT NULL,
	asset_id INTEGER,
	trigger_event TEXT NOT NULL,
	status TEXT NOT NULL,          -- running, completed, failed, skipped
	started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP,
	error_message TEXT,
	execution_trace TEXT,          -- JSON step log
	FOREIGN KEY (action_id) REFERENCES asset_actions(id) ON DELETE CASCADE,
	FOREIGN KEY (asset_id) REFERENCES assets(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_action_exec_logs_action_id ON asset_action_execution_logs(action_id);
CREATE INDEX IF NOT EXISTS idx_asset_action_exec_logs_asset_id ON asset_action_execution_logs(asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_action_exec_logs_status ON asset_action_execution_logs(status);
CREATE INDEX IF NOT EXISTS idx_asset_action_exec_logs_started_at ON asset_action_execution_logs(started_at);
