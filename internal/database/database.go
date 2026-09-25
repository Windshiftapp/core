// Package database provides database connection and transaction management.
package database

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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

//go:embed schema/object_translations.sql
var objectTranslationsSchema string

//go:embed schema/time_tracking.sql
var timeTrackingSchema string

//go:embed schema/portal.sql
var portalSchema string

//go:embed schema/portal_auth.sql
var portalAuthSchema string

//go:embed schema/portal_webauthn.sql
var portalWebauthnSchema string

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

//go:embed schema/templates.sql
var templatesSchema string

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

//go:embed schema/incidents.sql
var incidentsSchema string

//go:embed schema/condition_sets.sql
var conditionSetsSchema string

//go:embed schema/approvals.sql
var approvalsSchema string

//go:embed schema/integrations.sql
var integrationsSchema string

//go:embed schema/auth_policy.sql
var authPolicySchema string

//go:embed schema/pages.sql
var pagesSchema string

//go:embed schema/page_labels.sql
var pageLabelsSchema string

//go:embed schema/agents.sql
var agentsSchema string

//go:embed schema/events.sql
var eventsSchema string

//go:embed schema/action_events.sql
var actionEventsSchema string

//go:embed schema/action_event_targets.sql
var actionEventTargetsSchema string

//go:embed schema/asset_action_events.sql
var assetActionEventsSchema string

// DB wraps a sql.DB connection with a dedicated write connection
type DB struct {
	*sql.DB
	writeConn  *sql.DB // Dedicated single connection for writes
	instanceID uint64
}

// NewDB opens SQLite with WAL, required pragmas, and separate read/write pools.
// Use one write connection to preserve SQLite's single-writer invariant.
func NewDB(dataSourceName string, readConns, writeConns int) (*DB, error) {
	// Add SQLite-specific connection parameters for better concurrency handling
	// Check if DSN already has parameters (for shared in-memory test databases)
	separator := "?"
	if strings.Contains(dataSourceName, "?") {
		separator = "&"
	}

	connectionString := dataSourceName +
		separator + "_pragma=busy_timeout(5000)" +
		"&_journal_mode=WAL" +
		"&_pragma=foreign_keys(ON)" +
		"&_txlock=immediate" +
		// Use SQLite-parsable UTC timestamps; startup repairs legacy rows.
		"&_time_format=sqlite" +
		"&_timezone=UTC" +
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

	// Set connection pool settings for SQLite (configured via --max-read-conns)
	readIdle := readConns / 10
	if readIdle < 1 {
		readIdle = 1
	}
	db.SetMaxOpenConns(readConns)
	db.SetMaxIdleConns(readIdle)

	// Create dedicated write connection with only 1 max connection to serialize writes
	writeConn, err := sql.Open("sqlite", connectionString)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to open write connection: %w", err)
	}

	// Write pool sized via --max-write-conns. Default 1 serializes writes;
	// raising it lets WAL handle a small amount of write concurrency.
	writeConn.SetMaxOpenConns(writeConns)
	writeConn.SetMaxIdleConns(writeConns)

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

	return &DB{DB: db, writeConn: writeConn, instanceID: newDatabaseInstanceID()}, nil
}

// InstanceID distinguishes database pools created during this process.
func (db *DB) InstanceID() uint64 {
	return db.instanceID
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

// Initialize creates the SQLite schema and applies the ordered migration catalog.
func (db *DB) Initialize() error {
	return initializeSQLiteDatabase(&SQLiteDB{DB: db})
}

// initializeDefaultData creates the default data for a fresh installation.
func (db *DB) initializeDefaultData() error {
	return seedFreshInstall(db.DB, func(tx *sql.Tx) seedWriter {
		return sqliteSeedWriter{NewSQLiteTx(tx)}
	})
}

// IsUniqueConstraintError checks if the error is a unique constraint violation.
func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
			sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return false
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
		// Postgres has a single pool; sized via readConns. writeConns is
		// SQLite-specific (no separate write pool on Postgres).
		return NewPostgresDB(connectionString, readConns)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func strPtr(s string) *string { return &s }
