package logger

import (
	"strings"
	"testing"

	"windshift/internal/database"
)

// TestLogAudit_DetailsMarshalFailure_WritesSentinel covers the recovery path
// when json.Marshal(event.Details) fails (e.g. when Details contains an
// unmarshalable value like a channel). The audit row must still be inserted,
// with a sentinel details payload that records the marshal error rather than
// silently storing NULL.
func TestLogAudit_DetailsMarshalFailure_WritesSentinel(t *testing.T) {
	db, err := database.NewSQLiteDB("file:logger_audit_marshal_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecWrite(`CREATE TABLE audit_logs (
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

	// channels can't be JSON-marshaled — triggers the sentinel branch.
	event := AuditEvent{
		UserID:       1,
		Username:     "tester",
		ActionType:   "test.action",
		ResourceType: "test",
		Details:      map[string]interface{}{"bad": make(chan int)},
		Success:      true,
	}

	if err := LogAudit(db, event); err != nil {
		t.Fatalf("LogAudit returned error: %v", err)
	}

	var details *string
	if err := db.QueryRow(`SELECT details FROM audit_logs WHERE action_type = ? AND username = ?`,
		"test.action", "tester").Scan(&details); err != nil {
		t.Fatalf("query audit row: %v", err)
	}
	if details == nil {
		t.Fatalf("expected sentinel details, got NULL")
	}
	if !strings.Contains(*details, "details_marshal_error") {
		t.Errorf("expected sentinel marker in details, got %q", *details)
	}
}

// TestLogAudit_DetailsMarshalSuccess_StoresJSON is the happy path: marshalable
// Details land in the row as their JSON form.
func TestLogAudit_DetailsMarshalSuccess_StoresJSON(t *testing.T) {
	db, err := database.NewSQLiteDB("file:logger_audit_marshal_ok_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecWrite(`CREATE TABLE audit_logs (
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

	if err := LogAudit(db, AuditEvent{
		UserID:       2,
		Username:     "happy",
		ActionType:   "test.action",
		ResourceType: "test",
		Details:      map[string]interface{}{"old": "a", "new": "b"},
		Success:      true,
	}); err != nil {
		t.Fatalf("LogAudit returned error: %v", err)
	}

	var details *string
	if err := db.QueryRow(`SELECT details FROM audit_logs WHERE username = ?`, "happy").Scan(&details); err != nil {
		t.Fatalf("query audit row: %v", err)
	}
	if details == nil {
		t.Fatalf("expected details JSON, got NULL")
	}
	if strings.Contains(*details, "details_marshal_error") {
		t.Errorf("did not expect sentinel marker on happy path, got %q", *details)
	}
	if !strings.Contains(*details, `"old":"a"`) || !strings.Contains(*details, `"new":"b"`) {
		t.Errorf("expected marshaled key/value pairs in details, got %q", *details)
	}
}
