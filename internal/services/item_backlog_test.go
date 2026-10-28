package services

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

func TestGetBacklogItemsIncludeCompletedPreservesScopeAndOrder(t *testing.T) {
	db, err := database.NewSQLiteDBWithPoolSizes(filepath.Join(t.TempDir(), "windshift.db"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}

	statusID := func(name string) int {
		t.Helper()
		var id int
		if err := db.QueryRow("SELECT id FROM statuses WHERE name = ?", name).Scan(&id); err != nil {
			t.Fatalf("status %q: %v", name, err)
		}
		return id
	}
	itemTypeID := func(name string) int {
		t.Helper()
		var id int
		if err := db.QueryRow("SELECT id FROM item_types WHERE name = ?", name).Scan(&id); err != nil {
			t.Fatalf("item type %q: %v", name, err)
		}
		return id
	}

	openID := statusID("Open")
	inProgressID := statusID("In Progress")
	doneID := statusID("Done")
	storyID := itemTypeID("Story")
	epicID := itemTypeID("Epic")

	var verifiedCategoryID int
	if err := db.QueryRow(`
		INSERT INTO status_categories (name, color, is_completed)
		VALUES ('Accepted outcome', '#15803d', TRUE)
		RETURNING id`).Scan(&verifiedCategoryID); err != nil {
		t.Fatal(err)
	}
	var verifiedID int
	if err := db.QueryRow(`
		INSERT INTO statuses (name, description, category_id)
		VALUES ('Verified by QA', 'A custom completed status', ?)
		RETURNING id`, verifiedCategoryID).Scan(&verifiedID); err != nil {
		t.Fatal(err)
	}

	workspaceID := insertBacklogTestWorkspace(t, db, "Backlog scope", "BKS")
	otherWorkspaceID := insertBacklogTestWorkspace(t, db, "Outside scope", "OUT")
	if _, err := db.Exec(
		"INSERT INTO board_configurations (workspace_id, backlog_status_ids) VALUES (?, ?)",
		workspaceID,
		fmt.Sprintf("[%d]", openID),
	); err != nil {
		t.Fatal(err)
	}

	collectionQL := fmt.Sprintf("workspace_id = %d AND item_type_id != %d", workspaceID, epicID)
	var collectionID int
	if err := db.QueryRow(`
		INSERT INTO collections (name, ql_query, workspace_id)
		VALUES ('Stories and tasks', ?, ?)
		RETURNING id`, collectionQL, workspaceID).Scan(&collectionID); err != nil {
		t.Fatal(err)
	}

	insertBacklogTestItem(t, db, workspaceID, 1, storyID, openID, "Open story", "a")
	insertBacklogTestItem(t, db, workspaceID, 2, storyID, doneID, "Done story", "b")
	insertBacklogTestItem(t, db, workspaceID, 3, storyID, verifiedID, "Verified story", "c")
	insertBacklogTestItem(t, db, workspaceID, 4, storyID, inProgressID, "In progress story", "d")
	insertBacklogTestItem(t, db, workspaceID, 5, epicID, doneID, "Done epic", "e")
	insertBacklogTestItem(t, db, otherWorkspaceID, 1, storyID, doneID, "Inaccessible done story", "f")

	service := NewItemCRUDService(db)
	base := BacklogParams{
		CollectionID: collectionID,
		WorkspaceID:  workspaceID,
		WorkspaceIDs: []int{workspaceID},
		Pagination:   PaginationParams{Page: 1, Limit: 2},
	}

	t.Run("completed items remain hidden by default", func(t *testing.T) {
		items, total, err := service.GetBacklogItemsContext(context.Background(), base)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(items) != 1 || items[0].Title != "Open story" {
			t.Fatalf("default backlog = (%d, %v), want only Open story", total, backlogTestTitles(items))
		}
	})

	t.Run("completed category statuses extend the configured backlog", func(t *testing.T) {
		params := base
		params.IncludeCompleted = true

		first, total, err := service.GetBacklogItemsContext(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if got := fmt.Sprint(backlogTestTitles(first)); got != "[Open story Done story]" {
			t.Fatalf("first page = %s", got)
		}

		params.Pagination = PaginationParams{Page: 2, Limit: 2, Offset: 2}
		second, secondTotal, err := service.GetBacklogItemsContext(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		if secondTotal != 3 || fmt.Sprint(backlogTestTitles(second)) != "[Verified story]" {
			t.Fatalf("second page = (%d, %v), want Verified story", secondTotal, backlogTestTitles(second))
		}
	})
}

func insertBacklogTestWorkspace(t *testing.T, db database.Database, name, key string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(
		"INSERT INTO workspaces (name, key) VALUES (?, ?) RETURNING id",
		name,
		key,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertBacklogTestItem(
	t *testing.T,
	db database.Database,
	workspaceID, number, itemTypeID, statusID int,
	title, rank string,
) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description,
			status_id, custom_field_values, frac_index, last_active_at
		) VALUES (?, ?, ?, ?, '', ?, '{}', ?, CURRENT_TIMESTAMP)`,
		workspaceID, number, itemTypeID, title, statusID, rank,
	); err != nil {
		t.Fatal(err)
	}
}

func backlogTestTitles(items []models.Item) []string {
	titles := make([]string, len(items))
	for i := range items {
		titles[i] = items[i].Title
	}
	return titles
}
