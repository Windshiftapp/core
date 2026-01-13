-- Mentions system table for tracking @mentions in comments and descriptions

CREATE TABLE IF NOT EXISTS mentions (
	id SERIAL PRIMARY KEY,
	source_type TEXT NOT NULL,           -- 'comment' or 'item_description'
	source_id INTEGER NOT NULL,           -- comment.id or item.id
	mentioned_user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	created_by INTEGER,
	mentioned_user_display_name TEXT NOT NULL,  -- Snapshot at mention time
	notification_sent BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

	FOREIGN KEY (mentioned_user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,

	UNIQUE (source_type, source_id, mentioned_user_id)
);

CREATE INDEX IF NOT EXISTS idx_mentions_mentioned_user ON mentions(mentioned_user_id);
CREATE INDEX IF NOT EXISTS idx_mentions_source ON mentions(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_mentions_item ON mentions(item_id);
CREATE INDEX IF NOT EXISTS idx_mentions_workspace ON mentions(workspace_id);
