-- Milestone System Tables
CREATE TABLE IF NOT EXISTS milestone_categories (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	color TEXT NOT NULL,  -- Hex color code (e.g., "#3b82f6")
	description TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
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
	external_key TEXT,     -- Stable upsert key for automation (e.g. tag short-name "2.0"). Unique per workspace when set.
	position INTEGER NOT NULL DEFAULT 0,  -- Manual sort order, scoped per (is_global, workspace_id, category_id). Backfilled from target_date,name on upgrade.
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
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
	idempotency_key TEXT,
	state TEXT NOT NULL DEFAULT 'created',
	last_error TEXT,
	lease_token TEXT,
	lease_expires_at TIMESTAMPTZ,
	tag_name TEXT NOT NULL,
	name TEXT,
	body TEXT,
	is_draft BOOLEAN NOT NULL DEFAULT false,
	is_prerelease BOOLEAN NOT NULL DEFAULT false,
	target_commitish TEXT,
	workspace_repository_id INTEGER,
	tag_url TEXT,
	release_status TEXT NOT NULL DEFAULT 'tag_only',
	released_at TIMESTAMPTZ,
	assets_json JSONB NOT NULL DEFAULT '[]'::jsonb,
	last_synced_at TIMESTAMPTZ,
	scm_connection_id INTEGER, -- FK added in scm_postgres.sql (circular dep: items→milestones→scm→items)
	scm_repository TEXT,
	scm_release_id TEXT,
	scm_release_url TEXT,
	created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Junction table for item ↔ milestone many-to-many. One row per (item, milestone).
CREATE TABLE IF NOT EXISTS item_milestones (
	id SERIAL PRIMARY KEY,
	item_id INTEGER NOT NULL,
	milestone_id INTEGER NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	-- FK on item_id added in items_postgres.sql once items has been created.
	FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE CASCADE,
	UNIQUE(item_id, milestone_id)
);

-- Indexes for milestone system
CREATE INDEX IF NOT EXISTS idx_milestones_category_id ON milestones(category_id);
CREATE INDEX IF NOT EXISTS idx_milestones_status ON milestones(status);
CREATE INDEX IF NOT EXISTS idx_milestones_target_date ON milestones(target_date);
CREATE INDEX IF NOT EXISTS idx_milestones_workspace_id ON milestones(workspace_id);
CREATE INDEX IF NOT EXISTS idx_milestones_is_global ON milestones(is_global);
CREATE INDEX IF NOT EXISTS idx_milestone_releases_milestone_id ON milestone_releases(milestone_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_milestone_releases_idempotency
	ON milestone_releases(milestone_id, idempotency_key)
	WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_milestone_releases_repository_tag
	ON milestone_releases(workspace_repository_id, tag_name)
	WHERE workspace_repository_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_item_milestones_item_id ON item_milestones(item_id);
CREATE INDEX IF NOT EXISTS idx_item_milestones_milestone_id ON item_milestones(milestone_id);
-- Partial unique index: one external_key per workspace, but NULLs are unconstrained.
CREATE UNIQUE INDEX IF NOT EXISTS uq_milestones_workspace_external_key
	ON milestones(workspace_id, external_key)
	WHERE external_key IS NOT NULL;

-- Composite index supports the per-scope position ordering used by drag-and-drop
-- reorder. Existing installs use milestones_existing_postgres.sql before
-- catalog migrations, so this canonical fresh-install schema can include the
-- index.
CREATE INDEX IF NOT EXISTS idx_milestones_position ON milestones(is_global, workspace_id, category_id, position);
