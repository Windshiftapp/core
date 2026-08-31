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
	writeConn *sql.DB // Dedicated single connection for writes
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
		separator + "_busy_timeout=5000" +
		"&_journal_mode=WAL" +
		"&_foreign_keys=on" +
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

// Initialize creates the SQLite schema and applies the ordered migration catalog.
func (db *DB) Initialize() error {
	return initializeSQLiteDatabase(&SQLiteDB{DB: db})
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
	categoryIDs := make(map[string]int64)
	for _, cat := range defaultStatusCategories {
		var result sql.Result
		result, err = tx.Exec(
			"INSERT INTO status_categories (builtin_key, name, color, description, is_default, is_completed) VALUES (?, ?, ?, ?, ?, ?)",
			cat.builtinKey, cat.name, cat.color, cat.description, cat.isDefault, cat.isCompleted,
		)
		if err != nil {
			return fmt.Errorf("failed to create status category %s: %w", cat.name, err)
		}
		id, _ := result.LastInsertId()
		categoryIDs[cat.name] = id
	}

	// 2. Create default statuses
	statusIDs := make(map[string]int64)
	for _, status := range defaultStatuses {
		categoryID := categoryIDs[status.category]
		var result sql.Result
		result, err = tx.Exec(
			"INSERT INTO statuses (builtin_key, name, description, category_id, is_default) VALUES (?, ?, ?, ?, ?)",
			status.builtinKey, status.name, status.description, categoryID, status.isDefault,
		)
		if err != nil {
			return fmt.Errorf("failed to create status %s: %w", status.name, err)
		}
		id, _ := result.LastInsertId()
		statusIDs[status.name] = id
	}

	// 3. Create default workflow
	result, err := tx.Exec(
		"INSERT INTO workflows (builtin_key, name, description, is_default) VALUES (?, ?, ?, ?)",
		"default", "Default Workflow", "Basic workflow for getting work done", true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default workflow: %w", err)
	}
	workflowID, _ := result.LastInsertId()

	// 4. Create workflow transitions (simplified 3-status workflow)
	for i, transition := range defaultTransitions {
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
		"INSERT INTO screens (builtin_key, name, description) VALUES (?, ?, ?)",
		"default", "Default Screen", "Default screen with essential work item fields",
	)
	if err != nil {
		return fmt.Errorf("failed to create default screen: %w", err)
	}
	screenID, _ := result.LastInsertId()

	// 6. Add default fields to the screen
	for _, field := range defaultScreenFields {
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
		"INSERT INTO configuration_sets (builtin_key, name, description, workflow_id, is_default) VALUES (?, ?, ?, ?, ?)",
		"default", "Default Configuration", "Default configuration set with basic workflow and screen", workflowID, true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default configuration set: %w", err)
	}
	configSetID, _ := configResult.LastInsertId()

	// 8. Assign default screen to configuration set for all contexts
	for _, context := range defaultScreenContexts {
		_, err = tx.Exec(
			"INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context) VALUES (?, ?, ?)",
			configSetID, screenID, context,
		)
		if err != nil {
			return fmt.Errorf("failed to assign screen to configuration set for %s context: %w", context, err)
		}
	}

	// 9. Create default link types
	for _, linkType := range defaultLinkTypes {
		_, err = tx.Exec(
			"INSERT INTO link_types (builtin_key, name, description, forward_label, reverse_label, color, is_system, allowed_entity_types) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			linkType.builtinKey, linkType.name, linkType.description, linkType.forwardLabel, linkType.reverseLabel, linkType.color, linkType.isSystem, linkType.allowedEntityTypes,
		)
		if err != nil {
			return fmt.Errorf("failed to create link type %s: %w", linkType.name, err)
		}
	}

	// 11. Create default system settings
	for _, setting := range defaultSystemSettings {
		_, err = tx.Exec(
			"INSERT INTO system_settings (key, value, value_type, description, category) VALUES (?, ?, ?, ?, ?)",
			setting.key, setting.value, setting.valueType, setting.description, setting.category,
		)
		if err != nil {
			return fmt.Errorf("failed to create system setting %s: %w", setting.key, err)
		}
	}

	// 9. Create default hierarchy levels
	for _, hl := range defaultHierarchyLevels {
		_, err = tx.Exec(
			"INSERT INTO hierarchy_levels (builtin_key, level, name, description) VALUES (?, ?, ?, ?)",
			hl.builtinKey, hl.level, hl.name, hl.description,
		)
		if err != nil {
			return fmt.Errorf("failed to create hierarchy level %s: %w", hl.name, err)
		}
	}

	// 10. Create default item types with icons and colors
	for _, itemType := range defaultItemTypes {
		_, err = tx.Exec(
			"INSERT INTO item_types (builtin_key, configuration_set_id, name, description, icon, color, hierarchy_level, sort_order, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			itemType.builtinKey, configSetID, itemType.name, itemType.description, itemType.icon, itemType.color, itemType.hierarchyLevel, itemType.sortOrder, true,
		)
		if err != nil {
			return fmt.Errorf("failed to create default item type %s: %w", itemType.name, err)
		}
	}

	// 10b. Bind selected item types to the default configuration set (excluding Initiative for simplified setup)
	for _, typeName := range defaultItemTypeBindings {
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
	_, err = tx.Exec(
		"INSERT INTO channels (name, type, direction, description, status, is_default, config) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"Notification Mail", "smtp", "outbound", "Default SMTP channel for sending notification emails", "pending", true, defaultNotificationChannelConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification mail channel: %w", err)
	}

	// 12. Create default themes with dual light/dark nav colors
	for _, theme := range defaultThemes {
		_, err = tx.Exec(
			"INSERT INTO themes (builtin_key, name, description, is_default, is_active, nav_background_color_light, nav_text_color_light, nav_background_color_dark, nav_text_color_dark) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			theme.builtinKey, theme.name, theme.description, theme.isDefault, theme.isActive, theme.navBackgroundColorLight, theme.navTextColorLight, theme.navBackgroundColorDark, theme.navTextColorDark,
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
	if err := priorityRows.Err(); err != nil {
		return fmt.Errorf("failed to iterate priorities: %w", err)
	}

	// 14. Create default notification settings
	// (built-in email templates are seeded by emailutil.SeedTemplates from
	// the server bootstrap after Initialize completes — keeps the database
	// layer free of email-domain imports.)
	notificationSettingResult, err := tx.Exec(
		"INSERT INTO notification_settings (builtin_key, name, description, is_active, created_by) VALUES (?, ?, ?, ?, ?)",
		"default", "Default Notifications", "Standard notification rules for work item updates", true, nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification setting: %w", err)
	}
	notificationSettingID, _ := notificationSettingResult.LastInsertId()

	// 15. Create default notification event rules
	// NOTE: mention.created is NOT in this rules table by design — mentions
	// always notify the mentioned user (subject to workspace visibility),
	// which is enforced in mention_service.go without going through the
	// configurable rules system.

	for _, rule := range defaultNotificationEventRules {
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
