-- In-progress portal request form state preserved between sessions.
-- One row per (identity, request_type); identity is either portal_customer_id
-- or user_id (internal user filling out a portal form), never both.
--
-- This lives in its own schema file so it can be loaded AFTER request_types
-- and items, both of which the FKs reference. Postgres validates FK targets
-- at CREATE TABLE time; SQLite is permissive and keeps the same definition
-- inline in portal.sql.
CREATE TABLE IF NOT EXISTS portal_request_drafts (
	id SERIAL PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	request_type_id INTEGER NOT NULL,
	portal_customer_id INTEGER,
	user_id INTEGER,
	title TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	custom_field_values JSONB,
	current_step INTEGER NOT NULL DEFAULT 1,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (request_type_id) REFERENCES request_types(id) ON DELETE CASCADE,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	CHECK (portal_customer_id IS NOT NULL OR user_id IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_portal_request_drafts_pc
	ON portal_request_drafts(portal_customer_id, request_type_id)
	WHERE portal_customer_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_portal_request_drafts_user
	ON portal_request_drafts(user_id, request_type_id)
	WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_portal_request_drafts_updated_at
	ON portal_request_drafts(updated_at DESC);
