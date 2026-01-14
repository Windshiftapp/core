-- Content system tables (attachments, comments, links)

-- Attachment System Tables
-- Polymorphic attachment system: entity_type determines what item_id refers to
-- entity_type values: 'item' (work items), 'test_case', 'avatar', 'workspace_avatar'
CREATE TABLE IF NOT EXISTS attachments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id INTEGER, -- Entity ID (polymorphic, based on entity_type)
	entity_type TEXT DEFAULT 'item', -- Type of entity: item, test_case, avatar, workspace_avatar
	filename TEXT NOT NULL,
	original_filename TEXT NOT NULL,
	file_path TEXT NOT NULL,
	mime_type TEXT NOT NULL,
	file_size INTEGER NOT NULL,
	uploaded_by INTEGER,
	has_thumbnail BOOLEAN DEFAULT false,
	thumbnail_path TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_attachments_item_id ON attachments(item_id);
CREATE INDEX IF NOT EXISTS idx_attachments_uploaded_by ON attachments(uploaded_by);
CREATE INDEX IF NOT EXISTS idx_attachments_entity ON attachments(entity_type, item_id);

CREATE TABLE IF NOT EXISTS attachment_settings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	max_file_size INTEGER NOT NULL DEFAULT 52428800, -- 50MB default
	allowed_mime_types TEXT, -- JSON array of allowed MIME types
	attachment_path TEXT NOT NULL,
	enabled BOOLEAN DEFAULT true,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Comments System Tables
CREATE TABLE IF NOT EXISTS comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id INTEGER NOT NULL,
	author_id INTEGER,
	portal_customer_id INTEGER,
	content TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_comments_item_id ON comments(item_id);
CREATE INDEX IF NOT EXISTS idx_comments_author_id ON comments(author_id);
CREATE INDEX IF NOT EXISTS idx_comments_portal_customer_id ON comments(portal_customer_id);

-- Diagram System Tables
CREATE TABLE IF NOT EXISTS item_diagrams (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	diagram_data TEXT NOT NULL, -- JSON with elements, appState, files
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	created_by INTEGER,
	updated_by INTEGER,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_item_diagrams_item_id ON item_diagrams(item_id);
CREATE INDEX IF NOT EXISTS idx_item_diagrams_created_by ON item_diagrams(created_by);

-- Link management tables
CREATE TABLE IF NOT EXISTS link_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	forward_label TEXT NOT NULL,
	reverse_label TEXT NOT NULL,
	color TEXT DEFAULT '#6b7280',
	is_system BOOLEAN DEFAULT false,
	active BOOLEAN DEFAULT true,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS item_links (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	link_type_id INTEGER NOT NULL,
	source_type TEXT NOT NULL,
	source_id INTEGER NOT NULL,
	target_type TEXT NOT NULL,
	target_id INTEGER NOT NULL,
	created_by INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (link_type_id) REFERENCES link_types(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(link_type_id, source_type, source_id, target_type, target_id)
);

-- Indexes for link system performance
CREATE INDEX IF NOT EXISTS idx_item_links_source ON item_links(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_item_links_target ON item_links(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_item_links_type ON item_links(link_type_id);
CREATE INDEX IF NOT EXISTS idx_item_links_created_by ON item_links(created_by);
