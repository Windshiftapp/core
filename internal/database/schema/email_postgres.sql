-- Email channel tables for inbound email processing

-- Email providers configuration (Microsoft, Google, Generic IMAP)
CREATE TABLE IF NOT EXISTS email_providers (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	slug TEXT UNIQUE NOT NULL,
	type TEXT NOT NULL CHECK(type IN ('microsoft', 'google', 'generic')),
	is_enabled BOOLEAN NOT NULL DEFAULT false,
	oauth_client_id TEXT,
	oauth_client_secret_encrypted TEXT,
	oauth_scopes TEXT,
	oauth_tenant_id TEXT,
	imap_host TEXT,
	imap_port INTEGER,
	imap_encryption TEXT CHECK(imap_encryption IN ('ssl', 'tls', 'starttls', 'none') OR imap_encryption IS NULL),
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_email_providers_slug ON email_providers(slug);
CREATE INDEX IF NOT EXISTS idx_email_providers_type ON email_providers(type);
CREATE INDEX IF NOT EXISTS idx_email_providers_is_enabled ON email_providers(is_enabled);

-- Email channel state for tracking IMAP sync progress
CREATE TABLE IF NOT EXISTS email_channel_state (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL UNIQUE,
	last_uid INTEGER DEFAULT 0,
	uid_validity BIGINT DEFAULT 0,
	last_checked_at TIMESTAMPTZ,
	error_count INTEGER DEFAULT 0,
	last_error TEXT,
	failed_message_uid INTEGER NOT NULL DEFAULT 0,
	failed_message_uid_validity BIGINT NOT NULL DEFAULT 0,
	failed_message_count INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_channel_state_channel_id ON email_channel_state(channel_id);

-- Cross-process lease for OAuth token refresh/callback mutations. A process
-- crash is recovered by expires_at rather than wedging a channel forever.
CREATE TABLE IF NOT EXISTS email_credential_leases (
	channel_id INTEGER PRIMARY KEY,
	owner_token TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_credential_leases_expires_at
	ON email_credential_leases(expires_at);

-- Cross-process lease for mailbox polling. See email.sql for the invariant.
CREATE TABLE IF NOT EXISTS email_processing_leases (
	channel_id INTEGER PRIMARY KEY,
	owner_token TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_processing_leases_expires_at
	ON email_processing_leases(expires_at);

-- Email message tracking for deduplication and reply threading.
-- See email.sql for the dedup_key contract.
CREATE TABLE IF NOT EXISTS email_message_tracking (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	message_id TEXT NOT NULL,
	dedup_key TEXT NOT NULL DEFAULT '',
	in_reply_to TEXT,
	from_email TEXT NOT NULL,
	from_name TEXT,
	subject TEXT,
	item_id INTEGER,
	comment_id INTEGER,
	-- attachments_status: see email.sql for the column contract.
	attachments_status TEXT CHECK(attachments_status IN ('ok','partial','failed') OR attachments_status IS NULL),
	direction TEXT DEFAULT 'inbound' CHECK(direction IN ('inbound', 'outbound')),
	-- uid/uid_validity/rate_limited_at: see email.sql for the column contract.
	uid BIGINT NOT NULL DEFAULT 0,
	uid_validity BIGINT NOT NULL DEFAULT 0,
	rate_limited_at TIMESTAMPTZ,
	processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_email_message_tracking_channel_id ON email_message_tracking(channel_id);
CREATE INDEX IF NOT EXISTS idx_email_message_tracking_message_id ON email_message_tracking(message_id);
CREATE INDEX IF NOT EXISTS idx_email_message_tracking_in_reply_to ON email_message_tracking(in_reply_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_message_tracking_dedup ON email_message_tracking(channel_id, dedup_key);

-- Durable at-least-once queue for comment replies. See email.sql.
CREATE TABLE IF NOT EXISTS email_reply_outbox (
	id SERIAL PRIMARY KEY,
	comment_id INTEGER NOT NULL UNIQUE,
	channel_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	to_email TEXT NOT NULL,
	to_name TEXT NOT NULL DEFAULT '',
	subject TEXT NOT NULL,
	html_body TEXT NOT NULL,
	text_body TEXT NOT NULL,
	message_id TEXT NOT NULL,
	in_reply_to TEXT NOT NULL DEFAULT '',
	references_json TEXT NOT NULL DEFAULT '[]',
	from_email TEXT NOT NULL,
	from_name TEXT NOT NULL DEFAULT '',
	attempt_count INTEGER NOT NULL DEFAULT 0,
	next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_error TEXT,
	delivered_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_reply_outbox_pending
	ON email_reply_outbox(delivered_at, next_attempt_at);

-- Email OAuth state for tracking OAuth flow state
CREATE TABLE IF NOT EXISTS email_oauth_state (
	id SERIAL PRIMARY KEY,
	provider_id INTEGER,
	channel_id INTEGER,
	state TEXT UNIQUE NOT NULL,
	user_id INTEGER NOT NULL,
	restore_channel_enabled BOOLEAN NOT NULL DEFAULT false,
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES email_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_oauth_state_state ON email_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_email_oauth_state_provider_id ON email_oauth_state(provider_id);
CREATE INDEX IF NOT EXISTS idx_email_oauth_state_expires_at ON email_oauth_state(expires_at);
