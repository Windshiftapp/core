//go:build test

package repository

import (
	"testing"
	"time"

	"windshift/internal/handlers/testutils"
	"windshift/internal/models"
)

func TestItemRepository(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	repo := NewItemRepository(tdb.GetDatabase())
	testData := setupRepositoryTestData(t, tdb)

	t.Run("FindByID", func(t *testing.T) {
		item, err := repo.FindByID(testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.ID != testData.ItemID {
			t.Errorf("Expected ID %d, got %d", testData.ItemID, item.ID)
		}
		if item.Title != "Test Item" {
			t.Errorf("Expected title 'Test Item', got '%s'", item.Title)
		}
		if item.WorkspaceID != testData.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", testData.WorkspaceID, item.WorkspaceID)
		}
	})

	t.Run("FindByIDNotFound", func(t *testing.T) {
		_, err := repo.FindByID(99999)
		if err != ErrNotFound {
			t.Errorf("Expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("FindByIDWithDetails", func(t *testing.T) {
		item, err := repo.FindByIDWithDetails(testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.ID != testData.ItemID {
			t.Errorf("Expected ID %d, got %d", testData.ItemID, item.ID)
		}
		if item.WorkspaceName == "" {
			t.Error("Expected workspace name to be populated")
		}
		if item.WorkspaceKey == "" {
			t.Error("Expected workspace key to be populated")
		}
	})

	t.Run("GetWorkspaceID", func(t *testing.T) {
		workspaceID, err := repo.GetWorkspaceID(testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workspaceID != testData.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", testData.WorkspaceID, workspaceID)
		}
	})

	t.Run("GetWorkspaceIDNotFound", func(t *testing.T) {
		_, err := repo.GetWorkspaceID(99999)
		if err != ErrNotFound {
			t.Errorf("Expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("Exists", func(t *testing.T) {
		exists, err := repo.Exists(testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !exists {
			t.Error("Expected item to exist")
		}

		exists, err = repo.Exists(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if exists {
			t.Error("Expected item not to exist")
		}
	})

	t.Run("CreateAndUpdate", func(t *testing.T) {
		// Start transaction
		tx, err := tdb.GetDatabase().Begin()
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		// Get next item number
		nextNum, err := repo.GetNextWorkspaceItemNumber(tx, testData.WorkspaceID)
		if err != nil {
			tx.Rollback()
			t.Fatalf("Failed to get next item number: %v", err)
		}

		// Create item
		fracIndex := "a0"
		newItem := &models.Item{
			WorkspaceID:         testData.WorkspaceID,
			WorkspaceItemNumber: nextNum,
			Title:               "New Test Item",
			Description:         "Test description",
			Status:              "open",
			FracIndex:           &fracIndex,
		}

		id, err := repo.Create(tx, newItem)
		if err != nil {
			tx.Rollback()
			t.Fatalf("Failed to create item: %v", err)
		}
		tx.Commit()

		if id == 0 {
			t.Error("Expected non-zero ID")
		}

		// Verify created item
		created, err := repo.FindByID(id)
		if err != nil {
			t.Fatalf("Failed to find created item: %v", err)
		}
		if created.Title != "New Test Item" {
			t.Errorf("Expected title 'New Test Item', got '%s'", created.Title)
		}

		// Update item
		tx2, err := tdb.GetDatabase().Begin()
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		created.Title = "Updated Test Item"
		err = repo.Update(tx2, created)
		if err != nil {
			tx2.Rollback()
			t.Fatalf("Failed to update item: %v", err)
		}
		tx2.Commit()

		// Verify update
		updated, err := repo.FindByID(id)
		if err != nil {
			t.Fatalf("Failed to find updated item: %v", err)
		}
		if updated.Title != "Updated Test Item" {
			t.Errorf("Expected title 'Updated Test Item', got '%s'", updated.Title)
		}
	})
}

func TestItemHierarchy(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	repo := NewItemRepository(tdb.GetDatabase())
	testData := setupHierarchyTestData(t, tdb)

	t.Run("GetChildren", func(t *testing.T) {
		children, err := repo.GetChildren(testData.ParentID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 2 {
			t.Errorf("Expected 2 children, got %d", len(children))
		}
	})

	t.Run("GetChildrenEmpty", func(t *testing.T) {
		children, err := repo.GetChildren(testData.ChildID1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 0 {
			t.Errorf("Expected 0 children, got %d", len(children))
		}
	})

	t.Run("GetDescendantIDs", func(t *testing.T) {
		ids, err := repo.GetDescendantIDs(testData.ParentID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ids) < 2 {
			t.Errorf("Expected at least 2 descendants, got %d", len(ids))
		}
	})

	t.Run("GetRootItems", func(t *testing.T) {
		roots, err := repo.GetRootItems(testData.WorkspaceID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(roots) == 0 {
			t.Error("Expected at least 1 root item")
		}

		// Check that all returned items have no parent
		for _, item := range roots {
			if item.ParentID != nil {
				t.Errorf("Root item %d has parent_id set", item.ID)
			}
		}
	})
}

func TestItemWatch(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	repo := NewItemRepository(tdb.GetDatabase())
	testData := setupRepositoryTestData(t, tdb)

	t.Run("WatchAndUnwatch", func(t *testing.T) {
		// Initially not watching
		watching, err := repo.IsWatching(testData.UserID, testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if watching {
			t.Error("Expected not watching initially")
		}

		// Add watch
		err = repo.Watch(testData.UserID, testData.ItemID)
		if err != nil {
			t.Fatalf("Failed to add watch: %v", err)
		}

		// Verify watching
		watching, err = repo.IsWatching(testData.UserID, testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !watching {
			t.Error("Expected to be watching")
		}

		// Remove watch
		err = repo.Unwatch(testData.UserID, testData.ItemID)
		if err != nil {
			t.Fatalf("Failed to remove watch: %v", err)
		}

		// Verify not watching
		watching, err = repo.IsWatching(testData.UserID, testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if watching {
			t.Error("Expected not watching after unwatch")
		}
	})

	t.Run("GetWatchers", func(t *testing.T) {
		// Add watch
		err := repo.Watch(testData.UserID, testData.ItemID)
		if err != nil {
			t.Fatalf("Failed to add watch: %v", err)
		}

		watchers, err := repo.GetWatchers(testData.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(watchers) == 0 {
			t.Error("Expected at least 1 watcher")
		}

		found := false
		for _, w := range watchers {
			if w == testData.UserID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected user to be in watchers list")
		}
	})
}

func TestItemHistory(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	repo := NewItemRepository(tdb.GetDatabase())
	testData := setupRepositoryTestData(t, tdb)

	t.Run("RecordAndGetHistory", func(t *testing.T) {
		// Record history
		tx, err := tdb.GetDatabase().Begin()
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		entry := HistoryEntry{
			ItemID:    testData.ItemID,
			UserID:    testData.UserID,
			FieldName: "title",
			OldValue:  "Old Title",
			NewValue:  "New Title",
			ChangedAt: time.Now(),
		}

		err = repo.RecordHistory(tx, entry)
		if err != nil {
			tx.Rollback()
			t.Fatalf("Failed to record history: %v", err)
		}
		tx.Commit()

		// Get history
		history, err := repo.GetHistory(testData.ItemID, 10)
		if err != nil {
			t.Fatalf("Failed to get history: %v", err)
		}

		if len(history) == 0 {
			t.Error("Expected at least 1 history entry")
		}

		found := false
		for _, h := range history {
			if h.FieldName == "title" && h.OldValue == "Old Title" && h.NewValue == "New Title" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find recorded history entry")
		}
	})
}

// Test data setup helpers

type RepositoryTestData struct {
	WorkspaceID int
	ItemID      int
	StatusID    int
	PriorityID  int
	UserID      int
}

func setupRepositoryTestData(t *testing.T, tdb *testutils.TestDB) *RepositoryTestData {
	now := time.Now()

	// Create workspace
	result, err := tdb.DB.Exec(`
		INSERT INTO workspaces (name, key, description, created_at, updated_at)
		VALUES ('Test Workspace', 'TST', 'Test workspace', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := result.LastInsertId()
	workspaceID := int(workspaceID64)

	// Get status and priority from default data
	var statusID, priorityID int
	err = tdb.DB.QueryRow("SELECT id FROM statuses LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	err = tdb.DB.QueryRow("SELECT id FROM priorities LIMIT 1").Scan(&priorityID)
	if err != nil {
		t.Fatalf("Failed to get priority: %v", err)
	}

	// Get or create user
	var userID int
	err = tdb.DB.QueryRow("SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		userResult, err := tdb.DB.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES ('testuser', 'test@example.com', 'Test', 'User', 'hash', ?, ?)
		`, now, now)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		userID64, _ := userResult.LastInsertId()
		userID = int(userID64)
	}

	// Create test item
	itemResult, err := tdb.DB.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, status, is_task,
		                   frac_index, path, created_at, updated_at)
		VALUES (?, 1, 'Test Item', 'Test Description', 'open', 0, 'a0', '/1/', ?, ?)
	`, workspaceID, now, now)
	if err != nil {
		t.Fatalf("Failed to create test item: %v", err)
	}
	itemID64, _ := itemResult.LastInsertId()
	itemID := int(itemID64)

	return &RepositoryTestData{
		WorkspaceID: workspaceID,
		ItemID:      itemID,
		StatusID:    statusID,
		PriorityID:  priorityID,
		UserID:      userID,
	}
}

type HierarchyTestData struct {
	WorkspaceID int
	ParentID    int
	ChildID1    int
	ChildID2    int
	UserID      int
}

func setupHierarchyTestData(t *testing.T, tdb *testutils.TestDB) *HierarchyTestData {
	now := time.Now()

	// Create workspace
	result, err := tdb.DB.Exec(`
		INSERT INTO workspaces (name, key, description, created_at, updated_at)
		VALUES ('Hierarchy Test', 'HRC', 'Hierarchy test workspace', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := result.LastInsertId()
	workspaceID := int(workspaceID64)

	// Get or create user
	var userID int
	err = tdb.DB.QueryRow("SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		userResult, err := tdb.DB.Exec(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, created_at, updated_at)
			VALUES ('hierarchyuser', 'hierarchy@example.com', 'Hierarchy', 'User', 'hash', ?, ?)
		`, now, now)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		userID64, _ := userResult.LastInsertId()
		userID = int(userID64)
	}

	// Create parent item
	parentResult, err := tdb.DB.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, status, is_task,
		                   frac_index, path, created_at, updated_at)
		VALUES (?, 1, 'Parent Item', 'Parent Description', 'open', 0, 'a0', '/1/', ?, ?)
	`, workspaceID, now, now)
	if err != nil {
		t.Fatalf("Failed to create parent item: %v", err)
	}
	parentID64, _ := parentResult.LastInsertId()
	parentID := int(parentID64)

	// Create child items
	child1Result, err := tdb.DB.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, status, is_task,
		                   parent_id, frac_index, path, created_at, updated_at)
		VALUES (?, 2, 'Child Item 1', 'Child 1 Description', 'open', 0, ?, 'a0', '/1/2/', ?, ?)
	`, workspaceID, parentID, now, now)
	if err != nil {
		t.Fatalf("Failed to create child item 1: %v", err)
	}
	childID1Int64, _ := child1Result.LastInsertId()
	childID1 := int(childID1Int64)

	child2Result, err := tdb.DB.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, title, description, status, is_task,
		                   parent_id, frac_index, path, created_at, updated_at)
		VALUES (?, 3, 'Child Item 2', 'Child 2 Description', 'open', 0, ?, 'a1', '/1/3/', ?, ?)
	`, workspaceID, parentID, now, now)
	if err != nil {
		t.Fatalf("Failed to create child item 2: %v", err)
	}
	childID2Int64, _ := child2Result.LastInsertId()
	childID2 := int(childID2Int64)

	return &HierarchyTestData{
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		ChildID1:    childID1,
		ChildID2:    childID2,
		UserID:      userID,
	}
}
