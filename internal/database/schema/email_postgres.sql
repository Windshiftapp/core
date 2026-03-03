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
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_email_providers_slug ON email_providers(slug);
CREATE INDEX IF NOT EXISTS idx_email_providers_type ON email_providers(type);
CREATE INDEX IF NOT EXISTS idx_email_providers_is_enabled ON email_providers(is_enabled);

-- Email channel state for tracking IMAP sync progress
CREATE TABLE IF NOT EXISTS email_channel_state (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL UNIQUE,
	last_uid INTEGER DEFAULT 0,
	last_checked_at TIMESTAMP,
	error_count INTEGER DEFAULT 0,
	last_error TEXT,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_channel_state_channel_id ON email_channel_state(channel_id);

-- Email message tracking for deduplication and reply threading
CREATE TABLE IF NOT EXISTS email_message_tracking (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	message_id TEXT NOT NULL,
	in_reply_to TEXT,
	from_email TEXT NOT NULL,
	from_name TEXT,
	subject TEXT,
	item_id INTEGER,
	comment_id INTEGER,
	direction TEXT DEFAULT 'inbound' CHECK(direction IN ('inbound', 'outbound')),
	processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_email_message_tracking_channel_id ON email_message_tracking(channel_id);
CREATE INDEX IF NOT EXISTS idx_email_message_tracking_message_id ON email_message_tracking(message_id);
CREATE INDEX IF NOT EXISTS idx_email_message_tracking_in_reply_to ON email_message_tracking(in_reply_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_message_tracking_unique ON email_message_tracking(channel_id, message_id);

-- Email OAuth state for tracking OAuth flow state
CREATE TABLE IF NOT EXISTS email_oauth_state (
	id SERIAL PRIMARY KEY,
	provider_id INTEGER NOT NULL,
	channel_id INTEGER,
	state TEXT UNIQUE NOT NULL,
	user_id INTEGER NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (provider_id) REFERENCES email_providers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_oauth_state_state ON email_oauth_state(state);
CREATE INDEX IF NOT EXISTS idx_email_oauth_state_provider_id ON email_oauth_state(provider_id);
CREATE INDEX IF NOT EXISTS idx_email_oauth_state_expires_at ON email_oauth_state(expires_at);
