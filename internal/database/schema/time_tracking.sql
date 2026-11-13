-- Time tracking tables
-- Customer organisations (B2B entities for time tracking)
CREATE TABLE IF NOT EXISTS customer_organisations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	-- Stable identity for system-managed rows (e.g. the "no customer"
	-- placeholder). NULL for ordinary organisations.
	builtin_key TEXT,
	name TEXT NOT NULL,
	email TEXT,
	description TEXT,
	active BOOLEAN DEFAULT TRUE,
	avatar_url TEXT,
	custom_field_values TEXT,
	-- Flexible per-organisation attributes (portal request sharing, etc.).
	-- Empty object means "all defaults".
	settings TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_project_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	color TEXT DEFAULT '#3B82F6',
	display_order INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id INTEGER,
	category_id INTEGER,
	name TEXT NOT NULL,
	description TEXT,
	status TEXT DEFAULT 'Active',
	color TEXT,
	hourly_rate REAL DEFAULT 0,
	settings TEXT,
	active BOOLEAN DEFAULT TRUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (customer_id) REFERENCES customer_organisations(id) ON DELETE SET NULL,
	FOREIGN KEY (category_id) REFERENCES time_project_categories(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS workspace_time_project_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
	time_project_category_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (time_project_category_id) REFERENCES time_project_categories(id) ON DELETE CASCADE,
	UNIQUE(workspace_id, time_project_category_id)
);

CREATE TABLE IF NOT EXISTS time_worklogs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
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

CREATE INDEX IF NOT EXISTS idx_time_projects_customer_id ON time_projects(customer_id);
CREATE INDEX IF NOT EXISTS idx_time_projects_category_id ON time_projects(category_id);
CREATE INDEX IF NOT EXISTS idx_time_projects_status ON time_projects(status);
CREATE INDEX IF NOT EXISTS idx_workspace_time_project_categories_workspace_id ON workspace_time_project_categories(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_time_project_categories_category_id ON workspace_time_project_categories(time_project_category_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_project_id ON time_worklogs(project_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_date ON time_worklogs(date);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_item_id ON time_worklogs(item_id);
CREATE INDEX IF NOT EXISTS idx_time_worklogs_user_id ON time_worklogs(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_customer_organisations_builtin_key ON customer_organisations(builtin_key) WHERE builtin_key IS NOT NULL;

-- Project managers: can edit project, manage members, view all worklogs
CREATE TABLE IF NOT EXISTS time_project_managers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	manager_type TEXT NOT NULL CHECK (manager_type IN ('user', 'group')),
	manager_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(project_id, manager_type, manager_id)
);

-- Project members: can book time on project
CREATE TABLE IF NOT EXISTS time_project_members (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	member_type TEXT NOT NULL CHECK (member_type IN ('user', 'group')),
	member_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES time_projects(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(project_id, member_type, member_id)
);

CREATE INDEX IF NOT EXISTS idx_time_project_managers_project ON time_project_managers(project_id);
CREATE INDEX IF NOT EXISTS idx_time_project_members_project ON time_project_members(project_id);

-- Customer organisation managers: can manage org + view all related data
CREATE TABLE IF NOT EXISTS customer_organisation_managers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_organisation_id INTEGER NOT NULL,
	manager_type TEXT NOT NULL CHECK (manager_type IN ('user', 'group')),
	manager_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (customer_organisation_id) REFERENCES customer_organisations(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(customer_organisation_id, manager_type, manager_id)
);

-- Customer organisation members: whitelist for visibility when set
CREATE TABLE IF NOT EXISTS customer_organisation_members (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_organisation_id INTEGER NOT NULL,
	member_type TEXT NOT NULL CHECK (member_type IN ('user', 'group')),
	member_id INTEGER NOT NULL,
	granted_by INTEGER,
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (customer_organisation_id) REFERENCES customer_organisations(id) ON DELETE CASCADE,
	FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(customer_organisation_id, member_type, member_id)
);

CREATE INDEX IF NOT EXISTS idx_customer_organisation_managers_org ON customer_organisation_managers(customer_organisation_id);
CREATE INDEX IF NOT EXISTS idx_customer_organisation_members_org ON customer_organisation_members(customer_organisation_id);

-- migration: 0023_time_projects_active_sqlite

-- Draft worklogs: persisted, user-owned incomplete time entries produced when a
-- timer stops without complete booking information (project/description). They
-- are intentionally separate from time_worklogs so reporting, briefings, item
-- totals, and budget rollups cannot see unbooked time by construction.
CREATE TABLE IF NOT EXISTS time_worklog_drafts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
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
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	idempotency_key TEXT NOT NULL,
	worklog_id INTEGER NOT NULL,
	draft_id INTEGER,
	created_at INTEGER NOT NULL,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (worklog_id) REFERENCES time_worklogs(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_time_worklog_bookings_user_key ON time_worklog_bookings(user_id, idempotency_key);
