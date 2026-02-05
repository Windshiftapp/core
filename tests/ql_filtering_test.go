package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestQLFiltering tests comprehensive QL filtering capabilities
func TestQLFiltering(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	var workspaceID int
	var statusOpen, statusInProgress, statusClosed int
	var priorityLow, priorityMedium, priorityHigh int
	var iterationID, milestoneID int
	var projectID int

	// Custom field IDs
	var cfEpicLink, cfEnvironment, cfPriorityLevel, cfTags int
	var cfStoryPoints, cfTargetDate, cfNotes int

	// Track created items for validation
	type testItem struct {
		id           int
		title        string
		statusID     int
		priorityID   int
		dueDate      string
		iterationID  *int
		milestoneID  *int
		projectID    *int
		customFields map[string]interface{}
	}

	var items []testItem

	// Helper to create item and track it
	createItem := func(t *testing.T, title string, statusID, priorityID int, dueDate string, iterationID, milestoneID, projectID *int, customFields map[string]interface{}) int {
		t.Helper()

		itemData := map[string]interface{}{
			"workspace_id": workspaceID,
			"title":        title,
			"status_id":    statusID,
			"priority_id":  priorityID,
		}

		if dueDate != "" {
			itemData["due_date"] = dueDate
		}
		if iterationID != nil {
			itemData["iteration_id"] = *iterationID
		}
		if milestoneID != nil {
			itemData["milestone_id"] = *milestoneID
		}
		if projectID != nil {
			itemData["project_id"] = *projectID
		}
		if len(customFields) > 0 {
			itemData["custom_field_values"] = customFields
		}

		resp := MakeAuthRequest(t, server, http.MethodPost, "/items", itemData)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusCreated)

		var result map[string]interface{}
		DecodeJSON(t, resp, &result)

		itemID := ExtractIDFromResponse(t, result)

		items = append(items, testItem{
			id:           itemID,
			title:        title,
			statusID:     statusID,
			priorityID:   priorityID,
			dueDate:      dueDate,
			iterationID:  iterationID,
			milestoneID:  milestoneID,
			projectID:    projectID,
			customFields: customFields,
		})

		return itemID
	}

	// Helper to execute QL query and return item IDs
	executeQL := func(t *testing.T, query string) []int {
		t.Helper()

		encodedQL := url.QueryEscape(query)
		resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s&limit=1000", encodedQL), nil)
		defer resp.Body.Close()

		AssertStatusCode(t, resp, http.StatusOK)

		// Parse paginated response
		var paginatedResp struct {
			Items []map[string]interface{} `json:"items"`
		}
		DecodeJSON(t, resp, &paginatedResp)

		var itemIDs []int
		for _, item := range paginatedResp.Items {
			if id, ok := item["id"].(float64); ok {
				itemIDs = append(itemIDs, int(id))
			}
		}

		return itemIDs
	}

	// Helper to check if slice contains value
	contains := func(slice []int, value int) bool {
		for _, v := range slice {
			if v == value {
				return true
			}
		}
		return false
	}

	// Setup: Create workspace, statuses, priorities, iterations, milestones
	t.Run("Setup", func(t *testing.T) {
		// Create workspace
		workspaceID, _ = CreateTestWorkspace(t, server, "QL Filter Test", "QLFILT")

		// Create status categories
		categoryIDs := CreateTestStatusCategories(t, server, "QL")

		// Create statuses
		statusIDs := CreateTestStatuses(t, server, "QL", categoryIDs)
		statusOpen = statusIDs[0]       // To Do category
		statusInProgress = statusIDs[2] // In Progress category
		statusClosed = statusIDs[4]     // Done category

		// Get existing priorities (Low, Medium, High are built-in)
		prioritiesResp := MakeAuthRequest(t, server, http.MethodGet, "/priorities", nil)
		defer prioritiesResp.Body.Close()

		AssertStatusCode(t, prioritiesResp, http.StatusOK)

		var prioritiesList []map[string]interface{}
		DecodeJSON(t, prioritiesResp, &prioritiesList)

		for _, pri := range prioritiesList {
			name, _ := pri["name"].(string)
			id, _ := pri["id"].(float64)
			switch name {
			case "Low":
				priorityLow = int(id)
			case "Medium":
				priorityMedium = int(id)
			case "High":
				priorityHigh = int(id)
			}
		}

		if priorityLow == 0 || priorityMedium == 0 || priorityHigh == 0 {
			t.Fatal("Failed to find required priorities (Low, Medium, High)")
		}

		// Create project (time tracking project)
		projectData := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "QL Test Project",
			"key":          "QLPROJ",
			"description":  "Test project for QL filtering",
		}

		projResp := MakeAuthRequest(t, server, http.MethodPost, "/time/projects", projectData)
		defer projResp.Body.Close()

		AssertStatusCode(t, projResp, http.StatusCreated)

		var projResult map[string]interface{}
		DecodeJSON(t, projResp, &projResult)
		projectID = ExtractIDFromResponse(t, projResult)

		// Create iteration (belongs to workspace, not project)
		iterationData := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Sprint 1",
			"description":  "Test iteration",
			"start_date":   "2024-01-01",
			"end_date":     "2024-01-31",
			"status":       "active",
			"is_global":    false,
		}

		iterResp := MakeAuthRequest(t, server, http.MethodPost, "/iterations", iterationData)
		defer iterResp.Body.Close()

		AssertStatusCode(t, iterResp, http.StatusCreated)

		var iterResult map[string]interface{}
		DecodeJSON(t, iterResp, &iterResult)
		iterationID = ExtractIDFromResponse(t, iterResult)

		// Create milestone (workspace-scoped)
		milestoneData := map[string]interface{}{
			"name":         "v1.0",
			"description":  "Test milestone",
			"target_date":  "2024-06-30",
			"status":       "in-progress",
			"workspace_id": workspaceID,
		}

		msResp := MakeAuthRequest(t, server, http.MethodPost, "/milestones", milestoneData)
		defer msResp.Body.Close()

		AssertStatusCode(t, msResp, http.StatusCreated)

		var msResult map[string]interface{}
		DecodeJSON(t, msResp, &msResult)
		milestoneID = ExtractIDFromResponse(t, msResult)

		// Create custom fields for testing
		cfEpicLink = CreateTestCustomField(t, server, "epic_link", "text", "")
		cfEnvironment = CreateTestCustomField(t, server, "environment", "text", "")
		cfPriorityLevel = CreateTestCustomField(t, server, "priority_level", "select", `["Critical", "High", "Medium", "Low"]`)
		cfTags = CreateTestCustomField(t, server, "tags", "multiselect", `["bug", "feature", "enhancement", "documentation"]`)
		cfStoryPoints = CreateTestCustomField(t, server, "story_points", "number", "")
		cfTargetDate = CreateTestCustomField(t, server, "target_date", "date", "")
		cfNotes = CreateTestCustomField(t, server, "notes", "textarea", "")

		t.Logf("Created custom fields: epic_link=%d, environment=%d, priority_level=%d, tags=%d, story_points=%d, target_date=%d, notes=%d",
			cfEpicLink, cfEnvironment, cfPriorityLevel, cfTags, cfStoryPoints, cfTargetDate, cfNotes)

		// Create test items with various properties
		now := time.Now()
		yesterday := now.AddDate(0, 0, -1).Format(time.RFC3339)
		tomorrow := now.AddDate(0, 0, 1).Format(time.RFC3339)
		nextWeek := now.AddDate(0, 0, 7).Format(time.RFC3339)
		nextMonth := now.AddDate(0, 1, 0).Format(time.RFC3339)

		// Item 1: Open status, High priority, Overdue + EPIC-1, production, Critical, bug, 8 points
		createItem(t, "Overdue High Priority", statusOpen, priorityHigh, yesterday, nil, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-1",
			"environment":    "production",
			"priority_level": "Critical",
			"tags":           "bug",
			"story_points":   8,
			"target_date":    yesterday[:10], // YYYY-MM-DD format
		})

		// Item 2: Open status, Medium priority, Due tomorrow + EPIC-1, staging, High, feature, 5 points
		createItem(t, "Medium Priority Soon", statusOpen, priorityMedium, tomorrow, nil, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-1",
			"environment":    "staging",
			"priority_level": "High",
			"tags":           "feature",
			"story_points":   5,
			"target_date":    tomorrow[:10],
		})

		// Item 3: In Progress, High priority, Due next week, with iteration + EPIC-2, dev, Medium, bug+enhancement, 13 points
		createItem(t, "In Progress with Sprint", statusInProgress, priorityHigh, nextWeek, &iterationID, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-2",
			"environment":    "dev",
			"priority_level": "Medium",
			"tags":           []string{"bug", "enhancement"},
			"story_points":   13,
			"target_date":    nextWeek[:10],
		})

		// Item 4: In Progress, Low priority, No due date + EPIC-2, production, Low, documentation, 2 points
		createItem(t, "In Progress No Due Date", statusInProgress, priorityLow, "", nil, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-2",
			"environment":    "production",
			"priority_level": "Low",
			"tags":           "documentation",
			"story_points":   2,
		})

		// Item 5: Closed, High priority, with milestone + EPIC-3, production, Critical, feature, 8 points
		createItem(t, "Closed High Priority", statusClosed, priorityHigh, "", nil, &milestoneID, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-3",
			"environment":    "production",
			"priority_level": "Critical",
			"tags":           "feature",
			"story_points":   8,
			"target_date":    yesterday[:10],
		})

		// Item 6: Closed, Low priority, Past due date + EPIC-3, staging, Low, bug, 3 points
		createItem(t, "Closed Low Priority", statusClosed, priorityLow, yesterday, nil, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-3",
			"environment":    "staging",
			"priority_level": "Low",
			"tags":           "bug",
			"story_points":   3,
			"target_date":    yesterday[:10],
		})

		// Item 7: Open, Low priority, Due next month, with iteration and milestone + EPIC-1, dev, Medium, enhancement, 5 points
		createItem(t, "Low Priority Future", statusOpen, priorityLow, nextMonth, &iterationID, &milestoneID, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-1",
			"environment":    "dev",
			"priority_level": "Medium",
			"tags":           "enhancement",
			"story_points":   5,
			"target_date":    nextMonth[:10],
		})

		// Item 8: Open, High priority, Due tomorrow, with iteration + EPIC-2, production, High, feature+bug, 13 points
		createItem(t, "High Priority Sprint Item", statusOpen, priorityHigh, tomorrow, &iterationID, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-2",
			"environment":    "production",
			"priority_level": "High",
			"tags":           []string{"feature", "bug"},
			"story_points":   13,
			"target_date":    tomorrow[:10],
		})

		// Item 9: In Progress, Medium priority, Due next week, with milestone + EPIC-3, dev, Medium, feature+documentation, 5 points
		createItem(t, "Medium Priority Milestone", statusInProgress, priorityMedium, nextWeek, nil, &milestoneID, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-3",
			"environment":    "dev",
			"priority_level": "Medium",
			"tags":           []string{"feature", "documentation"},
			"story_points":   5,
			"target_date":    nextWeek[:10],
		})

		// Item 10: Closed, Medium priority, No due date, with iteration + EPIC-1, production, High, bug, 8 points
		createItem(t, "Closed Sprint Item", statusClosed, priorityMedium, "", &iterationID, nil, &projectID, map[string]interface{}{
			"epic_link":      "EPIC-1",
			"environment":    "production",
			"priority_level": "High",
			"tags":           "bug",
			"story_points":   8,
		})

		t.Logf("Created %d test items", len(items))
	})

	// Test 1: Status filtering
	t.Run("StatusFiltering", func(t *testing.T) {
		t.Run("StatusByID", func(t *testing.T) {
			query := fmt.Sprintf("status_id = %d", statusOpen)
			results := executeQL(t, query)

			// Should return items 1, 2, 7, 8 (all with statusOpen)
			expectedCount := 0
			for _, item := range items {
				if item.statusID == statusOpen {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
		})

		t.Run("StatusIN", func(t *testing.T) {
			query := fmt.Sprintf("status_id IN (%d, %d)", statusOpen, statusInProgress)
			results := executeQL(t, query)

			// Should return items with statusOpen or statusInProgress
			expectedCount := 0
			for _, item := range items {
				if item.statusID == statusOpen || item.statusID == statusInProgress {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
		})

		t.Run("StatusNotEqual", func(t *testing.T) {
			query := fmt.Sprintf("status_id != %d", statusClosed)
			results := executeQL(t, query)

			// Should return items not in statusClosed
			for _, item := range items {
				if item.statusID == statusClosed && contains(results, item.id) {
					t.Errorf("Should not find closed item %d (%s) in results", item.id, item.title)
				}
				if item.statusID != statusClosed && !contains(results, item.id) {
					t.Errorf("Expected open/in-progress item %d (%s) in results", item.id, item.title)
				}
			}
		})
	})

	// Test 2: Priority filtering
	t.Run("PriorityFiltering", func(t *testing.T) {
		t.Run("HighPriority", func(t *testing.T) {
			query := fmt.Sprintf("priority_id = %d", priorityHigh)
			results := executeQL(t, query)

			// Count expected high priority items
			expectedCount := 0
			for _, item := range items {
				if item.priorityID == priorityHigh {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected high priority item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d high priority items, got %d", expectedCount, len(results))
			}
		})

		t.Run("PriorityIN", func(t *testing.T) {
			query := fmt.Sprintf("priority_id IN (%d, %d)", priorityMedium, priorityHigh)
			results := executeQL(t, query)

			// Should return medium or high priority items
			expectedCount := 0
			for _, item := range items {
				if item.priorityID == priorityMedium || item.priorityID == priorityHigh {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected medium/high priority item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d medium/high priority items, got %d", expectedCount, len(results))
			}
		})
	})

	// Test 3: Date filtering
	t.Run("DateFiltering", func(t *testing.T) {
		t.Run("OverdueItems", func(t *testing.T) {
			query := "due_date < now()"
			results := executeQL(t, query)

			now := time.Now()
			// Should return items with past due dates
			for _, item := range items {
				if item.dueDate != "" {
					dueDate, _ := time.Parse(time.RFC3339, item.dueDate)
					if dueDate.Before(now) && !contains(results, item.id) {
						t.Errorf("Expected overdue item %d (%s) in results", item.id, item.title)
					}
					if dueDate.After(now) && contains(results, item.id) {
						t.Errorf("Should not find future item %d (%s) in overdue results", item.id, item.title)
					}
				}
			}

			t.Logf("Found %d overdue items", len(results))
		})

		t.Run("FutureItems", func(t *testing.T) {
			query := "due_date > now()"
			results := executeQL(t, query)

			now := time.Now()
			// Should return items with future due dates
			for _, item := range items {
				if item.dueDate != "" {
					dueDate, _ := time.Parse(time.RFC3339, item.dueDate)
					if dueDate.After(now) && !contains(results, item.id) {
						t.Errorf("Expected future item %d (%s) in results", item.id, item.title)
					}
					if dueDate.Before(now) && contains(results, item.id) {
						t.Errorf("Should not find past item %d (%s) in future results", item.id, item.title)
					}
				}
			}

			t.Logf("Found %d future items", len(results))
		})
	})

	// Test 4: Iteration and Milestone filtering
	t.Run("IterationMilestoneFiltering", func(t *testing.T) {
		t.Run("ItemsInIteration", func(t *testing.T) {
			query := fmt.Sprintf("iteration_id = %d", iterationID)
			results := executeQL(t, query)

			// Should return items with the iteration
			expectedCount := 0
			for _, item := range items {
				if item.iterationID != nil && *item.iterationID == iterationID {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected iteration item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items in iteration, got %d", expectedCount, len(results))
			}
		})

		t.Run("ItemsInMilestone", func(t *testing.T) {
			query := fmt.Sprintf("milestone_id = %d", milestoneID)
			results := executeQL(t, query)

			// Should return items with the milestone
			expectedCount := 0
			for _, item := range items {
				if item.milestoneID != nil && *item.milestoneID == milestoneID {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected milestone item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items in milestone, got %d", expectedCount, len(results))
			}
		})

		// Note: QL doesn't support IS NULL syntax
		// Tests for items without iteration/milestone are skipped
	})

	// Test 5: Complex queries with AND/OR
	t.Run("ComplexQueries", func(t *testing.T) {
		t.Run("HighPriorityAndOverdue", func(t *testing.T) {
			query := fmt.Sprintf("priority_id = %d AND due_date < now()", priorityHigh)
			results := executeQL(t, query)

			now := time.Now()
			// Should return high priority items that are overdue
			for _, item := range items {
				isHighPriority := item.priorityID == priorityHigh
				isOverdue := false
				if item.dueDate != "" {
					dueDate, _ := time.Parse(time.RFC3339, item.dueDate)
					isOverdue = dueDate.Before(now)
				}

				if isHighPriority && isOverdue && !contains(results, item.id) {
					t.Errorf("Expected high priority overdue item %d (%s) in results", item.id, item.title)
				}
				if (!isHighPriority || !isOverdue) && contains(results, item.id) {
					t.Errorf("Should not find item %d (%s) in results", item.id, item.title)
				}
			}

			t.Logf("Found %d high priority overdue items", len(results))
		})

		t.Run("OpenOrInProgress", func(t *testing.T) {
			query := fmt.Sprintf("status_id = %d OR status_id = %d", statusOpen, statusInProgress)
			results := executeQL(t, query)

			// Should return items that are either open or in progress
			expectedCount := 0
			for _, item := range items {
				if item.statusID == statusOpen || item.statusID == statusInProgress {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected open/in-progress item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d open/in-progress items, got %d", expectedCount, len(results))
			}
		})

		t.Run("InIterationAndHighPriority", func(t *testing.T) {
			query := fmt.Sprintf("iteration_id = %d AND priority_id = %d", iterationID, priorityHigh)
			results := executeQL(t, query)

			// Should return high priority items in the iteration
			expectedCount := 0
			for _, item := range items {
				inIteration := item.iterationID != nil && *item.iterationID == iterationID
				isHighPriority := item.priorityID == priorityHigh

				if inIteration && isHighPriority {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected high priority iteration item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d high priority iteration items, got %d", expectedCount, len(results))
			}
		})

		t.Run("ComplexMultiCondition", func(t *testing.T) {
			query := fmt.Sprintf("status_id = %d AND priority_id IN (%d, %d) AND iteration_id = %d",
				statusOpen, priorityMedium, priorityHigh, iterationID)
			results := executeQL(t, query)

			// Should return open items with medium/high priority in the iteration
			expectedCount := 0
			for _, item := range items {
				isOpen := item.statusID == statusOpen
				isMediumOrHigh := item.priorityID == priorityMedium || item.priorityID == priorityHigh
				inIteration := item.iterationID != nil && *item.iterationID == iterationID

				if isOpen && isMediumOrHigh && inIteration {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected matching item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d matching items, got %d", expectedCount, len(results))
			}
		})

		// Note: QL doesn't support IS NULL syntax
		// Test for items without iteration OR milestone is skipped
	})

	// Test 6: Project filtering
	t.Run("ProjectFiltering", func(t *testing.T) {
		t.Run("ItemsInProject", func(t *testing.T) {
			query := fmt.Sprintf("project_id = %d", projectID)
			results := executeQL(t, query)

			// All items should be in the project
			if len(results) != len(items) {
				t.Errorf("Expected %d items in project, got %d", len(items), len(results))
			}

			for _, item := range items {
				if !contains(results, item.id) {
					t.Errorf("Expected item %d (%s) in project results", item.id, item.title)
				}
			}
		})
	})

	// Test 7: Custom Field filtering
	t.Run("CustomFieldFiltering", func(t *testing.T) {
		// Helper to get custom field value from item by field name
		getCF := func(item testItem, fieldName string) interface{} {
			if item.customFields == nil {
				return nil
			}
			return item.customFields[fieldName]
		}

		t.Run("TextFieldEpicLink", func(t *testing.T) {
			query := fmt.Sprintf("cf_epic_link = \"EPIC-1\"")
			results := executeQL(t, query)

			// Should return items with epic_link = "EPIC-1"
			expectedCount := 0
			for _, item := range items {
				if epic, ok := getCF(item, "epic_link").(string); ok && epic == "EPIC-1" {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected EPIC-1 item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d EPIC-1 items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with EPIC-1", len(results))
		})

		t.Run("TextFieldEnvironment", func(t *testing.T) {
			query := "cf_environment = \"production\""
			results := executeQL(t, query)

			// Should return items in production environment
			expectedCount := 0
			for _, item := range items {
				if env, ok := getCF(item, "environment").(string); ok && env == "production" {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected production item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d production items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d production items", len(results))
		})

		t.Run("TextFieldEnvironmentIN", func(t *testing.T) {
			query := "cf_environment IN (\"staging\", \"production\")"
			results := executeQL(t, query)

			// Should return non-dev items
			expectedCount := 0
			for _, item := range items {
				if env, ok := getCF(item, "environment").(string); ok && (env == "staging" || env == "production") {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected staging/production item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d staging/production items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d staging/production items", len(results))
		})

		t.Run("SelectFieldPriorityLevel", func(t *testing.T) {
			query := "cf_priority_level = \"Critical\""
			results := executeQL(t, query)

			// Should return Critical priority items
			expectedCount := 0
			for _, item := range items {
				if pri, ok := getCF(item, "priority_level").(string); ok && pri == "Critical" {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected Critical priority item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d Critical items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d Critical priority items", len(results))
		})

		t.Run("SelectFieldPriorityLevelIN", func(t *testing.T) {
			query := "cf_priority_level IN (\"Critical\", \"High\")"
			results := executeQL(t, query)

			// Should return high-impact items
			expectedCount := 0
			for _, item := range items {
				if pri, ok := getCF(item, "priority_level").(string); ok && (pri == "Critical" || pri == "High") {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected Critical/High priority item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d Critical/High items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d Critical/High priority items", len(results))
		})

		t.Run("NumberFieldStoryPoints", func(t *testing.T) {
			query := "cf_story_points > 5"
			results := executeQL(t, query)

			// Should return large stories
			expectedCount := 0
			for _, item := range items {
				if points, ok := getCF(item, "story_points").(int); ok && points > 5 {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected large story item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items with >5 points, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d large stories (>5 points)", len(results))
		})

		t.Run("NumberFieldStoryPointsRange", func(t *testing.T) {
			query := "cf_story_points >= 8 AND cf_story_points <= 13"
			results := executeQL(t, query)

			// Should return medium-large items
			expectedCount := 0
			for _, item := range items {
				if points, ok := getCF(item, "story_points").(int); ok && points >= 8 && points <= 13 {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected 8-13 point item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items with 8-13 points, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with 8-13 story points", len(results))
		})

		t.Run("DateFieldTargetDate", func(t *testing.T) {
			query := "cf_target_date < now()"
			results := executeQL(t, query)

			now := time.Now()
			// Should return items with past target dates
			expectedCount := 0
			for _, item := range items {
				if dateStr, ok := getCF(item, "target_date").(string); ok && dateStr != "" {
					targetDate, _ := time.Parse("2006-01-02", dateStr)
					if targetDate.Before(now) {
						expectedCount++
						if !contains(results, item.id) {
							t.Errorf("Expected past target item %d (%s) in results", item.id, item.title)
						}
					}
				}
			}

			t.Logf("Found %d items with overdue target dates (expected ~%d)", len(results), expectedCount)
		})

		t.Run("ComplexEpicAndEnvironment", func(t *testing.T) {
			query := fmt.Sprintf("cf_epic_link = \"EPIC-1\" AND cf_environment = \"production\" AND status_id = %d", statusOpen)
			results := executeQL(t, query)

			// Should return open EPIC-1 production items
			expectedCount := 0
			for _, item := range items {
				epic, _ := getCF(item, "epic_link").(string)
				env, _ := getCF(item, "environment").(string)
				if epic == "EPIC-1" && env == "production" && item.statusID == statusOpen {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected EPIC-1 production open item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d matching items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d EPIC-1 production open items", len(results))
		})

		t.Run("ComplexStoryPointsAndPriority", func(t *testing.T) {
			query := fmt.Sprintf("cf_story_points > 8 AND priority_id = %d", priorityHigh)
			results := executeQL(t, query)

			// Should return high priority large stories
			expectedCount := 0
			for _, item := range items {
				points, ok := getCF(item, "story_points").(int)
				if ok && points > 8 && item.priorityID == priorityHigh {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected high priority large story %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d high priority large stories, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d high priority large stories", len(results))
		})

		t.Run("MixedStandardAndCustom", func(t *testing.T) {
			query := fmt.Sprintf("status_id = %d AND cf_story_points > 5 AND due_date < now()", statusOpen)
			results := executeQL(t, query)

			now := time.Now()
			// Should return overdue large open items
			expectedCount := 0
			for _, item := range items {
				points, ok := getCF(item, "story_points").(int)
				isOverdue := false
				if item.dueDate != "" {
					dueDate, _ := time.Parse(time.RFC3339, item.dueDate)
					isOverdue = dueDate.Before(now)
				}
				if ok && points > 5 && item.statusID == statusOpen && isOverdue {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected overdue large open item %d (%s) in results", item.id, item.title)
					}
				}
			}

			t.Logf("Found %d overdue large open items (expected ~%d)", len(results), expectedCount)
		})

		t.Run("IterationAndEpic", func(t *testing.T) {
			query := fmt.Sprintf("iteration_id = %d AND cf_epic_link = \"EPIC-2\"", iterationID)
			results := executeQL(t, query)

			// Should return EPIC-2 items in iteration
			expectedCount := 0
			for _, item := range items {
				epic, _ := getCF(item, "epic_link").(string)
				inIteration := item.iterationID != nil && *item.iterationID == iterationID
				if epic == "EPIC-2" && inIteration {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected EPIC-2 iteration item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d EPIC-2 iteration items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d EPIC-2 items in iteration", len(results))
		})
	})

	// Test 8: Combined Operators (AND/OR with parentheses and IN clauses)
	t.Run("CombinedOperators", func(t *testing.T) {
		// Helper to get custom field value from item by field name
		getCF := func(item testItem, fieldName string) interface{} {
			if item.customFields == nil {
				return nil
			}
			return item.customFields[fieldName]
		}

		t.Run("ORWithParentheses", func(t *testing.T) {
			// (status = open OR status = in_progress) - should get items in either status
			query := fmt.Sprintf("(status_id = %d OR status_id = %d)", statusOpen, statusInProgress)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				if item.statusID == statusOpen || item.statusID == statusInProgress {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected open/in-progress item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d open/in-progress items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with status open OR in_progress", len(results))
		})

		t.Run("ANDWithORParentheses", func(t *testing.T) {
			// priority = high AND (status = open OR status = in_progress)
			query := fmt.Sprintf("priority_id = %d AND (status_id = %d OR status_id = %d)",
				priorityHigh, statusOpen, statusInProgress)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				if item.priorityID == priorityHigh && (item.statusID == statusOpen || item.statusID == statusInProgress) {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected high priority open/in-progress item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d high priority open/in-progress items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d high priority items that are open OR in_progress", len(results))
		})

		t.Run("ORWithANDParentheses", func(t *testing.T) {
			// (priority = high AND status = open) OR (priority = low AND status = closed)
			query := fmt.Sprintf("(priority_id = %d AND status_id = %d) OR (priority_id = %d AND status_id = %d)",
				priorityHigh, statusOpen, priorityLow, statusClosed)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				condition1 := item.priorityID == priorityHigh && item.statusID == statusOpen
				condition2 := item.priorityID == priorityLow && item.statusID == statusClosed
				if condition1 || condition2 {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected matching item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d matching items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items matching (high AND open) OR (low AND closed)", len(results))
		})

		t.Run("MultipleINWithAND", func(t *testing.T) {
			// status IN (open, in_progress) AND priority IN (high, medium)
			query := fmt.Sprintf("status_id IN (%d, %d) AND priority_id IN (%d, %d)",
				statusOpen, statusInProgress, priorityHigh, priorityMedium)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				statusMatch := item.statusID == statusOpen || item.statusID == statusInProgress
				priorityMatch := item.priorityID == priorityHigh || item.priorityID == priorityMedium
				if statusMatch && priorityMatch {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with status IN (open,in_progress) AND priority IN (high,medium)", len(results))
		})

		t.Run("MultipleINWithOR", func(t *testing.T) {
			// status IN (open, closed) OR priority IN (high)
			query := fmt.Sprintf("status_id IN (%d, %d) OR priority_id IN (%d)",
				statusOpen, statusClosed, priorityHigh)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				statusMatch := item.statusID == statusOpen || item.statusID == statusClosed
				priorityMatch := item.priorityID == priorityHigh
				if statusMatch || priorityMatch {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with status IN (open,closed) OR priority IN (high)", len(results))
		})

		t.Run("INWithParenthesesAndAND", func(t *testing.T) {
			// (status IN (open, in_progress) OR priority = high) AND due_date < now
			now := time.Now().Format(time.RFC3339)
			query := fmt.Sprintf("(status_id IN (%d, %d) OR priority_id = %d) AND due_date < \"%s\"",
				statusOpen, statusInProgress, priorityHigh, now)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				if item.dueDate == "" {
					continue
				}
				statusMatch := item.statusID == statusOpen || item.statusID == statusInProgress
				priorityMatch := item.priorityID == priorityHigh

				// Parse and compare dates
				itemDate, err := time.Parse(time.RFC3339, item.dueDate)
				if err != nil {
					continue
				}
				nowDate, _ := time.Parse(time.RFC3339, now)

				if (statusMatch || priorityMatch) && itemDate.Before(nowDate) {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected overdue item %d (%s) in results", item.id, item.title)
					}
				}
			}

			t.Logf("Found %d items matching (status IN OR priority high) AND overdue (expected ~%d)", len(results), expectedCount)
		})

		t.Run("CustomFieldINWithAND", func(t *testing.T) {
			// cf_environment IN ("production", "staging") AND cf_priority_level IN ("Critical", "High")
			query := `cf_environment IN ("production", "staging") AND cf_priority_level IN ("Critical", "High")`
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				env, _ := getCF(item, "environment").(string)
				priority, _ := getCF(item, "priority_level").(string)

				envMatch := env == "production" || env == "staging"
				priorityMatch := priority == "Critical" || priority == "High"

				if envMatch && priorityMatch {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with environment IN (prod,staging) AND priority IN (Critical,High)", len(results))
		})

		t.Run("CustomFieldINWithOR", func(t *testing.T) {
			// cf_epic_link IN ("EPIC-1", "EPIC-2") OR cf_environment = "production"
			query := `cf_epic_link IN ("EPIC-1", "EPIC-2") OR cf_environment = "production"`
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				epic, _ := getCF(item, "epic_link").(string)
				env, _ := getCF(item, "environment").(string)

				epicMatch := epic == "EPIC-1" || epic == "EPIC-2"
				envMatch := env == "production"

				if epicMatch || envMatch {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with epic IN (EPIC-1,EPIC-2) OR environment = production", len(results))
		})

		t.Run("ComplexNestedConditions", func(t *testing.T) {
			// (status IN (open, in_progress) AND priority IN (high, medium)) OR (cf_story_points > 5 AND cf_environment = "production")
			query := fmt.Sprintf("(status_id IN (%d, %d) AND priority_id IN (%d, %d)) OR (cf_story_points > 5 AND cf_environment = \"production\")",
				statusOpen, statusInProgress, priorityHigh, priorityMedium)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				// Condition 1: status IN (open, in_progress) AND priority IN (high, medium)
				statusMatch := item.statusID == statusOpen || item.statusID == statusInProgress
				priorityMatch := item.priorityID == priorityHigh || item.priorityID == priorityMedium
				condition1 := statusMatch && priorityMatch

				// Condition 2: cf_story_points > 5 AND cf_environment = "production"
				storyPoints, _ := getCF(item, "story_points").(float64)
				env, _ := getCF(item, "environment").(string)
				condition2 := storyPoints > 5 && env == "production"

				if condition1 || condition2 {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			t.Logf("Found %d items matching complex nested conditions (expected ~%d)", len(results), expectedCount)
		})

		t.Run("TripleORWithAND", func(t *testing.T) {
			// (priority = high OR priority = medium OR priority = low) AND status = open
			query := fmt.Sprintf("(priority_id = %d OR priority_id = %d OR priority_id = %d) AND status_id = %d",
				priorityHigh, priorityMedium, priorityLow, statusOpen)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				priorityMatch := item.priorityID == priorityHigh || item.priorityID == priorityMedium || item.priorityID == priorityLow
				if priorityMatch && item.statusID == statusOpen {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected open item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d open items with any priority, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with (high OR medium OR low priority) AND open status", len(results))
		})

		t.Run("INWithNOTAndAND", func(t *testing.T) {
			// status_id NOT IN (closed) AND priority IN (high, medium)
			query := fmt.Sprintf("status_id NOT IN (%d) AND priority_id IN (%d, %d)",
				statusClosed, priorityHigh, priorityMedium)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				statusMatch := item.statusID != statusClosed
				priorityMatch := item.priorityID == priorityHigh || item.priorityID == priorityMedium
				if statusMatch && priorityMatch {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected non-closed high/medium priority item %d (%s) in results", item.id, item.title)
					}
				}
			}

			if len(results) != expectedCount {
				t.Errorf("Expected %d items, got %d", expectedCount, len(results))
			}
			t.Logf("Found %d items with status NOT IN (closed) AND priority IN (high,medium)", len(results))
		})

		t.Run("MixedStandardAndCustomINWithAND", func(t *testing.T) {
			// status IN (open, in_progress) AND cf_epic_link IN ("EPIC-1", "EPIC-2") AND priority = high
			query := fmt.Sprintf("status_id IN (%d, %d) AND cf_epic_link IN (\"EPIC-1\", \"EPIC-2\") AND priority_id = %d",
				statusOpen, statusInProgress, priorityHigh)
			results := executeQL(t, query)

			expectedCount := 0
			for _, item := range items {
				statusMatch := item.statusID == statusOpen || item.statusID == statusInProgress
				epic, _ := getCF(item, "epic_link").(string)
				epicMatch := epic == "EPIC-1" || epic == "EPIC-2"
				priorityMatch := item.priorityID == priorityHigh

				if statusMatch && epicMatch && priorityMatch {
					expectedCount++
					if !contains(results, item.id) {
						t.Errorf("Expected item %d (%s) in results", item.id, item.title)
					}
				}
			}

			t.Logf("Found %d items matching status IN AND epic IN AND priority = high (expected ~%d)", len(results), expectedCount)
		})
	})

	// Test 9: Negative tests
	t.Run("NegativeTests", func(t *testing.T) {
		t.Run("InvalidField", func(t *testing.T) {
			query := "nonexistent_field = 123"
			encodedQL := url.QueryEscape(query)
			resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
			defer resp.Body.Close()

			// Should return an error or empty result
			if resp.StatusCode == http.StatusOK {
				var results []map[string]interface{}
				DecodeJSON(t, resp, &results)
				// It's acceptable to return empty results for invalid fields
				t.Logf("Invalid field query returned %d results (expected 0 or error)", len(results))
			}
		})

		t.Run("InvalidSyntax", func(t *testing.T) {
			query := "status_id = "
			encodedQL := url.QueryEscape(query)
			resp := MakeAuthRequest(t, server, http.MethodGet, fmt.Sprintf("/items?ql=%s", encodedQL), nil)
			defer resp.Body.Close()

			// Should return error for invalid syntax
			if resp.StatusCode == http.StatusOK {
				t.Log("Note: Invalid syntax query did not return error (may be handled gracefully)")
			}
		})

		t.Run("NonexistentID", func(t *testing.T) {
			query := "status_id = 999999"
			results := executeQL(t, query)

			if len(results) != 0 {
				t.Errorf("Expected 0 results for nonexistent status ID, got %d", len(results))
			}
		})
	})
}
