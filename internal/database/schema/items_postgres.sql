-- Items table with complete schema (PostgreSQL)

CREATE TABLE IF NOT EXISTS items (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	workspace_item_number INTEGER NOT NULL DEFAULT 0,
	item_type_id INTEGER,
	title TEXT NOT NULL,
	description TEXT,
	is_task BOOLEAN DEFAULT false,
	iteration_id INTEGER,
	time_project_id INTEGER,
	project_id INTEGER,
	inherit_project BOOLEAN DEFAULT false,
	assignee_id INTEGER,
	creator_id INTEGER,
	reporter_id INTEGER,
	creator_portal_customer_id INTEGER,
	custom_field_values JSONB,
	virtual_field_data TEXT,
	calendar_data TEXT,
	-- Hierarchy fields
	parent_id INTEGER,
	path TEXT DEFAULT '/',
	-- Merge lifecycle: see items.sql for the column contract.
	merged_into_item_id BIGINT REFERENCES items(id) ON DELETE SET NULL,
	-- Personal task relationship (for linking personal workspace tasks to work items)
	related_work_item_id INTEGER,
	-- Estimation
	story_points REAL,
	estimate_minutes INTEGER,
	-- Manual sorting fields
	rank TEXT,
	-- migration: 20260807_items_frac_index_not_null
	-- Application writes allocate a canonical key in the active rank bucket.
	frac_index TEXT COLLATE "C" NOT NULL,
	-- Status and workflow fields
	status_id INTEGER,
	-- Portal/channel fields
	channel_id INTEGER,
	request_type_id INTEGER,
	-- Priority field (new system)
	priority_id INTEGER,
	-- Date fields
	due_date DATE,
	start_date DATE,
	end_date DATE,
	-- Timestamps
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	-- Recency for the board "Bubble Mode" sort; bumped on activity (comments,
	-- edits, transitions) but not on manual frac_index reorder. Deliberately
	-- no DEFAULT: the upgrade migration cannot add one on SQLite, so every
	-- insert path writes the column explicitly and fresh/upgraded schemas
	-- stay identical.
	last_active_at TIMESTAMPTZ,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE SET NULL,
	FOREIGN KEY (parent_id) REFERENCES items(id) ON DELETE CASCADE,
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
CREATE INDEX IF NOT EXISTS idx_items_iteration_id ON items(iteration_id);
CREATE INDEX IF NOT EXISTS idx_items_assignee_id ON items(assignee_id);
CREATE INDEX IF NOT EXISTS idx_items_creator_id ON items(creator_id);
CREATE INDEX IF NOT EXISTS idx_items_reporter_id ON items(reporter_id);
CREATE INDEX IF NOT EXISTS idx_items_creator_portal_customer_id ON items(creator_portal_customer_id);

-- Board "Bubble Mode" recency sort
CREATE INDEX IF NOT EXISTS idx_items_workspace_last_active ON items(workspace_id, last_active_at);

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

-- Fractional indexing indexes. frac_index is canonical and non-null, so the
-- indexes cover every item rather than maintaining a partial NULL subset.
-- The primary index is UNIQUE: GenerateFracIndexForNewItem and UpdateFracIndex
-- must not produce duplicate keys, and enforcing it at the DB turns silent
-- corruption into an INSERT/UPDATE error that callers can react to.
CREATE UNIQUE INDEX IF NOT EXISTS idx_items_frac_index ON items(frac_index);
CREATE INDEX IF NOT EXISTS idx_items_workspace_frac_index ON items(workspace_id, frac_index);
CREATE INDEX IF NOT EXISTS idx_items_workspace_parent_frac_index ON items(workspace_id, parent_id, frac_index);

-- Durable singleton coordination state for the 0.8.5 global rank
-- normalization. The 'legacy' phase stays admitted here only so databases
-- converted by the retired 0.8.5 checkpoint converter still load; the
-- application refuses any database whose phase is not a post-checkpoint
-- state.
CREATE TABLE IF NOT EXISTS global_rank_state (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	active_bucket INTEGER NOT NULL CHECK (active_bucket IN (0, 1, 2)),
	target_bucket INTEGER CHECK (target_bucket IS NULL OR target_bucket IN (0, 1, 2)),
	phase TEXT NOT NULL CHECK (phase IN ('legacy', 'stable', 'migrating', 'paused', 'failed')),
	direction TEXT CHECK (direction IS NULL OR direction IN ('high_to_low', 'low_to_high')),
	frontier TEXT,
	lease_owner TEXT,
	lease_expires_at TIMESTAMPTZ,
	migrated_count BIGINT NOT NULL DEFAULT 0 CHECK (migrated_count >= 0),
	total_count BIGINT NOT NULL DEFAULT 0 CHECK (total_count >= 0),
	last_error TEXT,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CHECK (target_bucket IS NULL OR target_bucket <> active_bucket),
	CHECK ((phase IN ('legacy', 'stable') AND target_bucket IS NULL AND direction IS NULL) OR phase IN ('migrating', 'paused', 'failed'))
);
INSERT INTO global_rank_state (id, active_bucket, phase)
VALUES (1, 0, 'stable')
ON CONFLICT (id) DO NOTHING;

-- Durable release checkpoint. Post-checkpoint binaries must validate this
-- marker before performing application-level startup mutations.
CREATE TABLE IF NOT EXISTS schema_checkpoint (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	version TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO schema_checkpoint (id, version)
VALUES (1, '0.8.5')
ON CONFLICT (id) DO NOTHING;

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
	changed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	field_name TEXT NOT NULL,
	old_value TEXT,
	new_value TEXT,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT
);

-- Index for efficient history queries (most common: get all history for an item)
CREATE INDEX IF NOT EXISTS idx_item_history_item_id_changed_at ON item_history(item_id, changed_at DESC);

-- Latest transition into the current status on list/board queries. The
-- partial predicate keeps unrelated history rows out of this hot-path index.
CREATE INDEX IF NOT EXISTS idx_item_history_current_status_latest
	ON item_history(item_id, new_value, changed_at DESC)
	WHERE field_name = 'status_id';

-- Index for querying history by user
CREATE INDEX IF NOT EXISTS idx_item_history_user_id ON item_history(user_id);

-- Permanent reservations for display keys retired by cross-workspace moves.
-- moved_item_id is deliberately not a foreign key: deleting the moved item
-- must never make its former key reusable.
CREATE TABLE IF NOT EXISTS item_key_reservations (
	workspace_id INTEGER NOT NULL,
	workspace_item_number INTEGER NOT NULL,
	moved_item_id INTEGER,
	destination_workspace_id INTEGER,
	destination_workspace_item_number INTEGER,
	moved_by INTEGER,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (workspace_id, workspace_item_number),
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (destination_workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL,
	FOREIGN KEY (moved_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_item_key_reservations_moved_item ON item_key_reservations(moved_item_id);

-- Item change log for collection delta polling (PostgreSQL)
CREATE TABLE IF NOT EXISTS item_change_log (
	id BIGSERIAL PRIMARY KEY,
	item_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	change_type TEXT NOT NULL CHECK (change_type IN ('upsert', 'delete')),
	changed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_item_change_log_workspace_id ON item_change_log(workspace_id, id);
CREATE INDEX IF NOT EXISTS idx_item_change_log_item_id ON item_change_log(item_id, id);
CREATE OR REPLACE FUNCTION log_item_change() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		INSERT INTO item_change_log(item_id, workspace_id, change_type) VALUES (OLD.id, OLD.workspace_id, 'delete');
		RETURN OLD;
	END IF;
	IF TG_OP = 'UPDATE' AND OLD.workspace_id <> NEW.workspace_id THEN
		INSERT INTO item_change_log(item_id, workspace_id, change_type) VALUES (OLD.id, OLD.workspace_id, 'delete');
	END IF;
	INSERT INTO item_change_log(item_id, workspace_id, change_type) VALUES (NEW.id, NEW.workspace_id, 'upsert');
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_items_change_insert ON items;
DROP TRIGGER IF EXISTS trg_items_change_update ON items;
DROP TRIGGER IF EXISTS trg_items_change_delete ON items;
CREATE TRIGGER trg_items_change_insert AFTER INSERT ON items FOR EACH ROW EXECUTE FUNCTION log_item_change();
CREATE TRIGGER trg_items_change_update AFTER UPDATE ON items FOR EACH ROW EXECUTE FUNCTION log_item_change();
CREATE TRIGGER trg_items_change_delete BEFORE DELETE ON items FOR EACH ROW EXECUTE FUNCTION log_item_change();

-- Late-bound FKs from earlier schema files that reference items(id).
-- Declared here because items has to exist first.
DO $$ BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'webhook_deliveries_item_id_fkey'
	) THEN
		ALTER TABLE webhook_deliveries
			ADD CONSTRAINT webhook_deliveries_item_id_fkey
			FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL;
	END IF;
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'item_milestones_item_id_fkey'
	) THEN
		ALTER TABLE item_milestones
			ADD CONSTRAINT item_milestones_item_id_fkey
			FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE;
	END IF;
END $$;

-- Ticket CSV import idempotency: see items.sql for the table contract.
CREATE TABLE IF NOT EXISTS item_import_rows (
	workspace_id INTEGER NOT NULL,
	external_ref TEXT NOT NULL,
	item_id INTEGER NOT NULL,
	job_id TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (workspace_id, external_ref),
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);
