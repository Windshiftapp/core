-- LDAP directory configuration and sync tables

-- LDAP directory configurations
CREATE TABLE IF NOT EXISTS ldap_configs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	enabled BOOLEAN DEFAULT 0,
	-- Connection settings
	host TEXT NOT NULL,
	port INTEGER NOT NULL DEFAULT 389,
	use_tls BOOLEAN DEFAULT 0,           -- Use STARTTLS
	use_ssl BOOLEAN DEFAULT 0,           -- Use LDAPS (port 636)
	skip_tls_verify BOOLEAN DEFAULT 0,   -- Skip TLS certificate verification (dev only)
	-- Bind credentials
	bind_dn TEXT NOT NULL,               -- DN for binding to LDAP (e.g., cn=admin,dc=example,dc=com)
	bind_password_encrypted TEXT NOT NULL, -- Encrypted bind password
	-- Search settings
	base_dn TEXT NOT NULL,               -- Base DN for user search (e.g., ou=people,dc=example,dc=com)
	user_filter TEXT DEFAULT '(objectClass=inetOrgPerson)', -- LDAP filter for users
	group_base_dn TEXT,                  -- Base DN for group search
	group_filter TEXT DEFAULT '(objectClass=groupOfNames)', -- LDAP filter for groups
	-- Attribute mapping
	attr_username TEXT DEFAULT 'uid',
	attr_email TEXT DEFAULT 'mail',
	attr_first_name TEXT DEFAULT 'givenName',
	attr_last_name TEXT DEFAULT 'sn',
	attr_display_name TEXT DEFAULT 'cn',
	attr_group_member TEXT DEFAULT 'member', -- Group membership attribute
	-- Sync settings
	sync_interval_minutes INTEGER DEFAULT 60,  -- Auto-sync interval (0 = disabled)
	auto_provision_users BOOLEAN DEFAULT 1,    -- Create users on sync
	auto_deactivate_users BOOLEAN DEFAULT 0,   -- Deactivate users removed from LDAP
	-- Metadata
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ldap_configs_enabled ON ldap_configs(enabled);

-- LDAP sync status tracking
CREATE TABLE IF NOT EXISTS ldap_sync_status (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	config_id INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending', -- pending, running, completed, failed
	started_at DATETIME,
	completed_at DATETIME,
	users_synced INTEGER DEFAULT 0,
	users_created INTEGER DEFAULT 0,
	users_updated INTEGER DEFAULT 0,
	users_deactivated INTEGER DEFAULT 0,
	error_message TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (config_id) REFERENCES ldap_configs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ldap_sync_status_config_id ON ldap_sync_status(config_id);
CREATE INDEX IF NOT EXISTS idx_ldap_sync_status_created_at ON ldap_sync_status(created_at);

-- LDAP user mapping (tracks which users came from LDAP)
CREATE TABLE IF NOT EXISTS ldap_user_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	config_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	ldap_dn TEXT NOT NULL,              -- Distinguished Name in LDAP
	ldap_uid TEXT NOT NULL,             -- UID attribute value
	last_synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (config_id) REFERENCES ldap_configs(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(config_id, ldap_dn)
);

CREATE INDEX IF NOT EXISTS idx_ldap_user_mappings_config_id ON ldap_user_mappings(config_id);
CREATE INDEX IF NOT EXISTS idx_ldap_user_mappings_user_id ON ldap_user_mappings(user_id);
CREATE INDEX IF NOT EXISTS idx_ldap_user_mappings_ldap_dn ON ldap_user_mappings(ldap_dn);
