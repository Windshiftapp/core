//go:build test

package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

// createCommentHandler creates a CommentHandler with test services
func createCommentHandler(t *testing.T, tdb *testutils.TestDB) *CommentHandler {
	t.Helper()
	permService, actTracker, notifService := createTestServices(t, *tdb)
	return NewCommentHandler(tdb.GetDatabase(), permService, actTracker, notifService)
}

// createTestItemForComments creates an item in the test workspace and returns its ID
func createTestItemForComments(t *testing.T, tdb *testutils.TestDB, data testutils.TestDataSet) int {
	t.Helper()
	permService, actTracker, notifService := createTestServices(t, *tdb)
	itemHandler := NewItemHandler(tdb.GetDatabase(), permService, actTracker, notifService)

	statusID := data.StatusID
	priorityID := data.PriorityID
	item := models.Item{
		WorkspaceID: data.WorkspaceID,
		Title:       "Item for Comments",
		StatusID:    &statusID,
		PriorityID:  &priorityID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items", item)
	rr := testutils.ExecuteAuthenticatedRequest(t, itemHandler.Create, req, nil)
	rr.AssertStatusCode(http.StatusCreated)

	var created models.Item
	rr.AssertJSONResponse(&created)
	return created.ID
}

func TestCommentHandler_CreateComment_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	body := map[string]interface{}{
		"content":   "This is a test comment",
		"author_id": data.UserID,
	}

	req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), body)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var comment models.Comment
	rr.AssertJSONResponse(&comment)

	if comment.ID == 0 {
		t.Error("Expected comment to have an ID")
	}
	if comment.ItemID != itemID {
		t.Errorf("Expected item_id %d, got %d", itemID, comment.ItemID)
	}
	if comment.Content != "This is a test comment" {
		t.Errorf("Expected content 'This is a test comment', got %q", comment.Content)
	}
}

func TestCommentHandler_CreateComment_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	tests := []struct {
		name        string
		body        map[string]interface{}
		expectedErr string
	}{
		{
			name:        "Empty content",
			body:        map[string]interface{}{"content": "", "author_id": data.UserID},
			expectedErr: "Content is required",
		},
		{
			name:        "Whitespace-only content",
			body:        map[string]interface{}{"content": "   ", "author_id": data.UserID},
			expectedErr: "Content is required",
		},
		{
			name:        "Missing author_id",
			body:        map[string]interface{}{"content": "Hello"},
			expectedErr: "Author ID is required",
		},
		{
			name:        "Zero author_id",
			body:        map[string]interface{}{"content": "Hello", "author_id": 0},
			expectedErr: "Author ID is required",
		},
		{
			name:        "Non-existent author",
			body:        map[string]interface{}{"content": "Hello", "author_id": 99999},
			expectedErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), tt.body)
			req.SetPathValue("id", testutils.IntToString(itemID))
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, req, nil)

			if rr.Code == http.StatusCreated {
				t.Errorf("Expected error response, got 201 Created")
			}
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestCommentHandler_CreateComment_InvalidItemID(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)

	body := map[string]interface{}{
		"content":   "Hello",
		"author_id": 1,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items/abc/comments", body)
	req.SetPathValue("id", "abc")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestCommentHandler_CreateComment_NonExistentItem(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)

	body := map[string]interface{}{
		"content":   "Hello",
		"author_id": data.UserID,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/items/99999/comments", body)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCommentHandler_GetComments_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	// Create multiple comments
	for i := 0; i < 3; i++ {
		body := map[string]interface{}{
			"content":   fmt.Sprintf("Comment %d", i+1),
			"author_id": data.UserID,
		}
		req := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), body)
		req.SetPathValue("id", testutils.IntToString(itemID))
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, req, nil)
	}

	// Get comments
	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/comments", itemID), nil)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetComments, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var comments []models.Comment
	rr.AssertJSONResponse(&comments)

	if len(comments) != 3 {
		t.Errorf("Expected 3 comments, got %d", len(comments))
	}

	// Verify author name is populated
	for _, c := range comments {
		if c.AuthorName == "" {
			t.Error("Expected author_name to be populated")
		}
	}
}

func TestCommentHandler_GetComments_EmptyList(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	req := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/comments", itemID), nil)
	req.SetPathValue("id", testutils.IntToString(itemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetComments, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	// Response should be null or empty array - either is fine
	body := strings.TrimSpace(rr.Body.String())
	if body != "null" && body != "[]" {
		t.Errorf("Expected null or empty array, got %q", body)
	}
}

func TestCommentHandler_GetComments_NonExistentItem(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)

	req := testutils.CreateJSONRequest(t, "GET", "/api/items/99999/comments", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetComments, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCommentHandler_UpdateComment_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	// Create a comment
	createBody := map[string]interface{}{
		"content":   "Original content",
		"author_id": data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, createReq, nil)

	var created models.Comment
	createRR.AssertJSONResponse(&created)

	// Update the comment
	updateBody := map[string]interface{}{
		"content": "Updated content",
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", fmt.Sprintf("/api/comments/%d", created.ID), updateBody)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateComment, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var updated models.Comment
	rr.AssertJSONResponse(&updated)

	if updated.Content != "Updated content" {
		t.Errorf("Expected content 'Updated content', got %q", updated.Content)
	}
	if updated.ID != created.ID {
		t.Errorf("Expected comment ID %d, got %d", created.ID, updated.ID)
	}
}

func TestCommentHandler_UpdateComment_EmptyContent(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	// Create a comment
	createBody := map[string]interface{}{
		"content":   "Original content",
		"author_id": data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, createReq, nil)

	var created models.Comment
	createRR.AssertJSONResponse(&created)

	// Try to update with empty content
	updateBody := map[string]interface{}{
		"content": "",
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", fmt.Sprintf("/api/comments/%d", created.ID), updateBody)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateComment, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "Content is required") {
		t.Errorf("Expected 'Content is required' error, got %q", rr.Body.String())
	}
}

func TestCommentHandler_UpdateComment_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)

	updateBody := map[string]interface{}{
		"content": "Updated content",
	}
	req := testutils.CreateJSONRequest(t, "PUT", "/api/comments/99999", updateBody)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateComment, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCommentHandler_UpdateComment_NotAuthor(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	// Create a comment as user 1
	createBody := map[string]interface{}{
		"content":   "Original content",
		"author_id": data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, createReq, nil)

	var created models.Comment
	createRR.AssertJSONResponse(&created)

	// Try to update as a different user (who doesn't have edit-others permission)
	otherUser := testutils.TestUserWithID(999)
	updateBody := map[string]interface{}{
		"content": "Hacked content",
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", fmt.Sprintf("/api/comments/%d", created.ID), updateBody)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateComment, updateReq, otherUser)

	rr.AssertStatusCode(http.StatusForbidden)
}

func TestCommentHandler_DeleteComment_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	// Create a comment
	createBody := map[string]interface{}{
		"content":   "Comment to delete",
		"author_id": data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, createReq, nil)

	var created models.Comment
	createRR.AssertJSONResponse(&created)

	// Delete the comment
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", fmt.Sprintf("/api/comments/%d", created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteComment, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify comment is gone by trying to get comments for the item
	getReq := testutils.CreateJSONRequest(t, "GET", fmt.Sprintf("/api/items/%d/comments", itemID), nil)
	getReq.SetPathValue("id", testutils.IntToString(itemID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetComments, getReq, nil)

	getRR.AssertStatusCode(http.StatusOK)
	body := strings.TrimSpace(getRR.Body.String())
	if body != "null" && body != "[]" {
		t.Errorf("Expected no comments after deletion, got %q", body)
	}
}

func TestCommentHandler_DeleteComment_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/comments/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteComment, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestCommentHandler_DeleteComment_NotAuthor(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	data := tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)
	itemID := createTestItemForComments(t, tdb, data)

	// Create a comment as user 1
	createBody := map[string]interface{}{
		"content":   "Comment by user 1",
		"author_id": data.UserID,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", fmt.Sprintf("/api/items/%d/comments", itemID), createBody)
	createReq.SetPathValue("id", testutils.IntToString(itemID))
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateComment, createReq, nil)

	var created models.Comment
	createRR.AssertJSONResponse(&created)

	// Try to delete as a different user
	otherUser := testutils.TestUserWithID(999)
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", fmt.Sprintf("/api/comments/%d", created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteComment, deleteReq, otherUser)

	rr.AssertStatusCode(http.StatusForbidden)
}

func TestCommentHandler_InvalidID_Scenarios(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := createCommentHandler(t, tdb)

	tests := []struct {
		name    string
		method  string
		handler testutils.TestHandler
	}{
		{"GetComments invalid ID", "GET", handler.GetComments},
		{"CreateComment invalid ID", "POST", handler.CreateComment},
		{"UpdateComment invalid ID", "PUT", handler.UpdateComment},
		{"DeleteComment invalid ID", "DELETE", handler.DeleteComment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, tt.method, "/api/comments/abc", map[string]interface{}{"content": "test", "author_id": 1})
			req.SetPathValue("id", "abc")
			rr := testutils.ExecuteAuthenticatedRequest(t, tt.handler, req, nil)
			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}
