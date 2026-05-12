package email

import (
	"context"
	"testing"

	"windshift/internal/database"
)

// TestProcessor_GrantChannelAccess_Idempotent verifies the ON CONFLICT DO NOTHING
// patch on the two portal_customer_channels inserts in grantChannelAccess
// (processor.go ~lines 191 and 198). Re-granting the same (customer, channel)
// pair must succeed without error and without producing a duplicate row.
//
// grantChannelAccess is private and runs inside processNewCustomer / processEmail
// which require an end-to-end IMAP+ItemHandler harness, so this test exercises
// the schema+SQL contract directly: the exact INSERT strings from processor.go
// run against the production portal_customer_channels schema.
func TestProcessor_GrantChannelAccess_Idempotent(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE portal_customer_channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			portal_customer_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(portal_customer_id, channel_id)
		)
	`); err != nil {
		t.Fatalf("create portal_customer_channels: %v", err)
	}

	// Same SQL string used in internal/email/processor.go.
	const insertSQL = `
		INSERT INTO portal_customer_channels (portal_customer_id, channel_id)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`

	ctx := context.Background()
	exec := func(customerID, channelID int) {
		t.Helper()
		if _, err := db.ExecContext(ctx, insertSQL, customerID, channelID); err != nil {
			t.Fatalf("exec (%d,%d): %v", customerID, channelID, err)
		}
	}
	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM portal_customer_channels`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	exec(5, 200)
	if got := count(); got != 1 {
		t.Fatalf("after first grant: got %d, want 1", got)
	}

	// Duplicate grant (e.g. customer emails the same channel twice): must no-op.
	exec(5, 200)
	if got := count(); got != 1 {
		t.Fatalf("after duplicate grant: got %d, want 1", got)
	}

	// Same customer, different channel (e.g. EmailConnectedPortalID grant): must add a row.
	exec(5, 201)
	if got := count(); got != 2 {
		t.Fatalf("after connected-portal grant: got %d, want 2", got)
	}

	// Different customer, same channel.
	exec(6, 200)
	if got := count(); got != 3 {
		t.Fatalf("after second customer on same channel: got %d, want 3", got)
	}
}
