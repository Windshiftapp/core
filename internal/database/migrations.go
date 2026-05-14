package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Driver name constants match the values returned by GetDriverName on each
// backend (SQLiteDB.GetDriverName → "sqlite", PostgresDB.GetDriverName →
// "postgres"). The Database interface comment claims "sqlite3" but that's
// stale; the actual returns are below.
const (
	driverSQLite   = "sqlite"
	driverPostgres = "postgres"
)

// Migration is one entry in the schema_migrations catalog. Version is a
// stable slug used as the schema_migrations primary key; Name is a human
// label. CheckSQLite / CheckPostgres are queries that return COUNT >= 1
// when the migration's effect is already present, used for retroactive
// backfill on existing installs upgrading past the introduction of the
// schema_migrations table. SQLite / Postgres carry the backend-specific
// DDL to apply when the check reports the effect is missing.
//
// An empty Check on a backend means the migration body always runs when
// the version isn't already stamped. An empty body on a backend means
// the migration is skipped on that backend — the row is still stamped
// so the catalog stays consistent across backends.
type Migration struct {
	Version       string
	Name          string
	CheckSQLite   string
	CheckPostgres string
	SQLite        string
	Postgres      string
}

// Catalog is the ordered list of migrations applied via runPendingMigrations.
// New migrations append with a date-prefixed Version slug such as
// "20260514_widgets_archived_at". Order matters only between migrations
// with row dependencies; otherwise entries may be reordered freely.
//
// Currently empty: the legacy migration arrays in database.go and
// postgres.go still own existing-install migrations. They are ported into
// this Catalog in subsequent commits.
var Catalog = []Migration{
	{
		Version: "20260514_email_message_tracking_dedup_key",
		Name:    "Add dedup_key to email_message_tracking",
		// Idempotency check: column already present means the migration ran
		// previously (the legacy unique index gets swapped inside the body).
		CheckSQLite:   "SELECT COUNT(*) FROM pragma_table_info('email_message_tracking') WHERE name='dedup_key'",
		CheckPostgres: "SELECT COUNT(*) FROM information_schema.columns WHERE table_name='email_message_tracking' AND column_name='dedup_key'",
		SQLite: `
			ALTER TABLE email_message_tracking ADD COLUMN dedup_key TEXT NOT NULL DEFAULT '';
			UPDATE email_message_tracking SET dedup_key = CASE
				WHEN message_id IS NOT NULL AND message_id <> '' THEN message_id
				ELSE 'legacy:' || CAST(id AS TEXT)
			END WHERE dedup_key = '';
			DROP INDEX IF EXISTS idx_email_message_tracking_unique;
			CREATE UNIQUE INDEX IF NOT EXISTS idx_email_message_tracking_dedup ON email_message_tracking(channel_id, dedup_key);
		`,
		Postgres: `
			ALTER TABLE email_message_tracking ADD COLUMN dedup_key TEXT NOT NULL DEFAULT '';
			UPDATE email_message_tracking SET dedup_key = CASE
				WHEN message_id IS NOT NULL AND message_id <> '' THEN message_id
				ELSE 'legacy:' || id::text
			END WHERE dedup_key = '';
			DROP INDEX IF EXISTS idx_email_message_tracking_unique;
			CREATE UNIQUE INDEX IF NOT EXISTS idx_email_message_tracking_dedup ON email_message_tracking(channel_id, dedup_key);
		`,
	},
	{
		Version:       "20260514_email_message_tracking_attachments_status",
		Name:          "Add attachments_status to email_message_tracking",
		CheckSQLite:   "SELECT COUNT(*) FROM pragma_table_info('email_message_tracking') WHERE name='attachments_status'",
		CheckPostgres: "SELECT COUNT(*) FROM information_schema.columns WHERE table_name='email_message_tracking' AND column_name='attachments_status'",
		SQLite:        `ALTER TABLE email_message_tracking ADD COLUMN attachments_status TEXT`,
		Postgres:      `ALTER TABLE email_message_tracking ADD COLUMN attachments_status TEXT`,
	},
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
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := applyMigration(db, driver, m); err != nil {
			return fmt.Errorf("migration %s (%s): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func loadAppliedMigrations(db Database) (map[string]struct{}, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}

func applyMigration(db Database, driver string, m Migration) error {
	var checkSQL, body string
	switch driver {
	case driverSQLite:
		checkSQL, body = m.CheckSQLite, m.SQLite
	case driverPostgres:
		checkSQL, body = m.CheckPostgres, m.Postgres
	default:
		return fmt.Errorf("unknown driver %q", driver)
	}

	// Migration is a no-op on this backend — stamp without running anything.
	if body == "" {
		return stampMigration(db, m, driver)
	}

	// Retroactive backfill: if the effect is already present, stamp without
	// re-running. Migrations with no Check always run.
	if checkSQL != "" {
		var count int
		if err := db.QueryRow(checkSQL).Scan(&count); err != nil {
			return fmt.Errorf("check: %w", err)
		}
		if count > 0 {
			return stampMigration(db, m, driver)
		}
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
