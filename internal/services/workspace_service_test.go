package services_test

import (
	"testing"

	"windshift/internal/database"
	"windshift/internal/services"
	"windshift/internal/testutils"
	"windshift/internal/testutils/factory"
)

// workspaceTestEnv contains test data for workspace service tests
type workspaceTestEnv struct {
	WorkspaceID   int
	WorkspaceName string
	WorkspaceKey  string
	UserID        int
}

// createWorkspaceTestDB creates a test database for workspace service tests
func createWorkspaceTestDB(t *testing.T) database.Database {
	t.Helper()
	tdb := testutils.CreateTestDB(t, true)
	t.Cleanup(func() { tdb.Close() })
	return tdb.GetDatabase()
}

// setupWorkspaceTestEnv creates test data for workspace service tests using the factory
func setupWorkspaceTestEnv(t *testing.T, db database.Database) workspaceTestEnv {
	t.Helper()
	f := factory.NewTestFactory(db)

	// Create user first
	userID, err := f.CreateUser(nil)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create workspace with explicit values for testing
	workspaceID, err := f.CreateWorkspace(factory.CreateWorkspaceOpts{
		Name:        "Test Workspace",
		Key:         "WST",
		Description: "Test workspace",
		CreatorID:   userID,
	})
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	return workspaceTestEnv{
		WorkspaceID:   workspaceID,
		WorkspaceName: "Test Workspace",
		WorkspaceKey:  "WST",
		UserID:        userID,
	}
}

func TestWorkspaceService_List(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("ReturnsAccessibleWorkspaces", func(t *testing.T) {
		workspaces, total, err := service.List(services.WorkspaceListParams{
			UserID: env.UserID,
			Limit:  100,
			Offset: 0,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(workspaces) == 0 {
			t.Error("Expected at least one workspace")
		}
		if total == 0 {
			t.Error("Expected total to be at least 1")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		workspaces, _, err := service.List(services.WorkspaceListParams{
			UserID: env.UserID,
			Limit:  1,
			Offset: 0,
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(workspaces) > 1 {
			t.Errorf("Expected at most 1 workspace with limit 1, got %d", len(workspaces))
		}
	})
}

func TestWorkspaceService_GetByID(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		workspace, err := service.GetByID(env.WorkspaceID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workspace.ID != env.WorkspaceID {
			t.Errorf("Expected workspace ID %d, got %d", env.WorkspaceID, workspace.ID)
		}
		if workspace.Name != env.WorkspaceName {
			t.Errorf("Expected workspace name '%s', got '%s'", env.WorkspaceName, workspace.Name)
		}
		if workspace.Key != env.WorkspaceKey {
			t.Errorf("Expected workspace key '%s', got '%s'", env.WorkspaceKey, workspace.Key)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetByID(99999)
		if err == nil {
			t.Error("Expected error for non-existent workspace")
		}
	})
}

func TestWorkspaceService_Create(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		params := services.CreateWorkspaceParams{
			Name:        "New Workspace",
			Key:         "new",
			Description: "A new workspace",
			CreatorID:   env.UserID,
		}

		result, err := service.Create(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Workspace.Name != "New Workspace" {
			t.Errorf("Expected name 'New Workspace', got '%s'", result.Workspace.Name)
		}
		// Key should be uppercased
		if result.Workspace.Key != "NEW" {
			t.Errorf("Expected key 'NEW', got '%s'", result.Workspace.Key)
		}
	})

	t.Run("DuplicateKey", func(t *testing.T) {
		params := services.CreateWorkspaceParams{
			Name:        "Duplicate Workspace",
			Key:         env.WorkspaceKey, // Same as existing
			Description: "Should fail",
			CreatorID:   env.UserID,
		}

		_, err := service.Create(params)
		if err == nil {
			t.Error("Expected error for duplicate key")
		}
	})
}

func TestWorkspaceService_Update(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		newName := "Updated Workspace"
		newDesc := "Updated description"
		params := services.UpdateWorkspaceParams{
			ID:          env.WorkspaceID,
			Name:        &newName,
			Description: &newDesc,
		}

		workspace, err := service.Update(params)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workspace.Name != "Updated Workspace" {
			t.Errorf("Expected name 'Updated Workspace', got '%s'", workspace.Name)
		}
		if workspace.Description != "Updated description" {
			t.Errorf("Expected description 'Updated description', got '%s'", workspace.Description)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		newName := "Non-existent"
		params := services.UpdateWorkspaceParams{
			ID:   99999,
			Name: &newName,
		}

		_, err := service.Update(params)
		if err == nil {
			t.Error("Expected error for non-existent workspace")
		}
	})
}

func TestWorkspaceService_Delete(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		// Create a workspace to delete
		result, _ := service.Create(services.CreateWorkspaceParams{
			Name:        "To Delete",
			Key:         "DEL",
			Description: "Will be deleted",
			CreatorID:   env.UserID,
		})

		err := service.Delete(result.Workspace.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Verify deletion
		_, err = service.GetByID(result.Workspace.ID)
		if err == nil {
			t.Error("Expected error for deleted workspace")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		err := service.Delete(99999)
		if err == nil {
			t.Error("Expected error for non-existent workspace")
		}
	})
}

func TestWorkspaceService_Exists(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("ExistingWorkspace", func(t *testing.T) {
		exists, err := service.Exists(env.WorkspaceID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !exists {
			t.Error("Expected workspace to exist")
		}
	})

	t.Run("NonExistentWorkspace", func(t *testing.T) {
		exists, err := service.Exists(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if exists {
			t.Error("Expected workspace to not exist")
		}
	})
}

func TestWorkspaceService_KeyExists(t *testing.T) {
	db := createWorkspaceTestDB(t)

	service := services.NewWorkspaceService(db)
	env := setupWorkspaceTestEnv(t, db)

	t.Run("ExistingKey", func(t *testing.T) {
		exists, err := service.KeyExists(env.WorkspaceKey)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !exists {
			t.Error("Expected key to exist")
		}
	})

	t.Run("ExistingKeyLowercase", func(t *testing.T) {
		// Should work case-insensitively (key is normalized to uppercase)
		exists, err := service.KeyExists("wst")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !exists {
			t.Error("Expected key to exist (case-insensitive)")
		}
	})

	t.Run("NonExistentKey", func(t *testing.T) {
		exists, err := service.KeyExists("NONEXISTENT")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if exists {
			t.Error("Expected key to not exist")
		}
	})
}
