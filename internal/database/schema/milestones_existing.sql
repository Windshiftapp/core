-- Existing-install compatibility bootstrap for the milestone tables.
--
-- migration: 20260727_milestone_release_attempts
--
-- Fresh installs use milestones.sql. This file is replayed before catalog
-- migrations on existing installs, so it must contain only the legacy table
-- shape and indexes whose columns already existed before those migrations.
-- Keep new columns and indexes in milestones.sql and the catalog instead.
CREATE TABLE IF NOT EXISTS milestone_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,
	description TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS milestones (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	target_date DATE,
	status TEXT NOT NULL DEFAULT 'planning',
	category_id INTEGER,
	is_global INTEGER NOT NULL DEFAULT 1,
	workspace_id INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (category_id) REFERENCES milestone_categories(id) ON DELETE SET NULL,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	CONSTRAINT milestones_scope_check CHECK (
		(is_global = TRUE AND workspace_id IS NULL) OR
		(is_global = FALSE AND workspace_id IS NOT NULL)
	)
);

CREATE TABLE IF NOT EXISTS milestone_releases (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	milestone_id INTEGER NOT NULL REFERENCES milestones(id) ON DELETE CASCADE,
	tag_name TEXT NOT NULL,
	name TEXT,
	body TEXT,
	is_draft INTEGER NOT NULL DEFAULT 0,
	is_prerelease INTEGER NOT NULL DEFAULT 0,
	target_commitish TEXT,
	workspace_repository_id INTEGER,
	tag_url TEXT,
	release_status TEXT NOT NULL DEFAULT 'tag_only',
	released_at DATETIME,
	assets_json TEXT NOT NULL DEFAULT '[]',
	last_synced_at DATETIME,
	scm_connection_id INTEGER REFERENCES workspace_scm_connections(id) ON DELETE SET NULL,
	scm_repository TEXT,
	scm_release_id TEXT,
	scm_release_url TEXT,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS item_milestones (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id INTEGER NOT NULL,
	milestone_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE CASCADE,
	UNIQUE(item_id, milestone_id)
);

CREATE INDEX IF NOT EXISTS idx_milestones_category_id ON milestones(category_id);
CREATE INDEX IF NOT EXISTS idx_milestones_status ON milestones(status);
CREATE INDEX IF NOT EXISTS idx_milestones_target_date ON milestones(target_date);
CREATE INDEX IF NOT EXISTS idx_milestones_workspace_id ON milestones(workspace_id);
CREATE INDEX IF NOT EXISTS idx_milestones_is_global ON milestones(is_global);
CREATE INDEX IF NOT EXISTS idx_milestone_releases_milestone_id ON milestone_releases(milestone_id);
CREATE INDEX IF NOT EXISTS idx_item_milestones_item_id ON item_milestones(item_id);
CREATE INDEX IF NOT EXISTS idx_item_milestones_milestone_id ON item_milestones(milestone_id);

-- migration: 20260716_milestone_scope_guard
