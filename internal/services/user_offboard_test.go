package services

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"windshift/internal/database"
)

func newOffboardEnv(t *testing.T) (database.Database, int) {
	t.Helper()

	dsn := fmt.Sprintf("file:offboard-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	res, err := db.Exec(`INSERT INTO users (email, username, first_name, last_name) VALUES (?, ?, ?, '')`,
		"hank@example.com", "hank", "Hank")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid64, _ := res.LastInsertId()
	return db, int(uid64)
}

func insertAPIToken(t *testing.T, db database.Database, userID int, name string) int {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO api_tokens (user_id, name, token_hash, token_prefix, permissions)
		VALUES (?, ?, ?, ?, '["read"]')
	`, userID, name, "hash-"+name, "crw_"+name)
	if err != nil {
		t.Fatalf("insert api_token %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func TestOffboardUser_RevokesAPITokensAndReturnsIDs(t *testing.T) {
	db, uid := newOffboardEnv(t)

	t1 := insertAPIToken(t, db, uid, "tok1")
	t2 := insertAPIToken(t, db, uid, "tok2")

	revoked, err := OffboardUser(db, uid)
	if err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}

	got := append([]int(nil), revoked...)
	sort.Ints(got)
	want := []int{t1, t2}
	sort.Ints(want)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("revoked IDs mismatch: got %v, want %v", got, want)
	}

	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE user_id = ?`, uid).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected 0 api_tokens for offboarded user, got %d", remaining)
	}
}

func TestOffboardUser_NoTokens_ReturnsEmptySlice(t *testing.T) {
	db, uid := newOffboardEnv(t)

	revoked, err := OffboardUser(db, uid)
	if err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if len(revoked) != 0 {
		t.Fatalf("expected empty slice, got %v", revoked)
	}
}
