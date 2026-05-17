package database

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// backfillPostgresTimestampTZ converts every column declared
// `TIMESTAMP WITHOUT TIME ZONE` (i.e. plain `TIMESTAMP` in DDL) to
// `TIMESTAMPTZ` (TIMESTAMP WITH TIME ZONE) on existing databases. New
// installs already get TIMESTAMPTZ from the *_postgres.sql files; this
// function exists for installs that predate that change.
//
// Why we bother: plain TIMESTAMP has no timezone in the stored value, so
// what wall-clock you get back depends on the *session* timezone of the
// reading client. With lib/pq's session forced to UTC (see
// ensurePostgresTimezoneUTC), writes and reads round-trip correctly, but
// any client that connects with a different timezone — psql, pgAdmin,
// dashboard tools, replicas, BI exports — sees a shifted value. TIMESTAMPTZ
// stores the UTC instant explicitly, so cross-client interpretation stays
// consistent.
//
// USING value AT TIME ZONE 'UTC' tells Postgres "interpret the existing
// wall-clock as UTC". This matches our app convention (the Go app always
// writes time.Time values that lib/pq formats in the session's tz, which is
// now UTC). Legacy rows written when the session tz was *not* UTC will be
// reinterpreted as UTC — meaning they're wrong by the previous server's
// UTC-offset hours. That is the least-bad assumption: the alternative is
// to leave them as plain TIMESTAMP forever, which keeps the ambiguity.
//
// Idempotent: skips columns that are already TIMESTAMPTZ. Schemas owned by
// Postgres itself (pg_catalog, information_schema, pg_*) are filtered out.
func backfillPostgresTimestampTZ(db *sql.DB) error {
	columns, err := discoverPostgresTimestampColumns(db)
	if err != nil {
		return fmt.Errorf("discover TIMESTAMP columns: %w", err)
	}
	if len(columns) == 0 {
		return nil
	}

	converted := 0
	failed := 0
	for _, c := range columns {
		// Identifiers come from information_schema so they're already valid,
		// but double-quote anyway in case a column name shadows a reserved word.
		stmt := fmt.Sprintf(
			`ALTER TABLE %s.%s ALTER COLUMN %s TYPE TIMESTAMPTZ USING %s AT TIME ZONE 'UTC'`,
			quotePGIdent(c.schema), quotePGIdent(c.table), quotePGIdent(c.column), quotePGIdent(c.column),
		)
		if _, err := db.Exec(stmt); err != nil {
			// Don't abort startup on one bad column — e.g. a view-backed
			// table or a column with a constraint we can't ALTER through.
			// Log and continue; operators can rerun manually.
			failed++
			slog.Warn("timestamp -> timestamptz alter failed",
				slog.String("component", "database"),
				slog.String("schema", c.schema),
				slog.String("table", c.table),
				slog.String("column", c.column),
				slog.Any("error", err))
			continue
		}
		converted++
	}
	slog.Info("postgres timestamp backfill complete",
		slog.String("component", "database"),
		slog.Int("converted", converted),
		slog.Int("failed", failed),
		slog.Int("candidates", len(columns)))
	return nil
}

type pgTimestampColumn struct {
	schema string
	table  string
	column string
}

// discoverPostgresTimestampColumns returns every (schema, table, column)
// where the column's data type is `timestamp without time zone`, scoped to
// schemas the user owns (so we don't try to ALTER pg_catalog or extensions).
// information_schema reports columns from views too — we filter those out by
// joining against pg_class with relkind = 'r' (ordinary table) so ALTER
// doesn't blow up trying to type-change a view-derived column.
func discoverPostgresTimestampColumns(db *sql.DB) ([]pgTimestampColumn, error) {
	rows, err := db.Query(`
		SELECT n.nspname, c.relname, a.attname
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_type t ON t.oid = a.atttypid
		WHERE c.relkind = 'r'
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		  AND t.typname = 'timestamp'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_%'
		ORDER BY n.nspname, c.relname, a.attnum
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []pgTimestampColumn
	for rows.Next() {
		var c pgTimestampColumn
		if err := rows.Scan(&c.schema, &c.table, &c.column); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// quotePGIdent quotes a Postgres identifier per the SQL standard: wrap in
// double quotes and double any embedded quotes. Identifiers in this file
// come from pg_attribute / pg_class so this is defensive.
func quotePGIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '"')
	for i := 0; i < len(name); i++ {
		if name[i] == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, name[i])
		}
	}
	out = append(out, '"')
	return string(out)
}
