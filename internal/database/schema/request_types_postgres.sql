
CREATE TABLE IF NOT EXISTS request_types (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT DEFAULT '',
	item_type_id INTEGER NOT NULL,
	icon TEXT DEFAULT 'FileText',
	color TEXT DEFAULT '#6b7280',
	display_order INTEGER DEFAULT 0,
	is_active BOOLEAN DEFAULT true,
	config TEXT DEFAULT NULL,
	visibility_group_ids JSONB DEFAULT NULL,
	visibility_org_ids JSONB DEFAULT NULL,
	workspace_id INTEGER DEFAULT NULL,
	title_template TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE RESTRICT,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL
);
