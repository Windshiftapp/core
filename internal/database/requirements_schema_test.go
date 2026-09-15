package database

import (
	"path/filepath"
	"testing"
)

func TestFreshSQLiteSchemaIncludesRequirementsTables(t *testing.T) {
	db, err := NewSQLiteDBWithPoolSizes(filepath.Join(t.TempDir(), "windshift.db"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"requirements", "requirement_sequences", "requirement_history"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %q is missing", table)
		}
	}

	var indexCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_pages_id_workspace_id'`).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatal("idx_pages_id_workspace_id is missing")
	}
}
