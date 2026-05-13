package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// newAuditLogStreamTestHandler returns an AuditLogHandler wired to an in-memory
// SQLite database with just the audit_logs table. Used to exercise the cursor-
// based streaming endpoint without spinning up the full server.
func newAuditLogStreamTestHandler(t *testing.T) (*AuditLogHandler, database.Database) {
	t.Helper()
	dsn := "file:auditlog_handler_" + t.Name() + "?mode=memory&cache=shared"
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

	return NewAuditLogHandler(repository.NewAuditLogRepository(db)), db
}

func seedStreamRow(t *testing.T, db database.Database, action string) int {
	t.Helper()
	res, err := db.ExecWrite(
		`INSERT INTO audit_logs (timestamp, username, action_type, resource_type, success)
		 VALUES (?, ?, ?, ?, ?)`,
		time.Now(), "tester", action, "test", true,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func TestStreamAuditLogsSince_BadInput(t *testing.T) {
	h, _ := newAuditLogStreamTestHandler(t)

	tests := []struct {
		name  string
		query string
	}{
		{"non-numeric after_id", "after_id=abc"},
		{"negative after_id", "after_id=-1"},
		{"zero limit", "limit=0"},
		{"non-numeric limit", "limit=xyz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/since?"+tc.query, nil)
			w := httptest.NewRecorder()
			h.StreamAuditLogsSince(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestStreamAuditLogsSince_HappyPath(t *testing.T) {
	h, db := newAuditLogStreamTestHandler(t)
	id1 := seedStreamRow(t, db, "a.one")
	id2 := seedStreamRow(t, db, "a.two")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/since?after_id=0", nil)
	w := httptest.NewRecorder()
	h.StreamAuditLogsSince(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp AuditLogStreamResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(resp.Entries))
	}
	if resp.Entries[0].ID != id1 || resp.Entries[1].ID != id2 {
		t.Errorf("entry ids = [%d, %d], want [%d, %d]", resp.Entries[0].ID, resp.Entries[1].ID, id1, id2)
	}
	if resp.NextAfterID != id2 {
		t.Errorf("next_after_id = %d, want %d", resp.NextAfterID, id2)
	}
	if resp.HasMore {
		t.Error("has_more = true, want false (returned fewer than limit)")
	}
}

func TestStreamAuditLogsSince_HasMoreSignalAtLimit(t *testing.T) {
	h, db := newAuditLogStreamTestHandler(t)
	seedStreamRow(t, db, "a")
	seedStreamRow(t, db, "b")
	seedStreamRow(t, db, "c")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/since?after_id=0&limit=2", nil)
	w := httptest.NewRecorder()
	h.StreamAuditLogsSince(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp AuditLogStreamResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.HasMore {
		t.Errorf("has_more = false, want true (got exactly limit=%d rows but more exist)", len(resp.Entries))
	}
}

func TestStreamAuditLogsSince_EmptyTablePreservesCursor(t *testing.T) {
	h, _ := newAuditLogStreamTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/since?after_id=42", nil)
	w := httptest.NewRecorder()
	h.StreamAuditLogsSince(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp AuditLogStreamResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(resp.Entries))
	}
	if resp.NextAfterID != 42 {
		t.Errorf("next_after_id = %d, want 42 (empty result must preserve input cursor so callers can blindly persist it)", resp.NextAfterID)
	}
	if resp.HasMore {
		t.Error("has_more = true, want false")
	}
}

func TestStreamAuditLogsSince_ExcessiveLimitAccepted(t *testing.T) {
	h, db := newAuditLogStreamTestHandler(t)
	seedStreamRow(t, db, "a")

	// limit far above the documented max must not be rejected; the handler
	// silently clamps to auditLogStreamMaxLimit instead of erroring.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/since?after_id=0&limit=99999", nil)
	w := httptest.NewRecorder()
	h.StreamAuditLogsSince(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (oversized limit should be clamped, not rejected); body=%s", w.Code, w.Body.String())
	}
}
