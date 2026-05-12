package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"windshift/internal/contextkeys"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

// Helpers shared by the negative-scenario regression tests for
// docs/bughunt1.md Run 2 findings #1–#4. Kept in-tree so the tests don't
// depend on the core-tests overlay (which has unrelated pre-existing build
// drift around NewConfigurationSetNotificationHandler).

// newNegativeTestDB returns a freshly-initialized SQLite database backed by a
// temp file, so each test runs against an isolated full schema. Closes on
// test cleanup.
func newNegativeTestDB(t *testing.T) database.Database {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "test.db")
	db, err := database.NewSQLiteDB(dbFile)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return db
}

// newNegativeTestPermissionService builds a real PermissionService configured
// to skip background warmup, so tests don't race against a goroutine.
func newNegativeTestPermissionService(t *testing.T, db database.Database) *services.PermissionService {
	t.Helper()
	cfg := services.DefaultPermissionCacheConfig()
	cfg.WarmupOnStartup = false
	cfg.TTL = 1 * time.Minute
	ps, err := services.NewPermissionService(db, cfg)
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}
	t.Cleanup(func() { ps.Close() })
	return ps
}

// seedNegativeTestUser inserts a test user with the given ID. Email and
// username are derived from the ID so multiple users can coexist without
// hitting UNIQUE constraints.
func seedNegativeTestUser(t *testing.T, db database.Database, userID int) {
	t.Helper()
	email := fmt.Sprintf("neg%d@example.com", userID)
	username := fmt.Sprintf("neguser%d", userID)
	if _, err := db.Exec(`
		INSERT INTO users (id, email, username, first_name, last_name, password_hash, is_active)
		VALUES (?, ?, ?, 'Neg', 'User', '$2a$10$hash', 1)
		ON CONFLICT DO NOTHING
	`, userID, email, username); err != nil {
		t.Fatalf("seed user %d: %v", userID, err)
	}
}

// authedRequest returns an http.Request whose context carries the given user,
// matching utils.GetCurrentUser's lookup key.
func authedRequest(method, target string, userID int, body interface{}) *http.Request {
	var r *http.Request
	if body != nil {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, target, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	user := &models.User{ID: userID, Email: "neg@example.com", Username: "neguser", IsActive: true}
	ctx := context.WithValue(r.Context(), contextkeys.User, user)
	return r.WithContext(ctx)
}

// decodeJSONBody parses the recorder body into the destination, failing the
// test on a malformed payload.
func decodeJSONBody(t *testing.T, rr *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
}
