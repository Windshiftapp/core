package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// backfillLegacyDatetimeFormat rewrites every DATETIME column value to the
// SQLite-parsable UTC format produced by _time_format=sqlite + _timezone=UTC.
// Runs two passes per column:
//
//   - **Legacy Go-String() format**: rows from the modernc.org/sqlite
//     pre-_time_format default, e.g.
//     "2026-05-15 23:13:34.623059 +0200 CEST m=+3772.605125585". SQLite's
//     DATE/STRFTIME/julianday return NULL on these — they broke GROUP BY DATE
//     (test reports trend → 500), DATE-range filters (review completed-items
//     → silently empty), and duration sorts (action execution logs julianday
//     → NULLs).
//   - **Non-UTC offset rows**: anything ending in an explicit ±HH:MM offset
//     other than +00:00. Without normalization, lex ordering on stored TEXT
//     disagrees with chronological ordering — '+0' < '+2' in ASCII, so a row
//     written at 14:00 CEST (+02:00) lex-sorts AFTER a row written at 14:00
//     UTC (+00:00) even though it happened TWO HOURS EARLIER.
//
// Both passes parse with Go's time.Parse, call .UTC(), and re-format using
// the canonical SQLite-friendly format. Idempotent: rows already in UTC
// don't match either filter.
//
// Detection markers chosen to be selective and never collide with other
// formats the driver writes:
//   - legacy: contains " m=+" (Go monotonic-clock suffix)
//   - non-UTC: GLOB-matches "...±HH:MM" tail AND that tail != "+00:00"
//
// Scope: every DATETIME / TIMESTAMP column in every user table, discovered
// dynamically via sqlite_master + pragma_table_info. Skips sqlite_* internal
// tables. Postgres callers do not exercise this code path — pq + the
// session timezone=UTC + TIMESTAMPTZ schema handle UTC correctly natively.
func backfillLegacyDatetimeFormat(db *sql.DB) error {
	columns, err := discoverDatetimeColumns(db)
	if err != nil {
		return fmt.Errorf("discover DATETIME columns: %w", err)
	}

	totalFixed := 0
	for _, c := range columns {
		fixed, err := backfillColumn(db, c.table, c.column)
		if err != nil {
			// Don't abort the whole loop on one bad column — log and
			// continue. A single missing/renamed column shouldn't block
			// startup.
			slog.Warn("datetime backfill failed for column",
				slog.String("component", "database"),
				slog.String("table", c.table),
				slog.String("column", c.column),
				slog.Any("error", err))
			continue
		}
		totalFixed += fixed
	}
	if totalFixed > 0 {
		slog.Info("datetime backfill complete",
			slog.String("component", "database"),
			slog.Int("rows_fixed", totalFixed),
			slog.Int("columns_scanned", len(columns)))
	}
	return nil
}

type datetimeColumn struct {
	table  string
	column string
}

func discoverDatetimeColumns(db *sql.DB) ([]datetimeColumn, error) {
	tables, err := db.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { _ = tables.Close() }()

	var out []datetimeColumn
	tableNames := []string{}
	for tables.Next() {
		var t string
		if err := tables.Scan(&t); err != nil {
			return nil, err
		}
		tableNames = append(tableNames, t)
	}
	if err := tables.Err(); err != nil {
		return nil, err
	}
	_ = tables.Close()

	for _, t := range tableNames {
		// pragma_table_info(?) doesn't accept a parameter binding in all
		// SQLite versions; substitute the validated identifier directly.
		// `t` came from sqlite_master so it's a real existing table name —
		// no user input here.
		cols, err := db.Query(fmt.Sprintf(`SELECT name, type FROM pragma_table_info(%q)`, t))
		if err != nil {
			return nil, fmt.Errorf("pragma_table_info(%q): %w", t, err)
		}
		for cols.Next() {
			var name, typ string
			if err := cols.Scan(&name, &typ); err != nil {
				_ = cols.Close()
				return nil, err
			}
			u := strings.ToUpper(typ)
			if strings.Contains(u, "DATETIME") || strings.Contains(u, "TIMESTAMP") {
				out = append(out, datetimeColumn{table: t, column: name})
			}
		}
		if cErr := cols.Err(); cErr != nil {
			_ = cols.Close()
			return nil, cErr
		}
		_ = cols.Close()
	}
	return out, nil
}

// legacyGoTimeFormat matches what time.Time.String() emits when the time
// carries a monotonic clock reading — which time.Now() always does.
const legacyGoTimeFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

// sqliteTimeFormat matches what _time_format=sqlite writes today and what
// SQLite's date/time functions parse natively.
const sqliteTimeFormat = "2006-01-02 15:04:05.999999999-07:00"

func backfillColumn(db *sql.DB, table, column string) (int, error) {
	// Identifier-quote table+column. They came from sqlite_master /
	// pragma_table_info so they're trusted, but quote anyway so reserved
	// words / hyphens / etc. don't trip us.
	tq := quoteIdent(table)
	cq := quoteIdent(column)

	type fix struct {
		rowid  int64
		newVal string
	}
	var fixes []fix

	// PASS 1 — legacy Go-String() format rows. CAST(... AS TEXT) bypasses
	// modernc.org/sqlite's read-side time normalization: a plain SELECT on a
	// DATETIME column quietly reformats recognized time strings before
	// handing them back to Go, which would mask the legacy values entirely.
	// The CAST forces the raw storage class through unchanged.
	legacyRows, err := db.Query(fmt.Sprintf(
		`SELECT rowid, CAST(%s AS TEXT) FROM %s WHERE %s LIKE '%% m=+%%'`,
		cq, tq, cq,
	))
	if err != nil {
		return 0, err
	}
	for legacyRows.Next() {
		var rowid int64
		var s string
		if err := legacyRows.Scan(&rowid, &s); err != nil {
			_ = legacyRows.Close()
			return 0, err
		}
		idx := strings.Index(s, " m=+")
		if idx < 0 {
			continue
		}
		trimmed := strings.TrimSpace(s[:idx])
		t, perr := time.Parse(legacyGoTimeFormat, trimmed)
		if perr != nil {
			// Not a parseable legacy timestamp — leave it alone. Better to
			// keep an unrecognized value than to clobber it.
			continue
		}
		fixes = append(fixes, fix{rowid: rowid, newVal: t.UTC().Format(sqliteTimeFormat)})
	}
	if rerr := legacyRows.Err(); rerr != nil {
		_ = legacyRows.Close()
		return 0, rerr
	}
	_ = legacyRows.Close()

	// PASS 2 — non-UTC offset rows. Anything that already looks like the
	// SQLite format ("YYYY-MM-DD HH:MM:SS[.fff]±HH:MM") but whose offset is
	// not +00:00 is parsed and re-emitted in UTC. The GLOB locks the shape
	// (so we don't trip on rows that just happen to contain the bytes
	// "+02:00" somewhere in their middle) and the offset-suffix check
	// excludes already-UTC rows. The legacy-format suffix LIKE '% m=+%' is
	// excluded because pass 1 already handles it.
	//
	// `SUBSTR(x, -6)` returns the last 6 characters (the offset). SQLite's
	// SUBSTR with a negative start counts from the end, no length needed.
	offsetRows, err := db.Query(fmt.Sprintf(
		`SELECT rowid, CAST(%s AS TEXT) FROM %s
		   WHERE %s GLOB '????-??-?? ??:??:??*'
		     AND SUBSTR(%s, -6, 1) IN ('+', '-')
		     AND SUBSTR(%s, -6) <> '+00:00'
		     AND %s NOT LIKE '%% m=+%%'`,
		cq, tq, cq, cq, cq, cq,
	))
	if err != nil {
		return 0, err
	}
	for offsetRows.Next() {
		var rowid int64
		var s string
		if err := offsetRows.Scan(&rowid, &s); err != nil {
			_ = offsetRows.Close()
			return 0, err
		}
		t, perr := time.Parse(sqliteTimeFormat, s)
		if perr != nil {
			// Try the no-subsecond variant — SQLite drops trailing zeros
			// after the decimal, so a whole-second timestamp comes back as
			// "2026-05-15 14:00:00+02:00".
			t, perr = time.Parse("2006-01-02 15:04:05-07:00", s)
			if perr != nil {
				continue
			}
		}
		fixes = append(fixes, fix{rowid: rowid, newVal: t.UTC().Format(sqliteTimeFormat)})
	}
	if rerr := offsetRows.Err(); rerr != nil {
		_ = offsetRows.Close()
		return 0, rerr
	}
	_ = offsetRows.Close()

	if len(fixes) == 0 {
		return 0, nil
	}

	// One transaction per column. Keeps the write window short and bounds
	// the busy-timeout exposure when other connections are reading.
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(fmt.Sprintf(`UPDATE %s SET %s = ? WHERE rowid = ?`, tq, cq))
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	for _, f := range fixes {
		if _, err := stmt.Exec(f.newVal, f.rowid); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return 0, err
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(fixes), nil
}

// quoteIdent wraps an identifier in double quotes, doubling any embedded
// quotes per the SQL standard. Identifiers in this package come from
// sqlite_master, so this is defensive rather than essential.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
