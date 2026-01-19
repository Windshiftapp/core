-- Portal customer authentication tables (PostgreSQL)

-- Magic link tokens for portal customer authentication
CREATE TABLE IF NOT EXISTS portal_customer_magic_links (
	id SERIAL PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	token TEXT NOT NULL UNIQUE,
	channel_id INTEGER,
	expires_at TIMESTAMP NOT NULL,
	used_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL
);

-- Portal customer sessions (separate from internal user sessions)
CREATE TABLE IF NOT EXISTS portal_customer_sessions (
	id SERIAL PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	session_token TEXT UNIQUE NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	ip_address TEXT,
	user_agent TEXT,
	is_active BOOLEAN DEFAULT true,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_portal_magic_links_token ON portal_customer_magic_links(token);
CREATE INDEX IF NOT EXISTS idx_portal_magic_links_customer_id ON portal_customer_magic_links(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_magic_links_expires_at ON portal_customer_magic_links(expires_at);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_token ON portal_customer_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_customer_id ON portal_customer_sessions(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_sessions_expires_at ON portal_customer_sessions(expires_at);
