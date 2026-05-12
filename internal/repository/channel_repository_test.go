package repository

import (
	"context"
	"sort"
	"testing"
	"time"

	"windshift/internal/database"
)

// TestChannelRepository_AddManager_Idempotent verifies the ON CONFLICT DO NOTHING
// patch on AddManager: re-adding the same (channel, manager_type, manager_id)
// triple succeeds silently rather than erroring on the UNIQUE constraint.
// Regression test for the runtime-SQLite-only INSERT OR IGNORE that broke on Postgres.
func TestChannelRepository_AddManager_Idempotent(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE channel_managers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER NOT NULL,
			manager_type TEXT NOT NULL,
			manager_id INTEGER NOT NULL,
			added_by INTEGER,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE(channel_id, manager_type, manager_id)
		)
	`); err != nil {
		t.Fatalf("create channel_managers: %v", err)
	}

	repo := NewChannelRepository(db)
	ctx := context.Background()

	addOnce := func(t *testing.T, channelID int, mgrType string, mgrID int) {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := repo.AddManager(ctx, tx, channelID, mgrType, mgrID, 99); err != nil {
			_ = tx.Rollback()
			t.Fatalf("AddManager(%d,%s,%d): %v", channelID, mgrType, mgrID, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	countRows := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM channel_managers`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	addOnce(t, 1, "user", 42)
	if got := countRows(t); got != 1 {
		t.Fatalf("after first insert: got %d rows, want 1", got)
	}

	// Duplicate insert: must not error, must not add a row.
	addOnce(t, 1, "user", 42)
	if got := countRows(t); got != 1 {
		t.Fatalf("after duplicate insert: got %d rows, want 1 (ON CONFLICT DO NOTHING should silently skip)", got)
	}

	// Different manager_id on the same channel: must add a new row.
	addOnce(t, 1, "user", 43)
	if got := countRows(t); got != 2 {
		t.Fatalf("after non-conflicting insert: got %d rows, want 2", got)
	}

	// Different manager_type for the same (channel, id): must add a new row.
	addOnce(t, 1, "group", 42)
	if got := countRows(t); got != 3 {
		t.Fatalf("after distinct manager_type: got %d rows, want 3", got)
	}
}

// TestChannelRepository_ListEnabledByTypeAndDirection covers the slim list
// used by the GET /api/items/{id}/webhooks endpoint after the SQL was
// pulled out of internal/handlers/webhook.go. Verifies type/direction/status
// filters all gate the result set independently.
func TestChannelRepository_ListEnabledByTypeAndDirection(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE channels (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			direction TEXT NOT NULL,
			status TEXT NOT NULL,
			config TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME,
			updated_at DATETIME
		)
	`); err != nil {
		t.Fatalf("create channels: %v", err)
	}

	now := time.Now()
	rows := []struct {
		id        int
		name      string
		typ       string
		direction string
		status    string
	}{
		{1, "match-a", "webhook", "outbound", "enabled"},
		{2, "match-b", "webhook", "outbound", "enabled"},
		{3, "wrong-status", "webhook", "outbound", "disabled"},
		{4, "wrong-direction", "webhook", "inbound", "enabled"},
		{5, "wrong-type", "smtp", "outbound", "enabled"},
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO channels (id, name, type, direction, status, config, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			r.id, r.name, r.typ, r.direction, r.status, "{}", now, now,
		); err != nil {
			t.Fatalf("insert row %d: %v", r.id, err)
		}
	}

	repo := NewChannelRepository(db)
	got, err := repo.ListEnabledByTypeAndDirection(context.Background(), "webhook", "outbound")
	if err != nil {
		t.Fatalf("ListEnabledByTypeAndDirection: %v", err)
	}

	names := make([]string, len(got))
	for i, c := range got {
		names[i] = c.Name
	}
	sort.Strings(names)

	want := []string{"match-a", "match-b"}
	if len(names) != len(want) {
		t.Fatalf("got %d channels (%v), want %d (%v)", len(names), names, len(want), want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("name[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}
