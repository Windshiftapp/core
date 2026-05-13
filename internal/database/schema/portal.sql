-- Portal customer tables
-- Portal customers (individual portal users)
CREATE TABLE IF NOT EXISTS portal_customers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	phone TEXT,
	user_id INTEGER,
	customer_organisation_id INTEGER,
	custom_field_values TEXT,
	is_primary BOOLEAN DEFAULT false,
	dismissed_passkey_prompt_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (customer_organisation_id) REFERENCES customer_organisations(id) ON DELETE SET NULL
);

-- Portal customer channel access control
CREATE TABLE IF NOT EXISTS portal_customer_channels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	portal_customer_id INTEGER NOT NULL,
	channel_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	UNIQUE(portal_customer_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_portal_customers_email ON portal_customers(email);
CREATE INDEX IF NOT EXISTS idx_portal_customers_user_id ON portal_customers(user_id);
CREATE INDEX IF NOT EXISTS idx_portal_customers_org_id ON portal_customers(customer_organisation_id);
CREATE INDEX IF NOT EXISTS idx_portal_customer_channels_customer_id ON portal_customer_channels(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_customer_channels_channel_id ON portal_customer_channels(channel_id);

-- Contact roles lookup table
CREATE TABLE IF NOT EXISTS contact_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Portal customer roles (many-to-many relationship)
CREATE TABLE IF NOT EXISTS portal_customer_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	portal_customer_id INTEGER NOT NULL,
	contact_role_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (contact_role_id) REFERENCES contact_roles(id) ON DELETE CASCADE,
	UNIQUE(portal_customer_id, contact_role_id)
);

CREATE INDEX IF NOT EXISTS idx_contact_roles_name ON contact_roles(name);
CREATE INDEX IF NOT EXISTS idx_portal_customer_roles_customer_id ON portal_customer_roles(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_customer_roles_role_id ON portal_customer_roles(contact_role_id);

-- Seed the default system contact role used when a portal customer is
-- created without an explicit role list. PortalCustomersHandler.Create
-- fails the request if this row is absent, so the seed must run at
-- schema-apply time.
INSERT OR IGNORE INTO contact_roles (name, description, is_system) VALUES
('Portal Customer', 'Default role assigned to portal customers', true);

-- In-progress portal request form state preserved between sessions.
-- One row per (identity, request_type); identity is either portal_customer_id
-- or user_id (internal user filling out a portal form), never both.
CREATE TABLE IF NOT EXISTS portal_request_drafts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id INTEGER NOT NULL,
	request_type_id INTEGER NOT NULL,
	portal_customer_id INTEGER,
	user_id INTEGER,
	title TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	custom_field_values TEXT,
	current_step INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
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
