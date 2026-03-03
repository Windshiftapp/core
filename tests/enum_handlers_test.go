package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Status Category Tests (Error Cases - Success cases in workflow_test.go)
// ============================================================================

func TestStatusCategoryErrorCases(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()

	// Create a category first for duplicate testing
	categoryData := map[string]interface{}{
		"name":  fmt.Sprintf("Test Category %d", timestamp),
		"color": "#3b82f6",
	}
	resp := MakeAuthRequest(t, server, http.MethodPost, "/status-categories", categoryData)
	resp.Body.Close()

	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"color": "#3b82f6",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/status-categories", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateMissingColor", func(t *testing.T) {
		data := map[string]interface{}{
			"name": fmt.Sprintf("Missing Color %d", timestamp),
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/status-categories", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateName", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  fmt.Sprintf("Test Category %d", timestamp),
			"color": "#10b981",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/status-categories", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/status-categories/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Updated Name",
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/status-categories/99999", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/status-categories/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/status-categories", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})
}

// ============================================================================
// Status Tests (Error Cases - Success cases in workflow_test.go)
// ============================================================================

func TestStatusErrorCases(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	// Create a category first
	categoryIDs := CreateTestStatusCategories(t, server, "StatusErr")
	timestamp := time.Now().Unix()

	// Create a status for duplicate testing
	statusData := map[string]interface{}{
		"name":        fmt.Sprintf("Test Status %d", timestamp),
		"category_id": categoryIDs[0],
	}
	resp := MakeAuthRequest(t, server, http.MethodPost, "/statuses", statusData)
	resp.Body.Close()

	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"category_id": categoryIDs[0],
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/statuses", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateMissingCategoryID", func(t *testing.T) {
		data := map[string]interface{}{
			"name": fmt.Sprintf("Missing Category %d", timestamp),
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/statuses", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateInvalidCategoryID", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Invalid Category %d", timestamp),
			"category_id": 99999,
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/statuses", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateName", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Test Status %d", timestamp),
			"category_id": categoryIDs[0],
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/statuses", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/statuses/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        "Updated Name",
			"category_id": categoryIDs[0],
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/statuses/99999", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/statuses/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/statuses", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})
}

// ============================================================================
// Iteration Type Tests (Full CRUD + Error Cases)
// ============================================================================

func TestIterationTypeOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()
	var iterationTypeID int

	// Success Cases
	t.Run("Create", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Sprint %d", timestamp),
			"color":       "#3b82f6",
			"description": "A sprint iteration type",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/iteration-types", data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Sprint %d", timestamp))
		AssertJSONField(t, result, "color", "#3b82f6")

		iterationTypeID = ExtractIDFromResponse(t, result)

		// Verify timestamps are present
		if _, ok := result["created_at"]; !ok {
			t.Error("Expected created_at in response")
		}
		if _, ok := result["updated_at"]; !ok {
			t.Error("Expected updated_at in response")
		}
	})

	t.Run("GetAll", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/iteration-types", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var iterationTypes []map[string]interface{}
		DecodeJSON(t, resp, &iterationTypes)

		if len(iterationTypes) == 0 {
			t.Error("Expected at least one iteration type")
		}

		// Verify it's an array, not null
		if iterationTypes == nil {
			t.Error("Expected array, got null")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/iteration-types/%d", iterationTypeID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Sprint %d", timestamp))
	})

	t.Run("Update", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Updated Sprint %d", timestamp),
			"color":       "#10b981",
			"description": "Updated description",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/iteration-types/%d", iterationTypeID), data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Updated Sprint %d", timestamp))
		AssertJSONField(t, result, "color", "#10b981")
	})

	// Error Cases
	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"color": "#3b82f6",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/iteration-types", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateMissingColor", func(t *testing.T) {
		data := map[string]interface{}{
			"name": fmt.Sprintf("No Color %d", timestamp),
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/iteration-types", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateName", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  fmt.Sprintf("Updated Sprint %d", timestamp),
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/iteration-types", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/iteration-types/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Updated Name",
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/iteration-types/99999", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/iteration-types/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/iteration-types", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	// Cleanup
	t.Run("Delete", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/iteration-types/%d", iterationTypeID), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNoContent)
	})
}

// ============================================================================
// Milestone Category Tests (Full CRUD + Error Cases)
// ============================================================================

func TestMilestoneCategoryOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()
	var milestoneCategoryID int

	// Success Cases
	t.Run("Create", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Release %d", timestamp),
			"color":       "#8b5cf6",
			"description": "A release milestone category",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/milestone-categories", data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Release %d", timestamp))

		milestoneCategoryID = ExtractIDFromResponse(t, result)

		// Verify timestamps
		if _, ok := result["created_at"]; !ok {
			t.Error("Expected created_at in response")
		}
	})

	t.Run("CreateWithDefaultColor", func(t *testing.T) {
		data := map[string]interface{}{
			"name": fmt.Sprintf("Default Color %d", timestamp),
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/milestone-categories", data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		// Should default to blue
		color, ok := result["color"].(string)
		if !ok || color == "" {
			t.Error("Expected default color to be set")
		}
	})

	t.Run("GetAll", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/milestone-categories", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var categories []map[string]interface{}
		DecodeJSON(t, resp, &categories)

		if len(categories) == 0 {
			t.Error("Expected at least one milestone category")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/milestone-categories/%d", milestoneCategoryID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Release %d", timestamp))
	})

	t.Run("Update", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Updated Release %d", timestamp),
			"color":       "#ec4899",
			"description": "Updated description",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/milestone-categories/%d", milestoneCategoryID), data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Updated Release %d", timestamp))
		AssertJSONField(t, result, "color", "#ec4899")
	})

	// Error Cases
	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"color": "#3b82f6",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/milestone-categories", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateName", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  fmt.Sprintf("Updated Release %d", timestamp),
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/milestone-categories", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("CreateDuplicateNameCaseInsensitive", func(t *testing.T) {
		// MilestoneCategory uses case-insensitive uniqueness
		data := map[string]interface{}{
			"name":  strings.ToUpper(fmt.Sprintf("Updated Release %d", timestamp)),
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/milestone-categories", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/milestone-categories/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Updated Name",
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/milestone-categories/99999", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/milestone-categories/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/milestone-categories", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	// Cleanup
	t.Run("Delete", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/milestone-categories/%d", milestoneCategoryID), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNoContent)
	})
}

// ============================================================================
// Priority Tests (Full CRUD + Error Cases)
// ============================================================================

func TestPriorityOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()
	var priorityID int

	// Success Cases
	t.Run("Create", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Critical %d", timestamp),
			"color":       "#ef4444",
			"icon":        "alert-triangle",
			"description": "Critical priority",
			"sort_order":  1,
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/priorities", data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Critical %d", timestamp))
		AssertJSONField(t, result, "color", "#ef4444")
		AssertJSONField(t, result, "icon", "alert-triangle")

		priorityID = ExtractIDFromResponse(t, result)

		// Verify timestamps
		if _, ok := result["created_at"]; !ok {
			t.Error("Expected created_at in response")
		}
	})

	t.Run("GetAll", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/priorities", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var priorities []map[string]interface{}
		DecodeJSON(t, resp, &priorities)

		if len(priorities) == 0 {
			t.Error("Expected at least one priority")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/priorities/%d", priorityID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Critical %d", timestamp))
	})

	t.Run("Update", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Updated Critical %d", timestamp),
			"color":       "#dc2626",
			"icon":        "alert-circle",
			"description": "Updated critical priority",
			"sort_order":  2,
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/priorities/%d", priorityID), data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Updated Critical %d", timestamp))
		AssertJSONField(t, result, "icon", "alert-circle")
	})

	// Error Cases
	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"color": "#3b82f6",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/priorities", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateName", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  fmt.Sprintf("Updated Critical %d", timestamp),
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/priorities", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/priorities/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Updated Name",
			"color": "#6b7280",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/priorities/99999", data)
		defer resp.Body.Close()
		// Note: Current behavior returns 500 instead of 404 - this is a known issue to fix in refactoring
		AssertStatusCode(t, resp, http.StatusInternalServerError)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/priorities/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/priorities", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	// Cleanup
	t.Run("Delete", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/priorities/%d", priorityID), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNoContent)
	})
}

// ============================================================================
// Hierarchy Level Tests (Full CRUD + Error Cases)
// ============================================================================

func TestHierarchyLevelOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()
	var hierarchyLevelID int

	// Note: Levels 0-4 are pre-seeded (Initiative, Epic, Story, Task, Sub-task)
	// Use level 10+ for testing to avoid conflicts

	// Success Cases
	t.Run("Create", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Custom Level %d", timestamp),
			"level":       10, // Use high level number to avoid conflict with seeded levels
			"description": "Custom hierarchy level",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/hierarchy-levels", data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Custom Level %d", timestamp))
		if level, ok := result["level"].(float64); !ok || int(level) != 10 {
			t.Errorf("Expected level 10, got %v", result["level"])
		}

		hierarchyLevelID = ExtractIDFromResponse(t, result)

		// Verify timestamps
		if _, ok := result["created_at"]; !ok {
			t.Error("Expected created_at in response")
		}
	})

	t.Run("GetAll", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/hierarchy-levels", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var levels []map[string]interface{}
		DecodeJSON(t, resp, &levels)

		if len(levels) == 0 {
			t.Error("Expected at least one hierarchy level")
		}

		// Verify ordering by level
		for i := 1; i < len(levels); i++ {
			prevLevel := levels[i-1]["level"].(float64)
			currLevel := levels[i]["level"].(float64)
			if currLevel < prevLevel {
				t.Error("Hierarchy levels should be ordered by level")
			}
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/hierarchy-levels/%d", hierarchyLevelID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Custom Level %d", timestamp))
	})

	t.Run("Update", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Updated Level %d", timestamp),
			"level":       11, // Use different level to avoid conflict
			"description": "Updated hierarchy level",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/hierarchy-levels/%d", hierarchyLevelID), data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Updated Level %d", timestamp))
	})

	// Error Cases
	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"level": 12,
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/hierarchy-levels", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateLevel", func(t *testing.T) {
		// Hierarchy levels have a unique constraint on `level`, not name
		data := map[string]interface{}{
			"name":  fmt.Sprintf("Another Level %d", timestamp),
			"level": 11, // Same level as the updated one above
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/hierarchy-levels", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/hierarchy-levels/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name":  "Updated Name",
			"level": 20,
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/hierarchy-levels/99999", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/hierarchy-levels/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/hierarchy-levels", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	// Cleanup
	t.Run("Delete", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/hierarchy-levels/%d", hierarchyLevelID), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNoContent)
	})
}

// ============================================================================
// Contact Role Tests (Full CRUD + Error Cases)
// ============================================================================

func TestContactRoleOperations(t *testing.T) {
	server, _ := StartTestServer(t, GetDBType())
	CreateBearerToken(t, server)

	timestamp := time.Now().Unix()
	var contactRoleID int

	// Success Cases
	t.Run("Create", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Technical Lead %d", timestamp),
			"description": "Technical lead contact role",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/contact-roles", data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Technical Lead %d", timestamp))

		contactRoleID = ExtractIDFromResponse(t, result)

		// User-created roles should not be system roles
		if isSystem, ok := result["is_system"].(bool); ok && isSystem {
			t.Error("User-created role should not be a system role")
		}

		// Verify timestamps
		if _, ok := result["created_at"]; !ok {
			t.Error("Expected created_at in response")
		}
	})

	t.Run("GetAll", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/contact-roles", nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var roles []map[string]interface{}
		DecodeJSON(t, resp, &roles)

		if len(roles) == 0 {
			t.Error("Expected at least one contact role")
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/contact-roles/%d", contactRoleID), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Technical Lead %d", timestamp))
	})

	t.Run("Update", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        fmt.Sprintf("Updated Tech Lead %d", timestamp),
			"description": "Updated description",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, fmt.Sprintf("/contact-roles/%d", contactRoleID), data)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		AssertJSONField(t, result, "name", fmt.Sprintf("Updated Tech Lead %d", timestamp))
	})

	// Error Cases
	t.Run("CreateMissingName", func(t *testing.T) {
		data := map[string]interface{}{
			"description": "Missing name",
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/contact-roles", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	t.Run("CreateDuplicateName", func(t *testing.T) {
		data := map[string]interface{}{
			"name": fmt.Sprintf("Updated Tech Lead %d", timestamp),
		}
		resp := MakeAuthRequest(t, server, http.MethodPost, "/contact-roles", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusConflict)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodGet, "/contact-roles/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		data := map[string]interface{}{
			"name": "Updated Name",
		}
		resp := MakeAuthRequest(t, server, http.MethodPut, "/contact-roles/99999", data)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, "/contact-roles/99999", nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNotFound)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		resp := MakeAuthRequestRaw(t, server, http.MethodPost, "/contact-roles", "not valid json")
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusBadRequest)
	})

	// Cleanup
	t.Run("Delete", func(t *testing.T) {
		resp := MakeAuthRequest(t, server, http.MethodDelete, fmt.Sprintf("/contact-roles/%d", contactRoleID), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusNoContent)
	})
}
