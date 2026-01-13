//go:build test

package handlers

import (
	"net/http"
	"testing"
	"windshift/internal/handlers/testutils"
	"windshift/internal/models"

)

// Test Case Management Tests

func TestTestCaseHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestCaseHandler(tdb.DB.DB)

	testCase := models.TestCase{
		Title:       "Login Functionality Test",
		Description: "Test user login with valid credentials",
		Steps:       "1. Navigate to login page\n2. Enter valid username and password\n3. Click login button",
		Expected:    "User should be logged in successfully and redirected to dashboard",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", testCase)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.TestCase
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created test case to have an ID")
	}
	if response.Title != testCase.Title {
		t.Errorf("Expected title %s, got %s", testCase.Title, response.Title)
	}
	if response.Description != testCase.Description {
		t.Errorf("Expected description %s, got %s", testCase.Description, response.Description)
	}
	if response.Steps != testCase.Steps {
		t.Errorf("Expected steps %s, got %s", testCase.Steps, response.Steps)
	}
	if response.Expected != testCase.Expected {
		t.Errorf("Expected expected result %s, got %s", testCase.Expected, response.Expected)
	}
}

func TestTestCaseHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestCaseHandler(tdb.DB.DB)

	// Create test cases
	testCases := []models.TestCase{
		{
			Title:       "Test Case 1",
			Description: "First test case",
			Steps:       "Step 1",
			Expected:    "Result 1",
		},
		{
			Title:       "Test Case 2", 
			Description: "Second test case",
			Steps:       "Step 2",
			Expected:    "Result 2",
		},
	}

	for _, tc := range testCases {
		req := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", tc)
		testutils.ExecuteRequest(t, handler.Create, req)
	}

	// Get all test cases
	req := testutils.CreateJSONRequest(t, "GET", "/api/test-cases", nil)
	rr := testutils.ExecuteRequest(t, handler.GetAll, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.TestCase
	rr.AssertJSONResponse(&response)

	if len(response) != 2 {
		t.Errorf("Expected 2 test cases, got %d", len(response))
	}
}

func TestTestCaseHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestCaseHandler(tdb.DB.DB)

	// Create test case
	testCase := models.TestCase{
		Title:       "Get Test Case",
		Description: "Test case for GET endpoint",
		Steps:       "Test steps",
		Expected:    "Expected result",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", testCase)
	createRR := testutils.ExecuteRequest(t, handler.Create, createReq)

	var created models.TestCase
	createRR.AssertJSONResponse(&created)

	// Get the test case
	req := testutils.CreateJSONRequest(t, "GET", "/api/test-cases/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteRequest(t, handler.Get, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.TestCase
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Title != testCase.Title {
		t.Errorf("Expected title %s, got %s", testCase.Title, response.Title)
	}
}

func TestTestCaseHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestCaseHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "GET", "/api/test-cases/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteRequest(t, handler.Get, req)

	rr.AssertStatusCode(http.StatusNotFound)
}

// Test Set Management Tests

func TestTestSetHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestSetHandler(tdb.DB.DB)

	testSet := models.TestSet{
		Name:        "Smoke Test Suite",
		Description: "Basic smoke tests for core functionality",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/test-sets", testSet)
	rr := testutils.ExecuteRequest(t, handler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.TestSet
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created test set to have an ID")
	}
	if response.Name != testSet.Name {
		t.Errorf("Expected name %s, got %s", testSet.Name, response.Name)
	}
	if response.Description != testSet.Description {
		t.Errorf("Expected description %s, got %s", testSet.Description, response.Description)
	}
}

func TestTestSetHandler_AddTestCase_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	testCaseHandler := NewTestCaseHandler(tdb.DB.DB)
	testSetHandler := NewTestSetHandler(tdb.DB.DB)

	// Create test case
	testCase := models.TestCase{
		Title:       "Login Test",
		Description: "Test login functionality",
		Steps:       "Enter credentials and login",
		Expected:    "Successfully logged in",
	}

	createCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", testCase)
	createCaseRR := testutils.ExecuteRequest(t, testCaseHandler.Create, createCaseReq)

	var createdCase models.TestCase
	createCaseRR.AssertJSONResponse(&createdCase)

	// Create test set
	testSet := models.TestSet{
		Name:        "Login Test Suite",
		Description: "Tests for login functionality",
	}

	createSetReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets", testSet)
	createSetRR := testutils.ExecuteRequest(t, testSetHandler.Create, createSetReq)

	var createdSet models.TestSet
	createSetRR.AssertJSONResponse(&createdSet)

	// Add test case to test set
	addRequest := struct {
		TestCaseID int `json:"test_case_id"`
	}{
		TestCaseID: createdCase.ID,
	}
	
	addCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets/"+testutils.IntToString(createdSet.ID)+"/test-cases", addRequest)
	addCaseReq.SetPathValue("id", testutils.IntToString(createdSet.ID))
	rr := testutils.ExecuteRequest(t, testSetHandler.AddTestCase, addCaseReq)

	rr.AssertStatusCode(http.StatusCreated)
}

// Test Run Management Tests

func TestTestRunHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test data
	testCaseHandler := NewTestCaseHandler(tdb.DB.DB)
	testSetHandler := NewTestSetHandler(tdb.DB.DB)
	testRunHandler := NewTestRunHandler(tdb.DB.DB)

	// Create test case
	testCase := models.TestCase{
		Title:       "API Test",
		Description: "Test API functionality",
		Steps:       "Call API endpoint",
		Expected:    "200 response",
	}

	createCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", testCase)
	createCaseRR := testutils.ExecuteRequest(t, testCaseHandler.Create, createCaseReq)

	var createdCase models.TestCase
	createCaseRR.AssertJSONResponse(&createdCase)

	// Create test set
	testSet := models.TestSet{
		Name:        "API Test Suite",
		Description: "API functionality tests",
	}

	createSetReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets", testSet)
	createSetRR := testutils.ExecuteRequest(t, testSetHandler.Create, createSetReq)

	var createdSet models.TestSet
	createSetRR.AssertJSONResponse(&createdSet)

	// Add test case to set
	addRequest := struct {
		TestCaseID int `json:"test_case_id"`
	}{
		TestCaseID: createdCase.ID,
	}
	
	addCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets/"+testutils.IntToString(createdSet.ID)+"/test-cases", addRequest)
	addCaseReq.SetPathValue("id", testutils.IntToString(createdSet.ID))
	testutils.ExecuteRequest(t, testSetHandler.AddTestCase, addCaseReq)

	// Create test run
	testRun := models.TestRun{
		SetID:     createdSet.ID,
		Name:      "Sprint 1 Test Run",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/test-runs", testRun)
	rr := testutils.ExecuteRequest(t, testRunHandler.Create, req)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.TestRun
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created test run to have an ID")
	}
	if response.Name != testRun.Name {
		t.Errorf("Expected name %s, got %s", testRun.Name, response.Name)
	}
	if response.SetID != testRun.SetID {
		t.Errorf("Expected test set ID %d, got %d", testRun.SetID, response.SetID)
	}
}

func TestTestRunHandler_GetResults_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create minimal test setup for results retrieval
	testCaseHandler := NewTestCaseHandler(tdb.DB.DB)
	testSetHandler := NewTestSetHandler(tdb.DB.DB)
	testRunHandler := NewTestRunHandler(tdb.DB.DB)

	// Create test case
	testCase := models.TestCase{
		Title: "Results Test", Description: "Test case for results", Steps: "Execute", Expected: "Pass",
	}
	createCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", testCase)
	createCaseRR := testutils.ExecuteRequest(t, testCaseHandler.Create, createCaseReq)
	var createdCase models.TestCase
	createCaseRR.AssertJSONResponse(&createdCase)

	// Create test set
	testSet := models.TestSet{Name: "Results Suite", Description: "Results test suite"}
	createSetReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets", testSet)
	createSetRR := testutils.ExecuteRequest(t, testSetHandler.Create, createSetReq)
	var createdSet models.TestSet
	createSetRR.AssertJSONResponse(&createdSet)

	// Add test case to set
	addRequest := struct {
		TestCaseID int `json:"test_case_id"`
	}{
		TestCaseID: createdCase.ID,
	}
	
	addCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets/"+testutils.IntToString(createdSet.ID)+"/test-cases", addRequest)
	addCaseReq.SetPathValue("id", testutils.IntToString(createdSet.ID))
	testutils.ExecuteRequest(t, testSetHandler.AddTestCase, addCaseReq)

	// Create test run (this should automatically create results)
	testRun := models.TestRun{SetID: createdSet.ID, Name: "Results Run"}
	createRunReq := testutils.CreateJSONRequest(t, "POST", "/api/test-runs", testRun)
	createRunRR := testutils.ExecuteRequest(t, testRunHandler.Create, createRunReq)
	var createdRun models.TestRun
	createRunRR.AssertJSONResponse(&createdRun)

	// Get test results for this run
	req := testutils.CreateJSONRequest(t, "GET", "/api/test-runs/"+testutils.IntToString(createdRun.ID)+"/results", nil)
	req.SetPathValue("id", testutils.IntToString(createdRun.ID))
	rr := testutils.ExecuteRequest(t, testRunHandler.GetResults, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	// Should have at least one result for our test case
	var results []models.TestResult
	rr.AssertJSONResponse(&results)

	if len(results) != 1 {
		t.Errorf("Expected 1 test result, got %d", len(results))
	}

	if len(results) > 0 {
		if results[0].TestCaseID != createdCase.ID {
			t.Errorf("Expected test case ID %d, got %d", createdCase.ID, results[0].TestCaseID)
		}
		if results[0].Status != "not_run" {
			t.Errorf("Expected initial status 'not_run', got %s", results[0].Status)
		}
	}
}

// Integration Test: Full Test Management Workflow

func TestTestManagementWorkflow_Integration(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Initialize handlers
	testCaseHandler := NewTestCaseHandler(tdb.DB.DB)
	testSetHandler := NewTestSetHandler(tdb.DB.DB)
	testRunHandler := NewTestRunHandler(tdb.DB.DB)

	// Step 1: Create multiple test cases
	testCases := []models.TestCase{
		{Title: "Login Test", Description: "Test login", Steps: "Enter credentials", Expected: "Login success"},
		{Title: "Logout Test", Description: "Test logout", Steps: "Click logout", Expected: "Logout success"},
		{Title: "Profile Test", Description: "Test profile", Steps: "View profile", Expected: "Profile displayed"},
	}

	var createdCases []models.TestCase
	for _, tc := range testCases {
		req := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", tc)
		rr := testutils.ExecuteRequest(t, testCaseHandler.Create, req)
		var created models.TestCase
		rr.AssertJSONResponse(&created)
		createdCases = append(createdCases, created)
	}

	// Step 2: Create test set
	testSet := models.TestSet{
		Name:        "User Management Test Suite",
		Description: "Comprehensive user management tests",
	}

	createSetReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets", testSet)
	createSetRR := testutils.ExecuteRequest(t, testSetHandler.Create, createSetReq)

	var createdSet models.TestSet
	createSetRR.AssertJSONResponse(&createdSet)

	// Step 3: Add all test cases to the set
	for _, tc := range createdCases {
		addRequest := struct {
			TestCaseID int `json:"test_case_id"`
		}{
			TestCaseID: tc.ID,
		}
		
		addReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets/"+testutils.IntToString(createdSet.ID)+"/test-cases", addRequest)
		addReq.SetPathValue("id", testutils.IntToString(createdSet.ID))
		rr := testutils.ExecuteRequest(t, testSetHandler.AddTestCase, addReq)
		rr.AssertStatusCode(http.StatusCreated)
	}

	// Step 4: Create test run
	testRun := models.TestRun{
		SetID: createdSet.ID,
		Name:  "Release 1.0 Test Run",
	}

	createRunReq := testutils.CreateJSONRequest(t, "POST", "/api/test-runs", testRun)
	createRunRR := testutils.ExecuteRequest(t, testRunHandler.Create, createRunReq)

	var createdRun models.TestRun
	createRunRR.AssertJSONResponse(&createdRun)

	// Step 5: Verify that test results were created automatically
	resultsReq := testutils.CreateJSONRequest(t, "GET", "/api/test-runs/"+testutils.IntToString(createdRun.ID)+"/results", nil)
	resultsReq.SetPathValue("id", testutils.IntToString(createdRun.ID))
	resultsRR := testutils.ExecuteRequest(t, testRunHandler.GetResults, resultsReq)

	resultsRR.AssertStatusCode(http.StatusOK)

	var results []models.TestResult
	resultsRR.AssertJSONResponse(&results)

	if len(results) != len(createdCases) {
		t.Errorf("Expected %d test results, got %d", len(createdCases), len(results))
	}

	// Step 6: Verify test run summary
	summaryReq := testutils.CreateJSONRequest(t, "GET", "/api/test-runs/"+testutils.IntToString(createdRun.ID), nil)
	summaryReq.SetPathValue("id", testutils.IntToString(createdRun.ID))
	summaryRR := testutils.ExecuteRequest(t, testRunHandler.Get, summaryReq)

	summaryRR.AssertStatusCode(http.StatusOK)

	var runSummary models.TestRun
	summaryRR.AssertJSONResponse(&runSummary)

	if runSummary.ID != createdRun.ID {
		t.Errorf("Expected run ID %d, got %d", createdRun.ID, runSummary.ID)
	}

	t.Logf("Successfully completed test management workflow with %d test cases", len(createdCases))
}

// Test Result Update Tests (using existing handler)

func TestTestRunHandler_UpdateResult_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Setup test data
	testCaseHandler := NewTestCaseHandler(tdb.DB.DB)
	testSetHandler := NewTestSetHandler(tdb.DB.DB)
	testRunHandler := NewTestRunHandler(tdb.DB.DB)

	// Create test case
	testCase := models.TestCase{
		Title: "Update Test", Description: "Test for updating results", Steps: "Execute test", Expected: "Pass",
	}
	createCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-cases", testCase)
	createCaseRR := testutils.ExecuteRequest(t, testCaseHandler.Create, createCaseReq)
	var createdCase models.TestCase
	createCaseRR.AssertJSONResponse(&createdCase)

	// Create test set
	testSet := models.TestSet{Name: "Update Suite", Description: "Update test suite"}
	createSetReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets", testSet)
	createSetRR := testutils.ExecuteRequest(t, testSetHandler.Create, createSetReq)
	var createdSet models.TestSet
	createSetRR.AssertJSONResponse(&createdSet)

	// Add test case to set
	addRequest := struct {
		TestCaseID int `json:"test_case_id"`
	}{
		TestCaseID: createdCase.ID,
	}
	
	addCaseReq := testutils.CreateJSONRequest(t, "POST", "/api/test-sets/"+testutils.IntToString(createdSet.ID)+"/test-cases", addRequest)
	addCaseReq.SetPathValue("id", testutils.IntToString(createdSet.ID))
	testutils.ExecuteRequest(t, testSetHandler.AddTestCase, addCaseReq)

	// Create test run (this should automatically create results)
	testRun := models.TestRun{SetID: createdSet.ID, Name: "Update Run"}
	createRunReq := testutils.CreateJSONRequest(t, "POST", "/api/test-runs", testRun)
	createRunRR := testutils.ExecuteRequest(t, testRunHandler.Create, createRunReq)
	var createdRun models.TestRun
	createRunRR.AssertJSONResponse(&createdRun)

	// Get the result ID that was created
	resultsReq := testutils.CreateJSONRequest(t, "GET", "/api/test-runs/"+testutils.IntToString(createdRun.ID)+"/results", nil)
	resultsReq.SetPathValue("id", testutils.IntToString(createdRun.ID))
	resultsRR := testutils.ExecuteRequest(t, testRunHandler.GetResults, resultsReq)
	
	var results []models.TestResult
	resultsRR.AssertJSONResponse(&results)
	
	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}
	
	resultID := results[0].ID

	// Update the result
	updateReq := struct {
		Status       string `json:"status"`
		ActualResult string `json:"actual_result"`
		Notes        string `json:"notes"`
	}{
		Status:       "passed",
		ActualResult: "Test executed successfully",
		Notes:        "All assertions passed",
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/test-runs/"+testutils.IntToString(createdRun.ID)+"/results/"+testutils.IntToString(resultID), updateReq)
	req.SetPathValue("id", testutils.IntToString(createdRun.ID))
	req.SetPathValue("resultId", testutils.IntToString(resultID))
	rr := testutils.ExecuteRequest(t, testRunHandler.UpdateResult, req)

	rr.AssertStatusCode(http.StatusOK)
}

func TestTestRunHandler_UpdateResult_InvalidRequest(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestRunHandler(tdb.DB.DB)

	// Test invalid JSON
	req := testutils.CreateJSONRequest(t, "PUT", "/api/test-runs/1/results/1", "invalid json")
	req.SetPathValue("id", "1")
	req.SetPathValue("resultId", "1")
	rr := testutils.ExecuteRequest(t, handler.UpdateResult, req)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// TestCase Update and Delete Tests

func TestTestCaseHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test case
	result, err := tdb.Exec(`
		INSERT INTO test_cases (title, description, steps, expected, created_at, updated_at)
		VALUES ('Original Title', 'Original desc', 'Original steps', 'Original expected', datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test case: %v", err)
	}
	testCaseID, _ := result.LastInsertId()

	handler := NewTestCaseHandler(tdb.DB.DB)

	updatedTestCase := models.TestCase{
		Title:       "Updated Title",
		Description: "Updated description",
		Steps:       "Updated steps",
		Expected:    "Updated expected result",
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/test-cases/"+testutils.IntToString(int(testCaseID)), updatedTestCase)
	req.SetPathValue("id", testutils.IntToString(int(testCaseID)))
	rr := testutils.ExecuteRequest(t, handler.Update, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.TestCase
	rr.AssertJSONResponse(&response)

	if response.Title != updatedTestCase.Title {
		t.Errorf("Expected title %s, got %s", updatedTestCase.Title, response.Title)
	}
	if response.Description != updatedTestCase.Description {
		t.Errorf("Expected description %s, got %s", updatedTestCase.Description, response.Description)
	}
	if response.Steps != updatedTestCase.Steps {
		t.Errorf("Expected steps %s, got %s", updatedTestCase.Steps, response.Steps)
	}
	if response.Expected != updatedTestCase.Expected {
		t.Errorf("Expected expected %s, got %s", updatedTestCase.Expected, response.Expected)
	}

	// Verify database was updated
	var title, description, steps, expected string
	err = tdb.QueryRow("SELECT title, description, steps, expected FROM test_cases WHERE id = ?", testCaseID).Scan(&title, &description, &steps, &expected)
	if err != nil {
		t.Fatalf("Failed to verify test case update: %v", err)
	}
	if title != updatedTestCase.Title || description != updatedTestCase.Description || steps != updatedTestCase.Steps || expected != updatedTestCase.Expected {
		t.Error("Database was not updated correctly")
	}
}

func TestTestCaseHandler_Update_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestCaseHandler(tdb.DB.DB)

	testCase := models.TestCase{Title: "Test", Description: "Test", Steps: "Test", Expected: "Test"}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/test-cases/invalid", testCase)
	req.SetPathValue("id", "invalid")
	rr := testutils.ExecuteRequest(t, handler.Update, req)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestTestCaseHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test case
	result, err := tdb.Exec(`
		INSERT INTO test_cases (title, description, steps, expected, created_at, updated_at)
		VALUES ('Test Case', 'Test description', 'Test steps', 'Test expected', datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test case: %v", err)
	}
	testCaseID, _ := result.LastInsertId()

	handler := NewTestCaseHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/test-cases/"+testutils.IntToString(int(testCaseID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(testCaseID)))
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify test case was deleted
	var count int
	err = tdb.QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ?", testCaseID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify test case deletion: %v", err)
	}
	if count != 0 {
		t.Error("Test case was not deleted from database")
	}
}

func TestTestCaseHandler_Delete_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestCaseHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/test-cases/invalid", nil)
	req.SetPathValue("id", "invalid")
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// TestSet Update and Delete Tests

func TestTestSetHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test set
	result, err := tdb.Exec(`
		INSERT INTO test_sets (name, description, created_at, updated_at)
		VALUES ('Original Set', 'Original description', datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test set: %v", err)
	}
	testSetID, _ := result.LastInsertId()

	handler := NewTestSetHandler(tdb.DB.DB)

	updatedTestSet := models.TestSet{
		Name:        "Updated Set Name",
		Description: "Updated set description",
	}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/test-sets/"+testutils.IntToString(int(testSetID)), updatedTestSet)
	req.SetPathValue("id", testutils.IntToString(int(testSetID)))
	rr := testutils.ExecuteRequest(t, handler.Update, req)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.TestSet
	rr.AssertJSONResponse(&response)

	if response.Name != updatedTestSet.Name {
		t.Errorf("Expected name %s, got %s", updatedTestSet.Name, response.Name)
	}
	if response.Description != updatedTestSet.Description {
		t.Errorf("Expected description %s, got %s", updatedTestSet.Description, response.Description)
	}

	// Verify database was updated
	var name, description string
	err = tdb.QueryRow("SELECT name, description FROM test_sets WHERE id = ?", testSetID).Scan(&name, &description)
	if err != nil {
		t.Fatalf("Failed to verify test set update: %v", err)
	}
	if name != updatedTestSet.Name || description != updatedTestSet.Description {
		t.Error("Database was not updated correctly")
	}
}

func TestTestSetHandler_Update_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestSetHandler(tdb.DB.DB)

	testSet := models.TestSet{Name: "Test", Description: "Test"}

	req := testutils.CreateJSONRequest(t, "PUT", "/api/test-sets/invalid", testSet)
	req.SetPathValue("id", "invalid")
	rr := testutils.ExecuteRequest(t, handler.Update, req)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestTestSetHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test set
	result, err := tdb.Exec(`
		INSERT INTO test_sets (name, description, created_at, updated_at)
		VALUES ('Test Set', 'Test description', datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test set: %v", err)
	}
	testSetID, _ := result.LastInsertId()

	handler := NewTestSetHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/test-sets/"+testutils.IntToString(int(testSetID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(testSetID)))
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify test set was deleted
	var count int
	err = tdb.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ?", testSetID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify test set deletion: %v", err)
	}
	if count != 0 {
		t.Error("Test set was not deleted from database")
	}
}

func TestTestSetHandler_Delete_InvalidID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewTestSetHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/test-sets/invalid", nil)
	req.SetPathValue("id", "invalid")
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestTestSetHandler_Delete_WithTestCases_Cascade(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	// Create test set
	setResult, err := tdb.Exec(`
		INSERT INTO test_sets (name, description, created_at, updated_at)
		VALUES ('Test Set', 'Test description', datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test set: %v", err)
	}
	testSetID, _ := setResult.LastInsertId()

	// Create test case and associate with set
	caseResult, err := tdb.Exec(`
		INSERT INTO test_cases (title, description, steps, expected, created_at, updated_at)
		VALUES ('Test Case', 'Test description', 'Test steps', 'Test expected', datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("Failed to create test case: %v", err)
	}
	testCaseID, _ := caseResult.LastInsertId()

	// Associate test case with set
	_, err = tdb.Exec(`
		INSERT INTO set_test_cases (set_id, test_case_id)
		VALUES (?, ?)
	`, testSetID, testCaseID)
	if err != nil {
		t.Fatalf("Failed to associate test case with set: %v", err)
	}

	handler := NewTestSetHandler(tdb.DB.DB)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/test-sets/"+testutils.IntToString(int(testSetID)), nil)
	req.SetPathValue("id", testutils.IntToString(int(testSetID)))
	rr := testutils.ExecuteRequest(t, handler.Delete, req)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify test set was deleted
	var setCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ?", testSetID).Scan(&setCount)
	if err != nil {
		t.Fatalf("Failed to verify test set deletion: %v", err)
	}
	if setCount != 0 {
		t.Error("Test set was not deleted from database")
	}

	// Verify association was deleted (cascade)
	var assocCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM set_test_cases WHERE set_id = ?", testSetID).Scan(&assocCount)
	if err != nil {
		t.Fatalf("Failed to verify association deletion: %v", err)
	}
	if assocCount != 0 {
		t.Error("Test set associations were not deleted")
	}

	// Verify test case still exists (should not cascade delete test cases)
	var caseCount int
	err = tdb.QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ?", testCaseID).Scan(&caseCount)
	if err != nil {
		t.Fatalf("Failed to verify test case exists: %v", err)
	}
	if caseCount != 1 {
		t.Error("Test case should still exist after set deletion")
	}
}