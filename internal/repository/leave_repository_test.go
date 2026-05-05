package repository

import (
	"testing"

	"windshift/internal/database"
)

// TestLeaveRepository_UserExists covers the substitute-user existence check
// that the leave handler delegates to the repository. The SQL is a literal
// move from internal/handlers/leave.go; this test exists so a future schema
// or driver change can't silently regress the migrated path.
func TestLeaveRepository_UserExists(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`); err != nil {
		t.Fatalf("create users: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email) VALUES (?, ?)`, 1, "alice@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	repo := NewLeaveRepository(db)

	cases := []struct {
		name string
		id   int
		want bool
	}{
		{"existing", 1, true},
		{"missing", 999, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.UserExists(tc.id)
			if err != nil {
				t.Fatalf("UserExists(%d): %v", tc.id, err)
			}
			if got != tc.want {
				t.Errorf("UserExists(%d) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}
