package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"time"

	// Register the PostgreSQL driver.
	_ "github.com/lib/pq"
)

//go:embed schema/base_tables_postgres.sql
var baseTablesSchemaPostgres string

//go:embed schema/items_postgres.sql
var itemsSchemaPostgres string

//go:embed schema/config_workflows_postgres.sql
var configWorkflowsSchemaPostgres string

//go:embed schema/object_translations_postgres.sql
var objectTranslationsSchemaPostgres string

//go:embed schema/time_tracking_postgres.sql
var timeTrackingSchemaPostgres string

//go:embed schema/portal_postgres.sql
var portalSchemaPostgres string

//go:embed schema/portal_auth_postgres.sql
var portalAuthSchemaPostgres string

//go:embed schema/portal_webauthn_postgres.sql
var portalWebauthnSchemaPostgres string

//go:embed schema/portal_drafts_postgres.sql
var portalDraftsSchemaPostgres string

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

//go:embed schema/templates_postgres.sql
var templatesSchemaPostgres string

//go:embed schema/llm_postgres.sql
var llmSchemaPostgres string

//go:embed schema/ldap_postgres.sql
var ldapSchemaPostgres string

//go:embed schema/email_postgres.sql
var emailSchemaPostgres string

//go:embed schema/asset_actions_postgres.sql
var assetActionsSchemaPostgres string

//go:embed schema/asset_reports_postgres.sql
var assetReportsSchemaPostgres string

//go:embed schema/daily_briefings_postgres.sql
var dailyBriefingsSchemaPostgres string

//go:embed schema/teams_postgres.sql
var teamsSchemaPostgres string

//go:embed schema/incidents_postgres.sql
var incidentsSchemaPostgres string

//go:embed schema/condition_sets_postgres.sql
var conditionSetsSchemaPostgres string

//go:embed schema/approvals_postgres.sql
var approvalsSchemaPostgres string

//go:embed schema/integrations_postgres.sql
var integrationsSchemaPostgres string

//go:embed schema/auth_policy_postgres.sql
var authPolicySchemaPostgres string

//go:embed schema/pages_postgres.sql
var pagesSchemaPostgres string

//go:embed schema/page_labels_postgres.sql
var pageLabelsSchemaPostgres string

//go:embed schema/agents_postgres.sql
var agentsSchemaPostgres string

//go:embed schema/events_postgres.sql
var eventsSchemaPostgres string

//go:embed schema/action_events_postgres.sql
var actionEventsSchemaPostgres string

//go:embed schema/action_event_targets_postgres.sql
var actionEventTargetsSchemaPostgres string

//go:embed schema/asset_action_events_postgres.sql
var assetActionEventsSchemaPostgres string

// PostgresDB implements the Database interface for PostgreSQL
type PostgresDB struct {
	db         *sql.DB
	dsn        string
	instanceID uint64
}

// schemaFile holds a schema file name and its content for execution
type schemaFile struct {
	name    string
	content string
}

// NewPostgresDB creates a new PostgreSQL database connection.
// maxConns sizes the pool (idle pool = maxConns/2, min 1).
func NewPostgresDB(connectionString string, maxConns int) (Database, error) {
	// Default the session timezone to UTC for consistent timestamp handling.
	connectionString = ensurePostgresTimezoneUTC(connectionString)

	// Open connection
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	idle := maxConns / 2
	if idle < 1 {
		idle = 1
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(idle)
	// Recycle connections so idle pools do not exhaust the server.
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	return &PostgresDB{
		db:         db,
		dsn:        connectionString,
		instanceID: newDatabaseInstanceID(),
	}, nil
}

// ensurePostgresTimezoneUTC defaults URL and keyword DSNs to UTC.
func ensurePostgresTimezoneUTC(dsn string) string {
	if strings.Contains(strings.ToLower(dsn), "timezone=") {
		return dsn
	}
	// URL-style DSNs start with "postgres://" or "postgresql://". Anything
	// else (including the empty string) is keyword-value.
	lower := strings.ToLower(dsn)
	urlStyle := strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://")
	if urlStyle {
		if strings.Contains(dsn, "?") {
			return dsn + "&timezone=UTC"
		}
		return dsn + "?timezone=UTC"
	}
	// Keyword-value form: space-separated. Empty DSN means "use defaults
	// from PG* env vars" — append directly.
	if dsn == "" {
		return "timezone=UTC"
	}
	return dsn + " timezone=UTC"
}

// ConvertPlaceholders converts bare SQLite ? placeholders to PostgreSQL $N.
// It preserves quoted strings, identifiers, comments, and JSONB operators.
func ConvertPlaceholders(query string) string {
	var result strings.Builder
	result.Grow(len(query) + 16)

	runes := []rune(query)
	n := len(runes)
	paramNum := 1

	for i := 0; i < n; {
		ch := runes[i]

		// Single-quoted string with '' escape.
		if ch == '\'' {
			result.WriteRune(ch)
			i++
			for i < n {
				c := runes[i]
				result.WriteRune(c)
				i++
				if c == '\'' {
					if i < n && runes[i] == '\'' {
						result.WriteRune(runes[i])
						i++
						continue
					}
					break
				}
			}
			continue
		}

		// Double-quoted identifier.
		if ch == '"' {
			result.WriteRune(ch)
			i++
			for i < n {
				c := runes[i]
				result.WriteRune(c)
				i++
				if c == '"' {
					break
				}
			}
			continue
		}

		// Line comment: -- until newline.
		if ch == '-' && i+1 < n && runes[i+1] == '-' {
			for i < n {
				c := runes[i]
				result.WriteRune(c)
				i++
				if c == '\n' {
					break
				}
			}
			continue
		}

		// Block comment: /* ... */.
		if ch == '/' && i+1 < n && runes[i+1] == '*' {
			result.WriteRune(runes[i])
			result.WriteRune(runes[i+1])
			i += 2
			for i+1 < n {
				if runes[i] == '*' && runes[i+1] == '/' {
					result.WriteRune(runes[i])
					result.WriteRune(runes[i+1])
					i += 2
					break
				}
				result.WriteRune(runes[i])
				i++
			}
			continue
		}

		// JSONB operators (?| ?& ??) and the bare ? placeholder.
		if ch == '?' {
			if i+1 < n {
				next := runes[i+1]
				if next == '|' || next == '&' || next == '?' {
					result.WriteRune(ch)
					result.WriteRune(next)
					i += 2
					continue
				}
			}
			fmt.Fprintf(&result, "$%d", paramNum)
			paramNum++
			i++
			continue
		}

		result.WriteRune(ch)
		i++
	}
	return result.String()
}

// GetDB returns the underlying *sql.DB for backward compatibility
func (p *PostgresDB) GetDB() *sql.DB {
	return p.db
}

// InstanceID distinguishes database pools created during this process.
func (p *PostgresDB) InstanceID() uint64 {
	return p.instanceID
}

// GetDriverName returns the database driver name
func (p *PostgresDB) GetDriverName() string {
	return "postgres"
}

// Query executes a query that returns rows
func (p *PostgresDB) Query(query string, args ...any) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return p.db.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func (p *PostgresDB) QueryRow(query string, args ...any) *sql.Row {
	query = ConvertPlaceholders(query)
	return p.db.QueryRow(query, args...)
}

// Exec executes a query that doesn't return rows
func (p *PostgresDB) Exec(query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.Exec(query, args...)
}

// QueryContext executes a query with context that returns rows
func (p *PostgresDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return p.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query with context that returns at most one row
func (p *PostgresDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	query = ConvertPlaceholders(query)
	return p.db.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a query with context that doesn't return rows
func (p *PostgresDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.ExecContext(ctx, query, args...)
}

// ExecWrite explicitly executes a write query
// For PostgreSQL, this is the same as Exec since MVCC handles concurrency
func (p *PostgresDB) ExecWrite(query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return p.db.Exec(query, args...)
}

// ExecWriteContext explicitly executes a write query with context
// For PostgreSQL, this is the same as ExecContext since MVCC handles concurrency
func (p *PostgresDB) ExecWriteContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
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

// Initialize sets up the PostgreSQL schema and applies the ordered catalog.
func (p *PostgresDB) Initialize() error {
	if _, err := p.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			checksum   TEXT NOT NULL DEFAULT '',
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		return fmt.Errorf("failed to bootstrap schema_migrations: %w", err)
	}

	var tableCount int
	if err := p.db.QueryRow(`
		SELECT COUNT(*)
		FROM unnest(ARRAY['workspaces', 'items', 'users', 'workflows']) AS core_table(name)
		WHERE to_regclass(core_table.name) IS NOT NULL
	`).Scan(&tableCount); err != nil {
		return fmt.Errorf("failed to check database initialization: %w", err)
	}

	if tableCount < 4 {
		tx, err := p.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin schema transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		for _, schemaFile := range p.getPostgresSchemaFiles() {
			content := strings.TrimSpace(schemaFile.content)
			if content == "" {
				continue
			}
			if _, err := tx.Exec(content); err != nil {
				return fmt.Errorf("failed to execute schema file %s: %w", schemaFile.name, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit schema transaction: %w", err)
		}
		slog.Debug("database schema initialized successfully", slog.String("component", "database"))

		if err := p.initializePostgresDefaultData(); err != nil {
			return fmt.Errorf("failed to initialize default data: %w", err)
		}
	} else {
		// Preflight: refuse a pre-0.8.5 database before any retained
		// migration can run (or fail mid-ALTER on a missing table).
		if err := ValidateCanonicalSchemaCheckpoint(p); err != nil {
			return fmt.Errorf("database startup refused: %w", err)
		}
	}

	if err := runPendingMigrations(p, Catalog); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
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
		{"auth_policy_postgres.sql", authPolicySchemaPostgres},
		{"webauthn_postgres.sql", webauthnSchemaPostgres},
		{"sso_postgres.sql", ssoSchemaPostgres},
		{"core_postgres.sql", coreSchemaPostgres},
		{"channels_postgres.sql", channelsSchemaPostgres},
		{"time_tracking_postgres.sql", timeTrackingSchemaPostgres},
		{"portal_postgres.sql", portalSchemaPostgres},
		{"portal_auth_postgres.sql", portalAuthSchemaPostgres},
		{"portal_webauthn_postgres.sql", portalWebauthnSchemaPostgres},
		{"workspace_postgres.sql", workspaceSchemaPostgres},
		{"config_workflows_postgres.sql", configWorkflowsSchemaPostgres},
		{"milestones_postgres.sql", milestonesSchemaPostgres},
		{"iterations_postgres.sql", iterationsSchemaPostgres},
		{"request_types_postgres.sql", requestTypesSchemaPostgres},
		{"items_postgres.sql", itemsSchemaPostgres},
		{"portal_drafts_postgres.sql", portalDraftsSchemaPostgres},
		{"time_worklogs_postgres.sql", timeWorklogsSchemaPostgres},
		{"content_postgres.sql", contentSchemaPostgres},
		{"email_postgres.sql", emailSchemaPostgres},
		{"mentions_postgres.sql", mentionsSchemaPostgres},
		{"notifications_postgres.sql", notificationsSchemaPostgres},
		{"permissions_postgres.sql", permissionsSchemaPostgres},
		{"system_postgres.sql", systemSchemaPostgres},
		{"object_translations_postgres.sql", objectTranslationsSchemaPostgres},
		{"user_preferences_postgres.sql", userPreferencesSchemaPostgres},
		{"tests_postgres.sql", testsSchemaPostgres},
		{"scm_postgres.sql", scmSchemaPostgres},
		{"assets_postgres.sql", assetsSchemaPostgres},
		{"asset_reports_postgres.sql", assetReportsSchemaPostgres},
		{"recurring_tasks_postgres.sql", recurringTasksSchemaPostgres},
		{"jira_import_postgres.sql", jiraImportSchemaPostgres},
		{"actions_postgres.sql", actionsSchemaPostgres},
		{"asset_actions_postgres.sql", assetActionsSchemaPostgres},
		{"labels_postgres.sql", labelsSchemaPostgres},
		{"templates_postgres.sql", templatesSchemaPostgres},
		{"llm_postgres.sql", llmSchemaPostgres},
		{"ldap_postgres.sql", ldapSchemaPostgres},
		{"daily_briefings_postgres.sql", dailyBriefingsSchemaPostgres},
		{"teams_postgres.sql", teamsSchemaPostgres},
		{"incidents_postgres.sql", incidentsSchemaPostgres},
		{"condition_sets_postgres.sql", conditionSetsSchemaPostgres},
		{"approvals_postgres.sql", approvalsSchemaPostgres},
		{"integrations_postgres.sql", integrationsSchemaPostgres},
		{"pages_postgres.sql", pagesSchemaPostgres},
		{"page_labels_postgres.sql", pageLabelsSchemaPostgres},
		{"agents_postgres.sql", agentsSchemaPostgres},
		{"events_postgres.sql", eventsSchemaPostgres},
		{"action_event_targets_postgres.sql", actionEventTargetsSchemaPostgres},
	}
}

// initializePostgresDefaultData initializes default data for a fresh
// PostgreSQL installation.
func (p *PostgresDB) initializePostgresDefaultData() error {
	return seedFreshInstall(p.db, func(tx *sql.Tx) seedWriter {
		return postgresSeedWriter{NewPostgresTx(tx)}
	})
}
