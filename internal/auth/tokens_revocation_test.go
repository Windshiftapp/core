package auth_test

import (
	"fmt"
	"strings"
	"testing"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/models"
)

// newTokenManagerEnv brings up an in-memory SQLite + a TokenManager with a
// real validation cache for tests that need to observe cache eviction.
func newTokenManagerEnv(t *testing.T) (*auth.TokenManager, database.Database, int) {
	t.Helper()

	dsn := fmt.Sprintf("file:tokens-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	res, err := db.Exec(`INSERT INTO users (email, username, first_name, last_name) VALUES (?, ?, ?, '')`,
		"tok@example.com", "tok", "Tok")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid64, _ := res.LastInsertId()
	uid := int(uid64)

	tm := auth.NewTokenManager(db, nil)
	return tm, db, uid
}

func mustCreateToken(t *testing.T, tm *auth.TokenManager, userID int, name string) (raw string, id int) {
	t.Helper()
	resp, err := tm.CreateToken(userID, models.APITokenCreate{
		Name:        name,
		Permissions: []string{"read"},
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return resp.Token, resp.APIToken.ID
}

func TestInvalidateTokens_EvictsCachedValidation(t *testing.T) {
	tm, db, uid := newTokenManagerEnv(t)

	rawA, idA := mustCreateToken(t, tm, uid, "a")
	rawB, idB := mustCreateToken(t, tm, uid, "b")

	// Prime the validation cache.
	if _, _, err := tm.ValidateToken(rawA); err != nil {
		t.Fatalf("ValidateToken A (initial): %v", err)
	}
	if _, _, err := tm.ValidateToken(rawB); err != nil {
		t.Fatalf("ValidateToken B (initial): %v", err)
	}

	// Simulate the deactivation cascade: rows deleted directly via SQL,
	// then InvalidateTokens called with the collected IDs.
	if _, err := db.Exec(`DELETE FROM api_tokens WHERE id IN (?, ?)`, idA, idB); err != nil {
		t.Fatalf("delete tokens: %v", err)
	}
	tm.InvalidateTokens([]int{idA, idB})

	if _, _, err := tm.ValidateToken(rawA); err == nil {
		t.Fatal("expected ValidateToken(A) to fail after InvalidateTokens")
	}
	if _, _, err := tm.ValidateToken(rawB); err == nil {
		t.Fatal("expected ValidateToken(B) to fail after InvalidateTokens")
	}
}

func TestInvalidateTokens_NilAndEmpty_AreNoOp(t *testing.T) {
	tm, _, _ := newTokenManagerEnv(t)
	// Should not panic, should not error.
	tm.InvalidateTokens(nil)
	tm.InvalidateTokens([]int{})
}

func TestInvalidateTokens_NonexistentID_IsSafe(t *testing.T) {
	tm, _, _ := newTokenManagerEnv(t)
	tm.InvalidateTokens([]int{99999, 88888})
}
