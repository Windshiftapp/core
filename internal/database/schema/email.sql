-- Email channel tables for inbound email processing

-- Email providers configuration (Microsoft, Google, Generic IMAP)
CREATE TABLE IF NOT EXISTS email_providers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	slug TEXT UNIQUE NOT NULL,
	type TEXT NOT NULL CHECK(type IN ('microsoft', 'google', 'generic')),
	is_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	oauth_client_id TEXT,
	oauth_client_secret_encrypted TEXT,
	oauth_scopes TEXT,
	oauth_tenant_id TEXT,
	imap_host TEXT,
	imap_port INTEGER,
	imap_encryption TEXT CHECK(imap_encryption IN ('ssl', 'tls', 'starttls', 'none') OR imap_encryption IS NULL),
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_email_providers_slug ON email_providers(slug);
CREATE INDEX IF NOT EXISTS idx_email_providers_type ON email_providers(type);
CREATE INDEX IF NOT EXISTS idx_email_providers_is_enabled ON email_providers(is_enabled);

-- Email channel state for tracking IMAP sync progress
CREATE TABLE IF NOT EXISTS email_channel_state (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id INTEGER NOT NULL UNIQUE,
	last_uid INTEGER DEFAULT 0,
	uid_validity INTEGER DEFAULT 0,
	last_checked_at DATETIME,
	error_count INTEGER DEFAULT 0,
	last_error TEXT,
	failed_message_uid INTEGER NOT NULL DEFAULT 0,
	failed_message_uid_validity INTEGER NOT NULL DEFAULT 0,
	failed_message_count INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_channel_state_channel_id ON email_channel_state(channel_id);

-- Cross-process lease for OAuth token refresh/callback mutations. A process
-- crash is recovered by expires_at rather than wedging a channel forever.
CREATE TABLE IF NOT EXISTS email_credential_leases (
	channel_id INTEGER PRIMARY KEY,
	owner_token TEXT NOT NULL,
	expires_at DATETIME NOT NULL,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_credential_leases_expires_at
	ON email_credential_leases(expires_at);

-- Cross-process lease for mailbox polling. Without this, a scheduled poll on
-- one server can race a manual or scheduled poll on another server and advance
-- last_uid while the first worker's item creation is still in flight.
CREATE TABLE IF NOT EXISTS email_processing_leases (
	channel_id INTEGER PRIMARY KEY,
	owner_token TEXT NOT NULL,
	expires_at DATETIME NOT NULL,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_processing_leases_expires_at
	ON email_processing_leases(expires_at);

-- Email message tracking for deduplication and reply threading.
-- dedup_key is the unique-per-channel handle: when message_id is present
-- (RFC 5322 §3.6.4 requires it but real-world mail sometimes omits it) the
-- key is just the message_id; otherwise the scheduler synthesizes
-- "synth:<channel_id>:<uidvalidity>:<uid>" so two MessageID-less emails in
-- the same channel don't collapse onto a single empty-string tracking row.
CREATE TABLE IF NOT EXISTS email_message_tracking (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id INTEGER NOT NULL,
	message_id TEXT NOT NULL,
	dedup_key TEXT NOT NULL DEFAULT '',
	in_reply_to TEXT,
	from_email TEXT NOT NULL,
	from_name TEXT,
	subject TEXT,
	item_id INTEGER,
	comment_id INTEGER,
	-- attachments_status records whether the email's attachments survived
	-- processing. NULL = no attachments / not yet computed. 'ok' = all stored.
	-- 'partial' = some stored, some dropped due to fatal write/insert errors
	-- (size and MIME-allowlist rejections are config decisions, not failures).
	-- 'failed' = the email had attachments but none were stored.
	attachments_status TEXT CHECK(attachments_status IN ('ok','partial','failed') OR attachments_status IS NULL),
	direction TEXT DEFAULT 'inbound' CHECK(direction IN ('inbound', 'outbound')),
	-- uid/uid_validity record the IMAP coordinates observed at fetch time so
	-- rate-limited mail can be requeued surgically within the right epoch.
	uid INTEGER NOT NULL DEFAULT 0,
	uid_validity INTEGER NOT NULL DEFAULT 0,
	-- rate_limited_at is set when per-sender flood protection declined to
	-- create a ticket. The mail stays in the mailbox for operator requeue.
	rate_limited_at DATETIME,
	processed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_email_message_tracking_channel_id ON email_message_tracking(channel_id);
CREATE INDEX IF NOT EXISTS idx_email_message_tracking_message_id ON email_message_tracking(message_id);
CREATE INDEX IF NOT EXISTS idx_email_message_tracking_in_reply_to ON email_message_tracking(in_reply_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_message_tracking_dedup ON email_message_tracking(channel_id, dedup_key);

-- Durable at-least-once queue for comment replies. Sending SMTP inline is an
-- optimization; a transient failure leaves this row pending for the
-- notification scheduler instead of silently losing the customer reply.
CREATE TABLE IF NOT EXISTS email_reply_outbox (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
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
	next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_error TEXT,
	delivered_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_reply_outbox_pending
	ON email_reply_outbox(delivered_at, next_attempt_at);

-- Email OAuth state for tracking OAuth flow state
CREATE TABLE IF NOT EXISTS email_oauth_state (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	provider_id INTEGER,
	channel_id INTEGER,
	state TEXT UNIQUE NOT NULL,
	user_id INTEGER NOT NULL,
	restore_channel_enabled BOOLEAN NOT NULL DEFAULT false,
	expires_at DATETIME NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES email_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_oauth_state_state ON email_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_email_oauth_state_provider_id ON email_oauth_state(provider_id);
CREATE INDEX IF NOT EXISTS idx_email_oauth_state_expires_at ON email_oauth_state(expires_at);

-- migration: 0000_baseline
