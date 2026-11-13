-- Time worklogs table (separated from time_tracking due to dependency on items table)
CREATE TABLE IF NOT EXISTS time_worklogs (
	id SERIAL PRIMARY KEY,
	project_id INTEGER NOT NULL,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	item_id INTEGER REFERENCES items(id) ON DELETE SET NULL,
	description TEXT NOT NULL,
	date INTEGER NOT NULL,
	start_time INTEGER NOT NULL,
	end_time INTEGER NOT NULL,
	duration_minutes INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_time_worklogs_project_id ON time_worklogs(project_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_date ON time_worklogs(date);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_item_id ON time_worklogs(item_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_user_id ON time_worklogs(user_id);
-- Draft worklogs: persisted, user-owned incomplete time entries produced when a
-- timer stops without complete booking information (project/description). They
-- are intentionally separate from time_worklogs so reporting, briefings, item
-- totals, and budget rollups cannot see unbooked time by construction.
CREATE TABLE IF NOT EXISTS time_worklog_drafts (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	project_id INTEGER,
	item_id INTEGER,
	workspace_id INTEGER,
	description TEXT,
	date INTEGER,
	start_time INTEGER,
	end_time INTEGER,
	duration_minutes INTEGER,
	source TEXT NOT NULL DEFAULT 'manual',
	source_timer_id INTEGER,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	version INTEGER NOT NULL DEFAULT 1,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE SET NULL,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_time_worklog_drafts_user_id ON time_worklog_drafts(user_id);
-- At most one draft per source timer, so a lost stop response can be retried
-- and resolve to the original draft instead of creating a second one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_time_worklog_drafts_source_timer ON time_worklog_drafts(user_id, source_timer_id) WHERE source_timer_id IS NOT NULL;

-- Booking idempotency: records the worklog produced by a caller-scoped
-- idempotency key so a retried booking replays the same worklog instead of
-- duplicating booked time. Timer-stop bookings use the key "timer:<id>".
CREATE TABLE IF NOT EXISTS time_worklog_bookings (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	idempotency_key TEXT NOT NULL,
	worklog_id INTEGER NOT NULL,
	draft_id INTEGER,
	created_at INTEGER NOT NULL,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (worklog_id) REFERENCES time_worklogs(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_time_worklog_bookings_user_key ON time_worklog_bookings(user_id, idempotency_key);
