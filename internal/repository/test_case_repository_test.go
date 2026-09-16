package repository

import (
	"path/filepath"
	"slices"
	"testing"

	"windshift/internal/database"
)

func TestTestCaseRepositorySearch(t *testing.T) {
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "test-case-search.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO workspaces (id, name, key) VALUES (1, 'Tests', 'TEST'), (2, 'Other', 'OTHER')`,
		`INSERT INTO test_folders (id, workspace_id, name) VALUES (1, 1, 'Folder')`,
		`INSERT INTO test_cases (id, workspace_id, folder_id, title, preconditions, priority, status, sort_order)
		 VALUES (1, 1, 1, 'Тест кейс', 'Пользователь вошёл', 'high', 'active', 1),
		        (2, 1, 1, 'Тест кейс второй', '', 'medium', 'draft', 2),
		        (3, 1, NULL, 'LOGIN test', '', 'low', 'draft', 3),
		        (4, 2, NULL, 'Тест кейс другого пространства', '', 'medium', 'draft', 4)`,
		`INSERT INTO test_labels (id, workspace_id, name) VALUES (1, 1, 'Регрессия')`,
		`INSERT INTO test_case_labels (test_case_id, label_id) VALUES (1, 1)`,
	} {
		if _, err := db.ExecWrite(statement); err != nil {
			t.Fatal(err)
		}
	}

	repo := NewTestCaseRepository(db)
	for _, tc := range []struct {
		name   string
		query  string
		all    bool
		offset int
		ids    []int
		total  int
	}{
		{name: "Cyrillic title inside folder", query: " Тест ке ", all: true, ids: []int{1}, total: 2},
		{name: "second page keeps filtered total", query: "Тест ке", all: true, offset: 1, ids: []int{2}, total: 2},
		{name: "root folder filter is preserved", query: "Тест ке", ids: []int{}, total: 0},
		{name: "ASCII title ignores case", query: "login TEST", all: true, ids: []int{3}, total: 1},
		{name: "Cyrillic preconditions", query: "Пользователь", all: true, ids: []int{1}, total: 1},
		{name: "ASCII priority ignores case", query: "HIGH", all: true, ids: []int{1}, total: 1},
		{name: "ASCII status ignores case", query: "ACTIVE", all: true, ids: []int{1}, total: 1},
		{name: "Cyrillic label", query: "Регресс", all: true, ids: []int{1}, total: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := TestCaseListParams{WorkspaceID: 1, All: tc.all, Search: tc.query, Limit: 1, Offset: tc.offset}
			rows, err := repo.FindAll(params)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int, len(rows))
			for i, row := range rows {
				ids[i] = row.ID
			}
			if !slices.Equal(ids, tc.ids) {
				t.Fatalf("FindAll IDs = %v, want %v", ids, tc.ids)
			}
			total, err := repo.Count(params)
			if err != nil {
				t.Fatal(err)
			}
			if total != tc.total {
				t.Fatalf("Count = %d, want %d", total, tc.total)
			}
		})
	}
}
