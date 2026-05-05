package repository

import (
	"strings"
	"testing"
	"time"
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
			wantWhere: "WHERE success = 1",
			wantArgs:  nil,
		},
		{
			name:      "success false",
			filters:   AuditLogFilters{Success: &no},
			wantWhere: "WHERE success = 0",
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
			wantWhere: "WHERE action_type = ? AND user_id = ? AND resource_type = ? AND success = 1 AND timestamp >= ? AND timestamp <= ? AND (username LIKE ? OR resource_name LIKE ? OR action_type LIKE ?)",
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
