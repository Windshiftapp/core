-- Milestone System Tables
CREATE TABLE IF NOT EXISTS milestone_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS milestones (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	target_date DATE,  -- nullable, optional target date
	status TEXT NOT NULL DEFAULT 'planning', -- planning, in-progress, completed, cancelled
	category_id INTEGER,
	is_global BOOLEAN NOT NULL DEFAULT true,  -- true=global, false=workspace-specific
	workspace_id INTEGER,  -- NULL if global, workspace reference if local
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (category_id) REFERENCES milestone_categories(id) ON DELETE SET NULL,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	CONSTRAINT milestones_scope_check CHECK (
		(is_global = true AND workspace_id IS NULL) OR
		(is_global = false AND workspace_id IS NOT NULL)
	)
);

CREATE TABLE IF NOT EXISTS milestone_releases (
	id SERIAL PRIMARY KEY,
	milestone_id INTEGER NOT NULL REFERENCES milestones(id) ON DELETE CASCADE,
	tag_name TEXT NOT NULL,
	name TEXT,
	body TEXT,
	is_draft BOOLEAN NOT NULL DEFAULT false,
	is_prerelease BOOLEAN NOT NULL DEFAULT false,
	target_commitish TEXT,
	scm_connection_id INTEGER, -- FK added in scm_postgres.sql (circular dep: items→milestones→scm→items)
	scm_repository TEXT,
	scm_release_id TEXT,
	scm_release_url TEXT,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for milestone system
CREATE INDEX IF NOT EXISTS idx_milestones_category_id ON milestones(category_id);
CREATE INDEX IF NOT EXISTS idx_milestones_status ON milestones(status);
CREATE INDEX IF NOT EXISTS idx_milestones_target_date ON milestones(target_date);
CREATE INDEX IF NOT EXISTS idx_milestones_workspace_id ON milestones(workspace_id);
CREATE INDEX IF NOT EXISTS idx_milestones_is_global ON milestones(is_global);
CREATE INDEX IF NOT EXISTS idx_milestone_releases_milestone_id ON milestone_releases(milestone_id);
