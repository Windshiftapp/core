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

// DB wraps a sql.DB connection with a dedicated write connection
type DB struct {
	*sql.DB
	writeConn *sql.DB // Dedicated single connection for writes
}

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
		}

		for _, m := range migrations {
			var count int
			if err := db.QueryRow(m.check).Scan(&count); err == nil && count == 0 {
				if _, err := db.Exec(m.alter); err != nil {
					slog.Warn("migration failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
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

		return nil
	}

	// Database needs full initialization
	schema := coreSchema + itemsSchema + requestTypeSchema + usersSchema + testsSchema + workspaceSchema + configWorkflowsSchema + timeTrackingSchema + channelsSchema + portalSchema + portalAuthSchema + milestonesSchema + iterationsSchema + contentSchema + mentionsSchema + notificationsSchema + permissionsSchema + systemSchema + userPreferencesSchema + webauthnSchema + ssoSchema + scmSchema + assetsSchema + recurringTasksSchema + jiraImportSchema + actionsSchema + emailSchema + assetReportsSchema + labelsSchema + llmSchema + ldapSchema

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
		{"system", "milestone", 6, false, "half"},
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
		name         string
		description  string
		forwardLabel string
		reverseLabel string
		color        string
		isSystem     bool
	}{
		{"Tests", "Test case tests work item", "tests", "tested by", "#10b981", true},
		{"Implements", "Work item implements another work item", "implements", "implemented by", "#3b82f6", true},
		{"Depends On", "Work item depends on another work item", "depends on", "blocks", "#f59e0b", true},
		{"Relates To", "General bidirectional relationship", "relates to", "relates to", "#6b7280", true},
		{"Links To", "General directional link", "links to", "linked from", "#64748b", true},
		{"Duplicates", "Work item is a duplicate of another", "duplicates", "duplicated by", "#ef4444", true},
		{"Child Of", "Alternative hierarchy relationship", "child of", "parent of", "#8b5cf6", true},
	}

	for _, linkType := range linkTypes {
		_, err = tx.Exec(
			"INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system) VALUES (?, ?, ?, ?, ?, ?)",
			linkType.name, linkType.description, linkType.forwardLabel, linkType.reverseLabel, linkType.color, linkType.isSystem,
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
		{"setup_completed", "false", "boolean", "Whether initial setup has been completed", "setup"},
		{"admin_user_created", "false", "boolean", "Whether admin user has been created", "setup"},
		{"calendar_feed_enabled", "true", "boolean", "Allow users to generate ICS calendar feed URLs", "security"},
		{"plugin_cli_exec_enabled", "false", "boolean", "Allow plugins to execute CLI commands", "security"},
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

// IsUniqueConstraintError checks if the error is a unique constraint violation
func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
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
