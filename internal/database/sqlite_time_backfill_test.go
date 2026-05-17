package database

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestBackfillLegacyDatetimeFormat verifies that legacy time.Time.String()
// values are rewritten to a format SQLite's DATE() can parse, while clean
// rows and unrelated columns are left alone.
func TestBackfillLegacyDatetimeFormat(t *testing.T) {
	// No _time_format=sqlite here — we want to seed legacy rows directly
	// via INSERT, so the raw text the driver wrote is what's actually
	// in storage.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	mustExec(t, db, `CREATE TABLE alpha (id INTEGER PRIMARY KEY, ts DATETIME, label TEXT)`)
	mustExec(t, db, `CREATE TABLE beta  (id INTEGER PRIMARY KEY, when_at TIMESTAMP)`)

	// Five flavors of input — the backfill must do the right thing for each:
	//   1. Legacy Go-format ("CEST m=+...") that DATE() cannot parse → fix + UTC
	//   2. Non-UTC offset (+02:00) → re-emit in UTC
	//   3. Already-UTC (+00:00) → untouched
	//   4. Naive (no offset, CURRENT_TIMESTAMP-style) → untouched (assumed UTC by convention)
	//   5. NULL → untouched
	const legacy = `2026-05-15 23:13:34.623059 +0200 CEST m=+3772.605125585`
	const nonUTC = `2026-05-15 23:13:34.623059+02:00`
	const alreadyUTC = `2026-05-15 21:13:34.623059+00:00`
	const naive = `2026-04-28 07:21:22`
	mustExec(t, db, `INSERT INTO alpha (ts, label) VALUES (?, 'legacy')`, legacy)
	mustExec(t, db, `INSERT INTO alpha (ts, label) VALUES (?, 'non_utc')`, nonUTC)
	mustExec(t, db, `INSERT INTO alpha (ts, label) VALUES (?, 'already_utc')`, alreadyUTC)
	mustExec(t, db, `INSERT INTO alpha (ts, label) VALUES (?, 'naive')`, naive)
	mustExec(t, db, `INSERT INTO alpha (ts, label) VALUES (NULL, 'null')`)
	mustExec(t, db, `INSERT INTO beta  (when_at) VALUES (?)`, legacy)

	// Pre-condition: SQLite's DATE() returns NULL for the legacy text.
	assertDateNull(t, db, `SELECT DATE(ts) FROM alpha WHERE label='legacy'`)

	if err := backfillLegacyDatetimeFormat(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// After backfill: DATE() works on the row that was previously broken.
	// 23:13 +0200 → 21:13 UTC, still 2026-05-15.
	assertDateEquals(t, db, `SELECT DATE(ts) FROM alpha WHERE label='legacy'`, "2026-05-15")

	// Helper that CASTs back to text so we read the raw storage class, not
	// the driver's read-time reformatted view.
	readRaw := func(label string) string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT CAST(ts AS TEXT) FROM alpha WHERE label=?`, label).Scan(&v); err != nil {
			t.Fatalf("scan %s: %v", label, err)
		}
		return v
	}

	// Legacy row: now in UTC. 23:13:34 +02:00 == 21:13:34 UTC.
	if got := readRaw("legacy"); got != `2026-05-15 21:13:34.623059+00:00` {
		t.Errorf("legacy row UTC-normalized incorrectly: got %q", got)
	}

	// Non-UTC row: same conversion. 23:13:34 +02:00 → 21:13:34 +00:00.
	if got := readRaw("non_utc"); got != `2026-05-15 21:13:34.623059+00:00` {
		t.Errorf("non-UTC row not normalized: got %q want %q", got, `2026-05-15 21:13:34.623059+00:00`)
	}

	// Already-UTC row: byte-identical to the input.
	if got := readRaw("already_utc"); got != alreadyUTC {
		t.Errorf("already-UTC row was rewritten: got %q want %q", got, alreadyUTC)
	}

	// Naive row: leave alone. It has no offset to interpret, so any rewrite
	// would have to assume a timezone — better to keep it as-is and rely on
	// the application's read-side convention.
	if got := readRaw("naive"); got != naive {
		t.Errorf("naive row was rewritten: got %q want %q", got, naive)
	}

	// NULL rows must remain NULL.
	var nullable sql.NullString
	if err := db.QueryRow(`SELECT ts FROM alpha WHERE label='null'`).Scan(&nullable); err != nil {
		t.Fatalf("scan null: %v", err)
	}
	if nullable.Valid {
		t.Errorf("null row became non-null: %q", nullable.String)
	}
	// TIMESTAMP-typed columns are picked up alongside DATETIME-typed ones.
	assertDateEquals(t, db, `SELECT DATE(when_at) FROM beta`, "2026-05-15")

	// Idempotent: a second pass touches nothing.
	if err := backfillLegacyDatetimeFormat(db); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	rows, err := db.Query(`SELECT ts FROM alpha WHERE ts LIKE '% m=+%' OR SUBSTR(CAST(ts AS TEXT), -6) GLOB '[+-]??:??' AND SUBSTR(CAST(ts AS TEXT), -6) <> '+00:00'`)
	if err != nil {
		t.Fatalf("post-check: %v", err)
	}
	if rows.Next() {
		var s string
		_ = rows.Scan(&s)
		t.Errorf("non-UTC value still present after backfill: %q", s)
	}
	_ = rows.Close()
}

// TestBackfillSkipsUnparseable ensures rows that match the LIKE filter but
// fail time.Parse are left untouched rather than clobbered with junk.
func TestBackfillSkipsUnparseable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	mustExec(t, db, `CREATE TABLE t (id INTEGER PRIMARY KEY, ts DATETIME)`)
	const bogus = `not a date m=+123` // contains the LIKE marker but is not a real timestamp
	mustExec(t, db, `INSERT INTO t (ts) VALUES (?)`, bogus)

	if err := backfillLegacyDatetimeFormat(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got string
	if err := db.QueryRow(`SELECT ts FROM t`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != bogus {
		t.Errorf("unparseable value was rewritten: got %q want %q", got, bogus)
	}
}

// TestBackfillNoOpOnCleanUTCInsert covers the steady-state path: insert a
// time.Time *already in UTC* through the driver (mirroring what toUTCArgs
// does at the wrapper boundary in production) and confirm the backfill is
// a true no-op — the row is already UTC, DATE() already works, nothing to
// do.
func TestBackfillNoOpOnCleanUTCInsert(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_time_format=sqlite")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	mustExec(t, db, `CREATE TABLE t (id INTEGER PRIMARY KEY, ts DATETIME)`)
	// Production binds time.Time via the SQLiteDB wrapper, which UTC()'s
	// every time.Time arg. Emulate that here.
	mustExec(t, db, `INSERT INTO t (ts) VALUES (?)`, time.Now().UTC())
	assertDateEquals(t, db, `SELECT DATE(ts) FROM t`, time.Now().UTC().Format("2006-01-02"))

	// Capture the raw stored text, then run the backfill, then confirm
	// nothing changed — UTC-offset rows must not be touched.
	var before, after string
	if err := db.QueryRow(`SELECT CAST(ts AS TEXT) FROM t`).Scan(&before); err != nil {
		t.Fatalf("scan before: %v", err)
	}
	if !strings.HasSuffix(before, "+00:00") {
		t.Fatalf("expected stored value to end in +00:00, got %q", before)
	}
	if err := backfillLegacyDatetimeFormat(db); err != nil {
		t.Fatalf("backfill no-op: %v", err)
	}
	if err := db.QueryRow(`SELECT CAST(ts AS TEXT) FROM t`).Scan(&after); err != nil {
		t.Fatalf("scan after: %v", err)
	}
	if before != after {
		t.Errorf("UTC row was rewritten: before=%q after=%q", before, after)
	}
}

// TestSQLiteDBWrapperNormalizesToUTC verifies that the SQLiteDB wrapper's
// toUTCArgs path actually does what its comment promises: every time.Time
// reaching the driver lands in UTC, regardless of what timezone the input
// time was in. This is the *load-bearing* layer for our UTC convention
// because the modernc.org/sqlite version we pin doesn't honor _timezone.
func TestSQLiteDBWrapperNormalizesToUTC(t *testing.T) {
	// Pick an unambiguous non-UTC timezone for the input. India Standard
	// Time is +05:30 — the non-zero minute offset makes mistakes (like
	// returning the input untouched, or accidentally dropping minutes)
	// stand out clearly in the assertion.
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("load IST: %v", err)
	}
	in := time.Date(2026, 5, 15, 23, 13, 34, 623059000, ist)

	out := toUTCArgs([]interface{}{in})
	got, ok := out[0].(time.Time)
	if !ok {
		t.Fatalf("toUTCArgs dropped the time.Time, got %T", out[0])
	}
	if got.Location() != time.UTC {
		t.Errorf("location not UTC: %v", got.Location())
	}
	wantUnix := in.Unix()
	if got.Unix() != wantUnix {
		t.Errorf("instant changed: in=%v out=%v", in, got)
	}

	// Pointer variant: must allocate a new *time.Time, not mutate caller's.
	ptr := &in
	out2 := toUTCArgs([]interface{}{ptr})
	gotPtr, ok := out2[0].(*time.Time)
	if !ok {
		t.Fatalf("toUTCArgs dropped the *time.Time, got %T", out2[0])
	}
	if gotPtr == ptr {
		t.Errorf("toUTCArgs returned the same pointer — should allocate a fresh one to avoid mutating caller's value")
	}
	if gotPtr.Location() != time.UTC {
		t.Errorf("pointer location not UTC: %v", gotPtr.Location())
	}
	if ptr.Location() == time.UTC {
		t.Errorf("caller's time was mutated to UTC")
	}

	// Nil *time.Time must round-trip as a nil *time.Time.
	var nilPtr *time.Time
	out3 := toUTCArgs([]interface{}{nilPtr})
	if gotNil, ok := out3[0].(*time.Time); !ok || gotNil != nil {
		t.Errorf("nil *time.Time mishandled: %#v", out3[0])
	}

	// Non-time args pass through unchanged.
	out4 := toUTCArgs([]interface{}{42, "hello", nil})
	if out4[0] != 42 || out4[1] != "hello" || out4[2] != nil {
		t.Errorf("non-time args mutated: %v", out4)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

func assertDateNull(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	var d sql.NullString
	if err := db.QueryRow(q).Scan(&d); err != nil {
		t.Fatalf("query %s: %v", q, err)
	}
	if d.Valid {
		t.Errorf("expected DATE() NULL for %s, got %q", q, d.String)
	}
}

func assertDateEquals(t *testing.T, db *sql.DB, q, want string) {
	t.Helper()
	var d sql.NullString
	if err := db.QueryRow(q).Scan(&d); err != nil {
		t.Fatalf("query %s: %v", q, err)
	}
	if !d.Valid {
		t.Errorf("expected DATE() %q for %s, got NULL", want, q)
		return
	}
	if d.String != want {
		t.Errorf("DATE() for %s = %q, want %q", q, d.String, want)
	}
}
