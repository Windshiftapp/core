-- Items table with complete schema (PostgreSQL)

CREATE TABLE IF NOT EXISTS items (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	workspace_item_number INTEGER NOT NULL DEFAULT 0,
	item_type_id INTEGER,
	title TEXT NOT NULL,
	description TEXT,
	is_task BOOLEAN DEFAULT false,
	milestone_id INTEGER,
	iteration_id INTEGER,
	time_project_id INTEGER,
	project_id INTEGER,
	inherit_project BOOLEAN DEFAULT false,
	assignee_id INTEGER,
	creator_id INTEGER,
	reporter_id INTEGER,
	creator_portal_customer_id INTEGER,
	custom_field_values TEXT,
	virtual_field_data TEXT,
	calendar_data TEXT,
	-- Hierarchy fields
	parent_id INTEGER,
	path TEXT DEFAULT '/',
	-- Personal task relationship (for linking personal workspace tasks to work items)
	related_work_item_id INTEGER,
	-- Manual sorting fields
	rank TEXT,
	frac_index TEXT COLLATE "C",
	-- Status and workflow fields
	status_id INTEGER,
	-- Portal/channel fields
	channel_id INTEGER,
	request_type_id INTEGER,
	-- Priority field (new system)
	priority_id INTEGER,
	-- Due date field
	due_date DATE,
	-- Timestamps
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE SET NULL,
	FOREIGN KEY (parent_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE SET NULL,
	FOREIGN KEY (iteration_id) REFERENCES iterations(id) ON DELETE SET NULL,
	FOREIGN KEY (time_project_id) REFERENCES time_projects(id) ON DELETE SET NULL,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE SET NULL,
	FOREIGN KEY (assignee_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (creator_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (reporter_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (creator_portal_customer_id) REFERENCES portal_customers(id) ON DELETE SET NULL,
	FOREIGN KEY (status_id) REFERENCES statuses(id) ON DELETE RESTRICT,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL,
	FOREIGN KEY (request_type_id) REFERENCES request_types(id) ON DELETE SET NULL,
	FOREIGN KEY (priority_id) REFERENCES priorities(id) ON DELETE SET NULL,
	FOREIGN KEY (related_work_item_id) REFERENCES items(id) ON DELETE SET NULL,
	UNIQUE(workspace_id, workspace_item_number)
);

-- Workspace and item type indexes
CREATE INDEX IF NOT EXISTS idx_items_workspace_id ON items(workspace_id);
CREATE INDEX IF NOT EXISTS idx_items_workspace_item_number ON items(workspace_id, workspace_item_number);
CREATE INDEX IF NOT EXISTS idx_items_item_type_id ON items(item_type_id);

-- Status and priority indexes
CREATE INDEX IF NOT EXISTS idx_items_status_id ON items(status_id);
CREATE INDEX IF NOT EXISTS idx_items_priority_id ON items(priority_id);
CREATE INDEX IF NOT EXISTS idx_items_is_task ON items(is_task);
CREATE INDEX IF NOT EXISTS idx_items_due_date ON items(due_date) WHERE due_date IS NOT NULL;

-- Assignment and milestone indexes
CREATE INDEX IF NOT EXISTS idx_items_milestone_id ON items(milestone_id);
CREATE INDEX IF NOT EXISTS idx_items_iteration_id ON items(iteration_id);
CREATE INDEX IF NOT EXISTS idx_items_assignee_id ON items(assignee_id);
CREATE INDEX IF NOT EXISTS idx_items_creator_id ON items(creator_id);
CREATE INDEX IF NOT EXISTS idx_items_reporter_id ON items(reporter_id);
CREATE INDEX IF NOT EXISTS idx_items_creator_portal_customer_id ON items(creator_portal_customer_id);

-- Time tracking indexes
CREATE INDEX IF NOT EXISTS idx_items_time_project_id ON items(time_project_id);
CREATE INDEX IF NOT EXISTS idx_items_project_id ON items(project_id);

-- Hierarchy indexes for efficient tree operations
CREATE INDEX IF NOT EXISTS idx_items_parent_id ON items(parent_id);
CREATE INDEX IF NOT EXISTS idx_items_path ON items(path);
CREATE INDEX IF NOT EXISTS idx_items_workspace_parent ON items(workspace_id, parent_id);

-- Rank indexes for lexorank ordering and drag-and-drop (with partial index for efficiency)
CREATE INDEX IF NOT EXISTS idx_items_rank ON items(rank) WHERE rank IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_items_workspace_rank ON items(workspace_id, rank) WHERE rank IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_items_workspace_parent_rank ON items(workspace_id, parent_id, rank) WHERE rank IS NOT NULL;

-- Fractional indexing indexes (with partial index for efficiency)
CREATE INDEX IF NOT EXISTS idx_items_frac_index ON items(frac_index) WHERE frac_index IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_items_workspace_frac_index ON items(workspace_id, frac_index) WHERE frac_index IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_items_workspace_parent_frac_index ON items(workspace_id, parent_id, frac_index) WHERE frac_index IS NOT NULL;

-- Portal/channel indexes
CREATE INDEX IF NOT EXISTS idx_items_channel_id ON items(channel_id);
CREATE INDEX IF NOT EXISTS idx_items_request_type_id ON items(request_type_id);

-- Personal task relationship index
CREATE INDEX IF NOT EXISTS idx_items_related_work_item_id ON items(related_work_item_id);

-- Item history table for tracking changes to items (PostgreSQL)
CREATE TABLE IF NOT EXISTS item_history (
	id SERIAL PRIMARY KEY,
	item_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	changed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	field_name TEXT NOT NULL,
	old_value TEXT,
	new_value TEXT,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);

-- Index for efficient history queries (most common: get all history for an item)
CREATE INDEX IF NOT EXISTS idx_item_history_item_id_changed_at ON item_history(item_id, changed_at DESC);

-- Index for querying history by user
CREATE INDEX IF NOT EXISTS idx_item_history_user_id ON item_history(user_id);
