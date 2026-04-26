// Package database provides database connection and transaction management.
package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema/items.sql
var itemsSchema string

//go:embed schema/request_types.sql
var requestTypeSchema string

//go:embed schema/users.sql
var usersSchema string

//go:embed schema/tests.sql
var testsSchema string

//go:embed schema/workspace.sql
var workspaceSchema string

//go:embed schema/config_workflows.sql
var configWorkflowsSchema string

//go:embed schema/time_tracking.sql
var timeTrackingSchema string

//go:embed schema/portal.sql
var portalSchema string

//go:embed schema/portal_auth.sql
var portalAuthSchema string

//go:embed schema/milestones.sql
var milestonesSchema string

//go:embed schema/iterations.sql
var iterationsSchema string

//go:embed schema/content.sql
var contentSchema string

//go:embed schema/notifications.sql
var notificationsSchema string

//go:embed schema/channels.sql
var channelsSchema string

//go:embed schema/permissions.sql
var permissionsSchema string

//go:embed schema/system.sql
var systemSchema string

//go:embed schema/core.sql
var coreSchema string

//go:embed schema/default_data.sql
var defaultDataSQL string

//go:embed schema/webauthn.sql
var webauthnSchema string

//go:embed schema/sso.sql
var ssoSchema string

//go:embed schema/scm.sql
var scmSchema string

//go:embed schema/mentions.sql
var mentionsSchema string

//go:embed schema/user_preferences.sql
var userPreferencesSchema string

//go:embed schema/assets.sql
var assetsSchema string

//go:embed schema/recurring_tasks.sql
var recurringTasksSchema string

//go:embed schema/jira_import.sql
var jiraImportSchema string

//go:embed schema/actions.sql
var actionsSchema string

//go:embed schema/email.sql
var emailSchema string

//go:embed schema/asset_reports.sql
var assetReportsSchema string

//go:embed schema/labels.sql
var labelsSchema string

//go:embed schema/llm.sql
var llmSchema string

//go:embed schema/ldap.sql
var ldapSchema string

//go:embed schema/asset_actions.sql
var assetActionsSchema string

//go:embed schema/daily_briefings.sql
var dailyBriefingsSchema string

//go:embed schema/teams.sql
var teamsSchema string

//go:embed schema/condition_sets.sql
var conditionSetsSchema string

//go:embed schema/integrations.sql
var integrationsSchema string

// DB wraps a sql.DB connection with a dedicated write connection
type DB struct {
	*sql.DB
	writeConn *sql.DB // Dedicated single connection for writes
}

// NewDB opens a SQLite database at dataSourceName and configures it for
// concurrent use: WAL journaling, a 5s busy timeout, foreign keys, and an
// immediate-locking txlock, plus a dedicated single-connection write pool so
// writes serialize without blocking reads.
//
// last review: ser, 210426
func NewDB(dataSourceName string) (*DB, error) {
	// Add SQLite-specific connection parameters for better concurrency handling
	// Check if DSN already has parameters (for shared in-memory test databases)
	separator := "?"
	if strings.Contains(dataSourceName, "?") {
		separator = "&"
	}

	connectionString := dataSourceName +
		separator + "_busy_timeout=5000" +
		"&_journal_mode=WAL" +
		"&_foreign_keys=on" +
		"&_txlock=immediate" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=temp_store(MEMORY)" +
		"&_pragma=cache_size(-16000)" +
		"&_pragma=mmap_size(0)" + // Disable mmap for better Docker compatibility
		"&_pragma=journal_size_limit(6144000)"

	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Explicitly set critical pragmas that must persist
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-262144", // 256MB cache
		"PRAGMA mmap_size=0",        // Disable mmap for better Docker compatibility
		"PRAGMA journal_size_limit=6144000",
	}

	for _, pragma := range pragmas {
		if _, err = db.Exec(pragma); err != nil {
			slog.Warn("failed to set pragma", slog.String("component", "database"), slog.String("pragma", pragma), slog.Any("error", err))
		}
	}

	// Set connection pool settings for SQLite
	db.SetMaxOpenConns(120) // Allow concurrent reads in WAL mode
	db.SetMaxIdleConns(12)  // Keep 10% idle connections

	// Create dedicated write connection with only 1 max connection to serialize writes
	writeConn, err := sql.Open("sqlite", connectionString)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to open write connection: %w", err)
	}

	writeConn.SetMaxOpenConns(1) // Single connection to serialize all writes
	writeConn.SetMaxIdleConns(1)

	if err := writeConn.Ping(); err != nil {
		_ = db.Close()
		_ = writeConn.Close()
		return nil, fmt.Errorf("failed to ping write connection: %w", err)
	}

	// Set critical pragmas on write connection (DSN params may not be applied by all drivers)
	writePragmas := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	}
	for _, pragma := range writePragmas {
		if _, err := writeConn.Exec(pragma); err != nil {
			slog.Warn("failed to set write connection pragma", slog.String("component", "database"), slog.String("pragma", pragma), slog.Any("error", err))
		}
	}

	return &DB{DB: db, writeConn: writeConn}, nil
}

// Close closes the database connections
func (db *DB) Close() error {
	var err1, err2 error
	if db.DB != nil {
		err1 = db.DB.Close()
	}
	if db.writeConn != nil {
		err2 = db.writeConn.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

func (db *DB) Initialize() error {
	// Check if database is already initialized by checking for core tables
	var tableCount int
	err := db.QueryRow(`
		SELECT COUNT(name) FROM sqlite_master 
		WHERE type='table' AND name IN ('workspaces', 'items', 'users', 'workflows')
	`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("failed to check database initialization: %w", err)
	}

	// If all core tables exist, database is already initialized
	if tableCount >= 4 {
		// Optimize query planner statistics (SQLite 3.46.0+)
		// This is safe to run on older versions - it will just be a no-op
		if _, err := db.Exec("PRAGMA optimize=0x10002"); err != nil {
			slog.Warn("PRAGMA optimize failed (may be using older SQLite)", slog.String("component", "database"), slog.Any("error", err))
		}

		// Run migrations for existing databases
		// last review: ser, 210426, NOTE: will be dropped after 0.7
		migrations := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('workspaces') WHERE name='display_mode'",
				alter: "ALTER TABLE workspaces ADD COLUMN display_mode TEXT DEFAULT 'default'",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('active_timers') WHERE name='user_id'",
				alter: "ALTER TABLE active_timers ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='start_date'",
				alter: "ALTER TABLE items ADD COLUMN start_date DATE",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='end_date'",
				alter: "ALTER TABLE items ADD COLUMN end_date DATE",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('board_configurations') WHERE name='roadmap_config'",
				alter: "ALTER TABLE board_configurations ADD COLUMN roadmap_config TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('board_configurations') WHERE name='card_fields'",
				alter: "ALTER TABLE board_configurations ADD COLUMN card_fields TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('workspaces') WHERE name='internal_comments_enabled'",
				alter: "ALTER TABLE workspaces ADD COLUMN internal_comments_enabled BOOLEAN DEFAULT FALSE",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('request_types') WHERE name='workspace_id'",
				alter: "ALTER TABLE request_types ADD COLUMN workspace_id INTEGER DEFAULT NULL REFERENCES workspaces(id) ON DELETE SET NULL",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('assets') WHERE name='import_job_id'",
				alter: "ALTER TABLE assets ADD COLUMN import_job_id TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='is_agent'",
				alter: "ALTER TABLE users ADD COLUMN is_agent BOOLEAN DEFAULT 0",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='agent_owner_user_id'",
				alter: "ALTER TABLE users ADD COLUMN agent_owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('email_channel_state') WHERE name='uid_validity'",
				alter: "ALTER TABLE email_channel_state ADD COLUMN uid_validity INTEGER DEFAULT 0",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('actions') WHERE name='actor_user_id'",
				alter: "ALTER TABLE actions ADD COLUMN actor_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('action_execution_logs') WHERE name='trigger_user_id'",
				alter: "ALTER TABLE action_execution_logs ADD COLUMN trigger_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('action_execution_logs') WHERE name='effective_actor_user_id'",
				alter: "ALTER TABLE action_execution_logs ADD COLUMN effective_actor_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('item_scm_links') WHERE name='smart_commits_applied_at'",
				alter: "ALTER TABLE item_scm_links ADD COLUMN smart_commits_applied_at DATETIME",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('workspace_scm_connections') WHERE name='smart_commits_enabled'",
				alter: "ALTER TABLE workspace_scm_connections ADD COLUMN smart_commits_enabled BOOLEAN DEFAULT 0",
			},
		}

		for _, m := range migrations {
			var count int
			if err := db.QueryRow(m.check).Scan(&count); err == nil && count == 0 {
				if _, err := db.Exec(m.alter); err != nil {
					slog.Warn("migration failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_users_is_agent ON users(is_agent)"); err != nil {
			slog.Warn("idx_users_is_agent migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Enforce is_agent immutability at the DB level: toggling the flag
		// would open a token-impersonation path (flip a user to agent, mint
		// a token, flip them back).
		if _, err := db.Exec(`
			CREATE TRIGGER IF NOT EXISTS users_is_agent_immutable
			BEFORE UPDATE OF is_agent ON users
			FOR EACH ROW
			WHEN IFNULL(NEW.is_agent, 0) IS NOT IFNULL(OLD.is_agent, 0)
			BEGIN
				SELECT RAISE(ABORT, 'is_agent is immutable');
			END
		`); err != nil {
			slog.Warn("users_is_agent_immutable trigger migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_users_agent_owner ON users(agent_owner_user_id) WHERE agent_owner_user_id IS NOT NULL"); err != nil {
			slog.Warn("idx_users_agent_owner migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// last review: ser, 210426, NOTE: all of these will be dropped from 0.7 onwards
		// Owner binding must be immutable: flipping it would silently
		// reassign every inherited permission of the agent.
		if _, err := db.Exec(`
			CREATE TRIGGER IF NOT EXISTS users_agent_owner_immutable
			BEFORE UPDATE OF agent_owner_user_id ON users
			FOR EACH ROW
			WHEN NEW.agent_owner_user_id IS NOT OLD.agent_owner_user_id
			BEGIN
				SELECT RAISE(ABORT, 'agent_owner_user_id is immutable');
			END
		`); err != nil {
			slog.Warn("users_agent_owner_immutable trigger migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Reject inserts that would create a non-agent user with an owner link.
		if _, err := db.Exec(`
			CREATE TRIGGER IF NOT EXISTS users_agent_owner_requires_agent_insert
			BEFORE INSERT ON users
			FOR EACH ROW
			WHEN NEW.agent_owner_user_id IS NOT NULL AND IFNULL(NEW.is_agent, 0) = 0
			BEGIN
				SELECT RAISE(ABORT, 'agent_owner_user_id requires is_agent');
			END
		`); err != nil {
			slog.Warn("users_agent_owner_requires_agent_insert trigger migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Seed user-managed agent feature flags.
		var umaCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM system_settings WHERE key = 'allow_user_managed_agents'`).Scan(&umaCount); err == nil && umaCount == 0 {
			if _, err := db.Exec(`INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('allow_user_managed_agents', 'false', 'boolean', 'Allow non-admin users to create and manage their own agent users from their profile', 'security')`); err != nil {
				slog.Warn("allow_user_managed_agents setting migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}
		var maxAgentsCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM system_settings WHERE key = 'max_agents_per_user'`).Scan(&maxAgentsCount); err == nil && maxAgentsCount == 0 {
			if _, err := db.Exec(`INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('max_agents_per_user', '5', 'integer', 'Maximum number of owned agents a single non-admin user may create', 'security')`); err != nil {
				slog.Warn("max_agents_per_user setting migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Create labels tables if they don't exist (for existing databases)
		if _, err := db.Exec(labelsSchema); err != nil {
			slog.Warn("labels migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create LLM tables if they don't exist (for existing databases)
		if _, err := db.Exec(llmSchema); err != nil {
			slog.Warn("llm migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create milestone_releases table if it doesn't exist and drop legacy SCM columns from milestones
		if _, err := db.Exec(milestonesSchema); err != nil {
			slog.Warn("milestones migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Drop legacy SCM columns from milestones table (moved to milestone_releases)
		scmColumnDrops := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('milestones') WHERE name='scm_connection_id'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_connection_id",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('milestones') WHERE name='scm_repository'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_repository",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('milestones') WHERE name='scm_release_id'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_release_id",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('milestones') WHERE name='scm_release_url'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_release_url",
			},
		}
		for _, m := range scmColumnDrops {
			var count int
			if err := db.QueryRow(m.check).Scan(&count); err == nil && count > 0 {
				if _, err := db.Exec(m.alter); err != nil {
					slog.Warn("milestone scm column drop failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		// Enforce uniqueness on items.frac_index. Pre-existing duplicates
		// (possible before the UpdateFracIndex cache-coherence fix) would
		// block the UNIQUE index, so null them out first, keeping the oldest
		// occurrence in each duplicate group. NULL items sort to the end of
		// the list via defaultOrderBy, so this is a safe recovery.
		var fracUniqueCount int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_items_frac_index' AND sql LIKE '%UNIQUE%'`,
		).Scan(&fracUniqueCount); err == nil && fracUniqueCount == 0 {
			if res, err := db.Exec(`
				UPDATE items SET frac_index = NULL
				WHERE frac_index IS NOT NULL
				  AND id NOT IN (
				      SELECT MIN(id) FROM items
				      WHERE frac_index IS NOT NULL
				      GROUP BY frac_index
				  )
			`); err != nil {
				slog.Warn("frac_index duplicate cleanup failed", slog.String("component", "database"), slog.Any("error", err))
			} else if n, _ := res.RowsAffected(); n > 0 {
				slog.Warn("nulled duplicate frac_index rows during UNIQUE migration", slog.String("component", "database"), slog.Int64("rows", n))
			}
			if _, err := db.Exec(`DROP INDEX IF EXISTS idx_items_frac_index`); err != nil {
				slog.Warn("drop non-unique idx_items_frac_index failed", slog.String("component", "database"), slog.Any("error", err))
			}
			if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_items_frac_index ON items(frac_index) WHERE frac_index IS NOT NULL`); err != nil {
				slog.Warn("create unique idx_items_frac_index failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add SAML columns to sso_providers (for existing databases)
		samlMigrations := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('sso_providers') WHERE name='saml_idp_metadata_url'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_idp_metadata_url TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('sso_providers') WHERE name='saml_idp_sso_url'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_idp_sso_url TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('sso_providers') WHERE name='saml_idp_certificate'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_idp_certificate TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('sso_providers') WHERE name='saml_sp_entity_id'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_sp_entity_id TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('sso_providers') WHERE name='saml_sign_requests'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_sign_requests BOOLEAN DEFAULT 0",
			},
		}
		for _, m := range samlMigrations {
			var count int
			if err := db.QueryRow(m.check).Scan(&count); err == nil && count == 0 {
				if _, err := db.Exec(m.alter); err != nil {
					slog.Warn("SAML migration failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		// Create LDAP tables if they don't exist (for existing databases)
		if _, err := db.Exec(ldapSchema); err != nil {
			slog.Warn("LDAP migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Drop workspace_everyone_roles table (permissions now derived from role assignments)
		if _, err := db.Exec(`DROP TABLE IF EXISTS workspace_everyone_roles`); err != nil {
			slog.Warn("workspace_everyone_roles drop failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create asset import tables if they don't exist (for existing databases)
		if _, err := db.Exec(assetsSchema); err != nil {
			slog.Warn("assets migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create custom_field_indexes table if it doesn't exist (for existing databases)
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS custom_field_indexes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				custom_field_id INTEGER NOT NULL,
				target_table TEXT NOT NULL,
				index_name TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (custom_field_id) REFERENCES custom_field_definitions(id) ON DELETE CASCADE,
				UNIQUE(custom_field_id, target_table)
			)
		`); err != nil {
			slog.Warn("custom_field_indexes migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Add max_custom_field_indexes_per_table system setting if it doesn't exist
		var settingCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM system_settings WHERE key = 'max_custom_field_indexes_per_table'`).Scan(&settingCount); err == nil && settingCount == 0 {
			if _, err := db.Exec(`INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('max_custom_field_indexes_per_table', '20', 'integer', 'Maximum number of custom field indexes per table', 'performance')`); err != nil {
				slog.Warn("max_custom_field_indexes_per_table setting migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add ai_chat_enabled system setting if it doesn't exist
		var aiChatCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM system_settings WHERE key = 'ai_chat_enabled'`).Scan(&aiChatCount); err == nil && aiChatCount == 0 {
			if _, err := db.Exec(`INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('ai_chat_enabled', 'true', 'boolean', 'Enable AI chat functionality', 'modules')`); err != nil {
				slog.Warn("ai_chat_enabled setting migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add ai_feature_config system setting if it doesn't exist
		var aiFeatureConfigCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM system_settings WHERE key = 'ai_feature_config'`).Scan(&aiFeatureConfigCount); err == nil && aiFeatureConfigCount == 0 {
			// Migrate from ai_chat_enabled: if it was false, mark ai_chat as disabled
			defaultCfg := `{}`
			var aiChatVal string
			if err := db.QueryRow(`SELECT value FROM system_settings WHERE key = 'ai_chat_enabled'`).Scan(&aiChatVal); err == nil && strings.EqualFold(aiChatVal, "false") {
				defaultCfg = `{"ai_chat":{"mode":"disabled","connection_id":0}}`
			}
			if _, err := db.Exec(`INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('ai_feature_config', ?, 'json', 'Per-feature AI LLM configuration', 'ai')`, defaultCfg); err != nil {
				slog.Warn("ai_feature_config setting migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Create daily_briefings table if it doesn't exist (for existing databases)
		if _, err := db.Exec(dailyBriefingsSchema); err != nil {
			slog.Warn("daily_briefings migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create asset_actions tables if they don't exist (for existing databases)
		if _, err := db.Exec(assetActionsSchema); err != nil {
			slog.Warn("asset_actions migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Add allowed_entity_types column to link_types
		var aetColCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('link_types') WHERE name='allowed_entity_types'").Scan(&aetColCount); err == nil && aetColCount == 0 {
			if _, err := db.Exec("ALTER TABLE link_types ADD COLUMN allowed_entity_types TEXT DEFAULT NULL"); err != nil {
				slog.Warn("link_types allowed_entity_types migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}
		// Seed allowed_entity_types for the "Tests" system link type
		if _, err := db.Exec(`UPDATE link_types SET allowed_entity_types = '["item","test_case"]' WHERE name = 'Tests' AND is_system = true AND allowed_entity_types IS NULL`); err != nil {
			slog.Warn("link_types Tests allowed_entity_types seed failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Add custom_field_id column to item_links (for linking custom field type)
		var cfColCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('item_links') WHERE name='custom_field_id'").Scan(&cfColCount); err == nil && cfColCount == 0 {
			if _, err := db.Exec("ALTER TABLE item_links ADD COLUMN custom_field_id INTEGER REFERENCES custom_field_definitions(id) ON DELETE CASCADE"); err != nil {
				slog.Warn("item_links custom_field_id migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
			if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_item_links_custom_field ON item_links(custom_field_id)"); err != nil {
				slog.Warn("item_links custom_field_id index creation failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Create user_invitations table if it doesn't exist (for existing databases)
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS user_invitations (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				token TEXT UNIQUE NOT NULL,
				expires_at DATETIME NOT NULL,
				used_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_user_invitations_token ON user_invitations(token);
			CREATE INDEX IF NOT EXISTS idx_user_invitations_user_id ON user_invitations(user_id);
		`); err != nil {
			slog.Warn("user_invitations migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create teams tables if they don't exist (for existing databases)
		if _, err := db.Exec(teamsSchema); err != nil {
			slog.Warn("teams migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create condition_sets tables if they don't exist (for existing databases)
		if _, err := db.Exec(conditionSetsSchema); err != nil {
			slog.Warn("condition_sets migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create integration tables if they don't exist (for existing databases)
		if _, err := db.Exec(integrationsSchema); err != nil {
			slog.Warn("integrations migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Add error_message to conditions
		var condErrMsgCol int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conditions') WHERE name='error_message'").Scan(&condErrMsgCol); err == nil && condErrMsgCol == 0 {
			if _, err := db.Exec("ALTER TABLE conditions ADD COLUMN error_message TEXT"); err != nil {
				slog.Warn("conditions error_message migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add mode to conditions
		var condModeCol int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conditions') WHERE name='mode'").Scan(&condModeCol); err == nil && condModeCol == 0 {
			if _, err := db.Exec("ALTER TABLE conditions ADD COLUMN mode TEXT NOT NULL DEFAULT 'condition'"); err != nil {
				slog.Warn("conditions mode migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add condition_set_id to configuration_sets
		var csCondSetCol int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('configuration_sets') WHERE name='condition_set_id'").Scan(&csCondSetCol); err == nil && csCondSetCol == 0 {
			if _, err := db.Exec("ALTER TABLE configuration_sets ADD COLUMN condition_set_id INTEGER REFERENCES condition_sets(id) ON DELETE SET NULL"); err != nil {
				slog.Warn("configuration_sets condition_set_id migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add condition_set_id to configuration_set_item_types
		var csitCondSetCol int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('configuration_set_item_types') WHERE name='condition_set_id'").Scan(&csitCondSetCol); err == nil && csitCondSetCol == 0 {
			if _, err := db.Exec("ALTER TABLE configuration_set_item_types ADD COLUMN condition_set_id INTEGER REFERENCES condition_sets(id) ON DELETE SET NULL"); err != nil {
				slog.Warn("configuration_set_item_types condition_set_id migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add teams.manage permission if it doesn't exist
		if _, err := db.Exec(`INSERT OR IGNORE INTO permissions (permission_key, permission_name, description, scope, is_system) VALUES ('teams.manage', 'Manage Teams', 'Can create, edit, and delete teams', 'global', 0)`); err != nil {
			slog.Warn("teams.manage permission migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create actions tables if they don't exist (for existing databases)
		if _, err := db.Exec(actionsSchema); err != nil {
			slog.Warn("actions migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create issue_sync tables if they don't exist (for existing databases)
		if _, err := db.Exec(scmSchema); err != nil {
			slog.Warn("scm migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Add public_board.manage permission if it doesn't exist
		if _, err := db.Exec(`INSERT OR IGNORE INTO permissions (permission_key, permission_name, description, scope, is_system) VALUES ('public_board.manage', 'Manage Public Boards', 'Can make collections public and configure public board sharing', 'global', 0)`); err != nil {
			slog.Warn("public_board.manage permission migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Add public_slug column to collections
		var slugColCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('collections') WHERE name='public_slug'").Scan(&slugColCount); err == nil && slugColCount == 0 {
			if _, err := db.Exec("ALTER TABLE collections ADD COLUMN public_slug TEXT"); err != nil {
				slog.Warn("public_slug migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
			if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_collections_public_slug ON collections(public_slug)"); err != nil {
				slog.Warn("public_slug index migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add filter_state column to collections (persists visual builder state)
		var filterStateColCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('collections') WHERE name='filter_state'").Scan(&filterStateColCount); err == nil && filterStateColCount == 0 {
			if _, err := db.Exec("ALTER TABLE collections ADD COLUMN filter_state TEXT"); err != nil {
				slog.Warn("filter_state migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add story_points column to items
		var spColCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='story_points'").Scan(&spColCount); err == nil && spColCount == 0 {
			if _, err := db.Exec("ALTER TABLE items ADD COLUMN story_points REAL"); err != nil {
				slog.Warn("story_points migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add due_date field to default screen if missing
		var dueDateFieldCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM screen_fields WHERE screen_id = 1 AND field_identifier = 'due_date'`).Scan(&dueDateFieldCount); err == nil && dueDateFieldCount == 0 {
			if _, err := db.Exec(`INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width) VALUES (1, 'system', 'due_date', 6, false, 'half')`); err != nil {
				slog.Warn("due_date screen field migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add labels field to default screen if missing. Drives ItemDetailSidebar
		// visibility via shouldShowSystemField('labels'); is_required=false so
		// WorkItemForm does not render a labels input on create/edit.
		var labelsFieldCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM screen_fields WHERE screen_id = 1 AND field_identifier = 'labels'`).Scan(&labelsFieldCount); err == nil && labelsFieldCount == 0 {
			if _, err := db.Exec(`INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width) VALUES (1, 'system', 'labels', 11, false, 'full')`); err != nil {
				slog.Warn("labels screen field migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Add config column to request_types (for form channel per-form settings)
		var rtConfigCol int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('request_types') WHERE name='config'").Scan(&rtConfigCol); err == nil && rtConfigCol == 0 {
			if _, err := db.Exec("ALTER TABLE request_types ADD COLUMN config TEXT DEFAULT NULL"); err != nil {
				slog.Warn("request_types config migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Asset report form-mode migration: add run_mode, item_type_id, workspace_id, config columns
		assetReportColumnMigrations := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('asset_reports') WHERE name='run_mode'",
				alter: "ALTER TABLE asset_reports ADD COLUMN run_mode TEXT NOT NULL DEFAULT 'direct'",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('asset_reports') WHERE name='item_type_id'",
				alter: "ALTER TABLE asset_reports ADD COLUMN item_type_id INTEGER DEFAULT NULL REFERENCES item_types(id) ON DELETE SET NULL",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('asset_reports') WHERE name='workspace_id'",
				alter: "ALTER TABLE asset_reports ADD COLUMN workspace_id INTEGER DEFAULT NULL REFERENCES workspaces(id) ON DELETE SET NULL",
			},
			{
				check: "SELECT COUNT(*) FROM pragma_table_info('asset_reports') WHERE name='config'",
				alter: "ALTER TABLE asset_reports ADD COLUMN config TEXT DEFAULT NULL",
			},
		}
		for _, m := range assetReportColumnMigrations {
			var cnt int
			if err := db.QueryRow(m.check).Scan(&cnt); err == nil && cnt == 0 {
				if _, err := db.Exec(m.alter); err != nil {
					slog.Warn("asset_reports migration failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		// Create asset_report_fields table if it doesn't exist (for existing databases)
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS asset_report_fields (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				asset_report_id INTEGER NOT NULL,
				field_identifier TEXT NOT NULL,
				field_type TEXT NOT NULL,
				is_required BOOLEAN DEFAULT false,
				display_order INTEGER DEFAULT 0,
				options TEXT,
				display_name TEXT,
				description TEXT,
				step_number INTEGER DEFAULT 1,
				virtual_field_type TEXT,
				virtual_field_options TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (asset_report_id) REFERENCES asset_reports(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_asset_report_fields_asset_report_id ON asset_report_fields(asset_report_id);
		`); err != nil {
			slog.Warn("asset_report_fields migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Migrate: create default configuration set for existing databases that have none
		var csCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM configuration_sets").Scan(&csCount); err == nil && csCount == 0 {
			if err := db.migrateDefaultConfigurationSet(); err != nil {
				slog.Warn("default configuration set migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Strip legacy base64 padding from SSH public-key fingerprints so they
		// match the OpenSSH format (ssh-keygen -lf / gossh.FingerprintSHA256).
		if _, err := db.Exec(`UPDATE user_credentials SET public_key_fingerprint = rtrim(public_key_fingerprint, '=') WHERE public_key_fingerprint LIKE '%=';`); err != nil {
			slog.Warn("ssh fingerprint padding migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create api_tokens table if missing. Older deployments were
		// initialized from a schema snapshot that predates this table,
		// and the only prior creation path is the fresh-install schema —
		// leaving existing DBs without it and every token INSERT failing
		// with 500. cli_auth_codes (below) also FK-references this, so
		// this must run first.
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS api_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				token_hash TEXT NOT NULL UNIQUE,
				token_prefix TEXT NOT NULL,
				permissions TEXT DEFAULT '["read"]',
				expires_at DATETIME NULL,
				last_used_at DATETIME NULL,
				is_temporary BOOLEAN DEFAULT false,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
			CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
			CREATE INDEX IF NOT EXISTS idx_api_tokens_expires_at ON api_tokens(expires_at);
		`); err != nil {
			slog.Warn("api_tokens migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create scm_processed_commits table if missing. Older deployments
		// predate smart-commit support; the sync loop writes a row per commit
		// it has already applied actions for, guaranteeing idempotency across
		// re-syncs.
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS scm_processed_commits (
				commit_sha              TEXT NOT NULL,
				workspace_repository_id INTEGER NOT NULL,
				processed_at            DATETIME DEFAULT CURRENT_TIMESTAMP,
				actions_applied         INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (commit_sha, workspace_repository_id),
				FOREIGN KEY (workspace_repository_id) REFERENCES workspace_repositories(id) ON DELETE CASCADE
			);
		`); err != nil {
			slog.Warn("scm_processed_commits migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		// Create cli_auth_codes table for the `ws init` onboarding flow.
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS cli_auth_codes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				code TEXT NOT NULL UNIQUE,
				state TEXT NOT NULL,
				callback_url TEXT NOT NULL,
				hostname TEXT NOT NULL,
				agent_name TEXT NOT NULL,
				requested_scopes TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending',
				approved_by_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
				agent_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
				token_id INTEGER REFERENCES api_tokens(id) ON DELETE SET NULL,
				token_plaintext TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME NOT NULL,
				consumed_at DATETIME
			);
			CREATE INDEX IF NOT EXISTS idx_cli_auth_codes_code ON cli_auth_codes(code);
			CREATE INDEX IF NOT EXISTS idx_cli_auth_codes_expires_at ON cli_auth_codes(expires_at);
		`); err != nil {
			slog.Warn("cli_auth_codes migration failed", slog.String("component", "database"), slog.Any("error", err))
		}

		return nil
	}

	// Database needs full initialization
	schema := coreSchema + itemsSchema + requestTypeSchema + usersSchema + testsSchema + workspaceSchema + configWorkflowsSchema + timeTrackingSchema + channelsSchema + portalSchema + portalAuthSchema + milestonesSchema + iterationsSchema + contentSchema + mentionsSchema + notificationsSchema + permissionsSchema + systemSchema + userPreferencesSchema + webauthnSchema + ssoSchema + scmSchema + assetsSchema + recurringTasksSchema + jiraImportSchema + actionsSchema + emailSchema + assetReportsSchema + labelsSchema + llmSchema + ldapSchema + assetActionsSchema + dailyBriefingsSchema + teamsSchema + conditionSetsSchema + integrationsSchema

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Initialize default data for new installations
	if err := db.initializeDefaultData(); err != nil {
		return fmt.Errorf("failed to initialize default data: %w", err)
	}

	return nil
}

// initializeDefaultData creates the default data for a fresh installation
func (db *DB) initializeDefaultData() error {
	// Check if we already have default data by looking for status categories
	var categoryCount int
	err := db.QueryRow("SELECT COUNT(*) FROM status_categories").Scan(&categoryCount)
	if err != nil {
		return fmt.Errorf("failed to check existing status categories: %w", err)
	}

	// If we already have status categories, assume default data exists
	if categoryCount > 0 {
		return nil
	}

	// Begin transaction for atomic initialization
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Create default status categories
	categories := []struct {
		name        string
		color       string
		description string
		isDefault   bool
		isCompleted bool
	}{
		{"To Do", "#d1d5db", "Work that hasn't been started", false, false},
		{"In Progress", "#3b82f6", "Work that is actively being done", true, false},
		{"Done", "#22c55e", "Work that has been completed", false, true},
	}

	categoryIDs := make(map[string]int64)
	for _, cat := range categories {
		var result sql.Result
		result, err = tx.Exec(
			"INSERT INTO status_categories (name, color, description, is_default, is_completed) VALUES (?, ?, ?, ?, ?)",
			cat.name, cat.color, cat.description, cat.isDefault, cat.isCompleted,
		)
		if err != nil {
			return fmt.Errorf("failed to create status category %s: %w", cat.name, err)
		}
		id, _ := result.LastInsertId()
		categoryIDs[cat.name] = id
	}

	// 2. Create default statuses
	statuses := []struct {
		name        string
		description string
		category    string
		isDefault   bool
	}{
		{"Open", "New work item, not yet started", "To Do", true},
		{"In Progress", "Currently being worked on", "In Progress", false},
		{"Done", "Work has been completed", "Done", false},
	}

	statusIDs := make(map[string]int64)
	for _, status := range statuses {
		categoryID := categoryIDs[status.category]
		var result sql.Result
		result, err = tx.Exec(
			"INSERT INTO statuses (name, description, category_id, is_default) VALUES (?, ?, ?, ?)",
			status.name, status.description, categoryID, status.isDefault,
		)
		if err != nil {
			return fmt.Errorf("failed to create status %s: %w", status.name, err)
		}
		id, _ := result.LastInsertId()
		statusIDs[status.name] = id
	}

	// 3. Create default workflow
	result, err := tx.Exec(
		"INSERT INTO workflows (name, description, is_default) VALUES (?, ?, ?)",
		"Default Workflow", "Basic workflow for getting work done", true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default workflow: %w", err)
	}
	workflowID, _ := result.LastInsertId()

	// 4. Create workflow transitions (simplified 3-status workflow)
	transitions := []struct {
		from string // empty string means initial status
		to   string
	}{
		{"", "Open"}, // Initial transition
		{"Open", "In Progress"},
		{"Open", "Done"}, // Direct completion from Open
		{"In Progress", "Done"},
	}

	for i, transition := range transitions {
		var fromStatusID *int64
		if transition.from != "" {
			id := statusIDs[transition.from]
			fromStatusID = &id
		}
		toStatusID := statusIDs[transition.to]

		_, err = tx.Exec(
			"INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order) VALUES (?, ?, ?, ?)",
			workflowID, fromStatusID, toStatusID, i,
		)
		if err != nil {
			return fmt.Errorf("failed to create transition from %s to %s: %w", transition.from, transition.to, err)
		}
	}

	// 5. Create default screen with basic fields
	result, err = tx.Exec(
		"INSERT INTO screens (name, description) VALUES (?, ?)",
		"Default Screen", "Default screen with essential work item fields",
	)
	if err != nil {
		return fmt.Errorf("failed to create default screen: %w", err)
	}
	screenID, _ := result.LastInsertId()

	// 6. Add default fields to the screen
	screenFields := []struct {
		fieldType       string
		fieldIdentifier string
		displayOrder    int
		isRequired      bool
		fieldWidth      string
	}{
		{"system", "title", 1, true, "full"},
		{"system", "description", 2, false, "full"},
		{"system", "status", 3, true, "half"},
		{"system", "priority", 4, false, "half"},
		{"system", "assignee", 5, false, "half"},
		{"system", "due_date", 6, false, "half"},
		{"system", "milestone", 7, false, "half"},
		{"system", "iteration", 8, false, "half"},
		{"system", "start_date", 9, false, "half"},
		{"system", "end_date", 10, false, "half"},
		{"system", "labels", 11, false, "full"},
	}

	for _, field := range screenFields {
		_, err = tx.Exec(
			"INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width) VALUES (?, ?, ?, ?, ?, ?)",
			screenID, field.fieldType, field.fieldIdentifier, field.displayOrder, field.isRequired, field.fieldWidth,
		)
		if err != nil {
			return fmt.Errorf("failed to add field %s to default screen: %w", field.fieldIdentifier, err)
		}
	}

	// 7. Create default configuration set
	configResult, err := tx.Exec(
		"INSERT INTO configuration_sets (name, description, workflow_id, is_default) VALUES (?, ?, ?, ?)",
		"Default Configuration", "Default configuration set with basic workflow and screen", workflowID, true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default configuration set: %w", err)
	}
	configSetID, _ := configResult.LastInsertId()

	// 8. Assign default screen to configuration set for all contexts
	contexts := []string{"create", "edit", "view"}
	for _, context := range contexts {
		_, err = tx.Exec(
			"INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context) VALUES (?, ?, ?)",
			configSetID, screenID, context,
		)
		if err != nil {
			return fmt.Errorf("failed to assign screen to configuration set for %s context: %w", context, err)
		}
	}

	// 9. Create default link types
	linkTypes := []struct {
		name               string
		description        string
		forwardLabel       string
		reverseLabel       string
		color              string
		isSystem           bool
		allowedEntityTypes *string // JSON array or nil
	}{
		{"Tests", "Test case tests work item", "tests", "tested by", "#10b981", true, strPtr(`["item","test_case"]`)},
		{"Implements", "Work item implements another work item", "implements", "implemented by", "#3b82f6", true, nil},
		{"Depends On", "Work item depends on another work item", "depends on", "blocks", "#f59e0b", true, nil},
		{"Relates To", "General bidirectional relationship", "relates to", "relates to", "#6b7280", true, nil},
		{"Links To", "General directional link", "links to", "linked from", "#64748b", true, nil},
		{"Duplicates", "Work item is a duplicate of another", "duplicates", "duplicated by", "#ef4444", true, nil},
		{"Child Of", "Alternative hierarchy relationship", "child of", "parent of", "#8b5cf6", true, nil},
	}

	for _, linkType := range linkTypes {
		_, err = tx.Exec(
			"INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system, allowed_entity_types) VALUES (?, ?, ?, ?, ?, ?, ?)",
			linkType.name, linkType.description, linkType.forwardLabel, linkType.reverseLabel, linkType.color, linkType.isSystem, linkType.allowedEntityTypes,
		)
		if err != nil {
			return fmt.Errorf("failed to create link type %s: %w", linkType.name, err)
		}
	}

	// 11. Create default system settings
	systemSettings := []struct {
		key         string
		value       string
		valueType   string
		description string
		category    string
	}{
		{"time_tracking_enabled", "true", "boolean", "Enable time tracking functionality", "modules"},
		{"test_management_enabled", "true", "boolean", "Enable test management functionality", "modules"},
		{"ai_chat_enabled", "true", "boolean", "Enable AI chat functionality", "modules"},
		{"ai_feature_config", "{}", "json", "Per-feature AI LLM configuration", "ai"},
		{"setup_completed", "false", "boolean", "Whether initial setup has been completed", "setup"},
		{"admin_user_created", "false", "boolean", "Whether admin user has been created", "setup"},
		{"calendar_feed_enabled", "true", "boolean", "Allow users to generate ICS calendar feed URLs", "security"},
		{"plugin_cli_exec_enabled", "false", "boolean", "Allow plugins to execute CLI commands", "security"},
		{"max_custom_field_indexes_per_table", "20", "integer", "Maximum number of custom field indexes per table", "performance"},
	}

	for _, setting := range systemSettings {
		_, err = tx.Exec(
			"INSERT INTO system_settings (key, value, value_type, description, category) VALUES (?, ?, ?, ?, ?)",
			setting.key, setting.value, setting.valueType, setting.description, setting.category,
		)
		if err != nil {
			return fmt.Errorf("failed to create system setting %s: %w", setting.key, err)
		}
	}

	// 9. Create default hierarchy levels
	hierarchyLevels := []struct {
		level       int
		name        string
		description string
	}{
		{0, "Initiative", "High-level strategic work spanning multiple epics"},
		{1, "Epic", "Large work item that can be broken down into stories"},
		{2, "Story", "User story or feature that delivers value"},
		{3, "Task", "Individual work item or technical task"},
		{4, "Sub-task", "Small piece of work within a larger task"},
	}

	for _, hl := range hierarchyLevels {
		_, err = tx.Exec(
			"INSERT INTO hierarchy_levels (level, name, description) VALUES (?, ?, ?)",
			hl.level, hl.name, hl.description,
		)
		if err != nil {
			return fmt.Errorf("failed to create hierarchy level %s: %w", hl.name, err)
		}
	}

	// 10. Create default item types with icons and colors
	defaultItemTypes := []struct {
		name           string
		description    string
		icon           string
		color          string
		hierarchyLevel int
		sortOrder      int
	}{
		{"Initiative", "Strategic initiative spanning multiple teams", "Target", "#7c3aed", 0, 1},
		{"Epic", "Large feature or capability", "Zap", "#2563eb", 1, 1},
		{"Story", "User story delivering value to end users", "BookOpen", "#059669", 2, 1},
		{"Task", "Development or operational task", "CheckSquare", "#dc2626", 3, 1},
		{"Bug", "Software defect that needs fixing", "Bug", "#ea580c", 3, 2},
		{"Sub-task", "Small work item within a larger task", "Minus", "#6b7280", 4, 1},
	}

	for _, itemType := range defaultItemTypes {
		_, err = tx.Exec(
			"INSERT INTO item_types (configuration_set_id, name, description, icon, color, hierarchy_level, sort_order, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			configSetID, itemType.name, itemType.description, itemType.icon, itemType.color, itemType.hierarchyLevel, itemType.sortOrder, true,
		)
		if err != nil {
			return fmt.Errorf("failed to create default item type %s: %w", itemType.name, err)
		}
	}

	// 10b. Bind selected item types to the default configuration set (excluding Initiative for simplified setup)
	itemTypesToBind := []string{"Epic", "Story", "Task", "Bug", "Sub-task"}
	for _, typeName := range itemTypesToBind {
		var itemTypeID int64
		err = tx.QueryRow("SELECT id FROM item_types WHERE name = ?", typeName).Scan(&itemTypeID)
		if err != nil {
			return fmt.Errorf("failed to get item type ID for %s: %w", typeName, err)
		}
		_, err = tx.Exec(
			"INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id) VALUES (?, ?)",
			configSetID, itemTypeID,
		)
		if err != nil {
			return fmt.Errorf("failed to bind item type %s to default configuration set: %w", typeName, err)
		}
	}

	// 11. Create default Notification Mail channel
	defaultChannelConfig := `{
		"smtp_host": "",
		"smtp_port": 587,
		"smtp_username": "",
		"smtp_password": "",
		"smtp_from_email": "",
		"smtp_from_name": "Windshift",
		"smtp_encryption": "tls"
	}`

	_, err = tx.Exec(
		"INSERT INTO channels (name, type, direction, description, status, is_default, config) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"Notification Mail", "smtp", "outbound", "Default SMTP channel for sending notification emails", "pending", true, defaultChannelConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification mail channel: %w", err)
	}

	// 12. Create default themes with dual light/dark nav colors
	defaultThemes := []struct {
		name                    string
		description             string
		isDefault               bool
		isActive                bool
		navBackgroundColorLight string
		navTextColorLight       string
		navBackgroundColorDark  string
		navTextColorDark        string
	}{
		{"Default", "Clean theme with standard navigation colors", true, true, "#ffffff", "#374151", "#1f2937", "#f3f4f6"},
		{"Ocean", "Professional blue-tinted navigation theme", false, false, "#f0f9ff", "#0c4a6e", "#0c4a6e", "#e0f2fe"},
		{"Forest", "Nature-inspired green navigation theme", false, false, "#f0fdf4", "#14532d", "#14532d", "#dcfce7"},
	}

	for _, theme := range defaultThemes {
		_, err = tx.Exec(
			"INSERT INTO themes (name, description, is_default, is_active, nav_background_color_light, nav_text_color_light, nav_background_color_dark, nav_text_color_dark) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			theme.name, theme.description, theme.isDefault, theme.isActive, theme.navBackgroundColorLight, theme.navTextColorLight, theme.navBackgroundColorDark, theme.navTextColorDark,
		)
		if err != nil {
			return fmt.Errorf("failed to create theme %s: %w", theme.name, err)
		}
	}

	// 13. Create default priorities if none exist
	var priorityCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM priorities").Scan(&priorityCount)
	if err != nil {
		return fmt.Errorf("failed to check existing priorities: %w", err)
	}

	if priorityCount == 0 {
		_, err = tx.Exec(defaultDataSQL)
		if err != nil {
			return fmt.Errorf("failed to create default priorities: %w", err)
		}
	}

	// 13b. Link all priorities to the default configuration set
	priorityRows, err := tx.Query("SELECT id FROM priorities")
	if err != nil {
		return fmt.Errorf("failed to query priorities: %w", err)
	}
	defer func() { _ = priorityRows.Close() }()

	for priorityRows.Next() {
		var priorityID int64
		if err = priorityRows.Scan(&priorityID); err != nil {
			return fmt.Errorf("failed to scan priority: %w", err)
		}
		_, err = tx.Exec(
			"INSERT OR IGNORE INTO configuration_set_priorities (configuration_set_id, priority_id) VALUES (?, ?)",
			configSetID, priorityID,
		)
		if err != nil {
			return fmt.Errorf("failed to link priority to default config set: %w", err)
		}
	}

	// 14. Create default notification settings
	notificationSettingResult, err := tx.Exec(
		"INSERT INTO notification_settings (name, description, is_active, created_by) VALUES (?, ?, ?, ?)",
		"Default Notifications", "Standard notification rules for work item updates", true, nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification setting: %w", err)
	}
	notificationSettingID, _ := notificationSettingResult.LastInsertId()

	// 15. Create default notification event rules
	defaultEventRules := []struct {
		eventType             string
		notifyAssignee        bool
		notifyCreator         bool
		notifyWatchers        bool
		notifyWorkspaceAdmins bool
	}{
		// Item assignment - notify the assignee
		{"item.assigned", true, false, false, false},
		// Comments - notify assignee and creator
		{"comment.created", true, true, false, false},
		// Status changes - notify assignee and creator
		{"status.changed", true, true, false, false},
	}

	for _, rule := range defaultEventRules {
		_, err = tx.Exec(
			`INSERT INTO notification_event_rules
			 (notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
			  notify_watchers, notify_workspace_admins)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			notificationSettingID, rule.eventType, true, rule.notifyAssignee,
			rule.notifyCreator, rule.notifyWatchers, rule.notifyWorkspaceAdmins,
		)
		if err != nil {
			return fmt.Errorf("failed to create notification rule for %s: %w", rule.eventType, err)
		}
	}

	// 16. Link notification setting to default configuration set
	_, err = tx.Exec(
		"INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id) VALUES (?, ?)",
		configSetID, notificationSettingID,
	)
	if err != nil {
		return fmt.Errorf("failed to link notification setting to configuration set: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit default data: %w", err)
	}

	return nil
}

// migrateDefaultConfigurationSet creates a default configuration set for existing databases
// that were set up before configuration sets were introduced.
func (db *DB) migrateDefaultConfigurationSet() error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Find a default workflow to link to
	var workflowID int64
	err = tx.QueryRow("SELECT id FROM workflows WHERE is_default = 1 LIMIT 1").Scan(&workflowID)
	if err != nil {
		// No workflow exists, nothing to link to
		slog.Info("no default workflow found, skipping configuration set migration", slog.String("component", "database"))
		return nil
	}

	// Create the default configuration set
	result, err := tx.Exec(
		"INSERT INTO configuration_sets (name, description, workflow_id, is_default) VALUES (?, ?, ?, ?)",
		"Default Configuration", "Default configuration set with basic workflow and screen", workflowID, true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default configuration set: %w", err)
	}
	configSetID, _ := result.LastInsertId()

	// Find an existing screen or create one
	var screenID int64
	err = tx.QueryRow("SELECT id FROM screens LIMIT 1").Scan(&screenID)
	if err != nil {
		// No screen exists, create one with default fields
		screenResult, err := tx.Exec(
			"INSERT INTO screens (name, description) VALUES (?, ?)",
			"Default Screen", "Default screen with essential work item fields",
		)
		if err != nil {
			return fmt.Errorf("failed to create default screen: %w", err)
		}
		screenID, _ = screenResult.LastInsertId()

		screenFields := []struct {
			fieldType       string
			fieldIdentifier string
			displayOrder    int
			isRequired      bool
			fieldWidth      string
		}{
			{"system", "title", 1, true, "full"},
			{"system", "description", 2, false, "full"},
			{"system", "status", 3, true, "half"},
			{"system", "priority", 4, false, "half"},
			{"system", "assignee", 5, false, "half"},
			{"system", "due_date", 6, false, "half"},
			{"system", "milestone", 7, false, "half"},
			{"system", "start_date", 8, false, "half"},
			{"system", "end_date", 9, false, "half"},
			{"system", "labels", 10, false, "full"},
		}

		for _, field := range screenFields {
			_, err = tx.Exec(
				"INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width) VALUES (?, ?, ?, ?, ?, ?)",
				screenID, field.fieldType, field.fieldIdentifier, field.displayOrder, field.isRequired, field.fieldWidth,
			)
			if err != nil {
				return fmt.Errorf("failed to add field %s to default screen: %w", field.fieldIdentifier, err)
			}
		}
	}

	// Assign screen to config set for create/edit/view contexts
	contexts := []string{"create", "edit", "view"}
	for _, ctx := range contexts {
		_, err = tx.Exec(
			"INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context) VALUES (?, ?, ?)",
			configSetID, screenID, ctx,
		)
		if err != nil {
			return fmt.Errorf("failed to assign screen to configuration set for %s context: %w", ctx, err)
		}
	}

	// Bind all existing item types to the config set
	_, err = tx.Exec(
		"INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id) SELECT ?, id FROM item_types",
		configSetID,
	)
	if err != nil {
		return fmt.Errorf("failed to bind item types to default configuration set: %w", err)
	}

	// Assign all existing priorities to the config set
	_, err = tx.Exec(
		"INSERT INTO configuration_set_priorities (configuration_set_id, priority_id) SELECT ?, id FROM priorities",
		configSetID,
	)
	if err != nil {
		return fmt.Errorf("failed to assign priorities to default configuration set: %w", err)
	}

	// Assign all workspaces that don't already have a config set
	_, err = tx.Exec(
		"INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id) SELECT id, ? FROM workspaces WHERE id NOT IN (SELECT workspace_id FROM workspace_configuration_sets)",
		configSetID,
	)
	if err != nil {
		return fmt.Errorf("failed to assign workspaces to default configuration set: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit default configuration set migration: %w", err)
	}

	slog.Info("created default configuration set for existing database", slog.String("component", "database"), slog.Int64("config_set_id", configSetID))
	return nil
}

// MigrateSelectFieldOptions migrates legacy string-array options to ID-based format (SQLite)
// Note: This is a no-op stub; the real implementation is on SQLiteDB which satisfies the Database interface.
func (db *DB) MigrateSelectFieldOptions() error {
	return nil
}

// IsUniqueConstraintError checks if the error is a unique constraint violation (SQLite + PostgreSQL)
func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "duplicate key")
}

// EnsureDefaultNotificationSettings creates default notification settings if they don't exist
// This should be called during application startup to ensure notifications work
func (db *DB) EnsureDefaultNotificationSettings() error {
	// Check if notification settings already exist
	var settingCount int
	err := db.QueryRow("SELECT COUNT(*) FROM notification_settings").Scan(&settingCount)
	if err != nil {
		return fmt.Errorf("failed to check existing notification settings: %w", err)
	}

	if settingCount > 0 {
		// Settings already exist, nothing to do
		return nil
	}

	// Find the default configuration set
	var configSetID int
	err = db.QueryRow("SELECT id FROM configuration_sets WHERE is_default = true LIMIT 1").Scan(&configSetID)
	if err == sql.ErrNoRows {
		slog.Debug("no default configuration set found, skipping notification settings initialization", slog.String("component", "database"))
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to find default configuration set: %w", err)
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Create notification setting
	result, err := tx.Exec(
		"INSERT INTO notification_settings (name, description, is_active, created_by) VALUES (?, ?, ?, ?)",
		"Default Notifications", "Standard notification rules for work item updates", true, nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create notification setting: %w", err)
	}
	notificationSettingID, _ := result.LastInsertId()

	// Create event rules
	eventRules := []struct {
		eventType      string
		notifyAssignee bool
		notifyCreator  bool
	}{
		{"item.assigned", true, false},
		{"comment.created", true, true},
		{"status.changed", true, true},
	}

	for _, rule := range eventRules {
		_, err = tx.Exec(
			`INSERT INTO notification_event_rules
			 (notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
			  notify_watchers, notify_workspace_admins)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			notificationSettingID, rule.eventType, true, rule.notifyAssignee, rule.notifyCreator, false, false,
		)
		if err != nil {
			return fmt.Errorf("failed to create rule for %s: %w", rule.eventType, err)
		}
	}

	// Link to default configuration set
	_, err = tx.Exec(
		"INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id) VALUES (?, ?)",
		configSetID, notificationSettingID,
	)
	if err != nil {
		return fmt.Errorf("failed to link notification setting: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit notification settings: %w", err)
	}

	slog.Debug("created default notification settings", slog.String("component", "database"))
	return nil
}

// NewDatabase creates a new database connection based on the driver and connection string
// Supported drivers: "sqlite3", "postgres"
// If driver is empty, it will be auto-detected from the connection string
func NewDatabase(driver, connectionString string, readConns, writeConns int) (Database, error) {
	// Auto-detect driver if not specified
	if driver == "" {
		if strings.HasPrefix(connectionString, "postgres://") || strings.HasPrefix(connectionString, "postgresql://") {
			driver = "postgres"
		} else {
			driver = "sqlite3"
		}
	}

	switch driver {
	case "sqlite3", "sqlite":
		return NewSQLiteDBWithPoolSizes(connectionString, readConns, writeConns)
	case "postgres", "postgresql":
		// PostgreSQL uses fixed 50 connections, readConns/writeConns params ignored
		return NewPostgresDB(connectionString)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func strPtr(s string) *string { return &s }
