package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/lib/pq"
)

//go:embed schema/base_tables_postgres.sql
var baseTablesSchemaPostgres string

//go:embed schema/items_postgres.sql
var itemsSchemaPostgres string

//go:embed schema/config_workflows_postgres.sql
var configWorkflowsSchemaPostgres string

//go:embed schema/time_tracking_postgres.sql
var timeTrackingSchemaPostgres string

//go:embed schema/portal_postgres.sql
var portalSchemaPostgres string

//go:embed schema/portal_auth_postgres.sql
var portalAuthSchemaPostgres string

//go:embed schema/milestones_postgres.sql
var milestonesSchemaPostgres string

//go:embed schema/iterations_postgres.sql
var iterationsSchemaPostgres string

//go:embed schema/content_postgres.sql
var contentSchemaPostgres string

//go:embed schema/notifications_postgres.sql
var notificationsSchemaPostgres string

//go:embed schema/channels_postgres.sql
var channelsSchemaPostgres string

//go:embed schema/permissions_postgres.sql
var permissionsSchemaPostgres string

//go:embed schema/system_postgres.sql
var systemSchemaPostgres string

//go:embed schema/core_postgres.sql
var coreSchemaPostgres string

//go:embed schema/users_postgres.sql
var usersSchemaPostgres string

//go:embed schema/workspace_postgres.sql
var workspaceSchemaPostgres string

//go:embed schema/request_types_postgres.sql
var requestTypesSchemaPostgres string

//go:embed schema/tests_postgres.sql
var testsSchemaPostgres string

//go:embed schema/time_worklogs_postgres.sql
var timeWorklogsSchemaPostgres string

//go:embed schema/default_data_postgres.sql
var defaultDataPostgresSQL string

//go:embed schema/webauthn_postgres.sql
var webauthnSchemaPostgres string

//go:embed schema/sso_postgres.sql
var ssoSchemaPostgres string

//go:embed schema/scm_postgres.sql
var scmSchemaPostgres string

//go:embed schema/mentions_postgres.sql
var mentionsSchemaPostgres string

//go:embed schema/user_preferences_postgres.sql
var userPreferencesSchemaPostgres string

//go:embed schema/assets_postgres.sql
var assetsSchemaPostgres string

//go:embed schema/recurring_tasks_postgres.sql
var recurringTasksSchemaPostgres string

//go:embed schema/jira_import_postgres.sql
var jiraImportSchemaPostgres string

//go:embed schema/actions_postgres.sql
var actionsSchemaPostgres string

//go:embed schema/labels_postgres.sql
var labelsSchemaPostgres string

//go:embed schema/llm_postgres.sql
var llmSchemaPostgres string

//go:embed schema/ldap_postgres.sql
var ldapSchemaPostgres string

//go:embed schema/email_postgres.sql
var emailSchemaPostgres string

// PostgresDB implements the Database interface for PostgreSQL
type PostgresDB struct {
	db  *sql.DB
	dsn string
}

// schemaFile holds a schema file name and its content for execution
type schemaFile struct {
	name    string
	content string
}

// NewPostgresDB creates a new PostgreSQL database connection with 50 connections
func NewPostgresDB(connectionString string) (Database, error) {
	// Open connection
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	// Set connection pool settings for high concurrency
	// Increased from 50 to 200 to handle more concurrent requests
	db.SetMaxOpenConns(200)
	db.SetMaxIdleConns(100)

	return &PostgresDB{
		db:  db,
		dsn: connectionString,
	}, nil
}

// ConvertPlaceholders converts SQLite-style ? placeholders to PostgreSQL-style $1, $2, etc.
// Exported so it can be used by transaction wrappers
func ConvertPlaceholders(query string) string {
	// Efficient replacement using strings.Builder to avoid O(n²) string concatenation
	var result strings.Builder
	result.Grow(len(query) + 100) // Pre-allocate space for efficiency

	paramNum := 1
	for _, ch := range query {
		if ch == '?' {
			result.WriteString(fmt.Sprintf("$%d", paramNum))
			paramNum++
		} else {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// GetDB returns the underlying *sql.DB for backward compatibility
func (p *PostgresDB) GetDB() *sql.DB {
	return p.db
}

// GetDriverName returns the database driver name
func (p *PostgresDB) GetDriverName() string {
	return "postgres"
}

// Query executes a query that returns rows
func (p *PostgresDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return p.db.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func (p *PostgresDB) QueryRow(query string, args ...interface{}) *sql.Row {
	query = ConvertPlaceholders(query)
	return p.db.QueryRow(query, args...)
}

// Exec executes a query that doesn't return rows
func (p *PostgresDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.Exec(query, args...)
}

// QueryContext executes a query with context that returns rows
func (p *PostgresDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return p.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query with context that returns at most one row
func (p *PostgresDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	query = ConvertPlaceholders(query)
	return p.db.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a query with context that doesn't return rows
func (p *PostgresDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.ExecContext(ctx, query, args...)
}

// ExecWrite explicitly executes a write query
// For PostgreSQL, this is the same as Exec since MVCC handles concurrency
func (p *PostgresDB) ExecWrite(query string, args ...interface{}) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.Exec(query, args...)
}

// ExecWriteContext explicitly executes a write query with context
// For PostgreSQL, this is the same as ExecContext since MVCC handles concurrency
func (p *PostgresDB) ExecWriteContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.ExecContext(ctx, query, args...)
}

// Begin starts a new transaction (returns wrapped transaction)
func (p *PostgresDB) Begin() (Tx, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	return NewPostgresTx(tx), nil
}

// BeginTx starts a new transaction with options (returns wrapped transaction)
func (p *PostgresDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := p.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return NewPostgresTx(tx), nil
}

// Close closes the database connection
func (p *PostgresDB) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Initialize sets up the database schema
func (p *PostgresDB) Initialize() error {
	// Check if database is already initialized by checking for core tables
	var tableCount int
	err := p.db.QueryRow(`
		SELECT COUNT(table_name) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN ('workspaces', 'items', 'users', 'workflows')
	`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("failed to check database initialization: %w", err)
	}

	// If all core tables exist, database is already initialized
	if tableCount >= 4 {
		// Run migrations for existing databases
		pgMigrations := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='workspaces' AND column_name='display_mode'",
				alter: "ALTER TABLE workspaces ADD COLUMN display_mode TEXT DEFAULT 'default'",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='active_timers' AND column_name='user_id'",
				alter: "ALTER TABLE active_timers ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE",
			},
		}

		for _, m := range pgMigrations {
			var count int
			if err = p.db.QueryRow(m.check).Scan(&count); err == nil && count == 0 {
				if _, err = p.db.Exec(m.alter); err != nil {
					slog.Warn("postgres migration failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		// Create labels tables if they don't exist (for existing databases)
		labelsContent := strings.TrimSpace(labelsSchemaPostgres)
		if labelsContent != "" {
			if _, err = p.db.Exec(labelsContent); err != nil {
				slog.Warn("labels postgres migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Create LLM tables if they don't exist (for existing databases)
		llmContent := strings.TrimSpace(llmSchemaPostgres)
		if llmContent != "" {
			if _, err = p.db.Exec(llmContent); err != nil {
				slog.Warn("llm postgres migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Create milestone_releases table if it doesn't exist and drop legacy SCM columns from milestones
		milestonesContent := strings.TrimSpace(milestonesSchemaPostgres)
		if milestonesContent != "" {
			if _, err = p.db.Exec(milestonesContent); err != nil {
				slog.Warn("milestones postgres migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		// Drop legacy SCM columns from milestones table (moved to milestone_releases)
		pgScmColumnDrops := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='milestones' AND column_name='scm_connection_id'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_connection_id",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='milestones' AND column_name='scm_repository'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_repository",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='milestones' AND column_name='scm_release_id'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_release_id",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='milestones' AND column_name='scm_release_url'",
				alter: "ALTER TABLE milestones DROP COLUMN scm_release_url",
			},
		}
		for _, m := range pgScmColumnDrops {
			var count int
			if err = p.db.QueryRow(m.check).Scan(&count); err == nil && count > 0 {
				if _, err = p.db.Exec(m.alter); err != nil {
					slog.Warn("milestone scm column drop failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		// Add SAML columns to sso_providers (for existing databases)
		samlPgMigrations := []struct {
			check string
			alter string
		}{
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sso_providers' AND column_name='saml_idp_metadata_url'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_idp_metadata_url TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sso_providers' AND column_name='saml_idp_sso_url'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_idp_sso_url TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sso_providers' AND column_name='saml_idp_certificate'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_idp_certificate TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sso_providers' AND column_name='saml_sp_entity_id'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_sp_entity_id TEXT",
			},
			{
				check: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sso_providers' AND column_name='saml_sign_requests'",
				alter: "ALTER TABLE sso_providers ADD COLUMN saml_sign_requests BOOLEAN DEFAULT FALSE",
			},
		}
		for _, m := range samlPgMigrations {
			var count int
			if err = p.db.QueryRow(m.check).Scan(&count); err == nil && count == 0 {
				if _, err = p.db.Exec(m.alter); err != nil {
					slog.Warn("SAML postgres migration failed", slog.String("component", "database"), slog.String("sql", m.alter), slog.Any("error", err))
				}
			}
		}

		// Create LDAP tables if they don't exist (for existing databases)
		ldapContent := strings.TrimSpace(ldapSchemaPostgres)
		if ldapContent != "" {
			if _, err = p.db.Exec(ldapContent); err != nil {
				slog.Warn("LDAP postgres migration failed", slog.String("component", "database"), slog.Any("error", err))
			}
		}

		return nil
	}

	// Database needs full initialization
	// Execute schema in a transaction
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Execute each schema file individually
	schemaFiles := p.getPostgresSchemaFiles()
	for _, sf := range schemaFiles {
		content := strings.TrimSpace(sf.content)
		if content == "" {
			continue
		}
		_, err = tx.Exec(content)
		if err != nil {
			return fmt.Errorf("failed to execute schema file %s: %w", sf.name, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema transaction: %w", err)
	}

	slog.Debug("database schema initialized successfully", slog.String("component", "database"))

	// Initialize default data for new installations
	if err := p.initializePostgresDefaultData(); err != nil {
		return fmt.Errorf("failed to initialize default data: %w", err)
	}

	return nil
}

// getPostgresSchemaFiles returns the PostgreSQL schema files in dependency order
func (p *PostgresDB) getPostgresSchemaFiles() []schemaFile {
	// Files are ordered to respect foreign key dependencies:
	// 0. Base tables (no foreign key dependencies) - MUST come first
	// 1. Users, WebAuthn, SSO
	// 2. Core/custom fields, Channels
	// 3. Time tracking base tables
	// 4. Portal/customers (depends on users, customer_organizations, channels)
	// 5. Workspaces - MUST come before tables that reference it
	// 6. Config/workflows, Milestones
	// 7. Iterations (depends on workspaces)
	// 8. Request types, Items (items depends on iterations)
	// 9. Time worklogs, Content, Mentions
	// 10. Notifications, Permissions, System, User preferences
	// 11. Tests, SCM, Assets, Recurring tasks, Jira import
	// 12. External integrations
	return []schemaFile{
		{"base_tables_postgres.sql", baseTablesSchemaPostgres},
		{"users_postgres.sql", usersSchemaPostgres},
		{"webauthn_postgres.sql", webauthnSchemaPostgres},
		{"sso_postgres.sql", ssoSchemaPostgres},
		{"core_postgres.sql", coreSchemaPostgres},
		{"channels_postgres.sql", channelsSchemaPostgres},
		{"time_tracking_postgres.sql", timeTrackingSchemaPostgres},
		{"portal_postgres.sql", portalSchemaPostgres},
		{"portal_auth_postgres.sql", portalAuthSchemaPostgres},
		{"workspace_postgres.sql", workspaceSchemaPostgres},
		{"config_workflows_postgres.sql", configWorkflowsSchemaPostgres},
		{"milestones_postgres.sql", milestonesSchemaPostgres},
		{"iterations_postgres.sql", iterationsSchemaPostgres},
		{"request_types_postgres.sql", requestTypesSchemaPostgres},
		{"items_postgres.sql", itemsSchemaPostgres},
		{"time_worklogs_postgres.sql", timeWorklogsSchemaPostgres},
		{"content_postgres.sql", contentSchemaPostgres},
		{"email_postgres.sql", emailSchemaPostgres},
		{"mentions_postgres.sql", mentionsSchemaPostgres},
		{"notifications_postgres.sql", notificationsSchemaPostgres},
		{"permissions_postgres.sql", permissionsSchemaPostgres},
		{"system_postgres.sql", systemSchemaPostgres},
		{"user_preferences_postgres.sql", userPreferencesSchemaPostgres},
		{"tests_postgres.sql", testsSchemaPostgres},
		{"scm_postgres.sql", scmSchemaPostgres},
		{"assets_postgres.sql", assetsSchemaPostgres},
		{"recurring_tasks_postgres.sql", recurringTasksSchemaPostgres},
		{"jira_import_postgres.sql", jiraImportSchemaPostgres},
		{"actions_postgres.sql", actionsSchemaPostgres},
		{"labels_postgres.sql", labelsSchemaPostgres},
		{"llm_postgres.sql", llmSchemaPostgres},
		{"ldap_postgres.sql", ldapSchemaPostgres},
	}
}

// initializePostgresDefaultData initializes default data for a fresh PostgreSQL installation
func (p *PostgresDB) initializePostgresDefaultData() error {
	// Check if we already have default data by looking for status categories
	var categoryCount int
	err := p.db.QueryRow("SELECT COUNT(*) FROM status_categories").Scan(&categoryCount)
	if err != nil {
		return fmt.Errorf("failed to check existing status categories: %w", err)
	}

	// If we already have status categories, assume default data exists
	if categoryCount > 0 {
		return nil
	}

	// Begin transaction for atomic initialization
	tx, err := p.db.Begin()
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
		var id int64
		err = tx.QueryRow(
			"INSERT INTO status_categories (name, color, description, is_default, is_completed) VALUES ($1, $2, $3, $4, $5) RETURNING id",
			cat.name, cat.color, cat.description, cat.isDefault, cat.isCompleted,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to create status category %s: %w", cat.name, err)
		}
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
		var id int64
		err = tx.QueryRow(
			"INSERT INTO statuses (name, description, category_id, is_default) VALUES ($1, $2, $3, $4) RETURNING id",
			status.name, status.description, categoryID, status.isDefault,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to create status %s: %w", status.name, err)
		}
		statusIDs[status.name] = id
	}

	// 3. Create default workflow
	var workflowID int64
	err = tx.QueryRow(
		"INSERT INTO workflows (name, description, is_default) VALUES ($1, $2, $3) RETURNING id",
		"Default Workflow", "Basic workflow for getting work done", true,
	).Scan(&workflowID)
	if err != nil {
		return fmt.Errorf("failed to create default workflow: %w", err)
	}

	// 4. Create workflow transitions
	transitions := []struct {
		from string // empty string means initial status
		to   string
	}{
		{"", "Open"},                 // Initial transition
		{"Open", "In Progress"},
		{"Open", "Done"},             // Direct completion from Open
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
			"INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order) VALUES ($1, $2, $3, $4)",
			workflowID, fromStatusID, toStatusID, i,
		)
		if err != nil {
			return fmt.Errorf("failed to create transition from %s to %s: %w", transition.from, transition.to, err)
		}
	}

	// 5. Create default screen with basic fields
	var screenID int64
	err = tx.QueryRow(
		"INSERT INTO screens (name, description) VALUES ($1, $2) RETURNING id",
		"Default Screen", "Default screen with essential work item fields",
	).Scan(&screenID)
	if err != nil {
		return fmt.Errorf("failed to create default screen: %w", err)
	}

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
			"INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width) VALUES ($1, $2, $3, $4, $5, $6)",
			screenID, field.fieldType, field.fieldIdentifier, field.displayOrder, field.isRequired, field.fieldWidth,
		)
		if err != nil {
			return fmt.Errorf("failed to add field %s to default screen: %w", field.fieldIdentifier, err)
		}
	}

	// 7. Create default configuration set
	var configSetID int64
	err = tx.QueryRow(
		"INSERT INTO configuration_sets (name, description, workflow_id, is_default) VALUES ($1, $2, $3, $4) RETURNING id",
		"Default Configuration", "Default configuration set with basic workflow and screen", workflowID, true,
	).Scan(&configSetID)
	if err != nil {
		return fmt.Errorf("failed to create default configuration set: %w", err)
	}

	// 8. Assign default screen to configuration set for all contexts
	contexts := []string{"create", "edit", "view"}
	for _, context := range contexts {
		_, err = tx.Exec(
			"INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context) VALUES ($1, $2, $3)",
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
			"INSERT INTO link_types (name, description, forward_label, reverse_label, color, is_system) VALUES ($1, $2, $3, $4, $5, $6)",
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
			"INSERT INTO system_settings (key, value, value_type, description, category) VALUES ($1, $2, $3, $4, $5)",
			setting.key, setting.value, setting.valueType, setting.description, setting.category,
		)
		if err != nil {
			return fmt.Errorf("failed to create system setting %s: %w", setting.key, err)
		}
	}

	// 12. Create default hierarchy levels
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
			"INSERT INTO hierarchy_levels (level, name, description) VALUES ($1, $2, $3)",
			hl.level, hl.name, hl.description,
		)
		if err != nil {
			return fmt.Errorf("failed to create hierarchy level %s: %w", hl.name, err)
		}
	}

	// 13. Create default item types with icons and colors
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
			"INSERT INTO item_types (configuration_set_id, name, description, icon, color, hierarchy_level, sort_order, is_default) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			configSetID, itemType.name, itemType.description, itemType.icon, itemType.color, itemType.hierarchyLevel, itemType.sortOrder, true,
		)
		if err != nil {
			return fmt.Errorf("failed to create default item type %s: %w", itemType.name, err)
		}
	}

	// 13b. Bind selected item types to the default configuration set (excluding Initiative for simplified setup)
	itemTypesToBind := []string{"Epic", "Story", "Task", "Bug", "Sub-task"}
	for _, typeName := range itemTypesToBind {
		var itemTypeID int64
		err = tx.QueryRow("SELECT id FROM item_types WHERE name = $1", typeName).Scan(&itemTypeID)
		if err != nil {
			return fmt.Errorf("failed to get item type ID for %s: %w", typeName, err)
		}
		_, err = tx.Exec(
			"INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id) VALUES ($1, $2)",
			configSetID, itemTypeID,
		)
		if err != nil {
			return fmt.Errorf("failed to bind item type %s to default configuration set: %w", typeName, err)
		}
	}

	// 14. Create default Notification Mail channel
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
		"INSERT INTO channels (name, type, direction, description, status, is_default, config) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		"Notification Mail", "smtp", "outbound", "Default SMTP channel for sending notification emails", "pending", true, defaultChannelConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification mail channel: %w", err)
	}

	// 14. Create default themes with dual light/dark nav colors
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
			"INSERT INTO themes (name, description, is_default, is_active, nav_background_color_light, nav_text_color_light, nav_background_color_dark, nav_text_color_dark) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			theme.name, theme.description, theme.isDefault, theme.isActive, theme.navBackgroundColorLight, theme.navTextColorLight, theme.navBackgroundColorDark, theme.navTextColorDark,
		)
		if err != nil {
			return fmt.Errorf("failed to create theme %s: %w", theme.name, err)
		}
	}

	// 12. Create default priorities if none exist
	var priorityCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM priorities").Scan(&priorityCount)
	if err != nil {
		return fmt.Errorf("failed to check existing priorities: %w", err)
	}

	if priorityCount == 0 {
		_, err = tx.Exec(defaultDataPostgresSQL)
		if err != nil {
			return fmt.Errorf("failed to create default priorities: %w", err)
		}
	}

	// 15. Create default notification templates (moved here to avoid semicolon parsing issues in schema files)
	emailHeaderTemplate := `<div class="header" style="background-color:#2563eb;color:white;padding:24px;text-align:center"><h1 style="margin:0;font-size:24px;font-weight:600">Windshift - Work Management</h1></div>`
	emailFooterTemplate := `<div class="footer" style="background-color:#f9fafb;padding:24px;text-align:center;font-size:14px;color:#6b7280;border-top:1px solid #e5e7eb"><p>This is an automated notification from <strong>Windshift - Work Management</strong>.<br><a href="#" style="color:#2563eb;text-decoration:none">View all notifications in Windshift</a></p><div style="font-size:12px;color:#9ca3af;margin-top:16px">To manage your notification preferences, please contact your administrator.</div></div>`

	_, err = tx.Exec(
		`INSERT INTO notification_templates (name, template_type, content, description, is_active) VALUES
		($1, $2, $3, $4, $5),
		($6, $7, $8, $9, $10)
		ON CONFLICT (name) DO NOTHING`,
		"email_header", "header", emailHeaderTemplate, "Email header template", true,
		"email_footer", "footer", emailFooterTemplate, "Email footer template", true,
	)
	if err != nil {
		return fmt.Errorf("failed to create default notification templates: %w", err)
	}

	// 16. Create default notification settings
	var notificationSettingID int64
	err = tx.QueryRow(
		"INSERT INTO notification_settings (name, description, is_active, created_by) VALUES ($1, $2, $3, $4) RETURNING id",
		"Default Notifications", "Standard notification rules for work item updates", true, nil,
	).Scan(&notificationSettingID)
	if err != nil {
		return fmt.Errorf("failed to create default notification setting: %w", err)
	}

	// 17. Create default notification event rules
	defaultEventRules := []struct {
		eventType      string
		notifyAssignee bool
		notifyCreator  bool
	}{
		{"item.assigned", true, false},
		{"comment.created", true, true},
		{"status.changed", true, true},
	}

	for _, rule := range defaultEventRules {
		_, err = tx.Exec(
			`INSERT INTO notification_event_rules
			 (notification_setting_id, event_type, is_enabled, notify_assignee, notify_creator,
			  notify_watchers, notify_workspace_admins)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			notificationSettingID, rule.eventType, true, rule.notifyAssignee,
			rule.notifyCreator, false, false,
		)
		if err != nil {
			return fmt.Errorf("failed to create notification rule for %s: %w", rule.eventType, err)
		}
	}

	// 18. Link notification setting to default configuration set
	_, err = tx.Exec(
		"INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id) VALUES ($1, $2)",
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

// EnsureDefaultNotificationSettings creates default notification settings if they don't exist
// This should be called during application startup to ensure notifications work
func (p *PostgresDB) EnsureDefaultNotificationSettings() error {
	// Check if notification settings already exist
	var settingCount int
	err := p.db.QueryRow("SELECT COUNT(*) FROM notification_settings").Scan(&settingCount)
	if err != nil {
		return fmt.Errorf("failed to check existing notification settings: %w", err)
	}

	if settingCount > 0 {
		// Settings already exist, nothing to do
		return nil
	}

	// Find the default configuration set
	var configSetID int
	err = p.db.QueryRow("SELECT id FROM configuration_sets WHERE is_default = true LIMIT 1").Scan(&configSetID)
	if err == sql.ErrNoRows {
		slog.Debug("no default configuration set found, skipping notification settings initialization", slog.String("component", "database"))
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to find default configuration set: %w", err)
	}

	// Begin transaction
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Create notification setting
	var notificationSettingID int64
	err = tx.QueryRow(
		"INSERT INTO notification_settings (name, description, is_active, created_by) VALUES ($1, $2, $3, $4) RETURNING id",
		"Default Notifications", "Standard notification rules for work item updates", true, nil,
	).Scan(&notificationSettingID)
	if err != nil {
		return fmt.Errorf("failed to create notification setting: %w", err)
	}

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
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			notificationSettingID, rule.eventType, true, rule.notifyAssignee, rule.notifyCreator, false, false,
		)
		if err != nil {
			return fmt.Errorf("failed to create rule for %s: %w", rule.eventType, err)
		}
	}

	// Link to default configuration set
	_, err = tx.Exec(
		"INSERT INTO configuration_set_notification_settings (configuration_set_id, notification_setting_id) VALUES ($1, $2)",
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

// CreateWorkspaceItemSequence creates a PostgreSQL sequence for workspace item numbers
func (p *PostgresDB) CreateWorkspaceItemSequence(workspaceID int64) error {
	seqName := fmt.Sprintf("workspace_%d_item_seq", workspaceID)
	_, err := p.db.Exec(fmt.Sprintf("CREATE SEQUENCE IF NOT EXISTS %s START 1", seqName))
	return err
}

// DropWorkspaceItemSequence drops the PostgreSQL sequence when workspace is deleted
func (p *PostgresDB) DropWorkspaceItemSequence(workspaceID int64) error {
	seqName := fmt.Sprintf("workspace_%d_item_seq", workspaceID)
	_, err := p.db.Exec(fmt.Sprintf("DROP SEQUENCE IF EXISTS %s", seqName))
	return err
}

// NextWorkspaceItemNumber gets the next item number from the workspace sequence
// This is atomic and requires no locking
func (p *PostgresDB) NextWorkspaceItemNumber(workspaceID int64) (int, error) {
	seqName := fmt.Sprintf("workspace_%d_item_seq", workspaceID)
	var nextVal int
	err := p.db.QueryRow(fmt.Sprintf("SELECT nextval('%s')", seqName)).Scan(&nextVal)
	return nextVal, err
}
