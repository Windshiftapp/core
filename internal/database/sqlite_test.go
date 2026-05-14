package database

import "testing"

func TestIsWriteQuery(t *testing.T) {
	cases := []struct {
		query string
		want  bool
	}{
		{"INSERT INTO foo VALUES (1)", true},
		{"UPDATE foo SET a=1", true},
		{"DELETE FROM foo", true},
		{"REPLACE INTO foo VALUES (1)", true},
		{"MERGE INTO foo USING bar ON ...", true},
		{"WITH x AS (...) INSERT INTO foo SELECT * FROM x", true},
		{"WITH x AS (...) SELECT * FROM x", true}, // read-only CTE: accepted false positive
		{"CREATE TABLE foo (id INT)", true},
		{"ALTER TABLE foo ADD COLUMN bar TEXT", true},
		{"DROP TABLE foo", true},
		{"VACUUM", true},
		{"TRUNCATE foo", true},
		{"  insert into foo values (1)", true}, // leading whitespace + lowercase
		{"SELECT * FROM foo", false},
		{"select 1", false},
		{"PRAGMA foreign_keys=ON", false}, // PRAGMA can be read or write but isn't routed
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got := isWriteQuery(tc.query)
			if got != tc.want {
				t.Errorf("isWriteQuery(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}
