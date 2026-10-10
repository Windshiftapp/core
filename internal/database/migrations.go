package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
)

// pgJSONBColumnsCheck reports whether every named column already has the
// JSONB type, so type-conversion migrations stamp instead of re-running.
func pgJSONBColumnsCheck(columns ...[2]string) func(Database) (bool, error) {
	return func(db Database) (bool, error) {
		for _, column := range columns {
			var count int
			err := db.QueryRow(fmt.Sprintf(
				"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='%s' AND column_name='%s' AND data_type='jsonb'",
				column[0], column[1],
			)).Scan(&count)
			if err != nil {
				return false, err
			}
			if count == 0 {
				return false, nil
			}
		}
		return true, nil
	}
}

const zammadSchemaMigrationSQLite = `
	CREATE TABLE zammad_connections (
		provider_id TEXT PRIMARY KEY,
		credential_id INTEGER NOT NULL,
		base_url TEXT NOT NULL,
		default_group_id INTEGER,
		default_group_name TEXT DEFAULT '',
		allowed_group_ids TEXT NOT NULL DEFAULT '[]',
		default_customer TEXT NOT NULL,
		correlation_field TEXT NOT NULL DEFAULT 'windshift_item_key',
		closed_state_ids TEXT NOT NULL DEFAULT '[]',
		completion_status_id INTEGER,
		applies_to_all_workspaces BOOLEAN NOT NULL DEFAULT false,
		last_tested_at DATETIME,
		last_test_error TEXT DEFAULT '',
		created_by INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (provider_id) REFERENCES integration_providers(id) ON DELETE CASCADE,
		FOREIGN KEY (credential_id) REFERENCES action_credentials(id) ON DELETE RESTRICT,
		FOREIGN KEY (completion_status_id) REFERENCES statuses(id) ON DELETE SET NULL,
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
	);
	CREATE INDEX idx_zammad_connections_credential ON zammad_connections(credential_id);
	CREATE TABLE zammad_connection_workspaces (
		provider_id TEXT NOT NULL,
		workspace_id INTEGER NOT NULL,
		PRIMARY KEY (provider_id, workspace_id),
		FOREIGN KEY (provider_id) REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);
	CREATE INDEX idx_zammad_connection_workspaces_workspace ON zammad_connection_workspaces(workspace_id);
	CREATE TABLE zammad_ticket_links (
		id TEXT PRIMARY KEY,
		item_id INTEGER NOT NULL,
		provider_id TEXT NOT NULL,
		item_integration_link_id TEXT,
		ticket_id INTEGER,
		ticket_number TEXT DEFAULT '',
		ticket_url TEXT DEFAULT '',
		group_id INTEGER,
		group_name TEXT DEFAULT '',
		correlation_key TEXT NOT NULL,
		sync_state TEXT NOT NULL DEFAULT 'pending',
		creating_started_at DATETIME,
		last_status_id INTEGER,
		last_status_name TEXT DEFAULT '',
		last_synced_at DATETIME,
		last_error TEXT DEFAULT '',
		completion_applied BOOLEAN NOT NULL DEFAULT false,
		sync_lock_until DATETIME,
		created_by INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
		FOREIGN KEY (provider_id) REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
		FOREIGN KEY (item_integration_link_id) REFERENCES item_integration_links(id) ON DELETE SET NULL,
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
		UNIQUE(item_id, provider_id),
		UNIQUE(provider_id, ticket_id),
		UNIQUE(provider_id, correlation_key)
	);
	CREATE INDEX idx_zammad_ticket_links_item ON zammad_ticket_links(item_id);
	CREATE INDEX idx_zammad_ticket_links_sync ON zammad_ticket_links(sync_state, last_synced_at);
`

const zammadSchemaMigrationPostgres = `
	CREATE TABLE zammad_connections (
		provider_id TEXT PRIMARY KEY REFERENCES integration_providers(id) ON DELETE CASCADE,
		credential_id INTEGER NOT NULL REFERENCES action_credentials(id) ON DELETE RESTRICT,
		base_url TEXT NOT NULL,
		default_group_id INTEGER,
		default_group_name TEXT DEFAULT '',
		allowed_group_ids TEXT NOT NULL DEFAULT '[]',
		default_customer TEXT NOT NULL,
		correlation_field TEXT NOT NULL DEFAULT 'windshift_item_key',
		closed_state_ids TEXT NOT NULL DEFAULT '[]',
		completion_status_id INTEGER REFERENCES statuses(id) ON DELETE SET NULL,
		applies_to_all_workspaces BOOLEAN NOT NULL DEFAULT false,
		last_tested_at TIMESTAMPTZ,
		last_test_error TEXT DEFAULT '',
		created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX idx_zammad_connections_credential ON zammad_connections(credential_id);
	CREATE TABLE zammad_connection_workspaces (
		provider_id TEXT NOT NULL REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
		workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		PRIMARY KEY (provider_id, workspace_id)
	);
	CREATE INDEX idx_zammad_connection_workspaces_workspace ON zammad_connection_workspaces(workspace_id);
	CREATE TABLE zammad_ticket_links (
		id TEXT PRIMARY KEY,
		item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
		provider_id TEXT NOT NULL REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
		item_integration_link_id TEXT REFERENCES item_integration_links(id) ON DELETE SET NULL,
		ticket_id INTEGER,
		ticket_number TEXT DEFAULT '',
		ticket_url TEXT DEFAULT '',
		group_id INTEGER,
		group_name TEXT DEFAULT '',
		correlation_key TEXT NOT NULL,
		sync_state TEXT NOT NULL DEFAULT 'pending',
		creating_started_at TIMESTAMPTZ,
		last_status_id INTEGER,
		last_status_name TEXT DEFAULT '',
		last_synced_at TIMESTAMPTZ,
		last_error TEXT DEFAULT '',
		completion_applied BOOLEAN NOT NULL DEFAULT false,
		sync_lock_until TIMESTAMPTZ,
		created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(item_id, provider_id),
		UNIQUE(provider_id, ticket_id),
		UNIQUE(provider_id, correlation_key)
	);
	CREATE INDEX idx_zammad_ticket_links_item ON zammad_ticket_links(item_id);
	CREATE INDEX idx_zammad_ticket_links_sync ON zammad_ticket_links(sync_state, last_synced_at);
`

// Migration is one entry in the schema_migrations catalog. Version is a
// stable slug used as the schema_migrations primary key; Name is a human
// label. CheckSQLite / CheckPostgres are queries that return COUNT >= 1
// when the migration's effect is already present, used for retroactive
// backfill on existing installs upgrading past the introduction of the
// schema_migrations table. SQLite / Postgres carry the backend-specific
// DDL to apply when the check reports the effect is missing. ApplySQLite /
// ApplyPostgres are reserved for migrations that cannot be expressed as one
// transactional SQL body (notably SQLite table rebuilds that must toggle
// foreign_keys outside their transaction). Their matching SQL field contains
// a stable implementation marker that participates in checksum validation.
//
// An empty Check on a backend means the migration body always runs when
// the version isn't already stamped. An empty body on a backend means
// the migration is skipped on that backend — the row is still stamped
// so the catalog stays consistent across backends.
type Migration struct {
	Version         string
	Name            string
	CheckSQLite     string
	CheckPostgres   string
	CheckSQLiteFn   func(Database) (bool, error)
	CheckPostgresFn func(Database) (bool, error)
	SQLite          string
	Postgres        string
	ApplySQLite     func(Database) error
	ApplyPostgres   func(Database) error

	// ReconcileChecksum permits intentional edits to schema_* compatibility
	// wrappers. Applied wrappers are not rerun; their checksum is advanced.
	ReconcileChecksum bool

	// Superseded accepts checksums from before validation was enforced and
	// restamps them once. New schema changes still require a new migration.
	Superseded []string
}

// acceptsSuperseded reports whether stored is a checksum this migration's body
// carried in an earlier release.
func (m Migration) acceptsSuperseded(stored string) bool {
	return slices.Contains(m.Superseded, stored)
}

// Catalog is the ordered list of migrations applied via runPendingMigrations.
// New migrations append with a date-prefixed Version slug such as
// "20260514_widgets_archived_at". Order matters only between migrations
// with row dependencies; otherwise entries may be reordered freely.
//
// 0.8.6 squashed the historical catalog: 0.8.5 is the minimum supported
// schema, the compact catalog only carries the upgrades introduced after
// v0.8.5, and the Initialize implementations refuse databases without a
// valid canonical schema checkpoint before any migration runs. Retired
// schema_migrations rows on upgraded databases are ignored because the
// runner iterates the catalog, never the stored rows.
var Catalog = []Migration{
	{
		Version: "0000_baseline",
		Name:    "fresh-install baseline marker",
	},
	{
		Version:       "20260924_generic_import_jobs",
		Name:          "Generalize asset import jobs into a shared CSV import pipeline",
		CheckSQLite:   sqliteTableCheck("import_jobs"),
		CheckPostgres: pgTableCheck("import_jobs"),
		SQLite: `
			ALTER TABLE asset_import_jobs RENAME TO import_jobs;
			ALTER TABLE asset_import_uploads RENAME TO import_uploads;
			ALTER TABLE import_jobs RENAME COLUMN set_id TO scope_id;
			ALTER TABLE import_uploads RENAME COLUMN set_id TO scope_id;
			ALTER TABLE import_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'asset';
			ALTER TABLE import_uploads ADD COLUMN kind TEXT NOT NULL DEFAULT 'asset';
			DROP INDEX IF EXISTS idx_asset_import_jobs_set_id;
			DROP INDEX IF EXISTS idx_asset_import_jobs_status;
			DROP INDEX IF EXISTS idx_asset_import_jobs_created_by;
			CREATE INDEX IF NOT EXISTS idx_import_jobs_scope ON import_jobs(scope_id);
			CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_import_jobs_created_by ON import_jobs(created_by);
		`,
		Postgres: `
			ALTER TABLE asset_import_jobs RENAME TO import_jobs;
			ALTER TABLE asset_import_uploads RENAME TO import_uploads;
			ALTER TABLE import_jobs RENAME COLUMN set_id TO scope_id;
			ALTER TABLE import_uploads RENAME COLUMN set_id TO scope_id;
			ALTER TABLE import_jobs ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'asset';
			ALTER TABLE import_uploads ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'asset';
			DROP INDEX IF EXISTS idx_asset_import_jobs_set_id;
			DROP INDEX IF EXISTS idx_asset_import_jobs_status;
			DROP INDEX IF EXISTS idx_asset_import_jobs_created_by;
			CREATE INDEX IF NOT EXISTS idx_import_jobs_scope ON import_jobs(scope_id);
			CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_import_jobs_created_by ON import_jobs(created_by);
		`,
	},
	{
		Version:       "20260814_workflow_transitions_from_all",
		Name:          "Allow workflow transitions from every other status",
		CheckSQLite:   "SELECT COUNT(*) FROM pragma_table_info('workflow_transitions') WHERE name='from_all_statuses'",
		CheckPostgres: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workflow_transitions' AND column_name='from_all_statuses'",
		SQLite:        "ALTER TABLE workflow_transitions ADD COLUMN from_all_statuses BOOLEAN NOT NULL DEFAULT false",
		Postgres:      "ALTER TABLE workflow_transitions ADD COLUMN IF NOT EXISTS from_all_statuses BOOLEAN NOT NULL DEFAULT false",
	},
	{
		Version:       "20260816_portal_approval_vote_uniqueness",
		Name:          "Enforce one portal-customer vote per approval step",
		CheckSQLite:   sqliteIndexCheck("uq_approval_decisions_one_vote_per_portal_customer"),
		CheckPostgres: pgIndexCheck("uq_approval_decisions_one_vote_per_portal_customer"),
		SQLite: `
			DELETE FROM approval_decisions
			WHERE id IN (
				SELECT id FROM (
					SELECT id, ROW_NUMBER() OVER (
						PARTITION BY approval_step_instance_id, actor_portal_customer_id
						ORDER BY created_at, id
					) AS duplicate_rank
					FROM approval_decisions
					WHERE actor_portal_customer_id IS NOT NULL
					  AND decision IN ('approve', 'reject')
				) duplicate_votes
				WHERE duplicate_rank > 1
			);
			CREATE UNIQUE INDEX uq_approval_decisions_one_vote_per_portal_customer
				ON approval_decisions(approval_step_instance_id, actor_portal_customer_id)
				WHERE actor_portal_customer_id IS NOT NULL AND decision IN ('approve', 'reject');
		`,
		Postgres: `
			DELETE FROM approval_decisions
			WHERE id IN (
				SELECT id FROM (
					SELECT id, ROW_NUMBER() OVER (
						PARTITION BY approval_step_instance_id, actor_portal_customer_id
						ORDER BY created_at, id
					) AS duplicate_rank
					FROM approval_decisions
					WHERE actor_portal_customer_id IS NOT NULL
					  AND decision IN ('approve', 'reject')
				) duplicate_votes
				WHERE duplicate_rank > 1
			);
			CREATE UNIQUE INDEX uq_approval_decisions_one_vote_per_portal_customer
				ON approval_decisions(approval_step_instance_id, actor_portal_customer_id)
				WHERE actor_portal_customer_id IS NOT NULL AND decision IN ('approve', 'reject');
		`,
	},
	{
		Version:       "20260815_workspaces_is_template",
		Name:          "Mark workspaces as reusable templates",
		CheckSQLite:   sqliteColumnCheck("workspaces", "is_template"),
		CheckPostgres: pgColumnCheck("workspaces", "is_template"),
		// The body originally added the column NOT NULL while the fresh
		// schema files declared it nullable. The canonical contract is
		// nullable; Superseded advances databases stamped by unreleased
		// main builds that ran the NOT NULL body.
		Superseded: []string{"ea8a11f5aff9de67107eaaa4a23a1519397546f69d693b77beb1dd53c9478054"},
		SQLite: `
			ALTER TABLE workspaces ADD COLUMN is_template BOOLEAN DEFAULT false;
			CREATE INDEX IF NOT EXISTS idx_workspaces_template_active
				ON workspaces(is_template, active)
				WHERE is_template = true;
		`,
		Postgres: `
			ALTER TABLE workspaces ADD COLUMN is_template BOOLEAN DEFAULT false;
			CREATE INDEX IF NOT EXISTS idx_workspaces_template_active
				ON workspaces(is_template, active)
				WHERE is_template = true;
		`,
	},
	{
		Version:       "20260823_cfv_cleanup_retries",
		Name:          "Add retry scheduling to custom field maintenance jobs",
		CheckSQLite:   sqliteColumnCheck("pending_custom_field_cleanups", "attempt_count"),
		CheckPostgres: pgColumnCheck("pending_custom_field_cleanups", "attempt_count"),
		SQLite: `
			ALTER TABLE pending_custom_field_cleanups ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE pending_custom_field_cleanups ADD COLUMN next_attempt_at DATETIME;
		`,
		Postgres: `
			ALTER TABLE pending_custom_field_cleanups ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE pending_custom_field_cleanups ADD COLUMN next_attempt_at TIMESTAMPTZ;
		`,
	},
	{
		Version:       "20260824_agent_skill_page_snapshots",
		Name:          "Snapshot pages referenced by agent skills",
		CheckSQLite:   sqliteColumnCheck("workspace_agent_skill_pages", "content_snapshot"),
		CheckPostgres: pgColumnCheck("workspace_agent_skill_pages", "content_snapshot"),
		SQLite: `
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN title_snapshot TEXT NOT NULL DEFAULT '';
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN content_snapshot TEXT NOT NULL DEFAULT '';
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN page_updated_at_snapshot DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00';
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN snapshot_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00';
			UPDATE workspace_agent_skill_pages
			SET title_snapshot = COALESCE((SELECT title FROM pages WHERE pages.id = page_id), ''),
			    content_snapshot = COALESCE((SELECT content FROM pages WHERE pages.id = page_id), ''),
			    page_updated_at_snapshot = COALESCE((SELECT updated_at FROM pages WHERE pages.id = page_id), CURRENT_TIMESTAMP),
			    snapshot_at = CURRENT_TIMESTAMP;
		`,
		Postgres: `
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN title_snapshot TEXT NOT NULL DEFAULT '';
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN content_snapshot TEXT NOT NULL DEFAULT '';
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN page_updated_at_snapshot TIMESTAMPTZ;
			ALTER TABLE workspace_agent_skill_pages ADD COLUMN snapshot_at TIMESTAMPTZ;
			UPDATE workspace_agent_skill_pages sp
			SET title_snapshot = p.title,
			    content_snapshot = p.content,
			    page_updated_at_snapshot = p.updated_at,
			    snapshot_at = CURRENT_TIMESTAMP
			FROM pages p WHERE p.id = sp.page_id;
			ALTER TABLE workspace_agent_skill_pages ALTER COLUMN page_updated_at_snapshot SET NOT NULL;
			ALTER TABLE workspace_agent_skill_pages ALTER COLUMN snapshot_at SET NOT NULL;
		`,
	},
	{
		Version:       "20260826_board_completed_item_retention",
		Name:          "Add completed item retention to board configurations",
		CheckSQLite:   sqliteColumnCheck("board_configurations", "completed_item_retention_days"),
		CheckPostgres: pgColumnCheck("board_configurations", "completed_item_retention_days"),
		SQLite:        "ALTER TABLE board_configurations ADD COLUMN completed_item_retention_days INTEGER",
		Postgres:      "ALTER TABLE board_configurations ADD COLUMN completed_item_retention_days INTEGER",
	},
	{
		Version:       "20260827_notification_provenance",
		Name:          "Add authorization provenance to notifications",
		CheckSQLite:   sqliteColumnCheck("notifications", "authorization_scope"),
		CheckPostgres: pgColumnCheck("notifications", "authorization_scope"),
		SQLite: `
			ALTER TABLE notifications ADD COLUMN authorization_scope TEXT NOT NULL DEFAULT 'legacy';
			ALTER TABLE notifications ADD COLUMN workspace_id INTEGER;
			ALTER TABLE notifications ADD COLUMN item_id INTEGER;
			ALTER TABLE notifications ADD COLUMN source_type TEXT;
			ALTER TABLE notifications ADD COLUMN source_id INTEGER;
			CREATE INDEX idx_notifications_workspace_id ON notifications(workspace_id);
		`,
		Postgres: `
			ALTER TABLE notifications ADD COLUMN authorization_scope TEXT NOT NULL DEFAULT 'legacy';
			ALTER TABLE notifications ADD COLUMN workspace_id INTEGER;
			ALTER TABLE notifications ADD COLUMN item_id INTEGER;
			ALTER TABLE notifications ADD COLUMN source_type TEXT;
			ALTER TABLE notifications ADD COLUMN source_id INTEGER;
			CREATE INDEX idx_notifications_workspace_id ON notifications(workspace_id);
		`,
	},
	{
		Version: "20260827_domain_event_engine",
		Name:    "Add durable domain event engine",
		CheckSQLite: `
			SELECT CASE WHEN COUNT(*) = 7 THEN 1 ELSE 0 END
			FROM sqlite_master
			WHERE type = 'table' AND name IN (
				'domain_event_streams',
				'domain_events',
				'domain_event_consumers',
				'domain_event_subscriptions',
				'domain_event_consumer_streams',
				'domain_event_deliveries',
				'domain_event_delivery_actions'
			)
		`,
		CheckPostgres: `
			SELECT CASE WHEN COUNT(*) = 7 THEN 1 ELSE 0 END
			FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name IN (
				'domain_event_streams',
				'domain_events',
				'domain_event_consumers',
				'domain_event_subscriptions',
				'domain_event_consumer_streams',
				'domain_event_deliveries',
				'domain_event_delivery_actions'
			)
		`,
		SQLite:   eventsSchema,
		Postgres: eventsSchemaPostgres,
	},
	{
		Version: "20260827_durable_action_consumer",
		Name:    "Add durable action consumer state",
		CheckSQLite: `
			SELECT CASE WHEN
				EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='action_event_targets')
				AND EXISTS (SELECT 1 FROM pragma_table_info('action_execution_logs') WHERE name='durable_event_key')
			THEN 1 ELSE 0 END
		`,
		CheckPostgres: `
			SELECT CASE WHEN
				EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='action_event_targets')
				AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='action_execution_logs' AND column_name='durable_event_key')
			THEN 1 ELSE 0 END
		`,
		SQLite:   actionEventTargetsSchema + actionEventsSchema,
		Postgres: actionEventTargetsSchemaPostgres + actionEventsSchemaPostgres,
	},
	{
		Version:       "20260827_durable_asset_action_consumer",
		Name:          "Add durable asset action execution identity",
		CheckSQLite:   sqliteColumnCheck("asset_action_execution_logs", "durable_event_key"),
		CheckPostgres: pgColumnCheck("asset_action_execution_logs", "durable_event_key"),
		SQLite:        assetActionEventsSchema,
		Postgres:      assetActionEventsSchemaPostgres,
	},
	{
		Version:       "20260827_scm_connection_health",
		Name:          "Add durable SCM connection health snapshots",
		CheckSQLite:   sqliteTableCheck("scm_connection_health"),
		CheckPostgres: pgTableCheck("scm_connection_health"),
		SQLite: `
			CREATE TABLE scm_connection_health (
				workspace_scm_connection_id INTEGER NOT NULL,
				operation TEXT NOT NULL,
				last_attempt_at DATETIME NOT NULL,
				last_success_at DATETIME,
				last_failure_at DATETIME,
				consecutive_failures INTEGER NOT NULL DEFAULT 0,
				checked_resources INTEGER NOT NULL DEFAULT 0,
				failed_resources INTEGER NOT NULL DEFAULT 0,
				last_error TEXT,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace_scm_connection_id, operation),
				FOREIGN KEY (workspace_scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_scm_connection_health_failures
				ON scm_connection_health(consecutive_failures, last_failure_at);
		`,
		Postgres: `
			CREATE TABLE scm_connection_health (
				workspace_scm_connection_id INTEGER NOT NULL,
				operation TEXT NOT NULL,
				last_attempt_at TIMESTAMPTZ NOT NULL,
				last_success_at TIMESTAMPTZ,
				last_failure_at TIMESTAMPTZ,
				consecutive_failures INTEGER NOT NULL DEFAULT 0,
				checked_resources INTEGER NOT NULL DEFAULT 0,
				failed_resources INTEGER NOT NULL DEFAULT 0,
				last_error TEXT,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace_scm_connection_id, operation),
				FOREIGN KEY (workspace_scm_connection_id) REFERENCES workspace_scm_connections(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_scm_connection_health_failures
				ON scm_connection_health(consecutive_failures, last_failure_at);
		`,
	},
	{
		Version: "20260829_builtin_translation_keys",
		Name:    "Add immutable keys for localized built-in records",
		CheckSQLite: `SELECT CASE WHEN
			(SELECT COUNT(*) FROM pragma_table_info('configuration_sets') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('workflows') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('screens') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('notification_settings') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('item_types') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('hierarchy_levels') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('priorities') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('status_categories') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('statuses') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('workspace_roles') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('link_types') WHERE name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM pragma_table_info('themes') WHERE name='builtin_key') = 1
		THEN 1 ELSE 0 END`,
		CheckPostgres: `SELECT CASE WHEN
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='configuration_sets' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workflows' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='screens' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='notification_settings' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='item_types' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='hierarchy_levels' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='priorities' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='status_categories' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='statuses' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workspace_roles' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='link_types' AND column_name='builtin_key') = 1 AND
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='themes' AND column_name='builtin_key') = 1
		THEN 1 ELSE 0 END`,
		SQLite: `
			ALTER TABLE configuration_sets ADD COLUMN builtin_key TEXT;
			ALTER TABLE workflows ADD COLUMN builtin_key TEXT;
			ALTER TABLE screens ADD COLUMN builtin_key TEXT;
			ALTER TABLE notification_settings ADD COLUMN builtin_key TEXT;
			ALTER TABLE item_types ADD COLUMN builtin_key TEXT;
			ALTER TABLE hierarchy_levels ADD COLUMN builtin_key TEXT;
			ALTER TABLE priorities ADD COLUMN builtin_key TEXT;
			ALTER TABLE status_categories ADD COLUMN builtin_key TEXT;
			ALTER TABLE statuses ADD COLUMN builtin_key TEXT;
			ALTER TABLE workspace_roles ADD COLUMN builtin_key TEXT;
			ALTER TABLE link_types ADD COLUMN builtin_key TEXT;
			ALTER TABLE themes ADD COLUMN builtin_key TEXT;

			UPDATE configuration_sets SET builtin_key='default' WHERE name='Default Configuration' AND is_default=true;
			UPDATE workflows SET builtin_key='default' WHERE name='Default Workflow' AND is_default=true;
			UPDATE screens SET builtin_key='default' WHERE name='Default Screen';
			UPDATE notification_settings SET builtin_key='default' WHERE name='Default Notifications';
			UPDATE item_types SET builtin_key=CASE name
				WHEN 'Initiative' THEN 'initiative' WHEN 'Epic' THEN 'epic' WHEN 'Story' THEN 'story'
				WHEN 'Task' THEN 'task' WHEN 'Bug' THEN 'bug' WHEN 'Sub-task' THEN 'subtask' END
			WHERE is_default=true AND name IN ('Initiative','Epic','Story','Task','Bug','Sub-task');
			UPDATE hierarchy_levels SET builtin_key=CASE level
				WHEN 0 THEN 'initiative' WHEN 1 THEN 'epic' WHEN 2 THEN 'story'
				WHEN 3 THEN 'task' WHEN 4 THEN 'activity' END WHERE level BETWEEN 0 AND 4;
			UPDATE priorities SET builtin_key=LOWER(name) WHERE name IN ('Critical','High','Medium','Low');
			UPDATE status_categories SET builtin_key=CASE name
				WHEN 'To Do' THEN 'to_do' WHEN 'In Progress' THEN 'in_progress' WHEN 'Done' THEN 'done' END
			WHERE name IN ('To Do','In Progress','Done');
			UPDATE statuses SET builtin_key=CASE name
				WHEN 'Open' THEN 'open' WHEN 'In Progress' THEN 'in_progress' WHEN 'Done' THEN 'done' END
			WHERE name IN ('Open','In Progress','Done');
			UPDATE workspace_roles SET builtin_key=LOWER(name)
			WHERE is_system=true AND name IN ('Viewer','Editor','Administrator','Tester');
			UPDATE link_types SET builtin_key=CASE name
				WHEN 'Tests' THEN 'tests' WHEN 'Implements' THEN 'implements'
				WHEN 'Depends On' THEN 'depends_on' WHEN 'Relates To' THEN 'relates_to'
				WHEN 'Links To' THEN 'links_to' WHEN 'Duplicates' THEN 'duplicates'
				WHEN 'Child Of' THEN 'child_of' WHEN 'Page' THEN 'page' END
			WHERE is_system=true AND name IN ('Tests','Implements','Depends On','Relates To','Links To','Duplicates','Child Of','Page');
			UPDATE themes SET builtin_key='default'
			WHERE name='Default' AND description='Clean theme with standard navigation colors'
				AND nav_background_color_light='#ffffff' AND nav_text_color_light='#374151'
				AND nav_background_color_dark='#1f2937' AND nav_text_color_dark='#f3f4f6';
			UPDATE themes SET builtin_key='ocean'
			WHERE name='Ocean' AND description='Professional blue-tinted navigation theme'
				AND nav_background_color_light='#f0f9ff' AND nav_text_color_light='#0c4a6e'
				AND nav_background_color_dark='#0c4a6e' AND nav_text_color_dark='#e0f2fe';
			UPDATE themes SET builtin_key='forest'
			WHERE name='Forest' AND description='Nature-inspired green navigation theme'
				AND nav_background_color_light='#f0fdf4' AND nav_text_color_light='#14532d'
				AND nav_background_color_dark='#14532d' AND nav_text_color_dark='#dcfce7';

			CREATE UNIQUE INDEX uq_configuration_sets_builtin_key ON configuration_sets(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_workflows_builtin_key ON workflows(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_screens_builtin_key ON screens(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_notification_settings_builtin_key ON notification_settings(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_item_types_builtin_key ON item_types(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_hierarchy_levels_builtin_key ON hierarchy_levels(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_priorities_builtin_key ON priorities(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_status_categories_builtin_key ON status_categories(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_statuses_builtin_key ON statuses(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_workspace_roles_builtin_key ON workspace_roles(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_link_types_builtin_key ON link_types(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_themes_builtin_key ON themes(builtin_key) WHERE builtin_key IS NOT NULL;
		`,
		Postgres: `
			ALTER TABLE configuration_sets ADD COLUMN builtin_key TEXT;
			ALTER TABLE workflows ADD COLUMN builtin_key TEXT;
			ALTER TABLE screens ADD COLUMN builtin_key TEXT;
			ALTER TABLE notification_settings ADD COLUMN builtin_key TEXT;
			ALTER TABLE item_types ADD COLUMN builtin_key TEXT;
			ALTER TABLE hierarchy_levels ADD COLUMN builtin_key TEXT;
			ALTER TABLE priorities ADD COLUMN builtin_key TEXT;
			ALTER TABLE status_categories ADD COLUMN builtin_key TEXT;
			ALTER TABLE statuses ADD COLUMN builtin_key TEXT;
			ALTER TABLE workspace_roles ADD COLUMN builtin_key TEXT;
			ALTER TABLE link_types ADD COLUMN builtin_key TEXT;
			ALTER TABLE themes ADD COLUMN builtin_key TEXT;

			UPDATE configuration_sets SET builtin_key='default' WHERE name='Default Configuration' AND is_default=true;
			UPDATE workflows SET builtin_key='default' WHERE name='Default Workflow' AND is_default=true;
			UPDATE screens SET builtin_key='default' WHERE name='Default Screen';
			UPDATE notification_settings SET builtin_key='default' WHERE name='Default Notifications';
			UPDATE item_types SET builtin_key=CASE name
				WHEN 'Initiative' THEN 'initiative' WHEN 'Epic' THEN 'epic' WHEN 'Story' THEN 'story'
				WHEN 'Task' THEN 'task' WHEN 'Bug' THEN 'bug' WHEN 'Sub-task' THEN 'subtask' END
			WHERE is_default=true AND name IN ('Initiative','Epic','Story','Task','Bug','Sub-task');
			UPDATE hierarchy_levels SET builtin_key=CASE level
				WHEN 0 THEN 'initiative' WHEN 1 THEN 'epic' WHEN 2 THEN 'story'
				WHEN 3 THEN 'task' WHEN 4 THEN 'activity' END WHERE level BETWEEN 0 AND 4;
			UPDATE priorities SET builtin_key=LOWER(name) WHERE name IN ('Critical','High','Medium','Low');
			UPDATE status_categories SET builtin_key=CASE name
				WHEN 'To Do' THEN 'to_do' WHEN 'In Progress' THEN 'in_progress' WHEN 'Done' THEN 'done' END
			WHERE name IN ('To Do','In Progress','Done');
			UPDATE statuses SET builtin_key=CASE name
				WHEN 'Open' THEN 'open' WHEN 'In Progress' THEN 'in_progress' WHEN 'Done' THEN 'done' END
			WHERE name IN ('Open','In Progress','Done');
			UPDATE workspace_roles SET builtin_key=LOWER(name)
			WHERE is_system=true AND name IN ('Viewer','Editor','Administrator','Tester');
			UPDATE link_types SET builtin_key=CASE name
				WHEN 'Tests' THEN 'tests' WHEN 'Implements' THEN 'implements'
				WHEN 'Depends On' THEN 'depends_on' WHEN 'Relates To' THEN 'relates_to'
				WHEN 'Links To' THEN 'links_to' WHEN 'Duplicates' THEN 'duplicates'
				WHEN 'Child Of' THEN 'child_of' WHEN 'Page' THEN 'page' END
			WHERE is_system=true AND name IN ('Tests','Implements','Depends On','Relates To','Links To','Duplicates','Child Of','Page');
			UPDATE themes SET builtin_key='default'
			WHERE name='Default' AND description='Clean theme with standard navigation colors'
				AND nav_background_color_light='#ffffff' AND nav_text_color_light='#374151'
				AND nav_background_color_dark='#1f2937' AND nav_text_color_dark='#f3f4f6';
			UPDATE themes SET builtin_key='ocean'
			WHERE name='Ocean' AND description='Professional blue-tinted navigation theme'
				AND nav_background_color_light='#f0f9ff' AND nav_text_color_light='#0c4a6e'
				AND nav_background_color_dark='#0c4a6e' AND nav_text_color_dark='#e0f2fe';
			UPDATE themes SET builtin_key='forest'
			WHERE name='Forest' AND description='Nature-inspired green navigation theme'
				AND nav_background_color_light='#f0fdf4' AND nav_text_color_light='#14532d'
				AND nav_background_color_dark='#14532d' AND nav_text_color_dark='#dcfce7';

			CREATE UNIQUE INDEX uq_configuration_sets_builtin_key ON configuration_sets(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_workflows_builtin_key ON workflows(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_screens_builtin_key ON screens(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_notification_settings_builtin_key ON notification_settings(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_item_types_builtin_key ON item_types(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_hierarchy_levels_builtin_key ON hierarchy_levels(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_priorities_builtin_key ON priorities(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_status_categories_builtin_key ON status_categories(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_statuses_builtin_key ON statuses(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_workspace_roles_builtin_key ON workspace_roles(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_link_types_builtin_key ON link_types(builtin_key) WHERE builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_themes_builtin_key ON themes(builtin_key) WHERE builtin_key IS NOT NULL;
		`,
	},
	{
		Version:       "20260831_gitlab_release_metadata",
		Name:          "Add repository-scoped SCM release metadata",
		CheckSQLite:   sqliteColumnCheck("milestone_releases", "release_status"),
		CheckPostgres: pgColumnCheck("milestone_releases", "release_status"),
		SQLite: `
			ALTER TABLE milestone_releases ADD COLUMN workspace_repository_id INTEGER;
			ALTER TABLE milestone_releases ADD COLUMN tag_url TEXT;
			ALTER TABLE milestone_releases ADD COLUMN release_status TEXT NOT NULL DEFAULT 'tag_only';
			ALTER TABLE milestone_releases ADD COLUMN released_at DATETIME;
			ALTER TABLE milestone_releases ADD COLUMN assets_json TEXT NOT NULL DEFAULT '[]';
			ALTER TABLE milestone_releases ADD COLUMN last_synced_at DATETIME;
			CREATE UNIQUE INDEX uq_milestone_releases_repository_tag
				ON milestone_releases(workspace_repository_id, tag_name)
				WHERE workspace_repository_id IS NOT NULL;
		`,
		Postgres: `
			ALTER TABLE milestone_releases ADD COLUMN workspace_repository_id INTEGER;
			ALTER TABLE milestone_releases ADD COLUMN tag_url TEXT;
			ALTER TABLE milestone_releases ADD COLUMN release_status TEXT NOT NULL DEFAULT 'tag_only';
			ALTER TABLE milestone_releases ADD COLUMN released_at TIMESTAMPTZ;
			ALTER TABLE milestone_releases ADD COLUMN assets_json JSONB NOT NULL DEFAULT '[]'::jsonb;
			ALTER TABLE milestone_releases ADD COLUMN last_synced_at TIMESTAMPTZ;
			CREATE UNIQUE INDEX uq_milestone_releases_repository_tag
				ON milestone_releases(workspace_repository_id, tag_name)
				WHERE workspace_repository_id IS NOT NULL;
		`,
	},
	{
		Version:       "20260831_scm_webhook_ingress",
		Name:          "Add manual SCM webhook ingress",
		CheckSQLite:   "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scm_webhooks'",
		CheckPostgres: "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='scm_webhooks'",
		SQLite: `
			CREATE TABLE scm_webhooks (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				workspace_repository_id INTEGER NOT NULL UNIQUE REFERENCES workspace_repositories(id) ON DELETE CASCADE,
				webhook_key TEXT NOT NULL UNIQUE,
				webhook_external_id TEXT,
				webhook_secret_encrypted TEXT NOT NULL,
				events TEXT NOT NULL DEFAULT '["push","tag_push","merge_request","note","release"]',
				is_active BOOLEAN NOT NULL DEFAULT true,
				last_delivery_at DATETIME,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE scm_webhook_deliveries (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				scm_webhook_id INTEGER NOT NULL REFERENCES scm_webhooks(id) ON DELETE CASCADE,
				delivery_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				payload_summary TEXT,
				status TEXT NOT NULL DEFAULT 'pending',
				error_message TEXT,
				processing_time_ms INTEGER,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(scm_webhook_id, delivery_id)
			);
			CREATE INDEX idx_scm_webhook_deliveries_status ON scm_webhook_deliveries(status, created_at);
		`,
		Postgres: `
			CREATE TABLE scm_webhooks (
				id SERIAL PRIMARY KEY,
				workspace_repository_id INTEGER NOT NULL UNIQUE REFERENCES workspace_repositories(id) ON DELETE CASCADE,
				webhook_key TEXT NOT NULL UNIQUE,
				webhook_external_id TEXT,
				webhook_secret_encrypted TEXT NOT NULL,
				events JSONB NOT NULL DEFAULT '["push","tag_push","merge_request","note","release"]'::jsonb,
				is_active BOOLEAN NOT NULL DEFAULT true,
				last_delivery_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE scm_webhook_deliveries (
				id SERIAL PRIMARY KEY,
				scm_webhook_id INTEGER NOT NULL REFERENCES scm_webhooks(id) ON DELETE CASCADE,
				delivery_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				payload_summary JSONB,
				status TEXT NOT NULL DEFAULT 'pending',
				error_message TEXT,
				processing_time_ms INTEGER,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(scm_webhook_id, delivery_id)
			);
			CREATE INDEX idx_scm_webhook_deliveries_status ON scm_webhook_deliveries(status, created_at);
		`,
	},
	{
		Version:       "20260831_object_translations",
		Name:          "Add instance-wide configurable object translations",
		CheckSQLite:   "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='object_translations'",
		CheckPostgres: "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='object_translations'",
		SQLite:        objectTranslationsSchema,
		Postgres:      objectTranslationsSchemaPostgres,
	},
	{
		Version: "20260831_sso_email_verified_attribute_mapping",
		Name:    "Add the verified-email claim to the default SSO attribute mapping",
		CheckSQLite: `SELECT COUNT(*) FROM pragma_table_info('sso_providers')
			WHERE name='attribute_mapping' AND dflt_value LIKE '%"email_verified":"email_verified"%'`,
		CheckPostgres: `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema=current_schema() AND table_name='sso_providers'
				AND column_name='attribute_mapping' AND column_default LIKE '%"email_verified":"email_verified"%'`,
		SQLite:      "applySQLiteSSOAttributeMappingDefault:v1",
		Postgres:    `ALTER TABLE sso_providers ALTER COLUMN attribute_mapping SET DEFAULT '{"email":"email","name":"name","given_name":"given_name","family_name":"family_name","username":"preferred_username","email_verified":"email_verified"}'`,
		ApplySQLite: applySQLiteSSOAttributeMappingDefault,
	},
	{
		Version:       "20260829_zammad_integration",
		Name:          "Add Zammad connections and durable ticket links",
		CheckSQLite:   sqliteTableCheck("zammad_ticket_links"),
		CheckPostgres: pgTableCheck("zammad_ticket_links"),
		SQLite:        zammadSchemaMigrationSQLite,
		Postgres:      zammadSchemaMigrationPostgres,
	},
	{
		Version:       "20260830_zammad_oauth_connections",
		Name:          "Add connection-scoped Zammad OAuth credentials",
		CheckSQLite:   sqliteTableCheck("zammad_oauth_tokens"),
		CheckPostgres: pgTableCheck("zammad_oauth_tokens"),
		SQLite: `
			ALTER TABLE zammad_connections ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'api_token';
			CREATE TABLE zammad_oauth_tokens (
				provider_id TEXT PRIMARY KEY REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
				expires_at DATETIME NOT NULL,
				reauthorization_required BOOLEAN NOT NULL DEFAULT false, refresh_lock_until DATETIME, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE zammad_oauth_state (
				state TEXT PRIMARY KEY, provider_id TEXT NOT NULL REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
				initiated_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at DATETIME NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_zammad_oauth_state_expires ON zammad_oauth_state(expires_at);
		`,
		Postgres: `
			ALTER TABLE zammad_connections ADD COLUMN IF NOT EXISTS auth_method TEXT NOT NULL DEFAULT 'api_token';
			CREATE TABLE zammad_oauth_tokens (
				provider_id TEXT PRIMARY KEY REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
				expires_at TIMESTAMPTZ NOT NULL,
				reauthorization_required BOOLEAN NOT NULL DEFAULT false, refresh_lock_until TIMESTAMPTZ, updated_at TIMESTAMPTZ DEFAULT NOW()
			);
			CREATE TABLE zammad_oauth_state (
				state TEXT PRIMARY KEY, provider_id TEXT NOT NULL REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
				initiated_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ DEFAULT NOW()
			);
			CREATE INDEX idx_zammad_oauth_state_expires ON zammad_oauth_state(expires_at);
		`,
	},
	{
		Version:       "20260830_zammad_oauth_generation",
		Name:          "Bind Zammad OAuth writes to connection generations and refresh claims",
		CheckSQLite:   sqliteColumnCheck("zammad_connections", "oauth_generation"),
		CheckPostgres: pgColumnCheck("zammad_connections", "oauth_generation"),
		SQLite: `
			ALTER TABLE zammad_connections ADD COLUMN oauth_generation INTEGER NOT NULL DEFAULT 1;
			ALTER TABLE zammad_connections ADD COLUMN oauth_attempt_id TEXT;
			ALTER TABLE zammad_oauth_tokens ADD COLUMN oauth_generation INTEGER NOT NULL DEFAULT 1;
			ALTER TABLE zammad_oauth_tokens ADD COLUMN refresh_claim_owner TEXT;
			ALTER TABLE zammad_oauth_state ADD COLUMN oauth_generation INTEGER NOT NULL DEFAULT 1;
			DELETE FROM zammad_oauth_state;
			CREATE UNIQUE INDEX idx_zammad_oauth_state_provider ON zammad_oauth_state(provider_id);
		`,
		Postgres: `
			ALTER TABLE zammad_connections ADD COLUMN IF NOT EXISTS oauth_generation BIGINT NOT NULL DEFAULT 1;
			ALTER TABLE zammad_connections ADD COLUMN IF NOT EXISTS oauth_attempt_id TEXT;
			ALTER TABLE zammad_oauth_tokens ADD COLUMN IF NOT EXISTS oauth_generation BIGINT NOT NULL DEFAULT 1;
			ALTER TABLE zammad_oauth_tokens ADD COLUMN IF NOT EXISTS refresh_claim_owner TEXT;
			ALTER TABLE zammad_oauth_state ADD COLUMN IF NOT EXISTS oauth_generation BIGINT NOT NULL DEFAULT 1;
			DELETE FROM zammad_oauth_state;
			CREATE UNIQUE INDEX IF NOT EXISTS idx_zammad_oauth_state_provider ON zammad_oauth_state(provider_id);
		`,
	},
	{
		Version: "20260830_zammad_ticket_link_metadata",
		Name:    "Add Zammad ticket ownership and retry metadata",
		CheckSQLite: `
			SELECT CASE WHEN COUNT(*) = 4 THEN 1 ELSE 0 END
			FROM pragma_table_info('zammad_ticket_links')
			WHERE name IN ('owner_id', 'owner_name', 'last_attempt_at', 'next_attempt_at')
		`,
		CheckPostgres: `
			SELECT CASE WHEN COUNT(*) = 4 THEN 1 ELSE 0 END
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'zammad_ticket_links'
			  AND column_name IN ('owner_id', 'owner_name', 'last_attempt_at', 'next_attempt_at')
		`,
		SQLite: `
			ALTER TABLE zammad_ticket_links ADD COLUMN owner_id INTEGER;
			ALTER TABLE zammad_ticket_links ADD COLUMN owner_name TEXT DEFAULT '';
			ALTER TABLE zammad_ticket_links ADD COLUMN last_attempt_at DATETIME;
			ALTER TABLE zammad_ticket_links ADD COLUMN next_attempt_at DATETIME;
		`,
		Postgres: `
			ALTER TABLE zammad_ticket_links ADD COLUMN IF NOT EXISTS owner_id INTEGER;
			ALTER TABLE zammad_ticket_links ADD COLUMN IF NOT EXISTS owner_name TEXT DEFAULT '';
			ALTER TABLE zammad_ticket_links ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
			ALTER TABLE zammad_ticket_links ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;
		`,
	},
	{
		Version:       "20260830_zammad_ticket_link_completion_postgres",
		Name:          "Backfill missing PostgreSQL Zammad completion marker",
		CheckSQLite:   `SELECT 1`,
		CheckPostgres: pgColumnCheck("zammad_ticket_links", "completion_applied"),
		SQLite:        `SELECT 1;`,
		Postgres: `
			ALTER TABLE zammad_ticket_links ADD COLUMN IF NOT EXISTS completion_applied BOOLEAN NOT NULL DEFAULT false;
		`,
	},
	{
		Version:       "20260831_zammad_connection_config_revision",
		Name:          "Add optimistic revision for Zammad connection configuration",
		CheckSQLite:   sqliteColumnCheck("zammad_connections", "config_revision"),
		CheckPostgres: pgColumnCheck("zammad_connections", "config_revision"),
		SQLite: `
			ALTER TABLE zammad_connections ADD COLUMN config_revision INTEGER NOT NULL DEFAULT 1;
		`,
		Postgres: `
			ALTER TABLE zammad_connections ADD COLUMN IF NOT EXISTS config_revision BIGINT NOT NULL DEFAULT 1;
		`,
	},
	{
		Version:       "20260831_zammad_ticket_sync_lock_owner",
		Name:          "Bind Zammad ticket sync leases to their owner",
		CheckSQLite:   sqliteColumnCheck("zammad_ticket_links", "sync_lock_owner"),
		CheckPostgres: pgColumnCheck("zammad_ticket_links", "sync_lock_owner"),
		SQLite: `
			ALTER TABLE zammad_ticket_links ADD COLUMN sync_lock_owner TEXT;
		`,
		Postgres: `
			ALTER TABLE zammad_ticket_links ADD COLUMN IF NOT EXISTS sync_lock_owner TEXT;
		`,
	},
	{
		Version: "20260831_zammad_ticket_link_item_restrict",
		Name:    "Require Zammad ticket unlink before item deletion",
		CheckSQLite: `
			SELECT COUNT(*)
			FROM pragma_foreign_key_list('zammad_ticket_links')
			WHERE "table" = 'items' AND "from" = 'item_id' AND on_delete = 'RESTRICT'
		`,
		CheckPostgres: `
			SELECT COUNT(*)
			FROM information_schema.referential_constraints rc
			JOIN information_schema.key_column_usage kcu
			  ON kcu.constraint_schema = rc.constraint_schema
			 AND kcu.constraint_name = rc.constraint_name
			WHERE rc.constraint_schema = current_schema()
			  AND kcu.table_schema = current_schema()
			  AND kcu.table_name = 'zammad_ticket_links'
			  AND kcu.column_name = 'item_id'
			  AND rc.delete_rule = 'RESTRICT'
		`,
		SQLite: `
			CREATE TABLE zammad_ticket_links_item_restrict (
				id TEXT PRIMARY KEY,
				item_id INTEGER NOT NULL,
				provider_id TEXT NOT NULL,
				item_integration_link_id TEXT,
				ticket_id INTEGER,
				ticket_number TEXT DEFAULT '',
				ticket_url TEXT DEFAULT '',
				group_id INTEGER,
				group_name TEXT DEFAULT '',
				owner_id INTEGER,
				owner_name TEXT DEFAULT '',
				correlation_key TEXT NOT NULL,
				sync_state TEXT NOT NULL DEFAULT 'pending',
				creating_started_at DATETIME,
				last_status_id INTEGER,
				last_status_name TEXT DEFAULT '',
				last_synced_at DATETIME,
				last_attempt_at DATETIME,
				next_attempt_at DATETIME,
				last_error TEXT DEFAULT '',
				completion_applied BOOLEAN NOT NULL DEFAULT false,
				sync_lock_until DATETIME,
				sync_lock_owner TEXT,
				created_by INTEGER,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE RESTRICT,
				FOREIGN KEY (provider_id) REFERENCES zammad_connections(provider_id) ON DELETE CASCADE,
				FOREIGN KEY (item_integration_link_id) REFERENCES item_integration_links(id) ON DELETE SET NULL,
				FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
				UNIQUE(item_id, provider_id),
				UNIQUE(provider_id, ticket_id),
				UNIQUE(provider_id, correlation_key)
			);
			INSERT INTO zammad_ticket_links_item_restrict (
				id, item_id, provider_id, item_integration_link_id, ticket_id,
				ticket_number, ticket_url, group_id, group_name, owner_id,
				owner_name, correlation_key, sync_state, creating_started_at,
				last_status_id, last_status_name, last_synced_at, last_attempt_at,
				next_attempt_at, last_error, completion_applied, sync_lock_until,
				sync_lock_owner, created_by, created_at, updated_at
			)
			SELECT
				id, item_id, provider_id, item_integration_link_id, ticket_id,
				ticket_number, ticket_url, group_id, group_name, owner_id,
				owner_name, correlation_key, sync_state, creating_started_at,
				last_status_id, last_status_name, last_synced_at, last_attempt_at,
				next_attempt_at, last_error, completion_applied, sync_lock_until,
				sync_lock_owner, created_by, created_at, updated_at
			FROM zammad_ticket_links;
			DROP TABLE zammad_ticket_links;
			ALTER TABLE zammad_ticket_links_item_restrict RENAME TO zammad_ticket_links;
			CREATE INDEX idx_zammad_ticket_links_item ON zammad_ticket_links(item_id);
			CREATE INDEX idx_zammad_ticket_links_sync ON zammad_ticket_links(sync_state, last_synced_at);
		`,
		Postgres: `
			ALTER TABLE zammad_ticket_links
				DROP CONSTRAINT zammad_ticket_links_item_id_fkey;
			ALTER TABLE zammad_ticket_links
				ADD CONSTRAINT zammad_ticket_links_item_id_fkey
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE RESTRICT;
		`,
	},
	{
		Version:       "20260831_zammad_persisted_group_catalog",
		Name:          "Persist Zammad group names for least-privilege runtime access",
		CheckSQLite:   sqliteColumnCheck("zammad_connections", "allowed_groups"),
		CheckPostgres: pgColumnCheck("zammad_connections", "allowed_groups"),
		SQLite: `
			ALTER TABLE zammad_connections ADD COLUMN allowed_groups TEXT NOT NULL DEFAULT '[]';
			UPDATE zammad_connections
			SET allowed_groups = COALESCE((
				SELECT json_group_array(json_object(
					'id', CAST(group_id.value AS INTEGER),
					'name', CASE
						WHEN CAST(group_id.value AS INTEGER) = default_group_id THEN default_group_name
						ELSE ''
					END
				))
				FROM json_each(zammad_connections.allowed_group_ids) AS group_id
				WHERE group_id.type = 'integer' AND CAST(group_id.value AS INTEGER) > 0
			), '[]');
		`,
		Postgres: `
			ALTER TABLE zammad_connections ADD COLUMN IF NOT EXISTS allowed_groups TEXT NOT NULL DEFAULT '[]';
			UPDATE zammad_connections
			SET allowed_groups = COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'id', group_id.value::INTEGER,
					'name', CASE
						WHEN group_id.value::INTEGER = default_group_id THEN default_group_name
						ELSE ''
					END
				))::TEXT
				FROM jsonb_array_elements_text(zammad_connections.allowed_group_ids::jsonb) AS group_id(value)
				WHERE group_id.value ~ '^[1-9][0-9]*$'
			), '[]');
		`,
	},
	{
		Version: "20260901_zammad_canonical_workspace_scope",
		Name:    "Backfill managed-credential scope for existing Zammad connections",
		// A database with no Zammad connections has nothing to backfill, so
		// it counts as applied — including fresh installs whose schema files
		// already carry the final shape.
		CheckSQLite: `SELECT CASE WHEN
			NOT EXISTS (SELECT 1 FROM pragma_table_info('zammad_connections') WHERE name='applies_to_all_workspaces')
			OR NOT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='zammad_connection_workspaces')
			OR NOT EXISTS (SELECT 1 FROM zammad_connections LIMIT 1)
			THEN 1 ELSE 0 END`,
		CheckPostgres: `SELECT CASE WHEN
			NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='zammad_connections' AND column_name='applies_to_all_workspaces')
			OR NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='zammad_connection_workspaces')
			OR NOT EXISTS (SELECT 1 FROM zammad_connections)
			THEN 1 ELSE 0 END`,
		SQLite: `
			UPDATE action_credentials
			SET applies_to_all_workspaces = (
				SELECT zc.applies_to_all_workspaces FROM zammad_connections zc
				WHERE zc.credential_id = action_credentials.id
			)
			WHERE id IN (SELECT credential_id FROM zammad_connections)
			  AND json_extract(secret_metadata, '$._windshift_managed_credential') = 'v1'
			  AND json_extract(secret_metadata, '$.managed_by') = 'zammad'
			  AND json_extract(secret_metadata, '$.owner_id') = (
				SELECT zc.provider_id FROM zammad_connections zc
				WHERE zc.credential_id = action_credentials.id
			  );
			INSERT OR IGNORE INTO action_credential_workspaces (credential_id, workspace_id)
			SELECT zc.credential_id, zcw.workspace_id
			FROM zammad_connections zc
			JOIN zammad_connection_workspaces zcw ON zcw.provider_id = zc.provider_id
			JOIN action_credentials ac ON ac.id = zc.credential_id
			WHERE zc.applies_to_all_workspaces = false
			  AND json_extract(ac.secret_metadata, '$._windshift_managed_credential') = 'v1'
			  AND json_extract(ac.secret_metadata, '$.managed_by') = 'zammad'
			  AND json_extract(ac.secret_metadata, '$.owner_id') = zc.provider_id;
		`,
		Postgres: `
			UPDATE action_credentials ac
			SET applies_to_all_workspaces = zc.applies_to_all_workspaces
			FROM zammad_connections zc
			WHERE zc.credential_id = ac.id
			  AND ac.secret_metadata::jsonb ->> '_windshift_managed_credential' = 'v1'
			  AND ac.secret_metadata::jsonb ->> 'managed_by' = 'zammad'
			  AND ac.secret_metadata::jsonb ->> 'owner_id' = zc.provider_id;
			INSERT INTO action_credential_workspaces (credential_id, workspace_id)
			SELECT zc.credential_id, zcw.workspace_id
			FROM zammad_connections zc
			JOIN zammad_connection_workspaces zcw ON zcw.provider_id = zc.provider_id
			JOIN action_credentials ac ON ac.id = zc.credential_id
			WHERE zc.applies_to_all_workspaces = false
			  AND ac.secret_metadata::jsonb ->> '_windshift_managed_credential' = 'v1'
			  AND ac.secret_metadata::jsonb ->> 'managed_by' = 'zammad'
			  AND ac.secret_metadata::jsonb ->> 'owner_id' = zc.provider_id
				ON CONFLICT (credential_id, workspace_id) DO NOTHING;
		`,
	},
	{
		Version:       "20260901_daily_briefing_workspace_provenance",
		Name:          "Record daily briefing workspace provenance",
		CheckSQLite:   sqliteColumnCheck("daily_briefings", "source_workspace_ids"),
		CheckPostgres: pgColumnCheck("daily_briefings", "source_workspace_ids"),
		SQLite:        "ALTER TABLE daily_briefings ADD COLUMN source_workspace_ids TEXT",
		Postgres:      "ALTER TABLE daily_briefings ADD COLUMN source_workspace_ids TEXT",
	},
	{
		Version:       "20260905_asset_import_leases",
		Name:          "Fence asset import workers and recovery with expiring leases",
		CheckSQLite:   sqliteColumnCheck("import_jobs", "lease_expires_at"),
		CheckPostgres: pgColumnCheck("import_jobs", "lease_expires_at"),
		Superseded:    []string{"616606ef9da70200801ed9df1b8bc266f1b90ec9efe7e9d72e47ab4398beaf5a"},
		SQLite:        "ALTER TABLE import_jobs ADD COLUMN lease_expires_at BIGINT",
		Postgres:      "ALTER TABLE import_jobs ADD COLUMN lease_expires_at BIGINT",
	},
	{
		Version:       "20260905_asset_import_upload_ownership",
		Name:          "Bind asset import uploads to their uploader and set",
		CheckSQLite:   sqliteTableCheck("import_uploads"),
		CheckPostgres: pgTableCheck("import_uploads"),
		Superseded:    []string{"d64288c0cab5f1b271aad7f328535cb3abbae551f7e8e603cfbac6aca6cecf14"},
		SQLite: `CREATE TABLE IF NOT EXISTS import_uploads (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL DEFAULT 'asset',
 scope_id INTEGER NOT NULL,
 created_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at BIGINT NOT NULL
);`,
		Postgres: `CREATE TABLE IF NOT EXISTS import_uploads (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL DEFAULT 'asset',
 scope_id INTEGER NOT NULL,
 created_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at BIGINT NOT NULL
);`,
	},
	{
		Version:       "20260905_notification_reference_provenance",
		Name:          "Preserve referenced notification authorization",
		CheckSQLite:   sqliteColumnCheck("notifications", "referenced_entity_type"),
		CheckPostgres: pgColumnCheck("notifications", "referenced_entity_type"),
		SQLite: `
			ALTER TABLE notifications ADD COLUMN referenced_entity_type TEXT;
			ALTER TABLE notifications ADD COLUMN referenced_entity_id INTEGER;
			ALTER TABLE notifications ADD COLUMN referenced_workspace_id INTEGER;
			ALTER TABLE notifications ADD COLUMN referenced_workspace_permission TEXT;
		`,
		Postgres: `
			ALTER TABLE notifications ADD COLUMN referenced_entity_type TEXT;
			ALTER TABLE notifications ADD COLUMN referenced_entity_id INTEGER;
			ALTER TABLE notifications ADD COLUMN referenced_workspace_id INTEGER;
			ALTER TABLE notifications ADD COLUMN referenced_workspace_permission TEXT;
		`,
	},
	{
		Version:       "20260911_users_offboarded_at",
		Name:          "Add irreversible offboarded lifecycle state to users",
		CheckSQLite:   sqliteColumnCheck("users", "offboarded_at"),
		CheckPostgres: pgColumnCheck("users", "offboarded_at"),
		SQLite:        `ALTER TABLE users ADD COLUMN offboarded_at DATETIME`,
		Postgres:      `ALTER TABLE users ADD COLUMN IF NOT EXISTS offboarded_at TIMESTAMPTZ`,
	},
	{
		Version:       "20260911_users_scim_deleted_at",
		Name:          "Add hidden SCIM deprovisioning tombstone state to users",
		CheckSQLite:   sqliteColumnCheck("users", "scim_deleted_at"),
		CheckPostgres: pgColumnCheck("users", "scim_deleted_at"),
		SQLite:        `ALTER TABLE users ADD COLUMN scim_deleted_at DATETIME`,
		Postgres:      `ALTER TABLE users ADD COLUMN IF NOT EXISTS scim_deleted_at TIMESTAMPTZ`,
	},
	{
		Version:       "20260914_users_erased_at",
		Name:          "Add irreversible Article 17 erasure state to users (WI-1307)",
		CheckSQLite:   sqliteColumnCheck("users", "erased_at"),
		CheckPostgres: pgColumnCheck("users", "erased_at"),
		SQLite:        `ALTER TABLE users ADD COLUMN erased_at DATETIME`,
		Postgres:      `ALTER TABLE users ADD COLUMN IF NOT EXISTS erased_at TIMESTAMPTZ`,
	},
	{
		Version:       "20260914_user_erasure_records",
		Name:          "Add DSAR erasure completion evidence table (WI-1307)",
		CheckSQLite:   sqliteTableCheck("user_erasure_records"),
		CheckPostgres: pgTableCheck("user_erasure_records"),
		SQLite: `
			CREATE TABLE IF NOT EXISTS user_erasure_records (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				requested_by TEXT NOT NULL,
				requested_at DATETIME NOT NULL,
				approved_by INTEGER NOT NULL,
				executed_at DATETIME NOT NULL,
				policy_version TEXT NOT NULL,
				notes TEXT,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_user_erasure_records_user_id ON user_erasure_records(user_id);
		`,
		Postgres: `
			CREATE TABLE IF NOT EXISTS user_erasure_records (
				id SERIAL PRIMARY KEY,
				user_id INTEGER NOT NULL,
				requested_by TEXT NOT NULL,
				requested_at TIMESTAMPTZ NOT NULL,
				approved_by INTEGER NOT NULL,
				executed_at TIMESTAMPTZ NOT NULL,
				policy_version TEXT NOT NULL,
				notes TEXT,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_user_erasure_records_user_id ON user_erasure_records(user_id);
		`,
	},
	{
		Version:       "20260905_notification_email_claims",
		Name:          "Add recoverable notification email claims",
		CheckSQLite:   sqliteColumnCheck("notifications", "email_delivery_state"),
		CheckPostgres: pgColumnCheck("notifications", "email_delivery_state"),
		SQLite: `
			ALTER TABLE notifications ADD COLUMN email_delivery_state TEXT NOT NULL DEFAULT 'pending';
			ALTER TABLE notifications ADD COLUMN email_claim_owner TEXT;
			ALTER TABLE notifications ADD COLUMN email_claim_token TEXT;
			ALTER TABLE notifications ADD COLUMN email_claim_expires_at DATETIME;
			ALTER TABLE notifications ADD COLUMN email_delivery_attempts INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE notifications ADD COLUMN email_last_error TEXT;
			CREATE INDEX idx_notifications_email_delivery ON notifications(email_delivery_state, email_claim_expires_at);
		`,
		Postgres: `
			ALTER TABLE notifications ADD COLUMN email_delivery_state TEXT NOT NULL DEFAULT 'pending';
			ALTER TABLE notifications ADD COLUMN email_claim_owner TEXT;
			ALTER TABLE notifications ADD COLUMN email_claim_token TEXT;
			ALTER TABLE notifications ADD COLUMN email_claim_expires_at TIMESTAMPTZ;
			ALTER TABLE notifications ADD COLUMN email_delivery_attempts INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE notifications ADD COLUMN email_last_error TEXT;
			CREATE INDEX idx_notifications_email_delivery ON notifications(email_delivery_state, email_claim_expires_at);
		`,
	},
	{
		Version:       "20260905_notification_delivery_keys",
		Name:          "Deduplicate durable notification deliveries",
		CheckSQLite:   sqliteIndexCheck("uq_notifications_delivery_key"),
		CheckPostgres: pgIndexCheck("uq_notifications_delivery_key"),
		SQLite: `
			ALTER TABLE notifications ADD COLUMN delivery_key TEXT;
			CREATE UNIQUE INDEX uq_notifications_delivery_key ON notifications(delivery_key) WHERE delivery_key IS NOT NULL;
		`,
		Postgres: `
			ALTER TABLE notifications ADD COLUMN delivery_key TEXT;
			CREATE UNIQUE INDEX uq_notifications_delivery_key ON notifications(delivery_key) WHERE delivery_key IS NOT NULL;
		`,
	},
	{
		Version: "20260911_theme_logo_url",
		Name:    "Add an optional company logo URL to themes",
		CheckSQLite: `SELECT CASE WHEN
			(SELECT COUNT(*) FROM pragma_table_info('themes') WHERE name='logo_url') = 1
		THEN 1 ELSE 0 END`,
		CheckPostgres: `SELECT CASE WHEN
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='themes' AND column_name='logo_url') = 1
		THEN 1 ELSE 0 END`,
		SQLite: `
			ALTER TABLE themes ADD COLUMN logo_url TEXT;
		`,
		Postgres: `
			ALTER TABLE themes ADD COLUMN logo_url TEXT;
		`,
	},
	{
		Version:       "20260911_bdd_test_case_format",
		Name:          "Add BDD test case format storage",
		CheckSQLite:   sqliteColumnCheck("test_cases", "format"),
		CheckPostgres: pgColumnCheck("test_cases", "format"),
		SQLite: `
			ALTER TABLE test_cases ADD COLUMN format TEXT NOT NULL DEFAULT 'steps';
			CREATE TABLE IF NOT EXISTS test_case_bdd (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				test_case_id INTEGER NOT NULL UNIQUE,
				gherkin TEXT NOT NULL,
				feature_name TEXT NOT NULL DEFAULT '',
				scenario_keyword TEXT NOT NULL DEFAULT 'Scenario',
				spec TEXT NOT NULL DEFAULT '{}',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE
			);
			CREATE TABLE IF NOT EXISTS test_run_case_snapshots (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				run_id INTEGER NOT NULL,
				test_case_id INTEGER NOT NULL,
				title TEXT NOT NULL,
				preconditions TEXT DEFAULT '',
				gherkin TEXT NOT NULL,
				spec TEXT NOT NULL DEFAULT '{}',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE,
				FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
				UNIQUE(run_id, test_case_id)
			);
			CREATE TABLE IF NOT EXISTS test_example_results (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				run_id INTEGER NOT NULL,
				test_case_id INTEGER NOT NULL,
				example_index INTEGER NOT NULL,
				row_values TEXT NOT NULL DEFAULT '{}',
				status TEXT NOT NULL DEFAULT 'not_run',
				actual_result TEXT DEFAULT '',
				notes TEXT DEFAULT '',
				executed_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE,
				FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
				UNIQUE(run_id, test_case_id, example_index)
			);
			CREATE TABLE IF NOT EXISTS test_example_step_results (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				example_result_id INTEGER NOT NULL,
				step_number INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'not_run',
				actual_result TEXT DEFAULT '',
				notes TEXT DEFAULT '',
				item_id INTEGER,
				executed_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (example_result_id) REFERENCES test_example_results(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
				UNIQUE(example_result_id, step_number)
			);
			CREATE INDEX IF NOT EXISTS idx_test_example_results_run_id ON test_example_results(run_id);
			CREATE INDEX IF NOT EXISTS idx_test_example_step_results_result_id ON test_example_step_results(example_result_id);
		`,
		Postgres: `
			ALTER TABLE test_cases ADD COLUMN format TEXT NOT NULL DEFAULT 'steps';
			CREATE TABLE IF NOT EXISTS test_case_bdd (
				id SERIAL PRIMARY KEY,
				test_case_id INTEGER NOT NULL UNIQUE,
				gherkin TEXT NOT NULL,
				feature_name TEXT NOT NULL DEFAULT '',
				scenario_keyword TEXT NOT NULL DEFAULT 'Scenario',
				spec TEXT NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE
			);
			CREATE TABLE IF NOT EXISTS test_run_case_snapshots (
				id SERIAL PRIMARY KEY,
				run_id INTEGER NOT NULL,
				test_case_id INTEGER NOT NULL,
				title TEXT NOT NULL,
				preconditions TEXT DEFAULT '',
				gherkin TEXT NOT NULL,
				spec TEXT NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE,
				FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
				UNIQUE(run_id, test_case_id)
			);
			CREATE TABLE IF NOT EXISTS test_example_results (
				id SERIAL PRIMARY KEY,
				run_id INTEGER NOT NULL,
				test_case_id INTEGER NOT NULL,
				example_index INTEGER NOT NULL,
				row_values TEXT NOT NULL DEFAULT '{}',
				status TEXT NOT NULL DEFAULT 'not_run',
				actual_result TEXT DEFAULT '',
				notes TEXT DEFAULT '',
				executed_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE,
				FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
				UNIQUE(run_id, test_case_id, example_index)
			);
			CREATE TABLE IF NOT EXISTS test_example_step_results (
				id SERIAL PRIMARY KEY,
				example_result_id INTEGER NOT NULL,
				step_number INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'not_run',
				actual_result TEXT DEFAULT '',
				notes TEXT DEFAULT '',
				item_id INTEGER,
				executed_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (example_result_id) REFERENCES test_example_results(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
				UNIQUE(example_result_id, step_number)
			);
			CREATE INDEX IF NOT EXISTS idx_test_example_results_run_id ON test_example_results(run_id);
			CREATE INDEX IF NOT EXISTS idx_test_example_step_results_result_id ON test_example_step_results(example_result_id);
		`,
	},
	{
		Version:       "20260918_workspaces_personal_owner_uniqueness",
		Name:          "Enforce one personal workspace per owner",
		CheckSQLite:   sqliteIndexCheck("uq_workspaces_personal_owner"),
		CheckPostgres: pgIndexCheck("uq_workspaces_personal_owner"),
		SQLite: `
			-- Duplicates from the racy get-or-create keep their oldest row; the
			-- rest become inactive regular workspaces so no data is lost.
			UPDATE workspaces SET is_personal = FALSE, active = FALSE
			WHERE is_personal = TRUE AND id NOT IN (
				SELECT MIN(id) FROM workspaces WHERE is_personal = TRUE GROUP BY owner_id
			);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_workspaces_personal_owner ON workspaces(owner_id) WHERE is_personal = TRUE;
		`,
		Postgres: `
			UPDATE workspaces SET is_personal = false, active = false
			WHERE is_personal = true AND id NOT IN (
				SELECT MIN(id) FROM workspaces WHERE is_personal = true GROUP BY owner_id
			);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_workspaces_personal_owner ON workspaces(owner_id) WHERE is_personal = true;
		`,
	},
	{
		Version:       "20260918_personal_labels_unique_per_user",
		Name:          "Scope personal label name uniqueness to the owning user",
		CheckSQLite:   sqliteIndexCheck("uq_personal_labels_user_name"),
		CheckPostgres: pgIndexCheck("uq_personal_labels_user_name"),
		Postgres: `
			ALTER TABLE personal_labels DROP CONSTRAINT IF EXISTS personal_labels_name_key;
			CREATE UNIQUE INDEX IF NOT EXISTS uq_personal_labels_user_name
				ON personal_labels(COALESCE(user_id, 0), name);
		`,
		// SQLite cannot drop the inline UNIQUE(name) without a table rebuild.
		// The SQL field must stay non-empty: an empty body makes the runner
		// stamp the row without reaching Check or ApplySQLite.
		SQLite:      "applySQLitePersonalLabelsPerUserUnique:v1",
		ApplySQLite: applySQLitePersonalLabelsPerUserUnique,
		// Databases stamped while the SQLite field was empty carry the
		// empty-body checksum (sha256 of ""); advance them to the marker
		// checksum. Their rebuild still requires re-running the migration,
		// which the runner does not do for stamped rows.
		Superseded: []string{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	},
	{
		Version:       "20260918_kb_events",
		Name:          "Add portal knowledge-base usage-signal event log",
		CheckSQLite:   sqliteTableCheck("kb_events"),
		CheckPostgres: pgTableCheck("kb_events"),
		SQLite: `
			CREATE TABLE kb_events (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				channel_id INTEGER NOT NULL,
				portal_customer_id INTEGER,
				event_type TEXT NOT NULL,
				page_id INTEGER,
				workspace_id INTEGER,
				source TEXT,
				query TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
				FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE SET NULL,
				FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE SET NULL,
				CHECK (event_type IN ('search', 'no_result', 'view', 'deflection'))
			);
			CREATE INDEX idx_kb_events_channel_created ON kb_events(channel_id, created_at);
			CREATE INDEX idx_kb_events_type_created ON kb_events(event_type, created_at);
			CREATE INDEX idx_kb_events_customer_created ON kb_events(portal_customer_id, created_at);
		`,
		Postgres: `
			CREATE TABLE kb_events (
				id SERIAL PRIMARY KEY,
				channel_id INTEGER NOT NULL,
				portal_customer_id INTEGER,
				event_type TEXT NOT NULL,
				page_id INTEGER,
				workspace_id INTEGER,
				source TEXT,
				query TEXT,
				created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
				FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE SET NULL,
				FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE SET NULL,
				CHECK (event_type IN ('search', 'no_result', 'view', 'deflection'))
			);
			CREATE INDEX idx_kb_events_channel_created ON kb_events(channel_id, created_at);
			CREATE INDEX idx_kb_events_type_created ON kb_events(event_type, created_at);
			CREATE INDEX idx_kb_events_customer_created ON kb_events(portal_customer_id, created_at);
		`,
	},
	{
		Version:       "20260923_email_intake_rate_limit",
		Name:          "Track rate-limited inbound email for flood recovery",
		CheckSQLite:   sqliteColumnCheck("email_message_tracking", "rate_limited_at"),
		CheckPostgres: pgColumnCheck("email_message_tracking", "rate_limited_at"),
		SQLite: `
			ALTER TABLE email_message_tracking ADD COLUMN uid INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE email_message_tracking ADD COLUMN uid_validity INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE email_message_tracking ADD COLUMN rate_limited_at DATETIME;
		`,
		Postgres: `
			ALTER TABLE email_message_tracking ADD COLUMN IF NOT EXISTS uid BIGINT NOT NULL DEFAULT 0;
			ALTER TABLE email_message_tracking ADD COLUMN IF NOT EXISTS uid_validity BIGINT NOT NULL DEFAULT 0;
			ALTER TABLE email_message_tracking ADD COLUMN IF NOT EXISTS rate_limited_at TIMESTAMPTZ;
		`,
	},
	{
		Version:       "20260923_email_reply_outbox_discarded_at",
		Name:          "Let operators discard stuck outbound email replies",
		CheckSQLite:   sqliteColumnCheck("email_reply_outbox", "discarded_at"),
		CheckPostgres: pgColumnCheck("email_reply_outbox", "discarded_at"),
		SQLite: `
			ALTER TABLE email_reply_outbox ADD COLUMN discarded_at DATETIME;
		`,
		Postgres: `
			ALTER TABLE email_reply_outbox ADD COLUMN IF NOT EXISTS discarded_at TIMESTAMPTZ;
		`,
	},
	{
		Version:       "20260924_theme_dark_logo_url",
		Name:          "Add an optional dark-mode company logo URL to themes",
		CheckSQLite:   sqliteColumnCheck("themes", "logo_url_dark"),
		CheckPostgres: pgColumnCheck("themes", "logo_url_dark"),
		SQLite: `
			ALTER TABLE themes ADD COLUMN logo_url_dark TEXT;
		`,
		Postgres: `
			ALTER TABLE themes ADD COLUMN IF NOT EXISTS logo_url_dark TEXT;
		`,
	},
	{
		Version:       "20260924_items_merged_into",
		Name:          "Point merged duplicate tickets at their canonical item",
		CheckSQLite:   sqliteColumnCheck("items", "merged_into_item_id"),
		CheckPostgres: pgColumnCheck("items", "merged_into_item_id"),
		SQLite: `
			ALTER TABLE items ADD COLUMN merged_into_item_id INTEGER REFERENCES items(id) ON DELETE SET NULL;
		`,
		Postgres: `
			ALTER TABLE items ADD COLUMN IF NOT EXISTS merged_into_item_id BIGINT REFERENCES items(id) ON DELETE SET NULL;
		`,
	},
	{
		Version:       "20260924_item_import_rows",
		Name:          "Track ticket CSV import rows for retry idempotency",
		CheckSQLite:   sqliteTableCheck("item_import_rows"),
		CheckPostgres: pgTableCheck("item_import_rows"),
		SQLite: `
			CREATE TABLE item_import_rows (
				workspace_id INTEGER NOT NULL,
				external_ref TEXT NOT NULL,
				item_id INTEGER NOT NULL,
				job_id TEXT NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace_id, external_ref),
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
			);
		`,
		Postgres: `
			CREATE TABLE item_import_rows (
				workspace_id INTEGER NOT NULL,
				external_ref TEXT NOT NULL,
				item_id INTEGER NOT NULL,
				job_id TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (workspace_id, external_ref),
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
			);
		`,
	},
	{
		Version:       "20260925_items_team",
		Name:          "Assign a team to work items",
		CheckSQLite:   sqliteColumnCheck("items", "team_id"),
		CheckPostgres: pgColumnCheck("items", "team_id"),
		SQLite: `
			ALTER TABLE items ADD COLUMN team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL;
			CREATE INDEX IF NOT EXISTS idx_items_team_id ON items(team_id);
		`,
		Postgres: `
			ALTER TABLE items ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL;
			CREATE INDEX IF NOT EXISTS idx_items_team_id ON items(team_id);
		`,
	},
	{
		// Carries the on-call incident history forward from 0.8.9 (WI-1535):
		// rows archive to on_call_incidents_archive and the latest open
		// incident per item migrates into the new model. ReconcileChecksum
		// restamps installs that already ran the original destructive body.
		Version:           "20260925_incidents",
		Name:              "Model incidents as pager state on work items",
		CheckSQLite:       sqliteTableCheck("incidents"),
		CheckPostgres:     pgTableCheck("incidents"),
		ReconcileChecksum: true,
		SQLite: `
			CREATE TABLE incidents (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				item_id INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'triggered',
				urgency TEXT NOT NULL DEFAULT 'high',
				source TEXT NOT NULL DEFAULT 'manual',
				escalation_policy_id INTEGER,
				triggered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				acknowledged_at DATETIME,
				acknowledged_by INTEGER,
				resolved_at DATETIME,
				resolved_by INTEGER,
				escalation_step INTEGER NOT NULL DEFAULT 0,
				escalation_repeat_count INTEGER NOT NULL DEFAULT 0,
				next_escalation_at DATETIME,
				dedup_key TEXT,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
				FOREIGN KEY (escalation_policy_id) REFERENCES on_call_escalation_policies(id) ON DELETE SET NULL,
				FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL,
				FOREIGN KEY (resolved_by) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE INDEX idx_incidents_item_id ON incidents(item_id);
			CREATE INDEX idx_incidents_status ON incidents(status);
			CREATE INDEX idx_incidents_escalation_policy_id ON incidents(escalation_policy_id);
			CREATE INDEX idx_incidents_next_escalation ON incidents(next_escalation_at) WHERE status = 'triggered';
			CREATE UNIQUE INDEX uq_incidents_open_item ON incidents(item_id) WHERE status = 'triggered';
			ALTER TABLE items ADD COLUMN incident_id INTEGER REFERENCES incidents(id) ON DELETE SET NULL;
			CREATE UNIQUE INDEX uq_items_incident ON items(incident_id) WHERE incident_id IS NOT NULL;
			CREATE TABLE on_call_incidents_archive (
				id INTEGER PRIMARY KEY,
				escalation_policy_id INTEGER,
				item_id INTEGER,
				status TEXT,
				triggered_at DATETIME,
				acknowledged_at DATETIME,
				acknowledged_by INTEGER,
				resolved_at DATETIME,
				resolved_by INTEGER,
				current_escalation_step INTEGER,
				escalation_repeat_count INTEGER,
				created_at DATETIME
			);
			INSERT INTO on_call_incidents_archive
				SELECT id, escalation_policy_id, item_id, status, triggered_at, acknowledged_at,
					acknowledged_by, resolved_at, resolved_by, current_escalation_step,
					escalation_repeat_count, created_at
				FROM on_call_incidents;
			INSERT INTO incidents (item_id, status, urgency, source, escalation_policy_id,
					triggered_at, acknowledged_at, acknowledged_by, escalation_step,
					escalation_repeat_count, next_escalation_at, created_at, updated_at)
			SELECT o.item_id, o.status, 'high', 'manual',
				CASE WHEN EXISTS (
					SELECT 1 FROM on_call_escalation_policies p WHERE p.id = o.escalation_policy_id
				) THEN o.escalation_policy_id END,
				o.triggered_at, o.acknowledged_at, o.acknowledged_by,
				COALESCE(o.current_escalation_step, 0), COALESCE(o.escalation_repeat_count, 0),
				CASE WHEN o.status = 'triggered' THEN CURRENT_TIMESTAMP END,
				o.created_at, CURRENT_TIMESTAMP
			FROM on_call_incidents o
			WHERE o.item_id IS NOT NULL AND o.status IN ('triggered', 'acknowledged')
				AND NOT EXISTS (
					SELECT 1 FROM on_call_incidents o2
					WHERE o2.item_id = o.item_id AND o2.status IN ('triggered', 'acknowledged')
						AND o2.id > o.id
				);
			UPDATE items SET incident_id = (
				SELECT i.id FROM incidents i
				WHERE i.item_id = items.id AND i.status != 'resolved'
				ORDER BY i.id DESC LIMIT 1
			), updated_at = CURRENT_TIMESTAMP
			WHERE EXISTS (
				SELECT 1 FROM incidents i WHERE i.item_id = items.id
			);
			DROP TABLE IF EXISTS on_call_incidents;
		`,
		Postgres: `
			CREATE TABLE incidents (
				id SERIAL PRIMARY KEY,
				item_id INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'triggered',
				urgency TEXT NOT NULL DEFAULT 'high',
				source TEXT NOT NULL DEFAULT 'manual',
				escalation_policy_id INTEGER,
				triggered_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				acknowledged_at TIMESTAMPTZ,
				acknowledged_by INTEGER,
				resolved_at TIMESTAMPTZ,
				resolved_by INTEGER,
				escalation_step INTEGER NOT NULL DEFAULT 0,
				escalation_repeat_count INTEGER NOT NULL DEFAULT 0,
				next_escalation_at TIMESTAMPTZ,
				dedup_key TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
				FOREIGN KEY (escalation_policy_id) REFERENCES on_call_escalation_policies(id) ON DELETE SET NULL,
				FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL,
				FOREIGN KEY (resolved_by) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE INDEX IF NOT EXISTS idx_incidents_item_id ON incidents(item_id);
			CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);
			CREATE INDEX IF NOT EXISTS idx_incidents_escalation_policy_id ON incidents(escalation_policy_id);
			CREATE INDEX IF NOT EXISTS idx_incidents_next_escalation ON incidents(next_escalation_at) WHERE status = 'triggered';
			CREATE UNIQUE INDEX IF NOT EXISTS uq_incidents_open_item ON incidents(item_id) WHERE status = 'triggered';
			ALTER TABLE items ADD COLUMN IF NOT EXISTS incident_id INTEGER REFERENCES incidents(id) ON DELETE SET NULL;
			CREATE UNIQUE INDEX IF NOT EXISTS uq_items_incident ON items(incident_id) WHERE incident_id IS NOT NULL;
			CREATE TABLE on_call_incidents_archive (
				id INTEGER PRIMARY KEY,
				escalation_policy_id INTEGER,
				item_id INTEGER,
				status TEXT,
				triggered_at TIMESTAMPTZ,
				acknowledged_at TIMESTAMPTZ,
				acknowledged_by INTEGER,
				resolved_at TIMESTAMPTZ,
				resolved_by INTEGER,
				current_escalation_step INTEGER,
				escalation_repeat_count INTEGER,
				created_at TIMESTAMPTZ
			);
			INSERT INTO on_call_incidents_archive
				SELECT id, escalation_policy_id, item_id, status, triggered_at, acknowledged_at,
					acknowledged_by, resolved_at, resolved_by, current_escalation_step,
					escalation_repeat_count, created_at
				FROM on_call_incidents;
			INSERT INTO incidents (item_id, status, urgency, source, escalation_policy_id,
					triggered_at, acknowledged_at, acknowledged_by, escalation_step,
					escalation_repeat_count, next_escalation_at, created_at, updated_at)
			SELECT o.item_id, o.status, 'high', 'manual',
				CASE WHEN EXISTS (
					SELECT 1 FROM on_call_escalation_policies p WHERE p.id = o.escalation_policy_id
				) THEN o.escalation_policy_id END,
				o.triggered_at, o.acknowledged_at, o.acknowledged_by,
				COALESCE(o.current_escalation_step, 0), COALESCE(o.escalation_repeat_count, 0),
				CASE WHEN o.status = 'triggered' THEN CURRENT_TIMESTAMP END,
				o.created_at, CURRENT_TIMESTAMP
			FROM on_call_incidents o
			WHERE o.item_id IS NOT NULL AND o.status IN ('triggered', 'acknowledged')
				AND NOT EXISTS (
					SELECT 1 FROM on_call_incidents o2
					WHERE o2.item_id = o.item_id AND o2.status IN ('triggered', 'acknowledged')
						AND o2.id > o.id
				);
			UPDATE items SET incident_id = (
				SELECT i.id FROM incidents i
				WHERE i.item_id = items.id AND i.status != 'resolved'
				ORDER BY i.id DESC LIMIT 1
			), updated_at = CURRENT_TIMESTAMP
			WHERE EXISTS (
				SELECT 1 FROM incidents i WHERE i.item_id = items.id
			);
			DROP TABLE IF EXISTS on_call_incidents;
		`,
	},
	{
		Version:       "20260926_incident_notification_state",
		Name:          "Schedule delayed and repeated incident notifications",
		CheckSQLite:   sqliteTableCheck("incident_notification_state"),
		CheckPostgres: pgTableCheck("incident_notification_state"),
		SQLite: `
			CREATE TABLE incident_notification_state (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				incident_id INTEGER NOT NULL,
				escalation_rule_id INTEGER NOT NULL,
				notification_rule_id INTEGER NOT NULL,
				repeat_index INTEGER NOT NULL DEFAULT 0,
				next_notification_at DATETIME NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE CASCADE,
				FOREIGN KEY (notification_rule_id) REFERENCES on_call_notification_rules(id) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX uq_incident_notification_state ON incident_notification_state(incident_id, notification_rule_id, repeat_index);
			CREATE INDEX idx_incident_notification_state_due ON incident_notification_state(next_notification_at);
		`,
		Postgres: `
			CREATE TABLE incident_notification_state (
				id SERIAL PRIMARY KEY,
				incident_id INTEGER NOT NULL,
				escalation_rule_id INTEGER NOT NULL,
				notification_rule_id INTEGER NOT NULL,
				repeat_index INTEGER NOT NULL DEFAULT 0,
				next_notification_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE CASCADE,
				FOREIGN KEY (notification_rule_id) REFERENCES on_call_notification_rules(id) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_incident_notification_state ON incident_notification_state(incident_id, notification_rule_id, repeat_index);
			CREATE INDEX IF NOT EXISTS idx_incident_notification_state_due ON incident_notification_state(next_notification_at);
		`,
	},
	{
		Version:       "20260927_asset_set_portal_access",
		Name:          "Add per-set portal access grants for asset sets",
		CheckSQLite:   sqliteTableCheck("asset_set_portal_access"),
		CheckPostgres: pgTableCheck("asset_set_portal_access"),
		SQLite: `
			CREATE TABLE IF NOT EXISTS asset_set_portal_access (
				set_id INTEGER PRIMARY KEY,
				granted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
				granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE
			);
		`,
		Postgres: `
			CREATE TABLE IF NOT EXISTS asset_set_portal_access (
				set_id INTEGER PRIMARY KEY REFERENCES asset_management_sets(id) ON DELETE CASCADE,
				granted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
				granted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
			);
		`,
	},
	{
		Version:       "20260928_sla_engine",
		Name:          "Add SLA calendars, metrics, goals, cycles, and jobs",
		CheckSQLite:   sqliteTableCheck("sla_metrics"),
		CheckPostgres: pgTableCheck("sla_metrics"),
		SQLite:        slaSchema,
		Postgres:      slaSchemaPostgres,
		// The canonical schema was corrected in place (BIGINT durations and
		// hot-path indexes) while this migration was unreleased, so advance the
		// checksum instead of failing databases stamped with the earlier body.
		ReconcileChecksum: true,
	},
	{
		Version:       "20260929_sla_warning_thresholds",
		Name:          "Add SLA warning thresholds",
		CheckSQLite:   sqliteTableCheck("sla_warning_thresholds"),
		CheckPostgres: pgTableCheck("sla_warning_thresholds"),
		SQLite:        slaWarningThresholdsSchema,
		Postgres:      slaWarningThresholdsSchemaPostgres,
	},
	{
		Version:       "20260930_sla_cycle_source_id",
		Name:          "Add SLA cycle import source identity",
		CheckSQLite:   sqliteColumnCheck("item_sla_cycles", "source_id"),
		CheckPostgres: pgColumnCheck("item_sla_cycles", "source_id"),
		SQLite:        slaImportSchema,
		Postgres:      slaImportSchemaPostgres,
	},
	{
		Version:       "20260932_sla_hot_indexes",
		Name:          "Add SLA job and cycle hot-path indexes",
		CheckSQLite:   sqliteIndexCheck("idx_item_sla_cycles_metric"),
		CheckPostgres: pgIndexCheck("idx_item_sla_cycles_metric"),
		SQLite: `
			CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_metric ON item_sla_cycles(metric_id);
			CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_goal ON item_sla_cycles(goal_id);
			CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_calendar ON item_sla_cycles(calendar_id);
			CREATE INDEX IF NOT EXISTS idx_sla_jobs_cycle ON sla_jobs(cycle_id);
			CREATE INDEX IF NOT EXISTS idx_sla_jobs_item ON sla_jobs(item_id);
			CREATE INDEX IF NOT EXISTS idx_sla_jobs_metric ON sla_jobs(metric_id);
		`,
		Postgres: `
			CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_metric ON item_sla_cycles(metric_id);
			CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_goal ON item_sla_cycles(goal_id);
			CREATE INDEX IF NOT EXISTS idx_item_sla_cycles_calendar ON item_sla_cycles(calendar_id);
			CREATE INDEX IF NOT EXISTS idx_sla_jobs_cycle ON sla_jobs(cycle_id);
			CREATE INDEX IF NOT EXISTS idx_sla_jobs_item ON sla_jobs(item_id);
			CREATE INDEX IF NOT EXISTS idx_sla_jobs_metric ON sla_jobs(metric_id);
		`,
	},
	{
		Version:       "20261001_action_trigger_marks",
		Name:          "Per-action inactivity trigger marks (WI-1132)",
		CheckSQLite:   sqliteTableCheck("action_trigger_marks"),
		CheckPostgres: pgTableCheck("action_trigger_marks"),
		SQLite: `
			CREATE TABLE action_trigger_marks (
				action_id INTEGER NOT NULL,
				item_id INTEGER NOT NULL,
				last_activity_at DATETIME NOT NULL,
				marked_at DATETIME NOT NULL,
				PRIMARY KEY (action_id, item_id),
				FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_action_trigger_marks_item ON action_trigger_marks(item_id);
		`,
		Postgres: `
			CREATE TABLE action_trigger_marks (
				action_id BIGINT NOT NULL,
				item_id BIGINT NOT NULL,
				last_activity_at TIMESTAMPTZ NOT NULL,
				marked_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (action_id, item_id),
				FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_action_trigger_marks_item ON action_trigger_marks(item_id);
		`,
	},
	{
		Version:       "20261001_canned_responses",
		Name:          "Workspace canned responses for support agents (WI-1138)",
		CheckSQLite:   sqliteTableCheck("canned_responses"),
		CheckPostgres: pgTableCheck("canned_responses"),
		SQLite: `
			CREATE TABLE canned_responses (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				workspace_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				body TEXT NOT NULL,
				is_private BOOLEAN NOT NULL DEFAULT false,
				is_active BOOLEAN NOT NULL DEFAULT true,
				created_by INTEGER,
				updated_by INTEGER,
				used_count INTEGER NOT NULL DEFAULT 0,
				last_used_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
				FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE UNIQUE INDEX uq_canned_responses_ws_name_ci ON canned_responses(LOWER(name), workspace_id);
			CREATE INDEX idx_canned_responses_ws_active ON canned_responses(workspace_id, is_active, name);
		`,
		Postgres: `
			CREATE TABLE canned_responses (
				id BIGSERIAL PRIMARY KEY,
				workspace_id BIGINT NOT NULL,
				name TEXT NOT NULL,
				body TEXT NOT NULL,
				is_private BOOLEAN NOT NULL DEFAULT false,
				is_active BOOLEAN NOT NULL DEFAULT true,
				created_by BIGINT,
				updated_by BIGINT,
				used_count INTEGER NOT NULL DEFAULT 0,
				last_used_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
				FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_canned_responses_ws_name_ci ON canned_responses(LOWER(name), workspace_id);
			CREATE INDEX IF NOT EXISTS idx_canned_responses_ws_active ON canned_responses(workspace_id, is_active, name);
		`,
	},
	{
		Version:       "20261001_board_view_settings",
		Name:          "Add view settings and convert board JSON columns to JSONB",
		CheckSQLite:   sqliteColumnCheck("board_configurations", "view_settings"),
		CheckPostgres: pgColumnCheck("board_configurations", "view_settings"),
		SQLite:        "ALTER TABLE board_configurations ADD COLUMN view_settings TEXT",
		Postgres: `
			ALTER TABLE board_configurations ADD COLUMN view_settings JSONB;
			ALTER TABLE board_configurations ALTER COLUMN backlog_status_ids TYPE JSONB USING NULLIF(btrim(backlog_status_ids), '')::jsonb;
			ALTER TABLE board_configurations ALTER COLUMN list_columns TYPE JSONB USING NULLIF(btrim(list_columns), '')::jsonb;
			ALTER TABLE board_configurations ALTER COLUMN roadmap_config TYPE JSONB USING NULLIF(btrim(roadmap_config), '')::jsonb;
			ALTER TABLE board_configurations ALTER COLUMN card_fields TYPE JSONB USING NULLIF(btrim(card_fields), '')::jsonb;
		`,
	},
	{
		Version: "20261002_jsonb_hygiene",
		Name:    "Convert remaining legacy JSON TEXT columns to JSONB",
		CheckPostgresFn: pgJSONBColumnsCheck(
			[2]string{"reviews", "review_data"},
			[2]string{"test_coverage_configurations", "requirement_item_type_ids"},
		),
		SQLite: "",
		Postgres: `
			ALTER TABLE reviews ALTER COLUMN review_data TYPE JSONB USING NULLIF(btrim(review_data), '')::jsonb;
			ALTER TABLE test_coverage_configurations ALTER COLUMN requirement_item_type_ids TYPE JSONB USING NULLIF(btrim(requirement_item_type_ids), '')::jsonb;
		`,
	},
	{
		Version:       "20261003_item_history_portal_actors",
		Name:          "Carry portal-customer and system actors in item history",
		CheckSQLite:   sqliteColumnCheck("item_history", "actor_kind"),
		CheckPostgres: pgColumnCheck("item_history", "actor_kind"),
		// SQLite cannot drop NOT NULL in place; ApplySQLite rebuilds the table.
		SQLite: "item_history actor attribution rebuild (applySQLiteItemHistoryPortalActors)",
		Postgres: `
			ALTER TABLE item_history ALTER COLUMN user_id DROP NOT NULL;
			ALTER TABLE item_history ADD COLUMN actor_kind TEXT NOT NULL DEFAULT 'user';
			ALTER TABLE item_history ADD COLUMN actor_portal_customer_id INTEGER REFERENCES portal_customers(id) ON DELETE SET NULL;
		`,
		ApplySQLite: applySQLiteItemHistoryPortalActors,
	},
	{
		Version:       "20261003_attachments_portal_uploader",
		Name:          "Attribute attachment uploads to portal customers",
		CheckSQLite:   sqliteColumnCheck("attachments", "uploaded_by_portal_customer_id"),
		CheckPostgres: pgColumnCheck("attachments", "uploaded_by_portal_customer_id"),
		SQLite: `
			ALTER TABLE attachments ADD COLUMN uploaded_by_portal_customer_id INTEGER REFERENCES portal_customers(id) ON DELETE SET NULL;
			CREATE INDEX idx_attachments_uploaded_by_portal_customer ON attachments(uploaded_by_portal_customer_id);
		`,
		Postgres: `
			ALTER TABLE attachments ADD COLUMN IF NOT EXISTS uploaded_by_portal_customer_id INTEGER REFERENCES portal_customers(id) ON DELETE SET NULL;
			CREATE INDEX IF NOT EXISTS idx_attachments_uploaded_by_portal_customer ON attachments(uploaded_by_portal_customer_id);
		`,
	},
	{
		Version: "20261004_import_jobs_scope_fk",
		Name:    "Drop the stale asset-set foreign key from the generic import tables",
		// The 20260924 rename preserved the original set_id →
		// asset_management_sets foreign keys under the new scope_id name, so
		// on every upgraded install ticket CSV imports (scope_id = workspace
		// id) fail the constraint and deleting an asset set can cascade into
		// unrelated import rows. Fresh installs never had the FKs; the checks
		// report the effect present when neither table carries such an FK.
		CheckSQLite: `
			SELECT CASE WHEN (SELECT COUNT(*) FROM pragma_foreign_key_list('import_jobs') WHERE "table"='asset_management_sets') = 0
				AND (SELECT COUNT(*) FROM pragma_foreign_key_list('import_uploads') WHERE "table"='asset_management_sets') = 0
			THEN 1 ELSE 0 END`,
		CheckPostgres: `
			SELECT CASE WHEN COUNT(*) = 0 THEN 1 ELSE 0 END
			FROM pg_constraint con
			JOIN pg_class child ON child.oid = con.conrelid
			JOIN pg_class parent ON parent.oid = con.confrelid
			JOIN pg_namespace n ON n.oid = child.relnamespace
			WHERE con.contype = 'f'
				AND n.nspname = current_schema()
				AND child.relname IN ('import_jobs', 'import_uploads')
				AND parent.relname = 'asset_management_sets'`,
		// SQLite cannot drop a constraint in place; rebuild both tables
		// without the asset-set FK, copying rows and restoring the indexes.
		SQLite: `
			CREATE TABLE import_jobs_scope_fk_rebuild (
				id TEXT PRIMARY KEY,
				kind TEXT NOT NULL DEFAULT 'asset',
				scope_id INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'queued',
				phase TEXT DEFAULT 'initializing',
				file_path TEXT NOT NULL,
				config_json TEXT,
				progress_json TEXT,
				error_message TEXT,
				created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				started_at DATETIME,
				completed_at DATETIME,
				lease_expires_at BIGINT
			);
			INSERT INTO import_jobs_scope_fk_rebuild
				SELECT id, kind, scope_id, status, phase, file_path, config_json, progress_json, error_message, created_by, created_at, started_at, completed_at, lease_expires_at
				FROM import_jobs;
			DROP TABLE import_jobs;
			ALTER TABLE import_jobs_scope_fk_rebuild RENAME TO import_jobs;
			CREATE INDEX IF NOT EXISTS idx_import_jobs_scope ON import_jobs(scope_id);
			CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_import_jobs_created_by ON import_jobs(created_by);
			CREATE TABLE import_uploads_scope_fk_rebuild (
				id TEXT PRIMARY KEY,
				kind TEXT NOT NULL DEFAULT 'asset',
				scope_id INTEGER NOT NULL,
				created_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				created_at BIGINT NOT NULL
			);
			INSERT INTO import_uploads_scope_fk_rebuild
				SELECT id, kind, scope_id, created_by, created_at
				FROM import_uploads;
			DROP TABLE import_uploads;
			ALTER TABLE import_uploads_scope_fk_rebuild RENAME TO import_uploads;
		`,
		Postgres: `
			DO $$
			DECLARE fk record;
			BEGIN
				FOR fk IN
					SELECT rel.relname AS table_name, con.conname AS constraint_name
					FROM pg_constraint con
					JOIN pg_class rel ON rel.oid = con.conrelid
					JOIN pg_class parent ON parent.oid = con.confrelid
					JOIN pg_namespace n ON n.oid = rel.relnamespace
					WHERE con.contype = 'f'
						AND n.nspname = current_schema()
						AND rel.relname IN ('import_jobs', 'import_uploads')
						AND parent.relname = 'asset_management_sets'
				LOOP
					EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', fk.table_name, fk.constraint_name);
				END LOOP;
			END $$;
		`,
	},
	{
		Version:       "20261005_portal_customers_erased_at",
		Name:          "Add irreversible Article 17 erasure state to portal customers (WI-1550)",
		CheckSQLite:   sqliteColumnCheck("portal_customers", "erased_at"),
		CheckPostgres: pgColumnCheck("portal_customers", "erased_at"),
		SQLite:        `ALTER TABLE portal_customers ADD COLUMN erased_at DATETIME`,
		Postgres:      `ALTER TABLE portal_customers ADD COLUMN IF NOT EXISTS erased_at TIMESTAMPTZ`,
	},
	{
		Version:       "20261005_customer_erasure_records",
		Name:          "Add DSAR erasure completion evidence table for portal customers (WI-1550)",
		CheckSQLite:   sqliteTableCheck("customer_erasure_records"),
		CheckPostgres: pgTableCheck("customer_erasure_records"),
		SQLite: `
			CREATE TABLE IF NOT EXISTS customer_erasure_records (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				portal_customer_id INTEGER NOT NULL,
				requested_by TEXT NOT NULL,
				requested_at DATETIME NOT NULL,
				approved_by INTEGER NOT NULL,
				executed_at DATETIME NOT NULL,
				policy_version TEXT NOT NULL,
				notes TEXT,
				FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_customer_erasure_records_customer_id ON customer_erasure_records(portal_customer_id);
		`,
		Postgres: `
			CREATE TABLE IF NOT EXISTS customer_erasure_records (
				id SERIAL PRIMARY KEY,
				portal_customer_id INTEGER NOT NULL,
				requested_by TEXT NOT NULL,
				requested_at TIMESTAMPTZ NOT NULL,
				approved_by INTEGER NOT NULL,
				executed_at TIMESTAMPTZ NOT NULL,
				policy_version TEXT NOT NULL,
				notes TEXT,
				FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_customer_erasure_records_customer_id ON customer_erasure_records(portal_customer_id);
		`,
	},
	{
		Version:       "20261006_portal_customers_deactivated_at",
		Name:          "Add security deactivation lifecycle state to portal customers (WI-1554)",
		CheckSQLite:   sqliteColumnCheck("portal_customers", "deactivated_at"),
		CheckPostgres: pgColumnCheck("portal_customers", "deactivated_at"),
		SQLite:        `ALTER TABLE portal_customers ADD COLUMN deactivated_at DATETIME`,
		Postgres:      `ALTER TABLE portal_customers ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMPTZ`,
	},
	{
		Version:       "20261006_portal_customers_created_via",
		Name:          "Record creation provenance on portal customers (WI-1553)",
		CheckSQLite:   sqliteColumnCheck("portal_customers", "created_via"),
		CheckPostgres: pgColumnCheck("portal_customers", "created_via"),
		// The DEFAULT backfills existing rows to 'unknown' in the same
		// statement — no separate rewrite is needed.
		SQLite:   `ALTER TABLE portal_customers ADD COLUMN created_via TEXT NOT NULL DEFAULT 'unknown'`,
		Postgres: `ALTER TABLE portal_customers ADD COLUMN IF NOT EXISTS created_via TEXT NOT NULL DEFAULT 'unknown'`,
	},
	{
		Version:       "20261006_item_import_rows_lookup_indexes",
		Name:          "Index ticket import mapping by job and item (WI-1596)",
		CheckSQLite:   sqliteIndexCheck("idx_item_import_rows_job_id"),
		CheckPostgres: pgIndexCheck("idx_item_import_rows_job_id"),
		SQLite: `
			CREATE INDEX IF NOT EXISTS idx_item_import_rows_job_id ON item_import_rows(job_id);
			CREATE INDEX IF NOT EXISTS idx_item_import_rows_item_id ON item_import_rows(item_id);
		`,
		Postgres: `
			CREATE INDEX IF NOT EXISTS idx_item_import_rows_job_id ON item_import_rows(job_id);
			CREATE INDEX IF NOT EXISTS idx_item_import_rows_item_id ON item_import_rows(item_id);
		`,
	},
	{
		Version:       "20261006_email_tracking_sender_indexes",
		Name:          "Index email sender lookups by channel and normalized address (WI-1597)",
		CheckSQLite:   sqliteIndexCheck("idx_email_message_tracking_sender"),
		CheckPostgres: pgIndexCheck("idx_email_message_tracking_sender"),
		SQLite: `
			CREATE INDEX IF NOT EXISTS idx_email_message_tracking_sender ON email_message_tracking(from_email);
			CREATE INDEX IF NOT EXISTS idx_email_message_tracking_channel_sender_time ON email_message_tracking(channel_id, LOWER(from_email), processed_at);
		`,
		Postgres: `
			CREATE INDEX IF NOT EXISTS idx_email_message_tracking_sender ON email_message_tracking(from_email);
			CREATE INDEX IF NOT EXISTS idx_email_message_tracking_channel_sender_time ON email_message_tracking(channel_id, LOWER(from_email), processed_at);
		`,
	},
	{
		// Incidents had a never-read dedup_key column and scheduled incident
		// notifications kept no escalation-rule reference (WI-1536). The drop
		// removes the dead column; the state table rebuild adds the missing
		// foreign key after clearing rows whose rule no longer exists.
		Version: "20261006_incident_state_hardening",
		Name:    "Drop incidents.dedup_key and reference escalation rules from notification state",
		CheckSQLite: `
			SELECT CASE WHEN (SELECT COUNT(*) FROM pragma_table_info('incidents') WHERE name='dedup_key') = 0
				AND (SELECT COUNT(*) FROM pragma_foreign_key_list('incident_notification_state') WHERE "table"='on_call_escalation_rules') > 0
			THEN 1 ELSE 0 END`,
		CheckPostgres: `
			SELECT CASE WHEN NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'incidents' AND column_name = 'dedup_key'
			) AND EXISTS (
				SELECT 1 FROM pg_constraint con
				JOIN pg_class child ON child.oid = con.conrelid
				JOIN pg_class parent ON parent.oid = con.confrelid
				JOIN pg_namespace n ON n.oid = child.relnamespace
				WHERE con.contype = 'f' AND n.nspname = current_schema()
					AND child.relname = 'incident_notification_state'
					AND parent.relname = 'on_call_escalation_rules'
			) THEN 1 ELSE 0 END`,
		SQLite: `
			ALTER TABLE incidents DROP COLUMN dedup_key;
			DELETE FROM incident_notification_state WHERE escalation_rule_id NOT IN (SELECT id FROM on_call_escalation_rules);
			CREATE TABLE incident_notification_state_state_fk_rebuild (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				incident_id INTEGER NOT NULL,
				escalation_rule_id INTEGER NOT NULL,
				notification_rule_id INTEGER NOT NULL,
				repeat_index INTEGER NOT NULL DEFAULT 0,
				next_notification_at DATETIME NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE CASCADE,
				FOREIGN KEY (escalation_rule_id) REFERENCES on_call_escalation_rules(id) ON DELETE CASCADE,
				FOREIGN KEY (notification_rule_id) REFERENCES on_call_notification_rules(id) ON DELETE CASCADE
			);
			INSERT INTO incident_notification_state_state_fk_rebuild
				SELECT id, incident_id, escalation_rule_id, notification_rule_id, repeat_index,
					next_notification_at, created_at, updated_at
				FROM incident_notification_state;
			DROP TABLE incident_notification_state;
			ALTER TABLE incident_notification_state_state_fk_rebuild RENAME TO incident_notification_state;
			CREATE UNIQUE INDEX uq_incident_notification_state ON incident_notification_state(incident_id, notification_rule_id, repeat_index);
			CREATE INDEX idx_incident_notification_state_due ON incident_notification_state(next_notification_at);
		`,
		Postgres: `
			ALTER TABLE incidents DROP COLUMN IF EXISTS dedup_key;
			DELETE FROM incident_notification_state WHERE escalation_rule_id NOT IN (SELECT id FROM on_call_escalation_rules);
			ALTER TABLE incident_notification_state DROP CONSTRAINT IF EXISTS incident_notification_state_escalation_rule_fkey;
			ALTER TABLE incident_notification_state ADD CONSTRAINT incident_notification_state_escalation_rule_fkey
				FOREIGN KEY (escalation_rule_id) REFERENCES on_call_escalation_rules(id) ON DELETE CASCADE;
		`,
	},
	{
		// Outbox delivery leases need an owner so a manual retry can tell a
		// live claim from retry backoff (WI-1572).
		Version:       "20261006_email_outbox_lease_owner",
		Name:          "Track outbound-email delivery lease ownership",
		CheckSQLite:   sqliteColumnCheck("email_reply_outbox", "lease_owner"),
		CheckPostgres: pgColumnCheck("email_reply_outbox", "lease_owner"),
		SQLite: `
			ALTER TABLE email_reply_outbox ADD COLUMN lease_owner TEXT;
		`,
		Postgres: `
			ALTER TABLE email_reply_outbox ADD COLUMN IF NOT EXISTS lease_owner TEXT;
		`,
	},
	{
		Version:         "20261002_view_settings_backfill_tools",
		Name:            "Backfill tools entries into stored workspace view-visibility overrides",
		CheckSQLiteFn:   checkViewSettingsToolsBackfill,
		CheckPostgresFn: checkViewSettingsToolsBackfill,
		SQLite:          "applyViewSettingsToolsBackfill:v1",
		Postgres:        "applyViewSettingsToolsBackfill:v1",
		ApplySQLite:     applyViewSettingsToolsBackfill,
		ApplyPostgres:   applyViewSettingsToolsBackfill,
	},
	{
		Version:       "20261007_support_queues",
		Name:          "Persisted, scope-aware support queues (WI-1603)",
		CheckSQLite:   sqliteTableCheck("queues"),
		CheckPostgres: pgTableCheck("queues"),
		SQLite: `
			CREATE TABLE queues (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				workspace_id INTEGER NOT NULL,
				collection_id INTEGER,
				name TEXT NOT NULL,
				ql_query TEXT NOT NULL,
				filter_state TEXT,
				position INTEGER NOT NULL DEFAULT 0,
				created_by INTEGER,
				builtin_key TEXT,
				is_hidden BOOLEAN NOT NULL DEFAULT false,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE,
				FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE INDEX idx_queues_workspace_scope ON queues(workspace_id, collection_id, position);
			CREATE UNIQUE INDEX uq_queues_ws_builtin ON queues(workspace_id, builtin_key) WHERE collection_id IS NULL AND builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX uq_queues_coll_builtin ON queues(collection_id, builtin_key) WHERE collection_id IS NOT NULL AND builtin_key IS NOT NULL;
		`,
		Postgres: `
			CREATE TABLE queues (
				id BIGSERIAL PRIMARY KEY,
				workspace_id BIGINT NOT NULL,
				collection_id BIGINT,
				name TEXT NOT NULL,
				ql_query TEXT NOT NULL,
				filter_state TEXT,
				position INTEGER NOT NULL DEFAULT 0,
				created_by BIGINT,
				builtin_key TEXT,
				is_hidden BOOLEAN NOT NULL DEFAULT false,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE,
				FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
			);
			CREATE INDEX IF NOT EXISTS idx_queues_workspace_scope ON queues(workspace_id, collection_id, position);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_queues_ws_builtin ON queues(workspace_id, builtin_key) WHERE collection_id IS NULL AND builtin_key IS NOT NULL;
			CREATE UNIQUE INDEX IF NOT EXISTS uq_queues_coll_builtin ON queues(collection_id, builtin_key) WHERE collection_id IS NOT NULL AND builtin_key IS NOT NULL;
		`,
	},
	{
		Version:       "20261008_item_participants",
		Name:          "External request participants on work items (WI-1136)",
		CheckSQLite:   sqliteTableCheck("item_participants"),
		CheckPostgres: pgTableCheck("item_participants"),
		SQLite: `
			CREATE TABLE item_participants (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				item_id INTEGER NOT NULL,
				portal_customer_id INTEGER NOT NULL,
				added_by INTEGER,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
				FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
				FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
				UNIQUE(item_id, portal_customer_id)
			);
			CREATE INDEX idx_item_participants_item ON item_participants(item_id);
			CREATE INDEX idx_item_participants_customer ON item_participants(portal_customer_id);
		`,
		Postgres: `
			CREATE TABLE item_participants (
				id BIGSERIAL PRIMARY KEY,
				item_id BIGINT NOT NULL,
				portal_customer_id BIGINT NOT NULL,
				added_by BIGINT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
				FOREIGN KEY (portal_customer_id) REFERENCES portal_customers(id) ON DELETE CASCADE,
				FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL,
				UNIQUE(item_id, portal_customer_id)
			);
			CREATE INDEX IF NOT EXISTS idx_item_participants_item ON item_participants(item_id);
			CREATE INDEX IF NOT EXISTS idx_item_participants_customer ON item_participants(portal_customer_id);
		`,
	},
	{
		Version:       "20261009_email_reply_outbox_per_recipient",
		Name:          "Allow one outbound reply per comment recipient (WI-1136)",
		CheckSQLite:   sqliteIndexCheck("uq_email_reply_outbox_comment_recipient"),
		CheckPostgres: pgIndexCheck("uq_email_reply_outbox_comment_recipient"),
		Postgres: `
			ALTER TABLE email_reply_outbox DROP CONSTRAINT IF EXISTS email_reply_outbox_comment_id_key;
			CREATE UNIQUE INDEX IF NOT EXISTS uq_email_reply_outbox_comment_recipient
				ON email_reply_outbox(comment_id, to_email);
		`,
		// SQLite cannot drop the inline UNIQUE(comment_id) without a table
		// rebuild. The SQL field stays non-empty so the runner reaches
		// ApplySQLite instead of stamping the row.
		SQLite:      "applySQLiteEmailReplyOutboxPerRecipient:v1",
		ApplySQLite: applySQLiteEmailReplyOutboxPerRecipient,
	},
	{
		Version:       "20261010_portal_org_sharing",
		Name:          "Portal organisation request sharing (WI-1139)",
		CheckSQLite:   sqliteColumnCheck("customer_organisations", "settings"),
		CheckPostgres: pgColumnCheck("customer_organisations", "settings"),
		SQLite: `
			ALTER TABLE customer_organisations ADD COLUMN settings TEXT NOT NULL DEFAULT '{}';
			ALTER TABLE contact_roles ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE items ADD COLUMN portal_org_shared BOOLEAN NOT NULL DEFAULT false;
		`,
		Postgres: `
			ALTER TABLE customer_organisations ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::JSONB;
			ALTER TABLE contact_roles ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE items ADD COLUMN IF NOT EXISTS portal_org_shared BOOLEAN NOT NULL DEFAULT false;
		`,
	},
	{
		Version:       "20261011_item_support_events",
		Name:          "Append-only ticket fact events for support metrics (WI-1133)",
		CheckSQLite:   sqliteTableCheck("item_support_events"),
		CheckPostgres: pgTableCheck("item_support_events"),
		SQLite: `
			CREATE TABLE IF NOT EXISTS item_support_events (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				workspace_id INTEGER NOT NULL,
				item_id INTEGER NOT NULL,
				kind TEXT NOT NULL,
				occurred_at DATETIME NOT NULL,
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_item_support_events_scope ON item_support_events(workspace_id, kind, occurred_at);
			CREATE INDEX IF NOT EXISTS idx_item_support_events_item ON item_support_events(item_id, kind);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_item_support_events_single_shot ON item_support_events(item_id, kind) WHERE kind IN ('first_response', 'resolved');

			INSERT INTO item_support_events (workspace_id, item_id, kind, occurred_at)
			SELECT i.workspace_id, i.id, 'first_response', MIN(c.created_at)
			FROM items i
			JOIN comments c ON c.item_id = i.id
			WHERE (i.channel_id IS NOT NULL OR i.creator_portal_customer_id IS NOT NULL)
				AND c.is_private = false
				AND c.author_id IS NOT NULL
				AND c.portal_customer_id IS NULL
			GROUP BY i.workspace_id, i.id
			ON CONFLICT DO NOTHING;

			INSERT INTO item_support_events (workspace_id, item_id, kind, occurred_at)
			SELECT i.workspace_id, ih.item_id, 'resolved', MIN(ih.changed_at)
			FROM items i
			JOIN item_history ih ON ih.item_id = i.id AND ih.field_name = 'status_id'
			JOIN statuses st ON ih.new_value = CAST(st.id AS TEXT)
			JOIN status_categories sc ON st.category_id = sc.id AND sc.is_completed = true
			WHERE (i.channel_id IS NOT NULL OR i.creator_portal_customer_id IS NOT NULL)
			GROUP BY i.workspace_id, ih.item_id
			ON CONFLICT DO NOTHING;

			INSERT INTO item_support_events (workspace_id, item_id, kind, occurred_at)
			SELECT i.workspace_id, ih.item_id, 'reopened', ih.changed_at
			FROM items i
			JOIN item_history ih ON ih.item_id = i.id AND ih.field_name = 'status_id'
			JOIN statuses st_old ON ih.old_value = CAST(st_old.id AS TEXT)
			JOIN status_categories sc_old ON st_old.category_id = sc_old.id AND sc_old.is_completed = true
			LEFT JOIN statuses st_new ON ih.new_value = CAST(st_new.id AS TEXT)
			LEFT JOIN status_categories sc_new ON st_new.category_id = sc_new.id
			WHERE (i.channel_id IS NOT NULL OR i.creator_portal_customer_id IS NOT NULL)
				AND COALESCE(sc_new.is_completed, false) = false;
		`,
		Postgres: `
			CREATE TABLE IF NOT EXISTS item_support_events (
				id BIGSERIAL PRIMARY KEY,
				workspace_id BIGINT NOT NULL,
				item_id BIGINT NOT NULL,
				kind TEXT NOT NULL,
				occurred_at TIMESTAMPTZ NOT NULL,
				FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
				FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_item_support_events_scope ON item_support_events(workspace_id, kind, occurred_at);
			CREATE INDEX IF NOT EXISTS idx_item_support_events_item ON item_support_events(item_id, kind);
			CREATE UNIQUE INDEX IF NOT EXISTS uq_item_support_events_single_shot ON item_support_events(item_id, kind) WHERE kind IN ('first_response', 'resolved');

			INSERT INTO item_support_events (workspace_id, item_id, kind, occurred_at)
			SELECT i.workspace_id, i.id, 'first_response', MIN(c.created_at)
			FROM items i
			JOIN comments c ON c.item_id = i.id
			WHERE (i.channel_id IS NOT NULL OR i.creator_portal_customer_id IS NOT NULL)
				AND c.is_private = false
				AND c.author_id IS NOT NULL
				AND c.portal_customer_id IS NULL
			GROUP BY i.workspace_id, i.id
			ON CONFLICT DO NOTHING;

			INSERT INTO item_support_events (workspace_id, item_id, kind, occurred_at)
			SELECT i.workspace_id, ih.item_id, 'resolved', MIN(ih.changed_at)
			FROM items i
			JOIN item_history ih ON ih.item_id = i.id AND ih.field_name = 'status_id'
			JOIN statuses st ON ih.new_value = CAST(st.id AS TEXT)
			JOIN status_categories sc ON st.category_id = sc.id AND sc.is_completed = true
			WHERE (i.channel_id IS NOT NULL OR i.creator_portal_customer_id IS NOT NULL)
			GROUP BY i.workspace_id, ih.item_id
			ON CONFLICT DO NOTHING;

			INSERT INTO item_support_events (workspace_id, item_id, kind, occurred_at)
			SELECT i.workspace_id, ih.item_id, 'reopened', ih.changed_at
			FROM items i
			JOIN item_history ih ON ih.item_id = i.id AND ih.field_name = 'status_id'
			JOIN statuses st_old ON ih.old_value = CAST(st_old.id AS TEXT)
			JOIN status_categories sc_old ON st_old.category_id = sc_old.id AND sc_old.is_completed = true
			LEFT JOIN statuses st_new ON ih.new_value = CAST(st_new.id AS TEXT)
			LEFT JOIN status_categories sc_new ON st_new.category_id = sc_new.id
			WHERE (i.channel_id IS NOT NULL OR i.creator_portal_customer_id IS NOT NULL)
				AND COALESCE(sc_new.is_completed, false) = false;
		`,
	},
	{
		Version: "20261002_zammad_ticket_change_history",
		Name:    "Record observed Zammad ticket field changes",
		CheckSQLite: `
			SELECT CASE WHEN EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'zammad_ticket_changes')
			THEN 1 ELSE 0 END
		`,
		CheckPostgres: `
			SELECT CASE WHEN EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'zammad_ticket_changes')
			THEN 1 ELSE 0 END
		`,
		SQLite: `
			CREATE TABLE zammad_ticket_changes (
				id TEXT PRIMARY KEY,
				ticket_link_id TEXT NOT NULL,
				field_name TEXT NOT NULL CHECK (field_name IN ('status', 'owner', 'group')),
				old_value_id INTEGER,
				old_value_name TEXT NOT NULL DEFAULT '',
				new_value_id INTEGER,
				new_value_name TEXT NOT NULL DEFAULT '',
				observed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (ticket_link_id) REFERENCES zammad_ticket_links(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_zammad_ticket_changes_link_observed ON zammad_ticket_changes(ticket_link_id, observed_at DESC);
		`,
		Postgres: `
			CREATE TABLE IF NOT EXISTS zammad_ticket_changes (
				id TEXT PRIMARY KEY,
				ticket_link_id TEXT NOT NULL REFERENCES zammad_ticket_links(id) ON DELETE CASCADE,
				field_name TEXT NOT NULL CHECK (field_name IN ('status', 'owner', 'group')),
				old_value_id INTEGER,
				old_value_name TEXT NOT NULL DEFAULT '',
				new_value_id INTEGER,
				new_value_name TEXT NOT NULL DEFAULT '',
				observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_zammad_ticket_changes_link_observed ON zammad_ticket_changes(ticket_link_id, observed_at DESC);
		`,
	},
	{
		Version:       "20260931_item_history_source",
		Name:          "Record the acting surface on item history rows",
		CheckSQLite:   sqliteColumnCheck("item_history", "source"),
		CheckPostgres: pgColumnCheck("item_history", "source"),
		SQLite:        "ALTER TABLE item_history ADD COLUMN source TEXT",
		Postgres:      "ALTER TABLE item_history ADD COLUMN IF NOT EXISTS source TEXT",
	},
	{
		Version:       "20260931_item_history_agent_run",
		Name:          "Link agent-written item history rows to the run that made them",
		CheckSQLite:   sqliteColumnCheck("item_history", "agent_run_id"),
		CheckPostgres: pgColumnCheck("item_history", "agent_run_id"),
		SQLite:        "ALTER TABLE item_history ADD COLUMN agent_run_id INTEGER",
		Postgres:      "ALTER TABLE item_history ADD COLUMN IF NOT EXISTS agent_run_id INTEGER",
	},
	{
		Version:       "20260931_llm_usage_calls",
		Name:          "Count provider round-trips on metered LLM usage rows",
		CheckSQLite:   sqliteColumnCheck("llm_usage", "calls"),
		CheckPostgres: pgColumnCheck("llm_usage", "calls"),
		SQLite:        "ALTER TABLE llm_usage ADD COLUMN calls INTEGER NOT NULL DEFAULT 1",
		Postgres:      "ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS calls INTEGER NOT NULL DEFAULT 1",
	},
	{
		Version:         "20261012_transactional_email_copy",
		Name:            "Align transactional email copy with real expiries and reply behavior",
		CheckSQLiteFn:   checkTransactionalEmailCopyMigration,
		CheckPostgresFn: checkTransactionalEmailCopyMigration,
		SQLite:          transactionalEmailCopyMigration,
		Postgres:        transactionalEmailCopyMigration,
	},
	{
		Version:         "20261013_portal_reply_markdown",
		Name:            "Render portal reply Markdown in the HTML email",
		CheckSQLiteFn:   checkPortalReplyMarkdownMigration,
		CheckPostgresFn: checkPortalReplyMarkdownMigration,
		SQLite:          portalReplyMarkdownMigration,
		Postgres:        portalReplyMarkdownMigration,
	},
	{
		Version:         "20261014_notification_digest_links",
		Name:            "Add clickable action links to notification digest emails",
		CheckSQLiteFn:   checkNotificationDigestLinksMigration,
		CheckPostgresFn: checkNotificationDigestLinksMigration,
		SQLite:          notificationDigestLinksMigration,
		Postgres:        notificationDigestLinksMigration,
	},
	{
		Version:       "20261015_request_types_kind",
		Name:          "Add system intake kind to request types (WI-1644)",
		CheckSQLite:   sqliteColumnCheck("request_types", "kind"),
		CheckPostgres: pgColumnCheck("request_types", "kind"),
		SQLite:        `ALTER TABLE request_types ADD COLUMN kind TEXT NOT NULL DEFAULT ''`,
		Postgres:      `ALTER TABLE request_types ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT ''`,
	},
	{
		Version:       "20261016_email_outbox_reply_to",
		Name:          "Persist the monitored reply address on queued customer replies (WI-1644)",
		CheckSQLite:   sqliteColumnCheck("email_reply_outbox", "reply_to_email"),
		CheckPostgres: pgColumnCheck("email_reply_outbox", "reply_to_email"),
		SQLite: `
			ALTER TABLE email_reply_outbox ADD COLUMN reply_to_email TEXT NOT NULL DEFAULT '';
			ALTER TABLE email_reply_outbox ADD COLUMN reply_to_name TEXT NOT NULL DEFAULT '';
		`,
		Postgres: `
			ALTER TABLE email_reply_outbox ADD COLUMN IF NOT EXISTS reply_to_email TEXT NOT NULL DEFAULT '';
			ALTER TABLE email_reply_outbox ADD COLUMN IF NOT EXISTS reply_to_name TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:         "20261018_intake_split",
		Name:            "Split email intake routing out of mailbox channels (WI-1644)",
		CheckSQLite:     sqliteTableCheck("intakes"),
		CheckPostgres:   pgTableCheck("intakes"),
		CheckSQLiteFn:   checkIntakeSplitMigration,
		CheckPostgresFn: checkIntakeSplitMigration,
		SQLite:          "applyIntakeSplitMigration:v1",
		Postgres:        "applyIntakeSplitMigration:v1",
		ApplySQLite:     applyIntakeSplitMigration,
		ApplyPostgres:   applyIntakeSplitMigration,
	},
	{
		Version:         "20261026_intake_workspace_portal",
		Name:            "Route email intakes by workspace with an optional portal link (WI-1644)",
		CheckSQLiteFn:   checkIntakeWorkspacePortalMigration,
		CheckPostgresFn: checkIntakeWorkspacePortalMigration,
		SQLite:          "applyIntakeWorkspacePortalMigration:v1",
		Postgres:        "applyIntakeWorkspacePortalMigration:v1",
		ApplySQLite:     applyIntakeWorkspacePortalMigration,
		ApplyPostgres:   applyIntakeWorkspacePortalMigration,
	},
	{
		Version:       "20261019_email_tracking_completed_at",
		Name:          "Keep email dedup after an intake-created item is deleted (WI-1658)",
		CheckSQLite:   sqliteColumnCheck("email_message_tracking", "completed_at"),
		CheckPostgres: pgColumnCheck("email_message_tracking", "completed_at"),
		SQLite:        `ALTER TABLE email_message_tracking ADD COLUMN completed_at DATETIME`,
		Postgres:      `ALTER TABLE email_message_tracking ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ`,
	},
	{
		// WI-1658 added completed_at but left pre-existing tracking rows NULL.
		// When one of their tickets was deleted the FK nulled item_id while the
		// marker stayed NULL, so preclaimTracking reclaimed the row and recreated
		// the ticket. Stamp the tombstones the column should have had: any row
		// that still references an item/comment is definitely completed, and an
		// orphaned row older than an hour (well past the 5-minute stale-claim
		// window) that was never rate-limited is a deleted-ticket tombstone
		// rather than a live claim. Rate-limited rows stay untouched so operators
		// can requeue them, and recent orphans stay untouched so a genuine retry
		// can finish.
		Version:         "20261020_email_tracking_completed_backfill",
		Name:            "Backfill completed_at for email tracking rows that predate the tombstone (WI-1658)",
		CheckSQLiteFn:   checkEmailTrackingCompletedBackfill,
		CheckPostgresFn: checkEmailTrackingCompletedBackfill,
		SQLite: `UPDATE email_message_tracking
		SET completed_at = processed_at
		WHERE completed_at IS NULL
		  AND rate_limited_at IS NULL
		  AND (item_id IS NOT NULL
		       OR comment_id IS NOT NULL
		       OR processed_at < datetime('now', '-1 hour'))`,
		Postgres: `UPDATE email_message_tracking
		SET completed_at = processed_at
		WHERE completed_at IS NULL
		  AND rate_limited_at IS NULL
		  AND (item_id IS NOT NULL
		       OR comment_id IS NOT NULL
		       OR processed_at < NOW() - INTERVAL '1 hour')`,
	},
	{
		Version:       "20261021_collections_private",
		Name:          "Add opt-out privacy flag to collections",
		CheckSQLite:   sqliteColumnCheck("collections", "is_private"),
		CheckPostgres: pgColumnCheck("collections", "is_private"),
		SQLite:        `ALTER TABLE collections ADD COLUMN is_private BOOLEAN NOT NULL DEFAULT false`,
		Postgres:      `ALTER TABLE collections ADD COLUMN IF NOT EXISTS is_private BOOLEAN NOT NULL DEFAULT false`,
	},
	{
		Version: "20261022_request_types_workspace_pinned",
		Name:    "Cascade request-type workspaces with the workspace (WI-1695)",
		// A fresh schema already cascades; an upgraded database still nulls the
		// route when its workspace is deleted.
		CheckSQLite:   `SELECT COUNT(*) FROM pragma_foreign_key_list('request_types') WHERE "from"='workspace_id' AND on_delete='CASCADE'`,
		CheckPostgres: `SELECT COUNT(*) FROM pg_constraint WHERE conrelid='request_types'::regclass AND contype='f' AND confdeltype='c' AND pg_get_constraintdef(oid) LIKE '%workspace_id%'`,
		SQLite:        "applyRequestTypeWorkspacePinned:v2",
		Postgres:      "applyRequestTypeWorkspacePinned:v2",
		Superseded:    []string{"6e5f5a942b5d7465e3a07d67c7f3872bed659b2ac644106147693e7619072944"},
		ApplySQLite:   applyRequestTypeWorkspacePinned,
		ApplyPostgres: applyRequestTypeWorkspacePinned,
	},
	{
		Version:       "20261023_asset_reports_workspace_pinned",
		Name:          "Pin form-mode asset report workspaces and cascade them with the workspace (WI-1697)",
		CheckSQLite:   `SELECT COUNT(*) FROM pragma_foreign_key_list('asset_reports') WHERE "from"='workspace_id' AND on_delete='CASCADE'`,
		CheckPostgres: `SELECT COUNT(*) FROM pg_constraint WHERE conrelid='asset_reports'::regclass AND contype='f' AND confdeltype='c' AND pg_get_constraintdef(oid) LIKE '%workspace_id%'`,
		SQLite:        "applyAssetReportsWorkspacePinned:v1",
		Postgres:      "applyAssetReportsWorkspacePinned:v1",
		ApplySQLite:   applyAssetReportsWorkspacePinned,
		ApplyPostgres: applyAssetReportsWorkspacePinned,
	},
	{
		Version:       "20261024_attachments_internal_visibility",
		Name:          "Hide internal attachments from portal customers (WI-1700)",
		CheckSQLite:   sqliteColumnCheck("attachments", "is_internal"),
		CheckPostgres: pgColumnCheck("attachments", "is_internal"),
		SQLite:        `ALTER TABLE attachments ADD COLUMN is_internal BOOLEAN NOT NULL DEFAULT false;`,
		Postgres:      `ALTER TABLE attachments ADD COLUMN IF NOT EXISTS is_internal BOOLEAN NOT NULL DEFAULT false;`,
	},
	{
		Version:       "20261025_jira_import_job_leases",
		Name:          "Lease Jira import jobs so restarts recover interrupted runs (WI-1703)",
		CheckSQLite:   sqliteColumnCheck("jira_import_jobs", "lease_expires_at"),
		CheckPostgres: pgColumnCheck("jira_import_jobs", "lease_expires_at"),
		SQLite:        `ALTER TABLE jira_import_jobs ADD COLUMN lease_expires_at BIGINT;`,
		Postgres:      `ALTER TABLE jira_import_jobs ADD COLUMN IF NOT EXISTS lease_expires_at BIGINT;`,
	},
}

// applyAssetReportsWorkspacePinned backfills form-mode asset reports that have
// no workspace and changes the asset_reports FKs so a workspace delete removes
// the report (CASCADE) and an item-type delete is refused (RESTRICT) instead of
// silently unpinning the report.
func applyAssetReportsWorkspacePinned(db Database) error {
	if err := backfillAssetReportWorkspaces(db); err != nil {
		return err
	}
	if db.GetDriverName() == driverPostgres {
		for _, constraint := range []string{"asset_reports_item_type_id_fkey", "asset_reports_workspace_id_fkey"} {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE asset_reports DROP CONSTRAINT IF EXISTS %q", constraint)); err != nil {
				return fmt.Errorf("drop asset_reports constraint %s: %w", constraint, err)
			}
		}
		if _, err := db.Exec(`
			ALTER TABLE asset_reports
				ADD CONSTRAINT asset_reports_item_type_id_fkey FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE RESTRICT,
				ADD CONSTRAINT asset_reports_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE`); err != nil {
			return fmt.Errorf("pin asset_reports FKs: %w", err)
		}
		return nil
	}
	return rebuildAssetReportsWorkspacePinned(db)
}

// backfillAssetReportWorkspaces pins every form-mode asset report that has no
// workspace to the portal's first served workspace. A report whose channel
// serves no workspace stays NULL and is rejected at runtime rather than
// deleted.
func backfillAssetReportWorkspaces(db Database) error {
	rows, err := db.Query(`
		SELECT ar.id, COALESCE(c.config, '{}')
		FROM asset_reports ar
		JOIN channels c ON c.id = ar.channel_id
		WHERE ar.run_mode = 'form' AND ar.workspace_id IS NULL
	`)
	if err != nil {
		return fmt.Errorf("list unpinned asset reports: %w", err)
	}
	type unpinned struct {
		id         int
		configJSON string
	}
	var pending []unpinned
	for rows.Next() {
		var row unpinned
		if err := rows.Scan(&row.id, &row.configJSON); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan unpinned asset report: %w", err)
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate unpinned asset reports: %w", err)
	}
	_ = rows.Close()

	for _, row := range pending {
		var cfg struct {
			PortalWorkspaceIDs []int `json:"portal_workspace_ids"`
		}
		if err := json.Unmarshal([]byte(row.configJSON), &cfg); err != nil {
			return fmt.Errorf("parse channel config for asset report %d: %w", row.id, err)
		}
		if len(cfg.PortalWorkspaceIDs) == 0 {
			slog.Warn("asset report has no workspace to pin; leaving it unpinned",
				"component", "migrations", "asset_report_id", row.id)
			continue
		}
		if _, err := db.ExecWrite(`UPDATE asset_reports SET workspace_id = ? WHERE id = ?`, cfg.PortalWorkspaceIDs[0], row.id); err != nil {
			return fmt.Errorf("pin asset report %d: %w", row.id, err)
		}
	}
	return nil
}

// rebuildAssetReportsWorkspacePinned rebuilds asset_reports for SQLite, which
// cannot alter a foreign-key action in place.
func rebuildAssetReportsWorkspacePinned(db Database) (retErr error) {
	sqliteDB, ok := db.(*SQLiteDB)
	if !ok {
		return fmt.Errorf("expected SQLite database, got %T", db)
	}

	ctx := context.Background()
	conn, err := sqliteDB.writeConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire SQLite write connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var foreignKeysEnabled bool
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeysEnabled {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); retErr == nil && err != nil {
				retErr = fmt.Errorf("restore foreign keys: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin asset_reports rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE asset_reports_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER NOT NULL,
			asset_set_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			cql_query TEXT DEFAULT '',
			icon TEXT DEFAULT 'Table2',
			color TEXT DEFAULT '#6b7280',
			display_order INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT TRUE,
			column_config TEXT DEFAULT NULL,
			visibility_group_ids TEXT DEFAULT NULL,
			visibility_org_ids TEXT DEFAULT NULL,
			run_mode TEXT NOT NULL DEFAULT 'direct',
			item_type_id INTEGER DEFAULT NULL,
			workspace_id INTEGER DEFAULT NULL,
			config TEXT DEFAULT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
			FOREIGN KEY (asset_set_id) REFERENCES asset_management_sets(id) ON DELETE CASCADE,
			FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE RESTRICT,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`,
		`INSERT INTO asset_reports_migration (id, channel_id, asset_set_id, name, description, cql_query, icon, color, display_order, is_active, column_config, visibility_group_ids, visibility_org_ids, run_mode, item_type_id, workspace_id, config, created_at, updated_at)
			SELECT id, channel_id, asset_set_id, name, description, cql_query, icon, color, display_order, is_active, column_config, visibility_group_ids, visibility_org_ids, run_mode, item_type_id, workspace_id, config, created_at, updated_at FROM asset_reports`,
		`DROP TABLE asset_reports`,
		`ALTER TABLE asset_reports_migration RENAME TO asset_reports`,
		`CREATE INDEX IF NOT EXISTS idx_asset_reports_channel_id ON asset_reports(channel_id)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_reports_asset_set_id ON asset_reports(asset_set_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild asset_reports: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit asset_reports rebuild: %w", err)
	}
	return nil
}

// applyRequestTypeWorkspacePinned removes the implicit "first served
// workspace" fallback for request types. It backfills legacy NULL routes from
// the channel's configured workspaces and changes the workspace FK from
// ON DELETE SET NULL to ON DELETE CASCADE so deleting a workspace can never
// leave a route unpinned. A route whose channel serves no workspace is left
// NULL and rejected at runtime rather than deleted.
func applyRequestTypeWorkspacePinned(db Database) error {
	if err := backfillRequestTypeWorkspaces(db); err != nil {
		return err
	}
	if db.GetDriverName() == driverPostgres {
		var constraintName string
		err := db.QueryRow(`SELECT conname FROM pg_constraint
			WHERE conrelid = 'request_types'::regclass AND contype = 'f'
				AND pg_get_constraintdef(oid) LIKE '%workspace_id%'`).Scan(&constraintName)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("find request_types workspace constraint: %w", err)
		}
		if constraintName != "" {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE request_types DROP CONSTRAINT %q", constraintName)); err != nil {
				return fmt.Errorf("drop request_types workspace constraint: %w", err)
			}
		}
		if _, err := db.Exec(`ALTER TABLE request_types ADD CONSTRAINT request_types_workspace_id_fkey
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE`); err != nil {
			return fmt.Errorf("cascade request_types workspace: %w", err)
		}
		return nil
	}
	return rebuildRequestTypesWorkspacePinned(db)
}

// backfillRequestTypeWorkspaces assigns a concrete workspace to every legacy
// NULL route that can have one. A single-workspace channel is unambiguous; a
// multi-workspace channel keeps today's behavior by pinning the first
// configured workspace. A route whose channel serves no workspace stays NULL
// and is rejected at runtime instead of being deleted.
func backfillRequestTypeWorkspaces(db Database) error {
	rows, err := db.Query(`
		SELECT rt.id, c.type, COALESCE(c.config, '{}')
		FROM request_types rt
		JOIN channels c ON c.id = rt.channel_id
		WHERE rt.workspace_id IS NULL
	`)
	if err != nil {
		return fmt.Errorf("list unpinned request types: %w", err)
	}
	type unpinned struct {
		id          int
		channelType string
		configJSON  string
	}
	var pending []unpinned
	for rows.Next() {
		var row unpinned
		if err := rows.Scan(&row.id, &row.channelType, &row.configJSON); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan unpinned request type: %w", err)
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate unpinned request types: %w", err)
	}
	_ = rows.Close()

	for _, row := range pending {
		var cfg struct {
			PortalWorkspaceIDs []int `json:"portal_workspace_ids"`
			FormWorkspaceIDs   []int `json:"form_workspace_ids"`
		}
		if err := json.Unmarshal([]byte(row.configJSON), &cfg); err != nil {
			return fmt.Errorf("parse channel config for request type %d: %w", row.id, err)
		}
		served := cfg.PortalWorkspaceIDs
		if row.channelType == "form" {
			served = cfg.FormWorkspaceIDs
		}
		if len(served) == 0 {
			slog.Warn("request type has no workspace to pin; leaving it unpinned",
				"component", "migrations", "request_type_id", row.id)
			continue
		}
		if _, err := db.ExecWrite(`UPDATE request_types SET workspace_id = ? WHERE id = ?`, served[0], row.id); err != nil {
			return fmt.Errorf("pin request type %d: %w", row.id, err)
		}
	}
	return nil
}

// rebuildRequestTypesWorkspacePinned rebuilds request_types for SQLite, which
// cannot alter a foreign-key action in place.
func rebuildRequestTypesWorkspacePinned(db Database) (retErr error) {
	sqliteDB, ok := db.(*SQLiteDB)
	if !ok {
		return fmt.Errorf("expected SQLite database, got %T", db)
	}

	ctx := context.Background()
	conn, err := sqliteDB.writeConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire SQLite write connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var foreignKeysEnabled bool
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeysEnabled {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); retErr == nil && err != nil {
				retErr = fmt.Errorf("restore foreign keys: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin request_types rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE request_types_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			item_type_id INTEGER NOT NULL,
			icon TEXT DEFAULT 'FileText',
			color TEXT DEFAULT '#6b7280',
			display_order INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT TRUE,
			config TEXT DEFAULT NULL,
			visibility_group_ids TEXT DEFAULT NULL,
			visibility_org_ids TEXT DEFAULT NULL,
			workspace_id INTEGER DEFAULT NULL,
			title_template TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
			FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE RESTRICT,
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`,
		`INSERT INTO request_types_migration (id, channel_id, name, description, item_type_id, icon, color, display_order, is_active, config, visibility_group_ids, visibility_org_ids, workspace_id, title_template, kind, created_at, updated_at)
			SELECT id, channel_id, name, description, item_type_id, icon, color, display_order, is_active, config, visibility_group_ids, visibility_org_ids, workspace_id, title_template, kind, created_at, updated_at FROM request_types`,
		`DROP TABLE request_types`,
		`ALTER TABLE request_types_migration RENAME TO request_types`,
		`CREATE INDEX IF NOT EXISTS idx_request_types_name ON request_types(name)`,
		`CREATE INDEX IF NOT EXISTS idx_request_types_display_order ON request_types(display_order)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild request_types: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit request_types rebuild: %w", err)
	}
	return nil
}

// checkEmailTrackingCompletedBackfill reports whether the completed_at backfill
// has nothing left to do. A fresh install (empty table) and an already-migrated
// database both return true, so the UPDATE only runs on an upgrade that still
// has legacy rows. The predicate mirrors the UPDATE exactly.
func checkEmailTrackingCompletedBackfill(db Database) (bool, error) {
	stale := "datetime('now', '-1 hour')"
	if db.GetDriverName() == driverPostgres {
		stale = "NOW() - INTERVAL '1 hour'"
	}
	var pending int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM email_message_tracking
		WHERE completed_at IS NULL
		  AND rate_limited_at IS NULL
		  AND (item_id IS NOT NULL
		       OR comment_id IS NOT NULL
		       OR processed_at < ` + stale + `)
	`).Scan(&pending)
	if err != nil {
		return false, err
	}
	return pending == 0, nil
}

// checkNotificationDigestLinksMigration reports the migration as already
// applied when the digest template already renders the action URL. Trivially
// true on fresh installs, where the table is empty at migration time.
func checkNotificationDigestLinksMigration(db Database) (bool, error) {
	var stale int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM notification_templates
		WHERE name = 'notification_batch' AND content NOT LIKE '%ActionURL%'
	`).Scan(&stale)
	if err != nil {
		return false, err
	}
	return stale == 0, nil
}

// notificationDigestLinksMigration rewrites the shipped digest template so the
// action URL the sender now provides becomes a clickable link. Each REPLACE is
// scoped to the exact shipped markup, so admin edits elsewhere survive and
// re-running is a no-op.
const notificationDigestLinksMigration = `
UPDATE notification_templates
SET content = REPLACE(
    REPLACE(
        content,
        '<div style="font-weight:600;font-size:14px;color:#0f172a;margin-bottom:4px;">{{.Title}}</div>',
        '<div style="font-weight:600;font-size:14px;color:#0f172a;margin-bottom:4px;">{{if .ActionURL}}<a href="{{.ActionURL}}" style="color:#0f172a;text-decoration:none;">{{.Title}}</a>{{else}}{{.Title}}{{end}}</div>'
    ),
    '<div style="font-size:12px;color:#9ca3af;margin-top:8px;">{{.FormattedTime}}</div>
</td>',
    '<div style="font-size:12px;color:#9ca3af;margin-top:8px;">{{.FormattedTime}}</div>{{if .ActionURL}}<div style="font-size:13px;margin-top:8px;"><a href="{{.ActionURL}}" style="color:#2874bb;text-decoration:underline;">View in Windshift</a></div>{{end}}
</td>'
)
WHERE name = 'notification_batch';
UPDATE notification_templates
SET text_body = REPLACE(
    text_body,
    '  {{.FormattedTime}}

{{end}}',
    '  {{.FormattedTime}}{{if .ActionURL}}
  View: {{.ActionURL}}{{end}}

{{end}}'
)
WHERE name = 'notification_batch';
`

// checkPortalReplyMarkdownMigration reports the migration as already applied
// when the portal reply template no longer renders raw {{.Content}}. Trivially
// true on fresh installs, where the table is empty at migration time.
func checkPortalReplyMarkdownMigration(db Database) (bool, error) {
	var stale int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM notification_templates
		WHERE name = 'portal_reply' AND content LIKE '%{{.Content}}%'
	`).Scan(&stale)
	if err != nil {
		return false, err
	}
	return stale == 0, nil
}

// portalReplyMarkdownCSS styles the rendered Markdown block for email clients.
// Kept quote-free so it embeds cleanly in the migration's SQL string literal.
const portalReplyMarkdownCSS = `<style>
.ws-md p{margin:0 0 12px;}
.ws-md p:last-child{margin-bottom:0;}
.ws-md a{color:#2874bb;text-decoration:underline;}
.ws-md h1,.ws-md h2,.ws-md h3{margin:16px 0 8px;color:#0f172a;}
.ws-md ul,.ws-md ol{margin:0 0 12px;padding-left:22px;}
.ws-md li{margin:0 0 4px;}
.ws-md table{border-collapse:collapse;width:100%;margin:0 0 12px;}
.ws-md th,.ws-md td{border:1px solid #d1d5db;padding:6px 10px;text-align:left;vertical-align:top;}
.ws-md th{background:#f3f4f6;font-weight:600;}
.ws-md code{font-family:ui-monospace,monospace;font-size:13px;background:#f3f4f6;padding:1px 4px;border-radius:3px;}
.ws-md pre{background:#f3f4f6;padding:12px;border-radius:6px;overflow-x:auto;}
.ws-md pre code{background:none;padding:0;}
.ws-md blockquote{margin:0 0 12px;padding:0 12px;border-left:3px solid #d1d5db;color:#4b5563;}
</style>`

// portalReplyMarkdownMigration switches the shipped portal reply template to
// render Markdown as HTML, and injects the matching styles into templates that
// predate the shared shell's <style> block. Both statements are idempotent and
// scoped so admin edits elsewhere survive.
const portalReplyMarkdownMigration = `
UPDATE notification_templates
SET content = REPLACE(content, '<title>{{.Subject}}</title>', '<title>{{.Subject}}</title>` + portalReplyMarkdownCSS + `')
WHERE content LIKE '%<title>{{.Subject}}</title>%' AND content NOT LIKE '%ws-md table%';
UPDATE notification_templates
SET content = REPLACE(REPLACE(content, '{{.Content}}', '{{markdown .Content}}'), 'white-space:pre-wrap;', '')
WHERE name = 'portal_reply';
`

// checkTransactionalEmailCopyMigration reports the migration as already applied
// when no shipped template still carries the old copy. Trivially true on fresh
// installs, where the table is empty at migration time (templates are seeded
// afterwards).
func checkTransactionalEmailCopyMigration(db Database) (bool, error) {
	var stale int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM notification_templates
		WHERE (name IN ('magic_link', 'approval_requested') AND (content LIKE '%valid for 15 minutes%' OR text_body LIKE '%valid for 15 minutes%'))
		   OR (name = 'email_verification' AND (content LIKE '%expires in 24 hours%' OR text_body LIKE '%expires in 24 hours%'))
		   OR (name = 'invitation' AND (content LIKE '%expires in 7 days%' OR text_body LIKE '%expires in 7 days%'))
		   OR (name = 'portal_reply' AND content LIKE '%please do not reply%')
	`).Scan(&stale)
	if err != nil {
		return false, err
	}
	return stale == 0, nil
}

// transactionalEmailCopyMigration rewrites the shipped transactional email copy
// in place. Each REPLACE targets only the exact phrase the default used to
// contain, so admin edits to other parts of a template survive and re-running is
// a no-op. Templates that no longer carry the phrase are left untouched.
const transactionalEmailCopyMigration = `
UPDATE notification_templates SET content = REPLACE(content, 'The link is valid for 15 minutes.', 'The link is valid for {{.ExpiresIn}}.'), text_body = REPLACE(text_body, 'The link is valid for 15 minutes:', 'The link is valid for {{.ExpiresIn}}:') WHERE name = 'magic_link';
UPDATE notification_templates SET content = REPLACE(content, 'The link is valid for 15 minutes.', 'The link is valid for {{.ExpiresIn}}.'), text_body = REPLACE(text_body, 'The link is valid for 15 minutes:', 'The link is valid for {{.ExpiresIn}}:') WHERE name = 'approval_requested';
UPDATE notification_templates SET content = REPLACE(content, 'This link expires in 24 hours.', 'This link expires in {{.ExpiresIn}}.'), text_body = REPLACE(text_body, 'This link expires in 24 hours.', 'This link expires in {{.ExpiresIn}}.') WHERE name = 'email_verification';
UPDATE notification_templates SET content = REPLACE(content, 'This invitation expires in 7 days.', 'This invitation expires in {{.ExpiresIn}}.'), text_body = REPLACE(text_body, 'This invitation expires in 7 days.', 'This invitation expires in {{.ExpiresIn}}.') WHERE name = 'invitation';
UPDATE notification_templates SET content = REPLACE(content, 'This is an automated email — please do not reply.', 'You''re receiving this because you have an open request with us.') WHERE name = 'portal_reply';
`

// viewSettingsToolsBackfillIDs lists the workspace tools ids as they existed
// when the migration shipped. The list is deliberately frozen: later id
// additions must not change what this one-time backfill does.
var viewSettingsToolsBackfillIDs = []string{
	"queue", "agents", "iterations", "milestones", "analytics", "actions", "pages",
}

// checkViewSettingsToolsBackfill reports the migration as already applied
// when no workspace-scope override lacks the tools ids — trivially true on
// fresh installs, where no board configuration rows exist yet.
func checkViewSettingsToolsBackfill(db Database) (bool, error) {
	rows, err := db.Query(
		`SELECT view_settings FROM board_configurations WHERE workspace_id IS NOT NULL AND view_settings IS NOT NULL`,
	)
	if err != nil {
		return false, fmt.Errorf("read board configuration view settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, fmt.Errorf("scan view settings: %w", err)
		}
		var settings struct {
			EnabledViews *[]string `json:"enabled_views"`
		}
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			continue
		}
		if settings.EnabledViews == nil || len(*settings.EnabledViews) == 0 {
			continue
		}
		needsBackfill := !slices.ContainsFunc(*settings.EnabledViews, func(id string) bool {
			return slices.Contains(viewSettingsToolsBackfillIDs, id)
		})
		if needsBackfill {
			return false, nil
		}
	}
	return rows.Err() == nil, rows.Err()
}

// applyViewSettingsToolsBackfill appends the tools ids to workspace-scope
// view-visibility overrides that predate toggleable tools entries. Such
// overrides can only hold the six collection views; without the backfill the
// tools entries would read as deliberately disabled. Idempotent: rows that
// already name any tools id are left alone.
func applyViewSettingsToolsBackfill(db Database) error {
	rows, err := db.Query(
		`SELECT id, view_settings FROM board_configurations WHERE workspace_id IS NOT NULL AND view_settings IS NOT NULL`,
	)
	if err != nil {
		return fmt.Errorf("read board configuration view settings: %w", err)
	}
	defer rows.Close()

	type backfill struct {
		id   int
		next string
	}
	var updates []backfill
	for rows.Next() {
		var id int
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return fmt.Errorf("scan view settings: %w", err)
		}
		var settings struct {
			EnabledViews *[]string `json:"enabled_views"`
		}
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			// Unreadable legacy payload: read-side normalization already
			// tolerates it, so there is nothing to backfill.
			continue
		}
		if settings.EnabledViews == nil || len(*settings.EnabledViews) == 0 {
			continue
		}
		hasTools := slices.ContainsFunc(*settings.EnabledViews, func(id string) bool {
			return slices.Contains(viewSettingsToolsBackfillIDs, id)
		})
		if hasTools {
			continue
		}
		merged := append(slices.Clone(*settings.EnabledViews), viewSettingsToolsBackfillIDs...)
		out, err := json.Marshal(struct {
			EnabledViews []string `json:"enabled_views"`
		}{merged})
		if err != nil {
			return fmt.Errorf("marshal backfilled view settings: %w", err)
		}
		updates = append(updates, backfill{id: id, next: string(out)})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate view settings: %w", err)
	}
	for _, u := range updates {
		if _, err := db.Exec(
			`UPDATE board_configurations SET view_settings = ? WHERE id = ?`, u.next, u.id,
		); err != nil {
			return fmt.Errorf("backfill view settings for configuration %d: %w", u.id, err)
		}
	}
	return nil
}

func applySQLitePersonalLabelsPerUserUnique(db Database) (retErr error) {
	sqliteDB, ok := db.(*SQLiteDB)
	if !ok {
		return fmt.Errorf("expected SQLite database, got %T", db)
	}

	ctx := context.Background()
	conn, err := sqliteDB.writeConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire SQLite write connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var foreignKeysEnabled bool
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeysEnabled {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); retErr == nil && err != nil {
				retErr = fmt.Errorf("restore foreign keys: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin personal_labels rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE personal_labels_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			color TEXT DEFAULT '#3B82F6',
			user_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`INSERT INTO personal_labels_migration (id, name, color, user_id, created_at, updated_at)
			SELECT id, name, color, user_id, created_at, updated_at FROM personal_labels`,
		`DROP TABLE personal_labels`,
		`ALTER TABLE personal_labels_migration RENAME TO personal_labels`,
		`CREATE INDEX idx_personal_labels_user_id ON personal_labels(user_id)`,
		`CREATE UNIQUE INDEX uq_personal_labels_user_name ON personal_labels(COALESCE(user_id, 0), name)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild personal_labels: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit personal_labels rebuild: %w", err)
	}

	return nil
}

// applySQLiteEmailReplyOutboxPerRecipient rebuilds email_reply_outbox without
// the inline UNIQUE(comment_id) constraint and adds the per-recipient unique
// index (WI-1136). SQLite cannot drop the constraint in place.
func applySQLiteEmailReplyOutboxPerRecipient(db Database) (retErr error) {
	sqliteDB, ok := db.(*SQLiteDB)
	if !ok {
		return fmt.Errorf("expected SQLite database, got %T", db)
	}

	ctx := context.Background()
	conn, err := sqliteDB.writeConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire SQLite write connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var foreignKeysEnabled bool
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeysEnabled {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); retErr == nil && err != nil {
				retErr = fmt.Errorf("restore foreign keys: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin email_reply_outbox rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE email_reply_outbox_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			comment_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			item_id INTEGER NOT NULL,
			to_email TEXT NOT NULL,
			to_name TEXT NOT NULL DEFAULT '',
			subject TEXT NOT NULL,
			html_body TEXT NOT NULL,
			text_body TEXT NOT NULL,
			message_id TEXT NOT NULL,
			in_reply_to TEXT NOT NULL DEFAULT '',
			references_json TEXT NOT NULL DEFAULT '[]',
			from_email TEXT NOT NULL,
			from_name TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			lease_owner TEXT,
			last_error TEXT,
			delivered_at DATETIME,
			discarded_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
			FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
			FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
		)`,
		`INSERT INTO email_reply_outbox_migration (
			id, comment_id, channel_id, item_id, to_email, to_name, subject, html_body, text_body,
			message_id, in_reply_to, references_json, from_email, from_name, attempt_count,
			next_attempt_at, lease_owner, last_error, delivered_at, discarded_at, created_at, updated_at
		)
		SELECT id, comment_id, channel_id, item_id, to_email, to_name, subject, html_body, text_body,
			message_id, in_reply_to, references_json, from_email, from_name, attempt_count,
			next_attempt_at, lease_owner, last_error, delivered_at, discarded_at, created_at, updated_at
		FROM email_reply_outbox`,
		`DROP TABLE email_reply_outbox`,
		`ALTER TABLE email_reply_outbox_migration RENAME TO email_reply_outbox`,
		`CREATE INDEX idx_email_reply_outbox_pending ON email_reply_outbox(delivered_at, next_attempt_at)`,
		`CREATE UNIQUE INDEX uq_email_reply_outbox_comment_recipient ON email_reply_outbox(comment_id, to_email)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild email_reply_outbox: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit email_reply_outbox rebuild: %w", err)
	}
	return nil
}

// applySQLiteItemHistoryPortalActors rebuilds item_history with a nullable
// user_id plus the actor_kind / actor_portal_customer_id columns. SQLite
// cannot drop a NOT NULL constraint in place, so the rows are copied into a
// fresh table and swapped. Existing rows all had a user actor, so they keep
// user_id and the default actor_kind 'user'.
func applySQLiteItemHistoryPortalActors(db Database) (retErr error) {
	sqliteDB, ok := db.(*SQLiteDB)
	if !ok {
		return fmt.Errorf("expected SQLite database, got %T", db)
	}

	ctx := context.Background()
	conn, err := sqliteDB.writeConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire SQLite write connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var foreignKeysEnabled bool
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeysEnabled {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); retErr == nil && err != nil {
				retErr = fmt.Errorf("restore foreign keys: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin item_history rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE item_history_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id INTEGER NOT NULL,
			user_id INTEGER,
			actor_kind TEXT NOT NULL DEFAULT 'user',
			actor_portal_customer_id INTEGER,
			changed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			field_name TEXT NOT NULL,
			old_value TEXT,
			new_value TEXT,
			FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
			FOREIGN KEY (actor_portal_customer_id) REFERENCES portal_customers(id) ON DELETE SET NULL
		)`,
		`INSERT INTO item_history_migration (id, item_id, user_id, actor_kind, changed_at, field_name, old_value, new_value)
			SELECT id, item_id, user_id, 'user', changed_at, field_name, old_value, new_value FROM item_history`,
		`DROP TABLE item_history`,
		`ALTER TABLE item_history_migration RENAME TO item_history`,
		`CREATE INDEX idx_item_history_item_id_changed_at ON item_history(item_id, changed_at DESC)`,
		`CREATE INDEX idx_item_history_current_status_latest
			ON item_history(item_id, new_value, changed_at DESC)
			WHERE field_name = 'status_id'`,
		`CREATE INDEX idx_item_history_user_id ON item_history(user_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild item_history: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit item_history rebuild: %w", err)
	}

	return nil
}

func applySQLiteSSOAttributeMappingDefault(db Database) (retErr error) {
	sqliteDB, ok := db.(*SQLiteDB)
	if !ok {
		return fmt.Errorf("expected SQLite database, got %T", db)
	}

	ctx := context.Background()
	conn, err := sqliteDB.writeConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire SQLite write connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var foreignKeysEnabled bool
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeysEnabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeysEnabled {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); retErr == nil && err != nil {
				retErr = fmt.Errorf("restore foreign keys: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SSO provider rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE sso_providers_migration (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slug TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL DEFAULT 'oidc',
			enabled BOOLEAN DEFAULT FALSE,
			is_default BOOLEAN DEFAULT FALSE,
			issuer_url TEXT,
			client_id TEXT,
			client_secret_encrypted TEXT,
			scopes TEXT DEFAULT 'openid email profile',
			auto_provision_users BOOLEAN DEFAULT FALSE,
			require_verified_email BOOLEAN DEFAULT TRUE,
			attribute_mapping TEXT DEFAULT '{"email":"email","name":"name","given_name":"given_name","family_name":"family_name","username":"preferred_username","email_verified":"email_verified"}',
			saml_idp_metadata_url TEXT,
			saml_idp_sso_url TEXT,
			saml_idp_certificate TEXT,
			saml_sp_entity_id TEXT,
			saml_sign_requests BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO sso_providers_migration (
			id, slug, name, provider_type, enabled, is_default,
			issuer_url, client_id, client_secret_encrypted, scopes,
			auto_provision_users, require_verified_email, attribute_mapping,
			saml_idp_metadata_url, saml_idp_sso_url, saml_idp_certificate,
			saml_sp_entity_id, saml_sign_requests, created_at, updated_at
		) SELECT
			id, slug, name, provider_type, enabled, is_default,
			issuer_url, client_id, client_secret_encrypted, scopes,
			auto_provision_users, require_verified_email, attribute_mapping,
			saml_idp_metadata_url, saml_idp_sso_url, saml_idp_certificate,
			saml_sp_entity_id, saml_sign_requests, created_at, updated_at
		FROM sso_providers`,
		`DROP TABLE sso_providers`,
		`ALTER TABLE sso_providers_migration RENAME TO sso_providers`,
		`CREATE INDEX idx_sso_providers_slug ON sso_providers(slug)`,
		`CREATE INDEX idx_sso_providers_enabled ON sso_providers(enabled)`,
		`CREATE INDEX idx_sso_providers_default ON sso_providers(is_default)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild sso_providers: %w", err)
		}
	}

	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("check foreign keys after SSO provider rebuild: %w", err)
	}
	if rows.Next() {
		var table, parent string
		var rowID any
		var foreignKeyID int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read foreign key violation after SSO provider rebuild: %w", err)
		}
		_ = rows.Close()
		return fmt.Errorf("foreign key violation after SSO provider rebuild: table %s row %v references %s constraint %d", table, rowID, parent, foreignKeyID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("check foreign keys after SSO provider rebuild: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close foreign key check after SSO provider rebuild: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SSO provider rebuild: %w", err)
	}

	return nil
}

// checkIntakeSplitMigration reports whether the intake split has been applied.
func checkIntakeSplitMigration(db Database) (bool, error) {
	var tableCheck, trackingCol, itemsCol string
	if db.GetDriverName() == driverPostgres {
		tableCheck = pgTableCheck("intakes")
		trackingCol = pgColumnCheck("email_message_tracking", "intake_id")
		itemsCol = pgColumnCheck("items", "intake_id")
	} else {
		tableCheck = sqliteTableCheck("intakes")
		trackingCol = sqliteColumnCheck("email_message_tracking", "intake_id")
		itemsCol = sqliteColumnCheck("items", "intake_id")
	}
	for _, q := range []string{tableCheck, trackingCol, itemsCol} {
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	return true, nil
}

const intakesTableSQLite = `CREATE TABLE IF NOT EXISTS intakes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	mailbox_id INTEGER NOT NULL,
	folder TEXT NOT NULL DEFAULT 'INBOX',
	target_type TEXT NOT NULL,
	target_id INTEGER NOT NULL,
	request_type_id INTEGER,
	item_type_id INTEGER,
	rate_limit_per_hour INTEGER,
	processing_disposition TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'enabled',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (mailbox_id) REFERENCES channels(id) ON DELETE CASCADE
)`

const intakesTablePostgres = `CREATE TABLE IF NOT EXISTS intakes (
	id SERIAL PRIMARY KEY,
	mailbox_id INTEGER NOT NULL,
	folder TEXT NOT NULL DEFAULT 'INBOX',
	target_type TEXT NOT NULL,
	target_id INTEGER NOT NULL,
	request_type_id INTEGER,
	item_type_id INTEGER,
	rate_limit_per_hour INTEGER,
	processing_disposition TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'enabled',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (mailbox_id) REFERENCES channels(id) ON DELETE CASCADE
)`

const intakeStateTableSQLite = `CREATE TABLE IF NOT EXISTS email_intake_state (
	intake_id INTEGER PRIMARY KEY,
	last_uid INTEGER DEFAULT 0,
	uid_validity INTEGER DEFAULT 0,
	failed_message_uid INTEGER NOT NULL DEFAULT 0,
	failed_message_uid_validity INTEGER NOT NULL DEFAULT 0,
	failed_message_count INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (intake_id) REFERENCES intakes(id) ON DELETE CASCADE
)`

const intakeStateTablePostgres = `CREATE TABLE IF NOT EXISTS email_intake_state (
	intake_id INTEGER PRIMARY KEY,
	last_uid INTEGER DEFAULT 0,
	uid_validity BIGINT DEFAULT 0,
	failed_message_uid INTEGER NOT NULL DEFAULT 0,
	failed_message_uid_validity BIGINT NOT NULL DEFAULT 0,
	failed_message_count INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (intake_id) REFERENCES intakes(id) ON DELETE CASCADE
)`

// applyIntakeSplitMigration creates the intake tables, adds provenance columns,
// and mints one intake per existing email channel from its routing config. The
// legacy routing fields stay in the channel config but are no longer read; the
// watermark moves to the per-intake state.
func applyIntakeSplitMigration(db Database) error {
	statements := make([]string, 0, 7)
	if db.GetDriverName() == driverPostgres {
		statements = append(statements,
			intakesTablePostgres,
			intakeStateTablePostgres,
			`ALTER TABLE email_message_tracking ADD COLUMN IF NOT EXISTS intake_id INTEGER`,
			`ALTER TABLE items ADD COLUMN IF NOT EXISTS intake_id INTEGER`,
		)
	} else {
		statements = append(statements,
			intakesTableSQLite,
			intakeStateTableSQLite,
			`ALTER TABLE email_message_tracking ADD COLUMN intake_id INTEGER`,
			`ALTER TABLE items ADD COLUMN intake_id INTEGER`,
		)
	}
	statements = append(statements,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_intakes_mailbox_folder ON intakes(mailbox_id, folder)`,
		`CREATE INDEX IF NOT EXISTS idx_intakes_target ON intakes(target_type, target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_items_intake_id ON items(intake_id)`,
	)
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("intake split ddl: %w", err)
		}
	}
	return backfillIntakesFromChannels(db)
}

// applyIntakeWorkspacePortalMigration reshapes intakes from the
// portal-XOR-workspace target into explicit workspace routing plus an optional
// portal link. Existing rows are preserved: workspace intakes keep their target
// workspace, and portal intakes keep the portal while gaining the workspace and
// item type their system Email request type already resolved. The apply is
// idempotent so a partial run can be retried without a transaction.
func applyIntakeWorkspacePortalMigration(db Database) error {
	pg := db.GetDriverName() == driverPostgres
	addColumn := func(column, typ string) error {
		if pg {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE intakes ADD COLUMN IF NOT EXISTS %s %s", column, typ)); err != nil {
				return fmt.Errorf("add intakes.%s: %w", column, err)
			}
			return nil
		}
		exists, err := intakeColumnExists(db, column)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE intakes ADD COLUMN %s %s", column, typ)); err != nil {
			return fmt.Errorf("add intakes.%s: %w", column, err)
		}
		return nil
	}

	for _, col := range []struct{ name, typ string }{
		{"workspace_id", "INTEGER"},
		{"portal_channel_id", "INTEGER"},
		{"status_reason", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := addColumn(col.name, col.typ); err != nil {
			return err
		}
	}

	if err := backfillIntakeWorkspacePortal(db); err != nil {
		return err
	}

	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_intakes_target`); err != nil {
		return fmt.Errorf("drop idx_intakes_target: %w", err)
	}
	for _, column := range []string{"target_type", "target_id"} {
		exists, err := intakeColumnExists(db, column)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := db.Exec("ALTER TABLE intakes DROP COLUMN " + column); err != nil {
			return fmt.Errorf("drop intakes.%s: %w", column, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_intakes_portal ON intakes(portal_channel_id)`); err != nil {
		return fmt.Errorf("create idx_intakes_portal: %w", err)
	}
	return nil
}

// checkIntakeWorkspacePortalMigration reports whether the reshape is complete:
// the new columns exist and the legacy target columns are gone. A partial run
// leaves it false so the apply is retried instead of stamped.
func checkIntakeWorkspacePortalMigration(db Database) (bool, error) {
	workspace, err := intakeColumnExists(db, "workspace_id")
	if err != nil {
		return false, err
	}
	portal, err := intakeColumnExists(db, "portal_channel_id")
	if err != nil {
		return false, err
	}
	targetType, err := intakeColumnExists(db, "target_type")
	if err != nil {
		return false, err
	}
	targetID, err := intakeColumnExists(db, "target_id")
	if err != nil {
		return false, err
	}
	return workspace && portal && !targetType && !targetID, nil
}

func intakeColumnExists(db Database, column string) (bool, error) {
	var count int
	query := sqliteColumnCheck("intakes", column)
	if db.GetDriverName() == driverPostgres {
		query = pgColumnCheck("intakes", column)
	}
	if err := db.QueryRow(query).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// backfillIntakeWorkspacePortal copies the legacy target into the new columns.
// Workspace intakes keep their workspace; portal intakes keep the portal and
// inherit the workspace and item type of the portal's system Email request
// type. Rows that cannot be resolved are parked as needs_attention rather than
// silently dropped or routed somewhere wrong.
func backfillIntakeWorkspacePortal(db Database) error {
	rows, err := db.Query(`SELECT id, target_type, target_id FROM intakes`)
	if err != nil {
		return fmt.Errorf("list intakes for workspace/portal backfill: %w", err)
	}
	type legacyIntake struct {
		id         int
		targetType string
		targetID   int
	}
	var legacy []legacyIntake
	for rows.Next() {
		var in legacyIntake
		if err := rows.Scan(&in.id, &in.targetType, &in.targetID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan intake for workspace/portal backfill: %w", err)
		}
		legacy = append(legacy, in)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate intakes for workspace/portal backfill: %w", err)
	}
	_ = rows.Close()

	for _, in := range legacy {
		switch in.targetType {
		case "workspace":
			if _, err := db.ExecWrite(`UPDATE intakes SET workspace_id = ? WHERE id = ?`, in.targetID, in.id); err != nil {
				return fmt.Errorf("backfill workspace intake %d: %w", in.id, err)
			}
		case "portal":
			workspaceID, itemTypeID, ok := migrationLegacyPortalRouting(db, in.targetID)
			if !ok {
				if _, err := db.ExecWrite(`
					UPDATE intakes
					SET portal_channel_id = ?, status = 'needs_attention',
					    status_reason = 'Portal no longer resolves a workspace; re-point this intake'
					WHERE id = ?
				`, in.targetID, in.id); err != nil {
					return fmt.Errorf("flag portal intake %d: %w", in.id, err)
				}
				continue
			}
			if _, err := db.ExecWrite(`
				UPDATE intakes
				SET portal_channel_id = ?, workspace_id = ?, item_type_id = COALESCE(item_type_id, ?)
				WHERE id = ?
			`, in.targetID, workspaceID, itemTypeID, in.id); err != nil {
				return fmt.Errorf("backfill portal intake %d: %w", in.id, err)
			}
		default:
			if _, err := db.ExecWrite(`
				UPDATE intakes SET status = 'needs_attention',
					status_reason = 'Unknown legacy intake target; re-point this intake'
				WHERE id = ?
			`, in.id); err != nil {
				return fmt.Errorf("flag legacy intake %d: %w", in.id, err)
			}
		}
	}
	return nil
}

// migrationLegacyPortalRouting resolves the workspace and item type a legacy
// portal intake was routing to, preferring the portal's system Email request
// type and falling back to the portal's single served workspace default.
func migrationLegacyPortalRouting(db Database, portalID int) (workspaceID, itemTypeID int, ok bool) {
	var rtWorkspace, rtItemType sql.NullInt64
	err := db.QueryRow(`
		SELECT workspace_id, item_type_id FROM request_types
		WHERE channel_id = ? AND kind = 'email'
		ORDER BY id
		LIMIT 1
	`, portalID).Scan(&rtWorkspace, &rtItemType)
	if err == nil && rtWorkspace.Valid && rtItemType.Valid {
		return int(rtWorkspace.Int64), int(rtItemType.Int64), true
	}

	var configJSON string
	if err := db.QueryRow(`SELECT COALESCE(config, '{}') FROM channels WHERE id = ?`, portalID).Scan(&configJSON); err != nil {
		return 0, 0, false
	}
	var cfg struct {
		PortalWorkspaceIDs []int `json:"portal_workspace_ids"`
	}
	if json.Unmarshal([]byte(configJSON), &cfg) != nil || len(cfg.PortalWorkspaceIDs) != 1 {
		return 0, 0, false
	}
	itemType, err := migrationDefaultItemTypeForWorkspace(db, cfg.PortalWorkspaceIDs[0])
	if err != nil {
		return 0, 0, false
	}
	return cfg.PortalWorkspaceIDs[0], itemType, true
}

func migrationDefaultItemTypeForWorkspace(db Database, workspaceID int) (int, error) {
	var itemTypeID int
	err := db.QueryRow(`
		SELECT cs.default_item_type_id FROM configuration_sets cs
		INNER JOIN workspace_configuration_sets wcs ON cs.id = wcs.configuration_set_id
		INNER JOIN configuration_set_item_types csit
			ON csit.configuration_set_id = cs.id AND csit.item_type_id = cs.default_item_type_id
		WHERE wcs.workspace_id = ? AND cs.default_item_type_id IS NOT NULL
		ORDER BY cs.is_default DESC
		LIMIT 1
	`, workspaceID).Scan(&itemTypeID)
	if err != nil {
		err = db.QueryRow(`
			SELECT csit.item_type_id FROM configuration_set_item_types csit
			INNER JOIN workspace_configuration_sets wcs ON wcs.configuration_set_id = csit.configuration_set_id
			WHERE wcs.workspace_id = ?
			ORDER BY csit.item_type_id
			LIMIT 1
		`, workspaceID).Scan(&itemTypeID)
	}
	if err != nil {
		err = db.QueryRow(`SELECT id FROM item_types WHERE is_default = true ORDER BY id LIMIT 1`).Scan(&itemTypeID)
	}
	return itemTypeID, err
}

// backfillIntakesFromChannels creates one intake per inbound email channel and
// copies the channel's watermark into the new per-intake state so no mail is
// refetched.
func backfillIntakesFromChannels(db Database) error {
	rows, err := db.Query(`
		SELECT id, COALESCE(config, '{}') FROM channels
		WHERE type = 'email' AND direction = 'inbound'
	`)
	if err != nil {
		return fmt.Errorf("list email channels for intake backfill: %w", err)
	}
	type created struct {
		channelID int
		intakeID  int
	}
	var createdIntakes []created
	for rows.Next() {
		var channelID int
		var configJSON string
		if err := rows.Scan(&channelID, &configJSON); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan email channel for intake backfill: %w", err)
		}
		var cfg struct {
			EmailConnectedPortalID     *int   `json:"email_connected_portal_id"`
			EmailWorkspaceID           int    `json:"email_workspace_id"`
			EmailItemTypeID            *int   `json:"email_item_type_id"`
			EmailMailbox               string `json:"email_mailbox"`
			EmailRateLimitPerHour      *int   `json:"email_rate_limit_per_hour"`
			EmailProcessingDisposition string `json:"email_processing_disposition"`
			EmailMarkAsRead            bool   `json:"email_mark_as_read"`
			EmailDeleteAfterProcess    bool   `json:"email_delete_after_process"`
		}
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			continue
		}

		var targetType string
		var targetID int
		var itemTypeID *int
		switch {
		case cfg.EmailConnectedPortalID != nil:
			targetType = "portal"
			targetID = *cfg.EmailConnectedPortalID
		case cfg.EmailWorkspaceID > 0:
			targetType = "workspace"
			targetID = cfg.EmailWorkspaceID
			itemTypeID = cfg.EmailItemTypeID
		default:
			continue
		}
		folder := cfg.EmailMailbox
		if folder == "" {
			folder = "INBOX"
		}
		disposition := cfg.EmailProcessingDisposition
		if disposition == "" {
			switch {
			case cfg.EmailDeleteAfterProcess:
				disposition = "delete"
			case cfg.EmailMarkAsRead:
				disposition = "mark_read"
			default:
				disposition = "leave"
			}
		}

		var intakeID int
		err := db.QueryRow(`
			INSERT INTO intakes (
				mailbox_id, folder, target_type, target_id, request_type_id, item_type_id,
				rate_limit_per_hour, processing_disposition, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, NULL, ?, ?, ?, 'enabled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, channelID, folder, targetType, targetID, itemTypeID, cfg.EmailRateLimitPerHour, disposition).Scan(&intakeID)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("create intake for channel %d: %w", channelID, err)
		}
		createdIntakes = append(createdIntakes, created{channelID: channelID, intakeID: intakeID})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate email channels for intake backfill: %w", err)
	}
	_ = rows.Close()

	for _, c := range createdIntakes {
		if _, err := db.ExecWrite(`
			INSERT INTO email_intake_state (
				intake_id, last_uid, uid_validity,
				failed_message_uid, failed_message_uid_validity, failed_message_count,
				created_at, updated_at
			)
			SELECT ?, last_uid, uid_validity,
			       failed_message_uid, failed_message_uid_validity, failed_message_count,
			       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			FROM email_channel_state WHERE channel_id = ?
			ON CONFLICT(intake_id) DO NOTHING
		`, c.intakeID, c.channelID); err != nil {
			return fmt.Errorf("copy channel %d watermark to intake %d: %w", c.channelID, c.intakeID, err)
		}
	}

	// Provenance for already-ingested mail: attribute historical rows to the
	// single intake that now represents their channel.
	if _, err := db.ExecWrite(`
		UPDATE email_message_tracking SET intake_id = (
			SELECT i.id FROM intakes i WHERE i.mailbox_id = email_message_tracking.channel_id ORDER BY i.id LIMIT 1
		)
		WHERE intake_id IS NULL AND channel_id IN (SELECT id FROM channels WHERE type = 'email')
	`); err != nil {
		return fmt.Errorf("backfill tracking intake_id: %w", err)
	}
	if _, err := db.ExecWrite(`
		UPDATE items SET intake_id = (
			SELECT i.id FROM intakes i WHERE i.mailbox_id = items.channel_id ORDER BY i.id LIMIT 1
		)
		WHERE intake_id IS NULL AND channel_id IN (SELECT id FROM channels WHERE type = 'email')
	`); err != nil {
		return fmt.Errorf("backfill item intake_id: %w", err)
	}
	return nil
}

func (m Migration) checksum(driver string) string {
	var body string
	switch driver {
	case driverSQLite:
		body = m.SQLite
	case driverPostgres:
		body = m.Postgres
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// runPendingMigrations applies catalog entries that aren't yet stamped in
// schema_migrations. For each pending migration: if its backend-specific
// Check predicate reports the effect is already present, the row is stamped
// without re-running the DDL (retroactive backfill); otherwise the DDL runs
// inside a transaction that ends with the stamp INSERT so the pair is
// atomic.
//
// Errors abort startup. There is no log-and-continue.
func runPendingMigrations(db Database, catalog []Migration) error {
	driver := db.GetDriverName()

	applied, err := loadAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("load applied migrations: %w", err)
	}

	for _, m := range catalog {
		if checksum, ok := applied[m.Version]; ok {
			expected := m.checksum(driver)
			if checksum != "" && checksum != expected && !m.ReconcileChecksum && !m.acceptsSuperseded(checksum) {
				return fmt.Errorf(
					"migration %s (%s): checksum mismatch: stored %s, expected %s",
					m.Version, m.Name, checksum, expected,
				)
			}
			// Backfill an unstamped row and bring recognized historical or
			// intentionally mutable checksums forward to the current value.
			if checksum != expected {
				if _, err := db.Exec(
					"UPDATE schema_migrations SET name = ?, checksum = ? WHERE version = ?",
					m.Name, expected, m.Version,
				); err != nil {
					return fmt.Errorf("migration %s (%s): restamp checksum: %w", m.Version, m.Name, err)
				}
			}
			continue
		}
		if err := applyMigration(db, driver, m); err != nil {
			return fmt.Errorf("migration %s (%s): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func loadAppliedMigrations(db Database) (map[string]string, error) {
	rows, err := db.Query("SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, err
		}
		out[version] = checksum
	}
	return out, rows.Err()
}

func applyMigration(db Database, driver string, m Migration) error {
	var checkSQL, body string
	var check func(Database) (bool, error)
	var apply func(Database) error
	switch driver {
	case driverSQLite:
		checkSQL, check, body, apply = m.CheckSQLite, m.CheckSQLiteFn, m.SQLite, m.ApplySQLite
	case driverPostgres:
		checkSQL, check, body, apply = m.CheckPostgres, m.CheckPostgresFn, m.Postgres, m.ApplyPostgres
	default:
		return fmt.Errorf("unknown driver %q", driver)
	}

	// Migration is a no-op on this backend — stamp without running anything.
	if body == "" {
		return stampMigration(db, m, driver)
	}

	// Retroactive backfill: if the effect is already present, stamp without
	// re-running. Migrations with no Check always run.
	if check != nil {
		alreadyApplied, err := check(db)
		if err != nil {
			return fmt.Errorf("check: %w", err)
		}
		if alreadyApplied {
			return stampMigration(db, m, driver)
		}
	} else if checkSQL != "" {
		var count int
		if err := db.QueryRow(checkSQL).Scan(&count); err != nil {
			return fmt.Errorf("check: %w", err)
		}
		if count > 0 {
			return stampMigration(db, m, driver)
		}
	}
	if apply != nil {
		if err := apply(db); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		return stampMigration(db, m, driver)
	}

	return WithTx(db, func(tx Tx) error {
		if _, err := tx.Exec(body); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		_, err := tx.Exec(
			"INSERT INTO schema_migrations(version, name, checksum) VALUES(?, ?, ?)",
			m.Version, m.Name, m.checksum(driver),
		)
		return err
	})
}

func stampMigration(db Database, m Migration, driver string) error {
	_, err := db.Exec(
		"INSERT INTO schema_migrations(version, name, checksum) VALUES(?, ?, ?)",
		m.Version, m.Name, m.checksum(driver),
	)
	return err
}
