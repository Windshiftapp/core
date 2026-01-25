package services

import (
	"fmt"
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

// planningTestEnv contains test data for planning service tests
type planningTestEnv struct {
	MilestoneID int
	IterationID int
	ProjectID   int
	WorkspaceID int
}

// createPlanningTestDB creates a test database for planning service tests
func createPlanningTestDB(t *testing.T) database.Database {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "planning_test.db")
	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("failed to initialize test database: %v", err)
	}
	return db
}

// setupPlanningTestEnv creates test data for planning service tests
func setupPlanningTestEnv(t *testing.T, db database.Database) planningTestEnv {
	t.Helper()

	// Create workspace
	workspaceResult, err := db.Exec(`
		INSERT INTO workspaces (name, key, description, active, is_personal, created_at, updated_at)
		VALUES ('Planning Test Workspace', 'PLN', 'Test workspace', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	workspaceID64, _ := workspaceResult.LastInsertId()
	workspaceID := int(workspaceID64)

	// Create milestone
	milestoneResult, err := db.Exec(`
		INSERT INTO milestones (name, description, target_date, status, created_at, updated_at)
		VALUES ('Test Milestone', 'A test milestone', '2025-12-31', 'planning', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create milestone: %v", err)
	}
	milestoneID64, _ := milestoneResult.LastInsertId()
	milestoneID := int(milestoneID64)

	// Create iteration
	iterationResult, err := db.Exec(`
		INSERT INTO iterations (name, description, start_date, end_date, status, is_global, created_at, updated_at)
		VALUES ('Test Iteration', 'A test iteration', '2025-01-01', '2025-01-14', 'planned', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create iteration: %v", err)
	}
	iterationID64, _ := iterationResult.LastInsertId()
	iterationID := int(iterationID64)

	// Create project
	projectResult, err := db.Exec(`
		INSERT INTO projects (name, description, active, created_at, updated_at)
		VALUES ('Test Project', 'A test project', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	projectID64, _ := projectResult.LastInsertId()
	projectID := int(projectID64)

	return planningTestEnv{
		MilestoneID: milestoneID,
		IterationID: iterationID,
		ProjectID:   projectID,
		WorkspaceID: workspaceID,
	}
}

// Milestone Tests

func TestPlanningService_ListMilestones(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	_ = setupPlanningTestEnv(t, db)

	milestones, total, err := service.ListMilestones(MilestoneListParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(milestones) == 0 {
		t.Error("Expected at least one milestone")
	}
	if total == 0 {
		t.Error("Expected total to be at least 1")
	}
}

func TestPlanningService_GetMilestone(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		milestone, err := service.GetMilestone(env.MilestoneID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if milestone.ID != env.MilestoneID {
			t.Errorf("Expected milestone ID %d, got %d", env.MilestoneID, milestone.ID)
		}
		if milestone.Name != "Test Milestone" {
			t.Errorf("Expected milestone name 'Test Milestone', got '%s'", milestone.Name)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetMilestone(99999)
		if err == nil {
			t.Error("Expected error for non-existent milestone")
		}
	})
}

func TestPlanningService_CreateMilestone(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateMilestoneParams{
		Name:        "New Milestone",
		Description: "A new milestone",
		TargetDate:  "2025-06-30",
		Status:      "planning",
	}

	milestone, err := service.CreateMilestone(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if milestone.Name != "New Milestone" {
		t.Errorf("Expected name 'New Milestone', got '%s'", milestone.Name)
	}
	if milestone.Status != "planning" {
		t.Errorf("Expected status 'planning', got '%s'", milestone.Status)
	}
}

func TestPlanningService_UpdateMilestone(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	params := UpdateMilestoneParams{
		ID:          env.MilestoneID,
		Name:        "Updated Milestone",
		Description: "Updated description",
		TargetDate:  "2025-11-30",
		Status:      "active",
	}

	milestone, err := service.UpdateMilestone(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if milestone.Name != "Updated Milestone" {
		t.Errorf("Expected name 'Updated Milestone', got '%s'", milestone.Name)
	}
	if milestone.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", milestone.Status)
	}
}

func TestPlanningService_DeleteMilestone(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	err := service.DeleteMilestone(env.MilestoneID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify deletion
	_, err = service.GetMilestone(env.MilestoneID)
	if err == nil {
		t.Error("Expected error for deleted milestone")
	}
}

// Iteration Tests

func TestPlanningService_ListIterations(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	_ = setupPlanningTestEnv(t, db)

	iterations, total, err := service.ListIterations(IterationListParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(iterations) == 0 {
		t.Error("Expected at least one iteration")
	}
	if total == 0 {
		t.Error("Expected total to be at least 1")
	}
}

func TestPlanningService_GetIteration(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		iteration, err := service.GetIteration(env.IterationID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if iteration.ID != env.IterationID {
			t.Errorf("Expected iteration ID %d, got %d", env.IterationID, iteration.ID)
		}
		if iteration.Name != "Test Iteration" {
			t.Errorf("Expected iteration name 'Test Iteration', got '%s'", iteration.Name)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetIteration(99999)
		if err == nil {
			t.Error("Expected error for non-existent iteration")
		}
	})
}

func TestPlanningService_IsIterationGlobal(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	t.Run("GlobalIteration", func(t *testing.T) {
		isGlobal, err := service.IsIterationGlobal(env.IterationID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !isGlobal {
			t.Error("Expected iteration to be global")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.IsIterationGlobal(99999)
		if err == nil {
			t.Error("Expected error for non-existent iteration")
		}
	})
}

func TestPlanningService_CreateIteration(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateIterationParams{
		Name:        "New Iteration",
		Description: "A new iteration",
		StartDate:   "2025-02-01",
		EndDate:     "2025-02-14",
		Status:      "planned",
		IsGlobal:    true,
	}

	iteration, err := service.CreateIteration(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if iteration.Name != "New Iteration" {
		t.Errorf("Expected name 'New Iteration', got '%s'", iteration.Name)
	}
	if !iteration.IsGlobal {
		t.Error("Expected iteration to be global")
	}
}

func TestPlanningService_UpdateIteration(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	// Update with is_global = true to avoid constraint violation
	params := UpdateIterationParams{
		ID:          env.IterationID,
		Name:        "Updated Iteration",
		Description: "Updated description",
		StartDate:   "2025-01-15",
		EndDate:     "2025-01-28",
		Status:      "active",
		IsGlobal:    true, // Keep global to avoid workspace_id constraint
	}

	iteration, err := service.UpdateIteration(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if iteration.Name != "Updated Iteration" {
		t.Errorf("Expected name 'Updated Iteration', got '%s'", iteration.Name)
	}
}

func TestPlanningService_DeleteIteration(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	err := service.DeleteIteration(env.IterationID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify deletion
	_, err = service.GetIteration(env.IterationID)
	if err == nil {
		t.Error("Expected error for deleted iteration")
	}
}

// Project Tests

func TestPlanningService_ListProjects(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	_ = setupPlanningTestEnv(t, db)

	projects, total, err := service.ListProjects(ProjectListParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(projects) == 0 {
		t.Error("Expected at least one project")
	}
	if total == 0 {
		t.Error("Expected total to be at least 1")
	}
}

func TestPlanningService_GetProject(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	t.Run("Success", func(t *testing.T) {
		project, err := service.GetProject(env.ProjectID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if project.ID != env.ProjectID {
			t.Errorf("Expected project ID %d, got %d", env.ProjectID, project.ID)
		}
		if project.Name != "Test Project" {
			t.Errorf("Expected project name 'Test Project', got '%s'", project.Name)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetProject(99999)
		if err == nil {
			t.Error("Expected error for non-existent project")
		}
	})
}

func TestPlanningService_CreateProject(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateProjectParams{
		Name:        "New Project",
		Description: "A new project",
		Active:      true,
	}

	project, err := service.CreateProject(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if project.Name != "New Project" {
		t.Errorf("Expected name 'New Project', got '%s'", project.Name)
	}
	if !project.Active {
		t.Error("Expected project to be active")
	}
}

func TestPlanningService_UpdateProject(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	params := UpdateProjectParams{
		ID:          env.ProjectID,
		Name:        "Updated Project",
		Description: "Updated description",
		Active:      false,
	}

	project, err := service.UpdateProject(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if project.Name != "Updated Project" {
		t.Errorf("Expected name 'Updated Project', got '%s'", project.Name)
	}
	if project.Active {
		t.Error("Expected project to be inactive")
	}
}

func TestPlanningService_DeleteProject(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	err := service.DeleteProject(env.ProjectID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify deletion
	_, err = service.GetProject(env.ProjectID)
	if err == nil {
		t.Error("Expected error for deleted project")
	}
}

// Additional Edge Case Tests

func TestPlanningService_GetMilestone_ZeroID(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	_, err := service.GetMilestone(0)
	if err == nil {
		t.Error("Expected error for zero ID")
	}
}

func TestPlanningService_GetMilestone_NegativeID(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	_, err := service.GetMilestone(-1)
	if err == nil {
		t.Error("Expected error for negative ID")
	}
}

func TestPlanningService_GetIteration_ZeroID(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	_, err := service.GetIteration(0)
	if err == nil {
		t.Error("Expected error for zero ID")
	}
}

func TestPlanningService_GetProject_ZeroID(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	_, err := service.GetProject(0)
	if err == nil {
		t.Error("Expected error for zero ID")
	}
}

func TestPlanningService_UpdateMilestone_NonExistent(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := UpdateMilestoneParams{
		ID:          99999,
		Name:        "Non-existent",
		Description: "Description",
		TargetDate:  "2025-12-31",
		Status:      "planning",
	}

	_, err := service.UpdateMilestone(params)
	if err == nil {
		t.Error("Expected error for non-existent milestone")
	}
}

func TestPlanningService_UpdateIteration_NonExistent(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := UpdateIterationParams{
		ID:          99999,
		Name:        "Non-existent",
		Description: "Description",
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-14",
		Status:      "planned",
		IsGlobal:    true,
	}

	_, err := service.UpdateIteration(params)
	if err == nil {
		t.Error("Expected error for non-existent iteration")
	}
}

func TestPlanningService_UpdateProject_NonExistent(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := UpdateProjectParams{
		ID:          99999,
		Name:        "Non-existent",
		Description: "Description",
		Active:      true,
	}

	_, err := service.UpdateProject(params)
	if err == nil {
		t.Error("Expected error for non-existent project")
	}
}

func TestPlanningService_DeleteMilestone_NonExistent(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	// Should not error when deleting non-existent
	err := service.DeleteMilestone(99999)
	if err != nil {
		t.Errorf("Expected no error for non-existent milestone delete, got: %v", err)
	}
}

func TestPlanningService_DeleteIteration_NonExistent(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	// Should not error when deleting non-existent
	err := service.DeleteIteration(99999)
	if err != nil {
		t.Errorf("Expected no error for non-existent iteration delete, got: %v", err)
	}
}

func TestPlanningService_DeleteProject_NonExistent(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	// Should not error when deleting non-existent
	err := service.DeleteProject(99999)
	if err != nil {
		t.Errorf("Expected no error for non-existent project delete, got: %v", err)
	}
}

func TestPlanningService_CreateMilestone_EmptyName(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateMilestoneParams{
		Name:        "",
		Description: "Description",
		TargetDate:  "2025-12-31",
		Status:      "planning",
	}

	// The service doesn't validate empty name, but the database might
	milestone, err := service.CreateMilestone(params)
	if err == nil {
		// If no error, name should be empty
		if milestone.Name != "" {
			t.Errorf("Expected empty name, got '%s'", milestone.Name)
		}
	}
	// Either way, this tests the behavior
}

func TestPlanningService_CreateIteration_EmptyName(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateIterationParams{
		Name:        "",
		Description: "Description",
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-14",
		Status:      "planned",
		IsGlobal:    true,
	}

	iteration, err := service.CreateIteration(params)
	if err == nil {
		// If no error, name should be empty
		if iteration.Name != "" {
			t.Errorf("Expected empty name, got '%s'", iteration.Name)
		}
	}
}

func TestPlanningService_CreateProject_EmptyName(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateProjectParams{
		Name:        "",
		Description: "Description",
		Active:      true,
	}

	project, err := service.CreateProject(params)
	if err == nil {
		// If no error, name should be empty
		if project.Name != "" {
			t.Errorf("Expected empty name, got '%s'", project.Name)
		}
	}
}

func TestPlanningService_CreateMilestone_DefaultStatus(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateMilestoneParams{
		Name:        "Test Milestone Default Status",
		Description: "Description",
		TargetDate:  "2025-12-31",
		Status:      "", // Empty status should default to "planning"
	}

	milestone, err := service.CreateMilestone(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if milestone.Status != "planning" {
		t.Errorf("Expected default status 'planning', got '%s'", milestone.Status)
	}
}

func TestPlanningService_CreateIteration_DefaultStatus(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	params := CreateIterationParams{
		Name:        "Test Iteration Default Status",
		Description: "Description",
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-14",
		Status:      "", // Empty status should default to "planned"
		IsGlobal:    true,
	}

	iteration, err := service.CreateIteration(params)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if iteration.Status != "planned" {
		t.Errorf("Expected default status 'planned', got '%s'", iteration.Status)
	}
}

func TestPlanningService_ListMilestones_Pagination(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	// Create multiple milestones
	for i := 0; i < 5; i++ {
		_, err := service.CreateMilestone(CreateMilestoneParams{
			Name:       fmt.Sprintf("Milestone %d", i),
			TargetDate: "2025-12-31",
			Status:     "planning",
		})
		if err != nil {
			t.Fatalf("Failed to create milestone: %v", err)
		}
	}

	t.Run("LimitWorks", func(t *testing.T) {
		milestones, total, err := service.ListMilestones(MilestoneListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(milestones) != 2 {
			t.Errorf("Expected 2 milestones, got %d", len(milestones))
		}
		if total < 5 {
			t.Errorf("Expected total >= 5, got %d", total)
		}
	})

	t.Run("OffsetWorks", func(t *testing.T) {
		milestones, _, err := service.ListMilestones(MilestoneListParams{Limit: 100, Offset: 3})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should skip first 3
		if len(milestones) < 2 {
			t.Errorf("Expected at least 2 milestones after offset 3, got %d", len(milestones))
		}
	})
}

func TestPlanningService_ListIterations_Pagination(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	// Create multiple iterations
	for i := 0; i < 5; i++ {
		_, err := service.CreateIteration(CreateIterationParams{
			Name:      fmt.Sprintf("Iteration %d", i),
			StartDate: fmt.Sprintf("2025-0%d-01", i+1),
			EndDate:   fmt.Sprintf("2025-0%d-14", i+1),
			Status:    "planned",
			IsGlobal:  true,
		})
		if err != nil {
			t.Fatalf("Failed to create iteration: %v", err)
		}
	}

	t.Run("LimitWorks", func(t *testing.T) {
		iterations, total, err := service.ListIterations(IterationListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(iterations) != 2 {
			t.Errorf("Expected 2 iterations, got %d", len(iterations))
		}
		if total < 5 {
			t.Errorf("Expected total >= 5, got %d", total)
		}
	})
}

func TestPlanningService_ListProjects_Pagination(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)

	// Create multiple projects
	for i := 0; i < 5; i++ {
		_, err := service.CreateProject(CreateProjectParams{
			Name:   fmt.Sprintf("Project %d", i),
			Active: true,
		})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}
	}

	t.Run("LimitWorks", func(t *testing.T) {
		projects, total, err := service.ListProjects(ProjectListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(projects) != 2 {
			t.Errorf("Expected 2 projects, got %d", len(projects))
		}
		if total < 5 {
			t.Errorf("Expected total >= 5, got %d", total)
		}
	})
}

func TestPlanningService_ListMilestones_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	// Delete all milestones
	db.Exec("DELETE FROM milestones")

	service := NewPlanningService(db)

	milestones, _, err := service.ListMilestones(MilestoneListParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if milestones == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestPlanningService_ListIterations_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	// Delete all iterations
	db.Exec("DELETE FROM iterations")

	service := NewPlanningService(db)

	iterations, _, err := service.ListIterations(IterationListParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if iterations == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestPlanningService_ListProjects_ReturnsEmptySliceNotNil(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	// Delete all projects
	db.Exec("DELETE FROM projects")

	service := NewPlanningService(db)

	projects, _, err := service.ListProjects(ProjectListParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if projects == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestPlanningService_GetMilestone_PopulatesAllFields(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	milestone, err := service.GetMilestone(env.MilestoneID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if milestone.ID == 0 {
		t.Error("Expected ID to be populated")
	}
	if milestone.Name == "" {
		t.Error("Expected Name to be populated")
	}
	if milestone.Status == "" {
		t.Error("Expected Status to be populated")
	}
	if milestone.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be populated")
	}
	if milestone.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be populated")
	}
}

func TestPlanningService_GetIteration_PopulatesAllFields(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	iteration, err := service.GetIteration(env.IterationID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if iteration.ID == 0 {
		t.Error("Expected ID to be populated")
	}
	if iteration.Name == "" {
		t.Error("Expected Name to be populated")
	}
	if iteration.Status == "" {
		t.Error("Expected Status to be populated")
	}
	if iteration.StartDate == "" {
		t.Error("Expected StartDate to be populated")
	}
	if iteration.EndDate == "" {
		t.Error("Expected EndDate to be populated")
	}
	if iteration.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be populated")
	}
}

func TestPlanningService_GetProject_PopulatesAllFields(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	project, err := service.GetProject(env.ProjectID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if project.ID == 0 {
		t.Error("Expected ID to be populated")
	}
	if project.Name == "" {
		t.Error("Expected Name to be populated")
	}
	if project.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be populated")
	}
	if project.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be populated")
	}
}

func TestPlanningService_IsIterationGlobal_True(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	isGlobal, err := service.IsIterationGlobal(env.IterationID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !isGlobal {
		t.Error("Expected iteration to be global")
	}
}

func TestPlanningService_IsIterationGlobal_False(t *testing.T) {
	db := createPlanningTestDB(t)
	defer db.Close()

	service := NewPlanningService(db)
	env := setupPlanningTestEnv(t, db)

	// Create non-global iteration
	iteration, err := service.CreateIteration(CreateIterationParams{
		Name:        "Non-Global Iteration",
		StartDate:   "2025-02-01",
		EndDate:     "2025-02-14",
		Status:      "planned",
		IsGlobal:    false,
		WorkspaceID: &env.WorkspaceID,
	})
	if err != nil {
		t.Fatalf("Failed to create iteration: %v", err)
	}

	isGlobal, err := service.IsIterationGlobal(iteration.ID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if isGlobal {
		t.Error("Expected iteration to not be global")
	}
}
