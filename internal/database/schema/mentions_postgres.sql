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
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,

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

-- Durable queue for the dedicated @mention email that carries the message
-- body. Kept separate from notification batching so the content survives
-- retries and is re-authorized immediately before SMTP delivery.
CREATE TABLE IF NOT EXISTS mention_email_outbox (
	id SERIAL PRIMARY KEY,
	mention_id INTEGER NOT NULL,
	-- In-app notification suppressed once this email is delivered, so the
	-- digest does not send a second, content-less copy of the same mention.
	notification_id INTEGER,
	user_id INTEGER NOT NULL,
	workspace_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	to_email TEXT NOT NULL,
	to_name TEXT NOT NULL DEFAULT '',
	actor_name TEXT NOT NULL DEFAULT '',
	item_key TEXT NOT NULL DEFAULT '',
	item_title TEXT NOT NULL DEFAULT '',
	source_type TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL DEFAULT '',
	action_url TEXT NOT NULL DEFAULT '',
	attempt_count INTEGER NOT NULL DEFAULT 0,
	next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	-- Set while a worker holds a delivery lease; NULL means next_attempt_at
	-- is retry backoff rather than a claim.
	lease_owner TEXT,
	last_error TEXT,
	delivered_at TIMESTAMPTZ,
	-- discarded_at marks a permanent skip (recipient lost access).
	discarded_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

	FOREIGN KEY (mention_id) REFERENCES mentions(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mention_email_outbox_pending
	ON mention_email_outbox(delivered_at, discarded_at, next_attempt_at);

-- One email per mention row; retries reuse the same row.
CREATE UNIQUE INDEX IF NOT EXISTS uq_mention_email_outbox_mention
	ON mention_email_outbox(mention_id);
