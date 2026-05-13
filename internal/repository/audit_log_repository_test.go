package repository

import (
	"strings"
	"testing"
	"time"

	"windshift/internal/database"
)

// TestBuildAuditLogWhere covers the SQL fragment + args produced for each
// filter combination. The clauses themselves are SQL strings; testing them
// directly is faster and more focused than spinning up a fake DB just to
// verify them via the surrounding queries.
func TestBuildAuditLogWhere(t *testing.T) {
	uid := 42
	yes := true
	no := false
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		filters   AuditLogFilters
		wantWhere string
		wantArgs  []any
	}{
		{
			name:      "empty filters return no WHERE clause",
			filters:   AuditLogFilters{},
			wantWhere: "",
			wantArgs:  nil,
		},
		{
			name:      "action_type only",
			filters:   AuditLogFilters{ActionType: "login.success"},
			wantWhere: "WHERE action_type = ?",
			wantArgs:  []any{"login.success"},
		},
		{
			name:      "user_id only",
			filters:   AuditLogFilters{UserID: &uid},
			wantWhere: "WHERE user_id = ?",
			wantArgs:  []any{42},
		},
		{
			name:      "success true",
			filters:   AuditLogFilters{Success: &yes},
			wantWhere: "WHERE success = true",
			wantArgs:  nil,
		},
		{
			name:      "success false",
			filters:   AuditLogFilters{Success: &no},
			wantWhere: "WHERE success = false",
			wantArgs:  nil,
		},
		{
			name:      "from + to bounds",
			filters:   AuditLogFilters{From: &from, To: &to},
			wantWhere: "WHERE timestamp >= ? AND timestamp <= ?",
			wantArgs:  []any{from, to},
		},
		{
			name:      "search expands to OR over three columns",
			filters:   AuditLogFilters{Search: "alice"},
			wantWhere: "WHERE (username LIKE ? OR resource_name LIKE ? OR action_type LIKE ?)",
			wantArgs:  []any{"%alice%", "%alice%", "%alice%"},
		},
		{
			name: "all filters combine with AND in declaration order",
			filters: AuditLogFilters{
				ActionType:   "user.delete",
				UserID:       &uid,
				ResourceType: "user",
				Success:      &yes,
				From:         &from,
				To:           &to,
				Search:       "bob",
			},
			wantWhere: "WHERE action_type = ? AND user_id = ? AND resource_type = ? AND success = true AND timestamp >= ? AND timestamp <= ? AND (username LIKE ? OR resource_name LIKE ? OR action_type LIKE ?)",
			wantArgs:  []any{"user.delete", 42, "user", from, to, "%bob%", "%bob%", "%bob%"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotWhere, gotArgs := buildAuditLogWhere(tc.filters)
			if gotWhere != tc.wantWhere {
				t.Errorf("where:\n got:  %q\n want: %q", gotWhere, tc.wantWhere)
			}
			if !argsEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args:\n got:  %v\n want: %v", gotArgs, tc.wantArgs)
			}
			// Sanity: every '?' placeholder has a matching arg.
			if got, want := strings.Count(gotWhere, "?"), len(gotArgs); got != want {
				t.Errorf("placeholder count = %d, args = %d", got, want)
			}
		})
	}
}

func argsEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newAuditLogTestDB creates an isolated in-memory SQLite database with just the
// audit_logs table. The schema mirrors the canonical column order expected by
// scanAuditLogRow (and the real system_postgres.sql definition).
func newAuditLogTestDB(t *testing.T) database.Database {
	t.Helper()
	dsn := "file:auditlog_" + t.Name() + "?mode=memory&cache=shared"
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		user_id INTEGER,
		username TEXT NOT NULL,
		ip_address TEXT,
		user_agent TEXT,
		action_type TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id INTEGER,
		resource_name TEXT,
		details TEXT,
		success BOOLEAN NOT NULL DEFAULT 1,
		error_message TEXT
	)`); err != nil {
		t.Fatalf("create audit_logs: %v", err)
	}
	return db
}

// seedAuditLog inserts a minimal audit_logs row and returns the assigned id.
func seedAuditLog(t *testing.T, db database.Database, action string) int {
	t.Helper()
	res, err := db.ExecWrite(
		`INSERT INTO audit_logs (timestamp, username, action_type, resource_type, success)
		 VALUES (?, ?, ?, ?, ?)`,
		time.Now(), "tester", action, "test", true,
	)
	if err != nil {
		t.Fatalf("insert audit_log: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return int(id)
}

// TestAuditLogRepository_ListSince covers the cursor-based streaming endpoint's
// repository contract: strict-greater-than cursor on id, ascending id order,
// limit-bounded, and never re-delivers a row whose id equals the cursor.
func TestAuditLogRepository_ListSince(t *testing.T) {
	t.Run("empty table returns empty slice", func(t *testing.T) {
		db := newAuditLogTestDB(t)
		repo := NewAuditLogRepository(db)

		entries, err := repo.ListSince(0, 100)
		if err != nil {
			t.Fatalf("ListSince: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("after_id=0 returns first row when single row exists", func(t *testing.T) {
		db := newAuditLogTestDB(t)
		repo := NewAuditLogRepository(db)
		id := seedAuditLog(t, db, "login.success")

		entries, err := repo.ListSince(0, 100)
		if err != nil {
			t.Fatalf("ListSince: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].ID != id {
			t.Errorf("entry id = %d, want %d", entries[0].ID, id)
		}
		if entries[0].ActionType != "login.success" {
			t.Errorf("action_type = %q, want %q", entries[0].ActionType, "login.success")
		}
	})

	t.Run("cursor matching a row excludes it (strict >)", func(t *testing.T) {
		db := newAuditLogTestDB(t)
		repo := NewAuditLogRepository(db)
		id1 := seedAuditLog(t, db, "a.one")
		id2 := seedAuditLog(t, db, "a.two")

		entries, err := repo.ListSince(id1, 100)
		if err != nil {
			t.Fatalf("ListSince: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].ID != id2 {
			t.Errorf("entry id = %d, want %d (the row at the cursor itself must not be re-delivered)", entries[0].ID, id2)
		}
	})

	t.Run("limit caps batch and order is ascending by id", func(t *testing.T) {
		db := newAuditLogTestDB(t)
		repo := NewAuditLogRepository(db)
		ids := []int{
			seedAuditLog(t, db, "a"),
			seedAuditLog(t, db, "b"),
			seedAuditLog(t, db, "c"),
			seedAuditLog(t, db, "d"),
			seedAuditLog(t, db, "e"),
		}

		entries, err := repo.ListSince(0, 3)
		if err != nil {
			t.Fatalf("ListSince: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
		for i, e := range entries {
			if e.ID != ids[i] {
				t.Errorf("entry[%d].ID = %d, want %d (entries must be in ascending id order)", i, e.ID, ids[i])
			}
		}
	})

	t.Run("paginating with next cursor returns remainder with no overlap", func(t *testing.T) {
		db := newAuditLogTestDB(t)
		repo := NewAuditLogRepository(db)
		ids := []int{
			seedAuditLog(t, db, "a"),
			seedAuditLog(t, db, "b"),
			seedAuditLog(t, db, "c"),
			seedAuditLog(t, db, "d"),
		}

		first, err := repo.ListSince(0, 2)
		if err != nil {
			t.Fatalf("ListSince batch 1: %v", err)
		}
		if len(first) != 2 {
			t.Fatalf("batch 1: expected 2 entries, got %d", len(first))
		}
		cursor := first[len(first)-1].ID

		second, err := repo.ListSince(cursor, 2)
		if err != nil {
			t.Fatalf("ListSince batch 2: %v", err)
		}
		if len(second) != 2 {
			t.Fatalf("batch 2: expected 2 entries, got %d", len(second))
		}
		if second[0].ID != ids[2] || second[1].ID != ids[3] {
			t.Errorf("batch 2 ids = [%d, %d], want [%d, %d]", second[0].ID, second[1].ID, ids[2], ids[3])
		}
	})
}
