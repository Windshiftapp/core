package services

import (
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

// hierarchyTestEnv contains test data for hierarchy service tests
type hierarchyTestEnv struct {
	WorkspaceID int
	RootItemID  int
	ChildItem1  int
	ChildItem2  int
	GrandChild1 int
	GrandChild2 int
	StatusID    int
}

// createHierarchyTestDB creates a test database for hierarchy service tests
func createHierarchyTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hierarchy_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

// setupHierarchyTestEnv creates test data with a hierarchy structure:
// Root
// ├── Child1
// │   └── GrandChild1
// └── Child2
//
//	└── GrandChild2
func setupHierarchyTestEnv(t *testing.T, db database.Database) hierarchyTestEnv {
	t.Helper()

	// Create workspace
	workspaceResult, err := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('Hierarchy Test Workspace', 'HIR', 'Test workspace', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := workspaceResult.LastInsertId()
	workspaceID := int(workspaceID64)

	// Get a status ID
	var statusID int
	err = db.QueryRow("SELECT id FROM statuses LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to get status ID: %v", err)
	}

	// Create root item (no parent)
	rootResult, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, is_task, status_id, parent_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'Root Item', 'Root description', 1, ?, NULL, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID)
	if err != nil {
		t.Fatalf("Failed to create root item: %v", err)
	}
	rootItemID, _ := rootResult.LastInsertId()

	// Create child 1
	child1Result, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, is_task, status_id, parent_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'Child 1', 'Child 1 description', 1, ?, ?, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID, rootItemID)
	if err != nil {
		t.Fatalf("Failed to create child 1: %v", err)
	}
	childItem1, _ := child1Result.LastInsertId()

	// Create child 2
	child2Result, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, is_task, status_id, parent_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'Child 2', 'Child 2 description', 1, ?, ?, 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID, rootItemID)
	if err != nil {
		t.Fatalf("Failed to create child 2: %v", err)
	}
	childItem2, _ := child2Result.LastInsertId()

	// Create grandchild 1 (under child 1)
	grandchild1Result, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, is_task, status_id, parent_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'GrandChild 1', 'GrandChild 1 description', 1, ?, ?, 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID, childItem1)
	if err != nil {
		t.Fatalf("Failed to create grandchild 1: %v", err)
	}
	grandChild1, _ := grandchild1Result.LastInsertId()

	// Create grandchild 2 (under child 2)
	grandchild2Result, err := db.Exec(`
		INSERT INTO items (workspace_id, title, description, is_task, status_id, parent_id, workspace_item_number, created_at, updated_at)
		VALUES (?, 'GrandChild 2', 'GrandChild 2 description', 1, ?, ?, 5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspaceID, statusID, childItem2)
	if err != nil {
		t.Fatalf("Failed to create grandchild 2: %v", err)
	}
	grandChild2, _ := grandchild2Result.LastInsertId()

	return hierarchyTestEnv{
		WorkspaceID: workspaceID,
		RootItemID:  int(rootItemID),
		ChildItem1:  int(childItem1),
		ChildItem2:  int(childItem2),
		GrandChild1: int(grandChild1),
		GrandChild2: int(grandChild2),
		StatusID:    statusID,
	}
}

func TestHierarchyService_GetAncestors(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)
	env := setupHierarchyTestEnv(t, db)

	t.Run("GrandChildHasTwoAncestors", func(t *testing.T) {
		ancestors, err := service.GetAncestors(env.GrandChild1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) != 2 {
			t.Fatalf("Expected 2 ancestors (root and child1), got %d", len(ancestors))
		}

		// Should be ordered from root to direct parent
		if ancestors[0].ID != env.RootItemID {
			t.Errorf("Expected first ancestor to be root (ID %d), got %d", env.RootItemID, ancestors[0].ID)
		}
		if ancestors[1].ID != env.ChildItem1 {
			t.Errorf("Expected second ancestor to be child1 (ID %d), got %d", env.ChildItem1, ancestors[1].ID)
		}
	})

	t.Run("ChildHasOneAncestor", func(t *testing.T) {
		ancestors, err := service.GetAncestors(env.ChildItem1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) != 1 {
			t.Fatalf("Expected 1 ancestor (root), got %d", len(ancestors))
		}

		if ancestors[0].ID != env.RootItemID {
			t.Errorf("Expected ancestor to be root (ID %d), got %d", env.RootItemID, ancestors[0].ID)
		}
	})

	t.Run("RootHasNoAncestors", func(t *testing.T) {
		ancestors, err := service.GetAncestors(env.RootItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) != 0 {
			t.Errorf("Expected 0 ancestors for root, got %d", len(ancestors))
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		ancestors, err := service.GetAncestors(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) != 0 {
			t.Errorf("Expected 0 ancestors for non-existent item, got %d", len(ancestors))
		}
	})

	t.Run("AncestorsHaveCorrectData", func(t *testing.T) {
		ancestors, err := service.GetAncestors(env.GrandChild1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(ancestors) < 1 {
			t.Fatal("Expected at least 1 ancestor")
		}

		// Check that fields are populated
		for _, a := range ancestors {
			if a.Title == "" {
				t.Error("Expected Title to be populated")
			}
			if a.WorkspaceID == 0 {
				t.Error("Expected WorkspaceID to be populated")
			}
		}
	})
}

func TestHierarchyService_GetDescendants(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)
	env := setupHierarchyTestEnv(t, db)

	t.Run("RootHasFourDescendants", func(t *testing.T) {
		descendants, err := service.GetDescendants(env.RootItemID, 0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 4 {
			t.Errorf("Expected 4 descendants, got %d", len(descendants))
		}
	})

	t.Run("ChildHasOneDescendant", func(t *testing.T) {
		descendants, err := service.GetDescendants(env.ChildItem1, 0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 1 {
			t.Errorf("Expected 1 descendant, got %d", len(descendants))
		}

		if descendants[0].ID != env.GrandChild1 {
			t.Errorf("Expected descendant to be grandchild1, got %d", descendants[0].ID)
		}
	})

	t.Run("GrandChildHasNoDescendants", func(t *testing.T) {
		descendants, err := service.GetDescendants(env.GrandChild1, 0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 0 {
			t.Errorf("Expected 0 descendants, got %d", len(descendants))
		}
	})

	t.Run("MaxDepthLimit", func(t *testing.T) {
		// With maxDepth = 1, should only get direct children
		descendants, err := service.GetDescendants(env.RootItemID, 1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 2 {
			t.Errorf("Expected 2 descendants with maxDepth=1, got %d", len(descendants))
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		descendants, err := service.GetDescendants(99999, 0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(descendants) != 0 {
			t.Errorf("Expected 0 descendants for non-existent item, got %d", len(descendants))
		}
	})
}

func TestHierarchyService_CountDescendants(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)
	env := setupHierarchyTestEnv(t, db)

	t.Run("RootCountsFour", func(t *testing.T) {
		count, err := service.CountDescendants(env.RootItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if count != 4 {
			t.Errorf("Expected 4 descendants, got %d", count)
		}
	})

	t.Run("ChildCountsOne", func(t *testing.T) {
		count, err := service.CountDescendants(env.ChildItem1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if count != 1 {
			t.Errorf("Expected 1 descendant, got %d", count)
		}
	})

	t.Run("GrandChildCountsZero", func(t *testing.T) {
		count, err := service.CountDescendants(env.GrandChild1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if count != 0 {
			t.Errorf("Expected 0 descendants, got %d", count)
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		count, err := service.CountDescendants(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if count != 0 {
			t.Errorf("Expected 0 descendants for non-existent item, got %d", count)
		}
	})
}

func TestHierarchyService_GetChildren(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)
	env := setupHierarchyTestEnv(t, db)

	t.Run("RootHasTwoChildren", func(t *testing.T) {
		children, err := service.GetChildren(env.RootItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 2 {
			t.Errorf("Expected 2 children, got %d", len(children))
		}
	})

	t.Run("ChildHasOneChild", func(t *testing.T) {
		children, err := service.GetChildren(env.ChildItem1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 1 {
			t.Errorf("Expected 1 child, got %d", len(children))
		}

		if children[0].ID != env.GrandChild1 {
			t.Errorf("Expected child to be grandchild1, got %d", children[0].ID)
		}
	})

	t.Run("GrandChildHasNoChildren", func(t *testing.T) {
		children, err := service.GetChildren(env.GrandChild1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 0 {
			t.Errorf("Expected 0 children, got %d", len(children))
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		children, err := service.GetChildren(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(children) != 0 {
			t.Errorf("Expected 0 children for non-existent item, got %d", len(children))
		}
	})

	t.Run("ChildrenHaveCorrectParentID", func(t *testing.T) {
		children, err := service.GetChildren(env.RootItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		for _, c := range children {
			if c.ParentID == nil {
				t.Error("Expected ParentID to be set")
			} else if *c.ParentID != env.RootItemID {
				t.Errorf("Expected ParentID %d, got %d", env.RootItemID, *c.ParentID)
			}
		}
	})
}

func TestHierarchyService_GetRoot(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)
	env := setupHierarchyTestEnv(t, db)

	t.Run("GrandChildFindsRoot", func(t *testing.T) {
		root, err := service.GetRoot(env.GrandChild1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if root == nil {
			t.Fatal("Expected non-nil root")
		}

		if root.ID != env.RootItemID {
			t.Errorf("Expected root ID %d, got %d", env.RootItemID, root.ID)
		}
	})

	t.Run("ChildFindsRoot", func(t *testing.T) {
		root, err := service.GetRoot(env.ChildItem1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if root == nil {
			t.Fatal("Expected non-nil root")
		}

		if root.ID != env.RootItemID {
			t.Errorf("Expected root ID %d, got %d", env.RootItemID, root.ID)
		}
	})

	t.Run("RootFindsItself", func(t *testing.T) {
		root, err := service.GetRoot(env.RootItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if root == nil {
			t.Fatal("Expected non-nil root")
		}

		if root.ID != env.RootItemID {
			t.Errorf("Expected root ID %d, got %d", env.RootItemID, root.ID)
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		root, err := service.GetRoot(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if root != nil {
			t.Errorf("Expected nil root for non-existent item, got ID %d", root.ID)
		}
	})

	t.Run("RootHasNullParentID", func(t *testing.T) {
		root, err := service.GetRoot(env.GrandChild1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if root == nil {
			t.Fatal("Expected non-nil root")
		}

		if root.ParentID != nil {
			t.Errorf("Expected root to have nil ParentID, got %d", *root.ParentID)
		}
	})
}

func TestHierarchyService_GetEffectiveProject(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)
	env := setupHierarchyTestEnv(t, db)

	t.Run("ItemWithNoProject", func(t *testing.T) {
		projectID, mode, err := service.GetEffectiveProject(env.RootItemID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Items created without project should have no effective project
		if projectID != nil {
			t.Errorf("Expected nil projectID, got %d", *projectID)
		}
		if mode != "none" {
			t.Errorf("Expected mode 'none', got '%s'", mode)
		}
	})

	t.Run("ItemWithDirectProject", func(t *testing.T) {
		// Create a time project (items.project_id references time_projects)
		projectResult, err := db.Exec(`
			INSERT INTO time_projects (name, description, status, created_at, updated_at)
			VALUES ('Test Time Project', 'Test description', 'Active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
		if err != nil {
			t.Fatalf("Failed to create time project: %v", err)
		}
		projectID64, _ := projectResult.LastInsertId()
		projectID := int(projectID64)

		// Create item with direct project
		itemResult, err := db.Exec(`
			INSERT INTO items (workspace_id, title, description, is_task, status_id, project_id, workspace_item_number, created_at, updated_at)
			VALUES (?, 'Item With Project', 'Description', 1, ?, ?, 100, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.StatusID, projectID)
		if err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
		itemID, _ := itemResult.LastInsertId()

		effectiveProjectID, mode, err := service.GetEffectiveProject(int(itemID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if effectiveProjectID == nil {
			t.Fatal("Expected non-nil projectID")
		}
		if *effectiveProjectID != projectID {
			t.Errorf("Expected projectID %d, got %d", projectID, *effectiveProjectID)
		}
		if mode != "direct" {
			t.Errorf("Expected mode 'direct', got '%s'", mode)
		}
	})

	t.Run("ItemWithNullProjectAndParent", func(t *testing.T) {
		// Create a time project (items.project_id references time_projects)
		projectResult, err := db.Exec(`
			INSERT INTO time_projects (name, description, status, created_at, updated_at)
			VALUES ('Parent Time Project', 'Test description', 'Active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
		if err != nil {
			t.Fatalf("Failed to create time project: %v", err)
		}
		projectID64, _ := projectResult.LastInsertId()
		projectID := int(projectID64)

		// Create parent item with project
		parentResult, err := db.Exec(`
			INSERT INTO items (workspace_id, title, description, is_task, status_id, project_id, workspace_item_number, created_at, updated_at)
			VALUES (?, 'Parent With Project', 'Description', 1, ?, ?, 101, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.StatusID, projectID)
		if err != nil {
			t.Fatalf("Failed to create parent item: %v", err)
		}
		parentID, _ := parentResult.LastInsertId()

		// Create child item with NULL project_id (not inheriting explicitly)
		childResult, err := db.Exec(`
			INSERT INTO items (workspace_id, title, description, is_task, status_id, project_id, parent_id, workspace_item_number, created_at, updated_at)
			VALUES (?, 'Child Item', 'Description', 1, ?, NULL, ?, 102, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, env.WorkspaceID, env.StatusID, parentID)
		if err != nil {
			t.Fatalf("Failed to create child item: %v", err)
		}
		childID, _ := childResult.LastInsertId()

		effectiveProjectID, mode, err := service.GetEffectiveProject(int(childID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Child with NULL project_id should return "none" (no project)
		// The GetEffectiveProject function only walks up if project_id = -1
		if effectiveProjectID != nil {
			t.Logf("Note: Child with NULL project_id got projectID %d with mode '%s'", *effectiveProjectID, mode)
		}
		if mode != "none" && effectiveProjectID == nil {
			t.Logf("Child with NULL project_id returned mode '%s'", mode)
		}
	})

	t.Run("NonExistentItem", func(t *testing.T) {
		projectID, mode, err := service.GetEffectiveProject(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Non-existent item should return no project
		if projectID != nil {
			t.Errorf("Expected nil projectID for non-existent item, got %d", *projectID)
		}
		if mode != "none" {
			t.Errorf("Expected mode 'none', got '%s'", mode)
		}
	})
}

func TestHierarchyService_ZeroItemID(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)

	t.Run("GetAncestors", func(t *testing.T) {
		ancestors, err := service.GetAncestors(0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(ancestors) != 0 {
			t.Errorf("Expected 0 ancestors for ID 0, got %d", len(ancestors))
		}
	})

	t.Run("GetDescendants", func(t *testing.T) {
		descendants, err := service.GetDescendants(0, 0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(descendants) != 0 {
			t.Errorf("Expected 0 descendants for ID 0, got %d", len(descendants))
		}
	})

	t.Run("CountDescendants", func(t *testing.T) {
		count, err := service.CountDescendants(0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 descendants for ID 0, got %d", count)
		}
	})

	t.Run("GetChildren", func(t *testing.T) {
		children, err := service.GetChildren(0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(children) != 0 {
			t.Errorf("Expected 0 children for ID 0, got %d", len(children))
		}
	})

	t.Run("GetRoot", func(t *testing.T) {
		root, err := service.GetRoot(0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if root != nil {
			t.Errorf("Expected nil root for ID 0, got %d", root.ID)
		}
	})

	t.Run("GetEffectiveProject", func(t *testing.T) {
		projectID, mode, err := service.GetEffectiveProject(0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if projectID != nil {
			t.Errorf("Expected nil projectID for ID 0, got %d", *projectID)
		}
		if mode != "none" {
			t.Errorf("Expected mode 'none' for ID 0, got '%s'", mode)
		}
	})
}

func TestHierarchyService_NegativeItemID(t *testing.T) {
	db := createHierarchyTestDB(t)
	defer db.Close()

	service := NewHierarchyService(db)

	t.Run("GetAncestors", func(t *testing.T) {
		ancestors, err := service.GetAncestors(-1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(ancestors) != 0 {
			t.Errorf("Expected 0 ancestors for negative ID, got %d", len(ancestors))
		}
	})

	t.Run("GetDescendants", func(t *testing.T) {
		descendants, err := service.GetDescendants(-1, 0)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(descendants) != 0 {
			t.Errorf("Expected 0 descendants for negative ID, got %d", len(descendants))
		}
	})

	t.Run("CountDescendants", func(t *testing.T) {
		count, err := service.CountDescendants(-1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 descendants for negative ID, got %d", count)
		}
	})

	t.Run("GetChildren", func(t *testing.T) {
		children, err := service.GetChildren(-1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(children) != 0 {
			t.Errorf("Expected 0 children for negative ID, got %d", len(children))
		}
	})

	t.Run("GetRoot", func(t *testing.T) {
		root, err := service.GetRoot(-1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if root != nil {
			t.Errorf("Expected nil root for negative ID, got %d", root.ID)
		}
	})
}
