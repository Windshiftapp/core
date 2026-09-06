package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
)

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
		CheckSQLite: `SELECT CASE WHEN
			NOT EXISTS (SELECT 1 FROM pragma_table_info('zammad_connections') WHERE name='applies_to_all_workspaces')
			OR NOT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='zammad_connection_workspaces')
			THEN 1 ELSE 0 END`,
		CheckPostgres: `SELECT CASE WHEN
			NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='zammad_connections' AND column_name='applies_to_all_workspaces')
			OR NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='zammad_connection_workspaces')
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
		CheckSQLite:   sqliteColumnCheck("asset_import_jobs", "lease_expires_at"),
		CheckPostgres: pgColumnCheck("asset_import_jobs", "lease_expires_at"),
		SQLite:        "ALTER TABLE asset_import_jobs ADD COLUMN lease_expires_at BIGINT",
		Postgres:      "ALTER TABLE asset_import_jobs ADD COLUMN lease_expires_at BIGINT",
	},
	{
		Version:       "20260905_asset_import_upload_ownership",
		Name:          "Bind asset import uploads to their uploader and set",
		CheckSQLite:   sqliteTableCheck("asset_import_uploads"),
		CheckPostgres: pgTableCheck("asset_import_uploads"),
		SQLite: `CREATE TABLE IF NOT EXISTS asset_import_uploads (
 id TEXT PRIMARY KEY,
 set_id INTEGER NOT NULL REFERENCES asset_management_sets(id) ON DELETE CASCADE,
 created_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at BIGINT NOT NULL
);`,
		Postgres: `CREATE TABLE IF NOT EXISTS asset_import_uploads (
 id TEXT PRIMARY KEY,
 set_id INTEGER NOT NULL REFERENCES asset_management_sets(id) ON DELETE CASCADE,
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
