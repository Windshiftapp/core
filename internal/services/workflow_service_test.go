package services

import (
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

// workflowTestEnv contains test data for workflow service tests
type workflowTestEnv struct {
	WorkspaceID int
	WorkflowID  int
	ConfigSetID int
	StatusID1   int
	StatusID2   int
	StatusID3   int
	CategoryID  int
}

// createWorkflowTestDB creates a test database for workflow service tests
func createWorkflowTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workflow_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

// setupWorkflowTestEnv creates test data for workflow service tests
func setupWorkflowTestEnv(t *testing.T, db database.Database) workflowTestEnv {
	t.Helper()

	// Create workspace
	workspaceResult, err := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('Workflow Test Workspace', 'WFL', 'Test workspace', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := workspaceResult.LastInsertId()
	workspaceID := int(workspaceID64)

	// Get existing status category
	var categoryID int
	err = db.QueryRow("SELECT id FROM status_categories LIMIT 1").Scan(&categoryID)
	if err != nil {
		t.Fatalf("Failed to get status category: %v", err)
	}

	// Create test statuses
	statusResult1, err := db.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES ('WF Open', 'Open status', ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, categoryID)
	if err != nil {
		t.Fatalf("Failed to create status 1: %v", err)
	}
	statusID1, _ := statusResult1.LastInsertId()

	statusResult2, err := db.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES ('WF In Progress', 'In progress status', ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, categoryID)
	if err != nil {
		t.Fatalf("Failed to create status 2: %v", err)
	}
	statusID2, _ := statusResult2.LastInsertId()

	statusResult3, err := db.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default, created_at, updated_at)
		VALUES ('WF Done', 'Done status', ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, categoryID)
	if err != nil {
		t.Fatalf("Failed to create status 3: %v", err)
	}
	statusID3, _ := statusResult3.LastInsertId()

	// Create test workflow
	workflowResult, err := db.Exec(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES ('Test Workflow', 'A test workflow', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}
	workflowID64, _ := workflowResult.LastInsertId()
	workflowID := int(workflowID64)

	// Create workflow transitions
	// Initial transition (from NULL to Open)
	_, err = db.Exec(`
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order)
		VALUES (?, NULL, ?, 1)
	`, workflowID, statusID1)
	if err != nil {
		t.Fatalf("Failed to create initial transition: %v", err)
	}

	// Open -> In Progress
	_, err = db.Exec(`
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order)
		VALUES (?, ?, ?, 2)
	`, workflowID, statusID1, statusID2)
	if err != nil {
		t.Fatalf("Failed to create transition 1->2: %v", err)
	}

	// In Progress -> Done
	_, err = db.Exec(`
		INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order)
		VALUES (?, ?, ?, 3)
	`, workflowID, statusID2, statusID3)
	if err != nil {
		t.Fatalf("Failed to create transition 2->3: %v", err)
	}

	// Create a configuration set
	configSetResult, err := db.Exec(`
		INSERT INTO configuration_sets (name, workflow_id, created_at, updated_at)
		VALUES ('Test Config Set', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workflowID)
	if err != nil {
		t.Fatalf("Failed to create configuration set: %v", err)
	}
	configSetID64, _ := configSetResult.LastInsertId()
	configSetID := int(configSetID64)

	// Associate workspace with configuration set
	_, err = db.Exec(`
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id)
		VALUES (?, ?)
	`, workspaceID, configSetID)
	if err != nil {
		t.Fatalf("Failed to associate workspace with config set: %v", err)
	}

	return workflowTestEnv{
		WorkspaceID: workspaceID,
		WorkflowID:  workflowID,
		ConfigSetID: configSetID,
		StatusID1:   int(statusID1),
		StatusID2:   int(statusID2),
		StatusID3:   int(statusID3),
		CategoryID:  categoryID,
	}
}

func TestWorkflowService_GetWorkflowIDForItem(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("WithConfigSet", func(t *testing.T) {
		workflowID, err := service.GetWorkflowIDForItem(env.WorkspaceID, nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflowID == nil {
			t.Fatal("Expected non-nil workflow ID")
		}
		if *workflowID != env.WorkflowID {
			t.Errorf("Expected workflow ID %d, got %d", env.WorkflowID, *workflowID)
		}
	})

	t.Run("FallbackToDefaultWorkflow", func(t *testing.T) {
		// Create a workspace without a config set
		result, _ := db.Exec(`
			INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
			VALUES ('No Config Workspace', 'NCW', 'No config set', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
		newWorkspaceID, _ := result.LastInsertId()

		// Mark workflow as default
		db.Exec("UPDATE workflows SET is_default = true WHERE id = ?", env.WorkflowID)

		workflowID, err := service.GetWorkflowIDForItem(int(newWorkspaceID), nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflowID == nil {
			t.Fatal("Expected non-nil workflow ID (should fallback to default)")
		}
	})
}

func TestWorkflowService_IsValidStatusTransition(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("SameStatusAlwaysValid", func(t *testing.T) {
		valid, err := service.IsValidStatusTransition(env.WorkspaceID, nil, int64(env.StatusID1), int64(env.StatusID1))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !valid {
			t.Error("Expected same status transition to be valid")
		}
	})

	t.Run("ValidTransition", func(t *testing.T) {
		valid, err := service.IsValidStatusTransition(env.WorkspaceID, nil, int64(env.StatusID1), int64(env.StatusID2))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !valid {
			t.Error("Expected Open -> In Progress transition to be valid")
		}
	})

	t.Run("InvalidTransition", func(t *testing.T) {
		valid, err := service.IsValidStatusTransition(env.WorkspaceID, nil, int64(env.StatusID1), int64(env.StatusID3))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if valid {
			t.Error("Expected Open -> Done transition to be invalid")
		}
	})
}

func TestWorkflowService_GetAvailableTransitions(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("FromOpenStatus", func(t *testing.T) {
		transitions, err := service.GetAvailableTransitions(env.WorkspaceID, nil, int64(env.StatusID1))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(transitions) != 1 {
			t.Errorf("Expected 1 transition from Open, got %d", len(transitions))
		}
	})

	t.Run("FromDoneStatus", func(t *testing.T) {
		transitions, err := service.GetAvailableTransitions(env.WorkspaceID, nil, int64(env.StatusID3))
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(transitions) != 0 {
			t.Errorf("Expected 0 transitions from Done, got %d", len(transitions))
		}
	})
}

func TestWorkflowService_GetInitialStatusID(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("ReturnsInitialStatus", func(t *testing.T) {
		statusID, err := service.GetInitialStatusID(env.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if statusID == nil {
			t.Fatal("Expected non-nil initial status ID")
		}
		if *statusID != env.StatusID1 {
			t.Errorf("Expected initial status ID %d, got %d", env.StatusID1, *statusID)
		}
	})

	t.Run("WorkflowWithoutInitialStatus", func(t *testing.T) {
		result, _ := db.Exec(`
			INSERT INTO workflows (name, description, is_default, created_at, updated_at)
			VALUES ('No Initial Workflow', 'No initial', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
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
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	_ = setupWorkflowTestEnv(t, db)

	workflows, err := service.List()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(workflows) == 0 {
		t.Error("Expected at least one workflow")
	}
}

func TestWorkflowService_GetByID(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		workflow, err := service.GetByID(env.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if workflow.ID != env.WorkflowID {
			t.Errorf("Expected workflow ID %d, got %d", env.WorkflowID, workflow.ID)
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
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("ExistingWorkflow", func(t *testing.T) {
		exists, err := service.Exists(env.WorkflowID)
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
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("ReturnsAllTransitions", func(t *testing.T) {
		transitions, err := service.GetTransitions(env.WorkflowID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// We created 3 transitions: initial, Open->InProgress, InProgress->Done
		if len(transitions) != 3 {
			t.Errorf("Expected 3 transitions, got %d", len(transitions))
		}
	})

	t.Run("EmptyForNoTransitions", func(t *testing.T) {
		result, _ := db.Exec(`
			INSERT INTO workflows (name, description, is_default, created_at, updated_at)
			VALUES ('Empty Transitions Workflow', 'No transitions', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`)
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
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)
	env := setupWorkflowTestEnv(t, db)

	t.Run("ReturnsTransitionsFromOpenStatus", func(t *testing.T) {
		transitions, err := service.GetTransitionsFromStatus(env.StatusID1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should return transitions from status 1 and initial (NULL) transitions
		// Status 1 -> Status 2, plus initial -> Status 1
		if len(transitions) < 1 {
			t.Errorf("Expected at least 1 transition, got %d", len(transitions))
		}
	})

	t.Run("ReturnsEmptyForFinalStatus", func(t *testing.T) {
		transitions, err := service.GetTransitionsFromStatus(env.StatusID3)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Done status has no outgoing transitions (except initial which is included)
		// So we check that there are no transitions where from_status_id = StatusID3
		foundFromDone := false
		for _, t := range transitions {
			if t.FromStatusID != nil && *t.FromStatusID == env.StatusID3 {
				foundFromDone = true
			}
		}
		if foundFromDone {
			t.Error("Expected no transitions originating from Done status")
		}
	})

	t.Run("IncludesInitialTransitions", func(t *testing.T) {
		transitions, err := service.GetTransitionsFromStatus(env.StatusID1)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should include transitions with NULL from_status_id (initial transitions)
		foundInitial := false
		for _, t := range transitions {
			if t.FromStatusID == nil {
				foundInitial = true
			}
		}
		if !foundInitial {
			t.Error("Expected to find initial transition (from_status_id is NULL)")
		}
	})

	t.Run("NonExistentStatus", func(t *testing.T) {
		transitions, err := service.GetTransitionsFromStatus(99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should still return initial transitions
		if transitions == nil {
			t.Error("Expected non-nil result")
		}
	})
}

func TestWorkflowService_GetWorkflowIDForItem_NoWorkspace(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	// Try to get workflow for non-existent workspace
	workflowID, err := service.GetWorkflowIDForItem(99999, nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return nil or fall back to default workflow
	// (depending on whether default workflow exists)
	_ = workflowID // Result depends on whether default workflow exists
}

func TestWorkflowService_IsValidStatusTransition_NoWorkflow(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	// Create workspace without config set
	result, _ := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('No Workflow Workspace', 'NWW', 'No workflow', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	workspaceID, _ := result.LastInsertId()

	// Remove default workflow
	db.Exec("UPDATE workflows SET is_default = false")

	// Without any workflow, any transition should be allowed
	valid, err := service.IsValidStatusTransition(int(workspaceID), nil, 1, 2)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !valid {
		t.Error("Expected transition to be valid when no workflow is configured")
	}
}

func TestWorkflowService_GetAvailableTransitions_NoWorkflow(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	// Create workspace without config set
	result, _ := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('No Workflow Workspace 2', 'NW2', 'No workflow', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	workspaceID, _ := result.LastInsertId()

	// Remove default workflow
	db.Exec("UPDATE workflows SET is_default = false")

	transitions, err := service.GetAvailableTransitions(int(workspaceID), nil, 1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return empty slice when no workflow configured
	if transitions == nil {
		t.Error("Expected empty slice, not nil")
	}
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions, got %d", len(transitions))
	}
}

func TestWorkflowService_GetInitialStatusID_NonExistentWorkflow(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	statusID, err := service.GetInitialStatusID(99999)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if statusID != nil {
		t.Errorf("Expected nil initial status for non-existent workflow, got %d", *statusID)
	}
}

func TestWorkflowService_GetByID_ZeroID(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	_, err := service.GetByID(0)
	if err == nil {
		t.Error("Expected error for zero ID")
	}
}

func TestWorkflowService_GetByID_NegativeID(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	_, err := service.GetByID(-1)
	if err == nil {
		t.Error("Expected error for negative ID")
	}
}

func TestWorkflowService_Exists_ZeroID(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	exists, err := service.Exists(0)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if exists {
		t.Error("Expected false for zero ID")
	}
}

func TestWorkflowService_GetTransitions_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	service := NewWorkflowService(db)

	// Create workflow without transitions
	result, _ := db.Exec(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES ('Empty Workflow', 'No transitions', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	workflowID, _ := result.LastInsertId()

	transitions, err := service.GetTransitions(int(workflowID))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if transitions == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestWorkflowService_List_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	// Delete all workflows
	db.Exec("DELETE FROM workflow_transitions")
	db.Exec("DELETE FROM configuration_sets")
	db.Exec("DELETE FROM workflows")

	service := NewWorkflowService(db)

	workflows, err := service.List()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if workflows == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestWorkflowService_GetTransitionsFromStatus_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createWorkflowTestDB(t)
	defer db.Close()

	// Delete all transitions
	db.Exec("DELETE FROM workflow_transitions")

	service := NewWorkflowService(db)

	transitions, err := service.GetTransitionsFromStatus(1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if transitions == nil {
		t.Error("Expected empty slice, got nil")
	}
}
