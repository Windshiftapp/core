-- LDAP directory configuration and sync tables (PostgreSQL)

-- LDAP directory configurations
CREATE TABLE IF NOT EXISTS ldap_configs (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	enabled BOOLEAN DEFAULT FALSE,
	-- Connection settings
	host TEXT NOT NULL,
	port INTEGER NOT NULL DEFAULT 389,
	use_tls BOOLEAN DEFAULT FALSE,
	use_ssl BOOLEAN DEFAULT FALSE,
	skip_tls_verify BOOLEAN DEFAULT FALSE,
	-- Bind credentials
	bind_dn TEXT NOT NULL,
	bind_password_encrypted TEXT NOT NULL,
	-- Search settings
	base_dn TEXT NOT NULL,
	user_filter TEXT DEFAULT '(objectClass=inetOrgPerson)',
	group_base_dn TEXT,
	group_filter TEXT DEFAULT '(objectClass=groupOfNames)',
	-- Attribute mapping
	attr_username TEXT DEFAULT 'uid',
	attr_email TEXT DEFAULT 'mail',
	attr_first_name TEXT DEFAULT 'givenName',
	attr_last_name TEXT DEFAULT 'sn',
	attr_display_name TEXT DEFAULT 'cn',
	attr_group_member TEXT DEFAULT 'member',
	-- Sync settings
	sync_interval_minutes INTEGER DEFAULT 60,
	auto_provision_users BOOLEAN DEFAULT TRUE,
	auto_deactivate_users BOOLEAN DEFAULT FALSE,
	-- Metadata
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ldap_configs_enabled ON ldap_configs(enabled);

-- LDAP sync status tracking
CREATE TABLE IF NOT EXISTS ldap_sync_status (
	id SERIAL PRIMARY KEY,
	config_id INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	started_at TIMESTAMP,
	completed_at TIMESTAMP,
	users_synced INTEGER DEFAULT 0,
	users_created INTEGER DEFAULT 0,
	users_updated INTEGER DEFAULT 0,
	users_deactivated INTEGER DEFAULT 0,
	error_message TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (config_id) REFERENCES ldap_configs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ldap_sync_status_config_id ON ldap_sync_status(config_id);
CREATE INDEX IF NOT EXISTS idx_ldap_sync_status_created_at ON ldap_sync_status(created_at);

-- LDAP user mapping
CREATE TABLE IF NOT EXISTS ldap_user_mappings (
	id SERIAL PRIMARY KEY,
	config_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	ldap_dn TEXT NOT NULL,
	ldap_uid TEXT NOT NULL,
	last_synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (config_id) REFERENCES ldap_configs(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	UNIQUE(config_id, ldap_dn)
);

CREATE INDEX IF NOT EXISTS idx_ldap_user_mappings_config_id ON ldap_user_mappings(config_id);
CREATE INDEX IF NOT EXISTS idx_ldap_user_mappings_user_id ON ldap_user_mappings(user_id);
CREATE INDEX IF NOT EXISTS idx_ldap_user_mappings_ldap_dn ON ldap_user_mappings(ldap_dn);
