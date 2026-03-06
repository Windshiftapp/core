-- Portal customer tables
-- Portal customers (individual portal users)
CREATE TABLE IF NOT EXISTS portal_customers (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	phone TEXT,
	user_id INTEGER,
	customer_organisation_id INTEGER,
	custom_field_values JSONB,
	is_primary BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
	FOREIGN KEY (customer_organisation_id) REFERENCES customer_organisations(id) ON DELETE SET NULL
);

-- Portal customer channel access control
CREATE TABLE IF NOT EXISTS portal_customer_channels (
	id SERIAL PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	channel_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
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
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	is_system BOOLEAN DEFAULT false,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Portal customer roles (many-to-many relationship)
CREATE TABLE IF NOT EXISTS portal_customer_roles (
	id SERIAL PRIMARY KEY,
	portal_customer_id INTEGER NOT NULL,
	contact_role_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
	FOREIGN KEY (contact_role_id) REFERENCES contact_roles(id) ON DELETE CASCADE,
	UNIQUE(portal_customer_id, contact_role_id)
);

CREATE INDEX IF NOT EXISTS idx_contact_roles_name ON contact_roles(name);
CREATE INDEX IF NOT EXISTS idx_portal_customer_roles_customer_id ON portal_customer_roles(portal_customer_id);
CREATE INDEX IF NOT EXISTS idx_portal_customer_roles_role_id ON portal_customer_roles(contact_role_id);
