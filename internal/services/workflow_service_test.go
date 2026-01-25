//go:build test

package services

import (
	"testing"
	"time"

	"windshift/internal/testutils"
)

// WorkflowServiceTestData contains test data for workflow service tests
type WorkflowServiceTestData struct {
	WorkspaceID    int
	WorkflowID     int
	ConfigSetID    int
	StatusID1      int
	StatusID2      int
	StatusID3      int
	CategoryID     int
	ItemTypeID     int
}

// setupWorkflowServiceTestData creates test data for workflow service tests
func setupWorkflowServiceTestData(t *testing.T, tdb *testutils.TestDB) *WorkflowServiceTestData {
	now := time.Now()

	// Create workspace
	result, err := tdb.DB.Exec(`
		INSERT INTO workspaces (name, key, description, created_at, updated_at)
		VALUES ('Workflow Test Workspace', 'WFL', 'Test workspace', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := result.LastInsertId()
	workspaceID := int(workspaceID64)

	// Get existing status category (created during database initialization)
	var categoryID int
	err = tdb.DB.QueryRow("SELECT id FROM status_categories LIMIT 1").Scan(&categoryID)
	if err != nil {
		t.Fatalf("Failed to get status category: %v", err)
	}

	// Create test statuses
	statusResult1, err := tdb.DB.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES ('WF Open', 'Open status', ?, 0, ?, ?)
	`, categoryID, now, now)
	if err != nil {
		t.Fatalf("Failed to create status 1: %v", err)
	}
	statusID1, _ := statusResult1.LastInsertId()

	statusResult2, err := tdb.DB.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES ('WF In Progress', 'In progress status', ?, 0, ?, ?)
	`, categoryID, now, now)
	if err != nil {
		t.Fatalf("Failed to create status 2: %v", err)
	}
	statusID2, _ := statusResult2.LastInsertId()

	statusResult3, err := tdb.DB.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES ('WF Done', 'Done status', ?, 0, ?, ?)
	`, categoryID, now, now)
	if err != nil {
		t.Fatalf("Failed to create status 3: %v", err)
	}
	statusID3, _ := statusResult3.LastInsertId()

	// Create test workflow
	workflowResult, err := tdb.DB.Exec(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES ('Test Workflow', 'A test workflow', 0, ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}
	workflowID64, _ := workflowResult.LastInsertId()
	workflowID := int(workflowID64)

	// Create workflow transitions
	// Initial transition (from NULL to Open)
	_, err = tdb.DB.Exec(`
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order)
		VALUES (?, NULL, ?, 1)
	`, workflowID, statusID1)
	if err != nil {
		t.Fatalf("Failed to create initial transition: %v", err)
	}

	// Open -> In Progress
	_, err = tdb.DB.Exec(`
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order)
		VALUES (?, ?, ?, 2)
	`, workflowID, statusID1, statusID2)
	if err != nil {
		t.Fatalf("Failed to create transition 1->2: %v", err)
	}

	// In Progress -> Done
	_, err = tdb.DB.Exec(`
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order)
		VALUES (?, ?, ?, 3)
	`, workflowID, statusID2, statusID3)
	if err != nil {
		t.Fatalf("Failed to create transition 2->3: %v", err)
	}

	// Create a configuration set
	configSetResult, err := tdb.DB.Exec(`
		INSERT INTO configuration_sets (name, workflow_id, created_at, updated_at)
		VALUES ('Test Config Set', ?, ?, ?)
	`, workflowID, now, now)
	if err != nil {
		t.Fatalf("Failed to create configuration set: %v", err)
	}
	configSetID64, _ := configSetResult.LastInsertId()
	configSetID := int(configSetID64)

	// Associate workspace with configuration set
	_, err = tdb.DB.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id)
		VALUES (?, ?)
	`, workspaceID, configSetID)
	if err != nil {
		t.Fatalf("Failed to associate workspace with config set: %v", err)
	}

	// Create an item type
	var itemTypeID int
	err = tdb.DB.QueryRow("SELECT id FROM item_types LIMIT 1").Scan(&itemTypeID)
	if err != nil {
		// Create one if none exists
		itemTypeResult, err := tdb.DB.Exec(`
			INSERT INTO item_types (name, description, hierarchy_level, sort_order, is_default, created_at, updated_at)
			VALUES ('Test Type', 'Test item type', 0, 1, 0, ?, ?)
		`, now, now)
		if err != nil {
			t.Fatalf("Failed to create item type: %v", err)
		}
		itemTypeID64, _ := itemTypeResult.LastInsertId()
		itemTypeID = int(itemTypeID64)
	}

	return &WorkflowServiceTestData{
		WorkspaceID: workspaceID,
		WorkflowID:  workflowID,
		ConfigSetID: configSetID,
		StatusID1:   int(statusID1),
		StatusID2:   int(statusID2),
		StatusID3:   int(statusID3),
		CategoryID:  categoryID,
		ItemTypeID:  itemTypeID,
	}
}

func TestWorkflowService_GetWorkflowIDForItem(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("WithConfigSet", func(t *testing.T) {
		workflowID, err := service.GetWorkflowIDForItem(testData.WorkspaceID, nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflowID == nil {
			t.Fatal("Expected non-nil workflow ID")
		}
		if *workflowID != testData.WorkflowID {
			t.Errorf("Expected workflow ID %d, got %d", testData.WorkflowID, *workflowID)
		}
	})

	t.Run("WithItemType", func(t *testing.T) {
		itemTypeID := testData.ItemTypeID
		workflowID, err := service.GetWorkflowIDForItem(testData.WorkspaceID, &itemTypeID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should still return the config set workflow since no item type override is set
		if workflowID == nil {
			t.Fatal("Expected non-nil workflow ID")
		}
	})

	t.Run("FallbackToDefaultWorkflow", func(t *testing.T) {
		// Create a workspace without a config set
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES ('No Config Workspace', 'NCW', 'No config set', ?, ?)
		`, now, now)
		newWorkspaceID, _ := result.LastInsertId()

		// Mark an existing workflow as default
		tdb.DB.Exec("UPDATE workflows SET is_default = true WHERE id = ?", testData.WorkflowID)

		workflowID, err := service.GetWorkflowIDForItem(int(newWorkspaceID), nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflowID == nil {
			t.Fatal("Expected non-nil workflow ID (should fallback to default)")
		}
	})

	t.Run("NoWorkflowConfigured", func(t *testing.T) {
		// Create a workspace without a config set
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES ('Isolated Workspace', 'ISW', 'Isolated', ?, ?)
		`, now, now)
		newWorkspaceID, _ := result.LastInsertId()

		// Clear default workflow
		tdb.DB.Exec("UPDATE workflows SET is_default = false")

		workflowID, err := service.GetWorkflowIDForItem(int(newWorkspaceID), nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflowID != nil {
			t.Error("Expected nil workflow ID when nothing is configured")
		}
	})
}

func TestWorkflowService_IsValidStatusTransition(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("SameStatusAlwaysValid", func(t *testing.T) {
		valid, err := service.IsValidStatusTransition(testData.WorkspaceID, nil, int64(testData.StatusID1), int64(testData.StatusID1))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !valid {
			t.Error("Expected same status transition to be valid")
		}
	})

	t.Run("ValidTransition", func(t *testing.T) {
		valid, err := service.IsValidStatusTransition(testData.WorkspaceID, nil, int64(testData.StatusID1), int64(testData.StatusID2))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !valid {
			t.Error("Expected Open -> In Progress transition to be valid")
		}
	})

	t.Run("InvalidTransition", func(t *testing.T) {
		// Open -> Done should be invalid (no direct transition)
		valid, err := service.IsValidStatusTransition(testData.WorkspaceID, nil, int64(testData.StatusID1), int64(testData.StatusID3))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if valid {
			t.Error("Expected Open -> Done transition to be invalid")
		}
	})

	t.Run("NoWorkflowAllowsAnyTransition", func(t *testing.T) {
		// Create workspace without workflow
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES ('No Workflow Workspace', 'NWW', 'No workflow', ?, ?)
		`, now, now)
		newWorkspaceID, _ := result.LastInsertId()

		// Ensure no default workflow
		tdb.DB.Exec("UPDATE workflows SET is_default = false")

		valid, err := service.IsValidStatusTransition(int(newWorkspaceID), nil, int64(testData.StatusID1), int64(testData.StatusID3))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !valid {
			t.Error("Expected any transition to be valid when no workflow is configured")
		}
	})
}

func TestWorkflowService_GetAvailableTransitions(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("FromOpenStatus", func(t *testing.T) {
		transitions, err := service.GetAvailableTransitions(testData.WorkspaceID, nil, int64(testData.StatusID1))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(transitions) != 1 {
			t.Errorf("Expected 1 transition from Open, got %d", len(transitions))
		}

		if len(transitions) > 0 && transitions[0].ID != testData.StatusID2 {
			t.Errorf("Expected transition to In Progress (ID %d), got ID %d", testData.StatusID2, transitions[0].ID)
		}
	})

	t.Run("FromInProgressStatus", func(t *testing.T) {
		transitions, err := service.GetAvailableTransitions(testData.WorkspaceID, nil, int64(testData.StatusID2))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(transitions) != 1 {
			t.Errorf("Expected 1 transition from In Progress, got %d", len(transitions))
		}
	})

	t.Run("FromDoneStatus", func(t *testing.T) {
		transitions, err := service.GetAvailableTransitions(testData.WorkspaceID, nil, int64(testData.StatusID3))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// No transitions defined from Done status
		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions from Done, got %d", len(transitions))
		}
	})

	t.Run("NoWorkflowReturnsEmpty", func(t *testing.T) {
		// Create workspace without workflow
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO workspaces (name, key, description, created_at, updated_at)
			VALUES ('Empty Workflow Workspace', 'EWW', 'Empty', ?, ?)
		`, now, now)
		newWorkspaceID, _ := result.LastInsertId()

		// Ensure no default workflow
		tdb.DB.Exec("UPDATE workflows SET is_default = false")

		transitions, err := service.GetAvailableTransitions(int(newWorkspaceID), nil, int64(testData.StatusID1))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(transitions) != 0 {
			t.Errorf("Expected empty transitions when no workflow, got %d", len(transitions))
		}
	})
}

func TestWorkflowService_GetInitialStatusID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("ReturnsInitialStatus", func(t *testing.T) {
		statusID, err := service.GetInitialStatusID(testData.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if statusID == nil {
			t.Fatal("Expected non-nil initial status ID")
		}
		if *statusID != testData.StatusID1 {
			t.Errorf("Expected initial status ID %d, got %d", testData.StatusID1, *statusID)
		}
	})

	t.Run("WorkflowWithoutInitialStatus", func(t *testing.T) {
		// Create a workflow without initial status
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO workflows (name, description, is_default, created_at, updated_at)
			VALUES ('No Initial Workflow', 'No initial', 0, ?, ?)
		`, now, now)
		newWorkflowID, _ := result.LastInsertId()

		statusID, err := service.GetInitialStatusID(int(newWorkflowID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if statusID != nil {
			t.Errorf("Expected nil initial status ID, got %d", *statusID)
		}
	})
}

func TestWorkflowService_List(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	_ = setupWorkflowServiceTestData(t, tdb)

	t.Run("ReturnsWorkflows", func(t *testing.T) {
		workflows, err := service.List()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(workflows) == 0 {
			t.Error("Expected at least one workflow")
		}
	})
}

func TestWorkflowService_GetByID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("Success", func(t *testing.T) {
		workflow, err := service.GetByID(testData.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflow.ID != testData.WorkflowID {
			t.Errorf("Expected workflow ID %d, got %d", testData.WorkflowID, workflow.ID)
		}
		if workflow.Name != "Test Workflow" {
			t.Errorf("Expected workflow name 'Test Workflow', got '%s'", workflow.Name)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetByID(99999)
		if err == nil {
			t.Error("Expected error for non-existent workflow")
		}
	})
}

func TestWorkflowService_Exists(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("ExistingWorkflow", func(t *testing.T) {
		exists, err := service.Exists(testData.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !exists {
			t.Error("Expected workflow to exist")
		}
	})

	t.Run("NonExistentWorkflow", func(t *testing.T) {
		exists, err := service.Exists(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if exists {
			t.Error("Expected workflow to not exist")
		}
	})
}

func TestWorkflowService_GetTransitions(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("ReturnsAllTransitions", func(t *testing.T) {
		transitions, err := service.GetTransitions(testData.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// We created 3 transitions: initial, Open->InProgress, InProgress->Done
		if len(transitions) != 3 {
			t.Errorf("Expected 3 transitions, got %d", len(transitions))
		}
	})

	t.Run("EmptyForNoTransitions", func(t *testing.T) {
		// Create a workflow without transitions
		now := time.Now()
		result, _ := tdb.DB.Exec(`
			INSERT INTO workflows (name, description, is_default, created_at, updated_at)
			VALUES ('Empty Transitions Workflow', 'No transitions', 0, ?, ?)
		`, now, now)
		newWorkflowID, _ := result.LastInsertId()

		transitions, err := service.GetTransitions(int(newWorkflowID))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions, got %d", len(transitions))
		}
	})
}

func TestWorkflowService_GetTransitionsFromStatus(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	service := NewWorkflowService(tdb.GetDatabase())
	testData := setupWorkflowServiceTestData(t, tdb)

	t.Run("FromOpenStatus", func(t *testing.T) {
		transitions, err := service.GetTransitionsFromStatus(testData.StatusID1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should include transitions from Open and initial transitions (NULL)
		if len(transitions) < 1 {
			t.Error("Expected at least one transition from Open status")
		}
	})

	t.Run("FromDoneStatus", func(t *testing.T) {
		transitions, err := service.GetTransitionsFromStatus(testData.StatusID3)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should still include initial transitions (NULL -> any)
		// But no direct transitions from Done
		for _, tr := range transitions {
			if tr.FromStatusID != nil && *tr.FromStatusID == testData.StatusID3 {
				t.Errorf("Unexpected transition from Done status")
			}
		}
	})
}
