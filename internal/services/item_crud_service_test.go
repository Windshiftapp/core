package services_test

import (
	"testing"

	"windshift/internal/database"
	"windshift/internal/services"
	"windshift/internal/testutils"
	"windshift/internal/testutils/factory"
)

// itemCRUDTestEnv contains test data for item CRUD service tests
type itemCRUDTestEnv struct {
	WorkspaceID int
	ItemTypeID  int
	StatusID    int
	PriorityID  int
	UserID      int
	ItemID      int
	ChildItemID int
}

// createItemCRUDTestDB creates a test database for item CRUD service tests
func createItemCRUDTestDB(t *testing.T) database.Database {
	t.Helper()
	tdb := testutils.CreateTestDB(t, true)
	t.Cleanup(func() { tdb.Close() })
	return tdb.GetDatabase()
}

// setupItemCRUDTestEnv creates test data for item CRUD service tests using the factory
func setupItemCRUDTestEnv(t *testing.T, db database.Database) itemCRUDTestEnv {
	t.Helper()
	f := factory.NewTestFactory(db)

	// Create user and workspace using factory
	userID, workspaceID, err := f.CreateUserAndWorkspace()
	if err != nil {
		t.Fatalf("Failed to create user and workspace: %v", err)
	}

	// Get default item type (from database initialization)
	var itemTypeID int
	err = db.QueryRow("SELECT id FROM item_types LIMIT 1").Scan(&itemTypeID)
	if err != nil {
		t.Fatalf("Failed to get item type: %v", err)
	}

	// Get default status (from database initialization)
	var statusID int
	err = db.QueryRow("SELECT id FROM statuses WHERE is_default = 1 LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get default status: %v", err)
	}

	// Get default priority (from database initialization)
	var priorityID int
	err = db.QueryRow("SELECT id FROM priorities LIMIT 1").Scan(&priorityID)
	if err != nil {
		t.Fatalf("Failed to get priority: %v", err)
	}

	// Create parent item using factory
	itemID, err := f.CreateItem(factory.CreateItemOpts{
		WorkspaceID: workspaceID,
		Title:       "Parent Item",
		Description: "A parent item",
		StatusID:    &statusID,
		PriorityID:  &priorityID,
		ItemTypeID:  &itemTypeID,
		CreatorID:   &userID,
	})
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	// Create child item using factory
	childItemID, err := f.CreateItem(factory.CreateItemOpts{
		WorkspaceID: workspaceID,
		Title:       "Child Item",
		Description: "A child item",
		StatusID:    &statusID,
		PriorityID:  &priorityID,
		ItemTypeID:  &itemTypeID,
		ParentID:    &itemID,
		CreatorID:   &userID,
	})
	if err != nil {
		t.Fatalf("Failed to create child item: %v", err)
	}

	return itemCRUDTestEnv{
		WorkspaceID: workspaceID,
		ItemTypeID:  itemTypeID,
		StatusID:    statusID,
		PriorityID:  priorityID,
		UserID:      userID,
		ItemID:      itemID,
		ChildItemID: childItemID,
	}
}

func TestItemCRUDService_GetByID(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		item, err := service.GetByID(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.ID != env.ItemID {
			t.Errorf("Expected item ID %d, got %d", env.ItemID, item.ID)
		}
		if item.Title != "Parent Item" {
			t.Errorf("Expected title 'Parent Item', got '%s'", item.Title)
		}
		if item.WorkspaceID != env.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", env.WorkspaceID, item.WorkspaceID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetByID(99999)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestItemCRUDService_GetByIDBasic(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		item, err := service.GetByIDBasic(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.ID != env.ItemID {
			t.Errorf("Expected item ID %d, got %d", env.ItemID, item.ID)
		}
		if item.Title != "Parent Item" {
			t.Errorf("Expected title 'Parent Item', got '%s'", item.Title)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetByIDBasic(99999)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestItemCRUDService_Exists(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("ExistingItem", func(t *testing.T) {
		exists, err := service.Exists(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !exists {
			t.Error("Expected item to exist")
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		exists, err := service.Exists(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if exists {
			t.Error("Expected item to not exist")
		}
	})
}

func TestItemCRUDService_GetWorkspaceID(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		workspaceID, err := service.GetWorkspaceID(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workspaceID != env.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", env.WorkspaceID, workspaceID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetWorkspaceID(99999)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestItemCRUDService_GetChildren(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("HasChildren", func(t *testing.T) {
		children, err := service.GetChildren(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 1 {
			t.Errorf("Expected 1 child, got %d", len(children))
		}
		if len(children) > 0 && children[0].ID != env.ChildItemID {
			t.Errorf("Expected child ID %d, got %d", env.ChildItemID, children[0].ID)
		}
	})

	t.Run("NoChildren", func(t *testing.T) {
		children, err := service.GetChildren(env.ChildItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 0 {
			t.Errorf("Expected 0 children, got %d", len(children))
		}
	})
}

func TestItemCRUDService_GetDescendants(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	// Create grandchild
	_, err := db.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
		                   status_id, priority_id, creator_id, parent_id, created_at, updated_at)
		VALUES (?, 3, ?, 'Grandchild Item', 'A grandchild item', ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, env.WorkspaceID, env.ItemTypeID, env.StatusID, env.PriorityID, env.UserID, env.ChildItemID)
	if err != nil {
		t.Fatalf("Failed to create grandchild: %v", err)
	}

	t.Run("HasDescendants", func(t *testing.T) {
		descendants, err := service.GetDescendants(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 2 {
			t.Errorf("Expected 2 descendants (child + grandchild), got %d", len(descendants))
		}
	})

	t.Run("NoDescendants", func(t *testing.T) {
		// Get grandchild ID
		var grandchildID int
		err := db.QueryRow("SELECT id FROM items WHERE title = 'Grandchild Item'").Scan(&grandchildID)
		if err != nil {
			t.Fatalf("Failed to get grandchild ID: %v", err)
		}

		descendants, err := service.GetDescendants(grandchildID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 0 {
			t.Errorf("Expected 0 descendants, got %d", len(descendants))
		}
	})
}

func TestItemCRUDService_GetAncestors(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("HasAncestors", func(t *testing.T) {
		ancestors, err := service.GetAncestors(env.ChildItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) != 1 {
			t.Errorf("Expected 1 ancestor, got %d", len(ancestors))
		}
		if len(ancestors) > 0 && ancestors[0].ID != env.ItemID {
			t.Errorf("Expected ancestor ID %d, got %d", env.ItemID, ancestors[0].ID)
		}
	})

	t.Run("NoAncestors", func(t *testing.T) {
		ancestors, err := service.GetAncestors(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) != 0 {
			t.Errorf("Expected 0 ancestors, got %d", len(ancestors))
		}
	})
}

func TestItemCRUDService_GetRootItems(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("ReturnsRootItems", func(t *testing.T) {
		roots, err := service.GetRootItems(env.WorkspaceID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(roots) != 1 {
			t.Errorf("Expected 1 root item, got %d", len(roots))
		}
		if len(roots) > 0 && roots[0].ID != env.ItemID {
			t.Errorf("Expected root item ID %d, got %d", env.ItemID, roots[0].ID)
		}
	})

	t.Run("EmptyWorkspace", func(t *testing.T) {
		roots, err := service.GetRootItems(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(roots) != 0 {
			t.Errorf("Expected 0 root items, got %d", len(roots))
		}
	})
}

func TestItemCRUDService_Delete(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("DeletesItemAndDescendants", func(t *testing.T) {
		// Create a new item to delete (so we don't affect other tests)
		result, err := db.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
			                   status_id, priority_id, creator_id, created_at, updated_at)
			VALUES (?, 10, ?, 'Delete Me', 'To be deleted', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.ItemTypeID, env.StatusID, env.PriorityID, env.UserID)
		if err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
		deleteID64, _ := result.LastInsertId()
		deleteID := int(deleteID64)

		// Create a child of this item
		childResult, err := db.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
			                   status_id, priority_id, creator_id, parent_id, created_at, updated_at)
			VALUES (?, 11, ?, 'Delete Me Child', 'Child to be deleted', ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.ItemTypeID, env.StatusID, env.PriorityID, env.UserID, deleteID)
		if err != nil {
			t.Fatalf("Failed to create child: %v", err)
		}
		childDeleteID64, _ := childResult.LastInsertId()
		childDeleteID := int(childDeleteID64)

		// Delete the parent
		deleteResult, err := service.Delete(deleteID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if deleteResult.DeletedCount != 2 {
			t.Errorf("Expected 2 deleted items, got %d", deleteResult.DeletedCount)
		}
		if len(deleteResult.DescendantIDs) != 1 {
			t.Errorf("Expected 1 descendant ID, got %d", len(deleteResult.DescendantIDs))
		}

		// Verify deletion
		exists, _ := service.Exists(deleteID)
		if exists {
			t.Error("Expected parent item to be deleted")
		}
		exists, _ = service.Exists(childDeleteID)
		if exists {
			t.Error("Expected child item to be deleted")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.Delete(99999)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestItemCRUDService_Copy(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		opts := services.CopyOptions{
			NewTitle:  "Copied Item",
			CreatorID: env.UserID,
		}

		result, err := service.Copy(env.ItemID, opts)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.NewItemID == 0 {
			t.Error("Expected new item ID to be set")
		}
		if result.CopyCount != 1 {
			t.Errorf("Expected copy count 1, got %d", result.CopyCount)
		}

		// Verify the copied item
		copied, err := service.GetByID(result.NewItemID)
		if err != nil {
			t.Fatalf("Failed to get copied item: %v", err)
		}

		if copied.Title != "Copied Item" {
			t.Errorf("Expected title 'Copied Item', got '%s'", copied.Title)
		}
		if copied.WorkspaceID != env.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", env.WorkspaceID, copied.WorkspaceID)
		}
	})

	t.Run("WithNewParent", func(t *testing.T) {
		// Create a new parent
		parentResult, err := db.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
			                   status_id, priority_id, creator_id, created_at, updated_at)
			VALUES (?, 20, ?, 'New Parent', 'New parent item', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.ItemTypeID, env.StatusID, env.PriorityID, env.UserID)
		if err != nil {
			t.Fatalf("Failed to create parent: %v", err)
		}
		newParentID64, _ := parentResult.LastInsertId()
		newParentID := int(newParentID64)

		opts := services.CopyOptions{
			NewTitle:    "Copied With Parent",
			CreatorID:   env.UserID,
			NewParentID: &newParentID,
		}

		result, err := service.Copy(env.ChildItemID, opts)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify the parent
		copied, err := service.GetByIDBasic(result.NewItemID)
		if err != nil {
			t.Fatalf("Failed to get copied item: %v", err)
		}

		if copied.ParentID == nil || *copied.ParentID != newParentID {
			t.Errorf("Expected parent ID %d, got %v", newParentID, copied.ParentID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		opts := services.CopyOptions{
			NewTitle:  "Won't Work",
			CreatorID: env.UserID,
		}

		_, err := service.Copy(99999, opts)
		if err == nil {
			t.Error("Expected error for non-existent item")
		}
	})
}

func TestItemCRUDService_List(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("ReturnsItems", func(t *testing.T) {
		workspaceID := env.WorkspaceID
		params := services.ItemListParams{
			Filters: services.ItemFilters{
				WorkspaceID: &workspaceID,
			},
			Pagination: services.PaginationParams{
				Limit:  100,
				Offset: 0,
			},
		}

		items, total, err := service.List(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(items) < 2 {
			t.Errorf("Expected at least 2 items, got %d", len(items))
		}
		if total < 2 {
			t.Errorf("Expected total at least 2, got %d", total)
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		workspaceID := env.WorkspaceID
		params := services.ItemListParams{
			Filters: services.ItemFilters{
				WorkspaceID: &workspaceID,
			},
			Pagination: services.PaginationParams{
				Limit:  1,
				Offset: 0,
			},
		}

		items, _, err := service.List(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(items) != 1 {
			t.Errorf("Expected 1 item with limit 1, got %d", len(items))
		}
	})

	t.Run("EmptyWorkspace", func(t *testing.T) {
		nonExistentID := 99999
		params := services.ItemListParams{
			Filters: services.ItemFilters{
				WorkspaceID: &nonExistentID,
			},
			Pagination: services.PaginationParams{
				Limit:  100,
				Offset: 0,
			},
		}

		items, total, err := service.List(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(items) != 0 {
			t.Errorf("Expected 0 items, got %d", len(items))
		}
		if total != 0 {
			t.Errorf("Expected total 0, got %d", total)
		}
	})
}

func TestItemCRUDService_Search(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("FindsByTitle", func(t *testing.T) {
		items, total, err := service.Search("Parent", []int{env.WorkspaceID}, services.PaginationParams{Limit: 100, Offset: 0})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(items) == 0 {
			t.Error("Expected at least 1 item matching 'Parent'")
		}
		if total == 0 {
			t.Error("Expected total to be at least 1")
		}
	})

	t.Run("NoResults", func(t *testing.T) {
		items, total, err := service.Search("NonexistentSearchTerm12345", []int{env.WorkspaceID}, services.PaginationParams{Limit: 100, Offset: 0})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(items) != 0 {
			t.Errorf("Expected 0 items, got %d", len(items))
		}
		if total != 0 {
			t.Errorf("Expected total 0, got %d", total)
		}
	})
}

func TestItemCRUDService_GetHistory(t *testing.T) {
	db := createItemCRUDTestDB(t)

	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	// Note: Items created via factory with CreatorID will have creation history.
	// Add an explicit update history entry to test retrieval.
	_, err := db.Exec(`
		INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
		VALUES (?, ?, 'title', 'Old Title', 'New Title', CURRENT_TIMESTAMP)
	`, env.ItemID, env.UserID)
	if err != nil {
		t.Fatalf("Failed to create history: %v", err)
	}

	t.Run("ReturnsHistory", func(t *testing.T) {
		history, err := service.GetHistory(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should have at least the manually added history entry
		if len(history) == 0 {
			t.Error("Expected at least 1 history entry")
		}

		// Find the title update entry we added
		var foundTitleUpdate bool
		for _, h := range history {
			if h.FieldName == "title" && h.OldValue != nil && *h.OldValue == "Old Title" {
				foundTitleUpdate = true
				if h.NewValue == nil || *h.NewValue != "New Title" {
					t.Errorf("Expected new value 'New Title', got '%v'", h.NewValue)
				}
				break
			}
		}
		if !foundTitleUpdate {
			t.Error("Expected to find title update history entry")
		}
	})

	t.Run("HasCreationHistory", func(t *testing.T) {
		// Items created with CreatorID have creation history
		history, err := service.GetHistory(env.ChildItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should have creation history entries
		if len(history) == 0 {
			t.Error("Expected creation history entries for item created with CreatorID")
		}
	})
}

func TestItemCRUDService_GetAttachments(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	// Add an attachment
	_, err := db.Exec(`
		INSERT INTO attachments (item_id, filename, original_filename, file_path, mime_type, file_size, has_thumbnail, uploaded_by, created_at)
		VALUES (?, 'stored_file.pdf', 'document.pdf', '/uploads/stored_file.pdf', 'application/pdf', 1024, 0, ?, CURRENT_TIMESTAMP)
	`, env.ItemID, env.UserID)
	if err != nil {
		t.Fatalf("Failed to create attachment: %v", err)
	}

	t.Run("ReturnsAttachments", func(t *testing.T) {
		attachments, err := service.GetAttachments(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(attachments) == 0 {
			t.Error("Expected at least 1 attachment")
		}
		if len(attachments) > 0 {
			if attachments[0].OriginalFilename != "document.pdf" {
				t.Errorf("Expected filename 'document.pdf', got '%s'", attachments[0].OriginalFilename)
			}
			if attachments[0].MimeType != "application/pdf" {
				t.Errorf("Expected mime type 'application/pdf', got '%s'", attachments[0].MimeType)
			}
		}
	})

	t.Run("NoAttachments", func(t *testing.T) {
		attachments, err := service.GetAttachments(env.ChildItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(attachments) != 0 {
			t.Errorf("Expected 0 attachments, got %d", len(attachments))
		}
	})
}

func TestItemCRUDService_GetWithEffectiveProject(t *testing.T) {
	db := createItemCRUDTestDB(t)
	
	service := services.NewItemCRUDService(db)
	env := setupItemCRUDTestEnv(t, db)

	t.Run("NoProject", func(t *testing.T) {
		item, err := service.GetWithEffectiveProject(env.ItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.ProjectInheritanceMode != "none" {
			t.Errorf("Expected inheritance mode 'none', got '%s'", item.ProjectInheritanceMode)
		}
	})

	t.Run("DirectProject", func(t *testing.T) {
		// Create a time_project (items.project_id references time_projects)
		projectResult, err := db.Exec(`
			INSERT INTO time_projects (name, description, created_at, updated_at)
			VALUES ('Test Time Project', 'A test time project', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
		if err != nil {
			t.Fatalf("Failed to create time_project: %v", err)
		}
		projectID64, _ := projectResult.LastInsertId()
		projectID := int(projectID64)

		// Create item with direct project
		itemResult, err := db.Exec(`
			INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
			                   status_id, priority_id, creator_id, project_id, inherit_project, created_at, updated_at)
			VALUES (?, 30, ?, 'Project Item', 'Item with project', ?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.ItemTypeID, env.StatusID, env.PriorityID, env.UserID, projectID)
		if err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
		itemID64, _ := itemResult.LastInsertId()
		itemWithProjectID := int(itemID64)

		item, err := service.GetWithEffectiveProject(itemWithProjectID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if item.ProjectInheritanceMode != "direct" {
			t.Errorf("Expected inheritance mode 'direct', got '%s'", item.ProjectInheritanceMode)
		}
		if item.EffectiveProjectID == nil || *item.EffectiveProjectID != projectID {
			t.Errorf("Expected effective project ID %d, got %v", projectID, item.EffectiveProjectID)
		}
	})
}
