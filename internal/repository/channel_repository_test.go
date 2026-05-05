package repository

import (
	"context"
	"sort"
	"testing"
	"time"

	"windshift/internal/database"
)

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
