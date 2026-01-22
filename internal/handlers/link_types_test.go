//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestLinkTypeHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	linkType := models.LinkType{
		Name:         "Blocks",
		Description:  "This item blocks another",
		ForwardLabel: "blocks",
		ReverseLabel: "is blocked by",
		Color:        "#ff0000",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.LinkType
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected created link type to have an ID")
	}
	if response.Name != linkType.Name {
		t.Errorf("Expected name %s, got %s", linkType.Name, response.Name)
	}
	if response.ForwardLabel != linkType.ForwardLabel {
		t.Errorf("Expected forward_label %s, got %s", linkType.ForwardLabel, response.ForwardLabel)
	}
	if response.ReverseLabel != linkType.ReverseLabel {
		t.Errorf("Expected reverse_label %s, got %s", linkType.ReverseLabel, response.ReverseLabel)
	}
	if !response.Active {
		t.Error("Expected link type to be active by default")
	}
	if response.IsSystem {
		t.Error("Expected user-created link type to not be a system type")
	}
}

func TestLinkTypeHandler_Create_DefaultColor(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	linkType := models.LinkType{
		Name:         "Custom Relates To",
		ForwardLabel: "relates to",
		ReverseLabel: "relates to",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.LinkType
	rr.AssertJSONResponse(&response)

	if response.Color != "#6b7280" {
		t.Errorf("Expected default color #6b7280, got %s", response.Color)
	}
}

func TestLinkTypeHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	tests := []struct {
		name        string
		linkType    models.LinkType
		expectedErr string
	}{
		{
			name:        "Missing name",
			linkType:    models.LinkType{ForwardLabel: "blocks", ReverseLabel: "is blocked by"},
			expectedErr: "Name, forward_label, and reverse_label are required",
		},
		{
			name:        "Missing forward_label",
			linkType:    models.LinkType{Name: "Blocks", ReverseLabel: "is blocked by"},
			expectedErr: "Name, forward_label, and reverse_label are required",
		},
		{
			name:        "Missing reverse_label",
			linkType:    models.LinkType{Name: "Blocks", ForwardLabel: "blocks"},
			expectedErr: "Name, forward_label, and reverse_label are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/link-types", tt.linkType)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestLinkTypeHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Create test link types
	linkTypes := []models.LinkType{
		{Name: "Blocks", ForwardLabel: "blocks", ReverseLabel: "is blocked by"},
		{Name: "Duplicates", ForwardLabel: "duplicates", ReverseLabel: "is duplicated by"},
	}

	for _, lt := range linkTypes {
		req := testutils.CreateJSONRequest(t, "POST", "/api/link-types", lt)
		testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/link-types", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response []models.LinkType
	rr.AssertJSONResponse(&response)

	// Should include at least our 2 created types (may also have system types)
	if len(response) < 2 {
		t.Errorf("Expected at least 2 link types, got %d", len(response))
	}
}

func TestLinkTypeHandler_GetAll_IncludeInactive(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Create an active link type
	linkType := models.LinkType{
		Name:         "Test Type",
		ForwardLabel: "forward",
		ReverseLabel: "reverse",
	}
	req := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, req, nil)

	var created models.LinkType
	createRR.AssertJSONResponse(&created)

	// Deactivate it via update
	created.Active = false
	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/link-types/"+testutils.IntToString(created.ID), created)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	// Get all without include_inactive - should not include the inactive one
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/link-types", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, getReq, nil)

	var response []models.LinkType
	rr.AssertJSONResponse(&response)

	for _, lt := range response {
		if lt.ID == created.ID {
			t.Error("Did not expect inactive link type in results")
		}
	}

	// Get all with include_inactive - should include it
	getReqWithInactive := testutils.CreateJSONRequest(t, "GET", "/api/link-types?include_inactive=true", nil)
	rrWithInactive := testutils.ExecuteAuthenticatedRequest(t, handler.GetAll, getReqWithInactive, nil)

	var responseWithInactive []models.LinkType
	rrWithInactive.AssertJSONResponse(&responseWithInactive)

	found := false
	for _, lt := range responseWithInactive {
		if lt.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected inactive link type in results when include_inactive=true")
	}
}

func TestLinkTypeHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Create test link type
	linkType := models.LinkType{
		Name:         "Test Get",
		Description:  "Test description",
		ForwardLabel: "forward",
		ReverseLabel: "reverse",
		Color:        "#123456",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.LinkType
	createRR.AssertJSONResponse(&created)

	// Get the link type
	req := testutils.CreateJSONRequest(t, "GET", "/api/link-types/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.LinkType
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != linkType.Name {
		t.Errorf("Expected name %s, got %s", linkType.Name, response.Name)
	}
	if response.Description != linkType.Description {
		t.Errorf("Expected description %s, got %s", linkType.Description, response.Description)
	}
}

func TestLinkTypeHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/link-types/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Get, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "Link type not found") {
		t.Errorf("Expected 'Link type not found', got %s", rr.Body.String())
	}
}

func TestLinkTypeHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Create test link type
	linkType := models.LinkType{
		Name:         "Original Name",
		Description:  "Original description",
		ForwardLabel: "original forward",
		ReverseLabel: "original reverse",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.LinkType
	createRR.AssertJSONResponse(&created)

	// Update the link type
	updateData := models.LinkType{
		Name:         "Updated Name",
		Description:  "Updated description",
		ForwardLabel: "updated forward",
		ReverseLabel: "updated reverse",
		Color:        "#abcdef",
		Active:       true,
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/link-types/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.LinkType
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %s", response.Name)
	}
	if response.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", response.Description)
	}
	if response.ForwardLabel != "updated forward" {
		t.Errorf("Expected forward_label 'updated forward', got %s", response.ForwardLabel)
	}
	if response.Color != "#abcdef" {
		t.Errorf("Expected color '#abcdef', got %s", response.Color)
	}
}

func TestLinkTypeHandler_Update_ValidationError(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Create test link type
	linkType := models.LinkType{
		Name:         "Test Type",
		ForwardLabel: "forward",
		ReverseLabel: "reverse",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.LinkType
	createRR.AssertJSONResponse(&created)

	// Try to update with missing required field
	updateData := models.LinkType{
		Name:         "",
		ForwardLabel: "forward",
		ReverseLabel: "reverse",
	}

	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/link-types/"+testutils.IntToString(created.ID), updateData)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Update, updateReq, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestLinkTypeHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Create test link type
	linkType := models.LinkType{
		Name:         "To Delete",
		ForwardLabel: "forward",
		ReverseLabel: "reverse",
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/link-types", linkType)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.Create, createReq, nil)

	var created models.LinkType
	createRR.AssertJSONResponse(&created)

	// Delete the link type
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/link-types/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify link type is deleted
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/link-types/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.Get, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestLinkTypeHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/link-types/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestLinkTypeHandler_Delete_SystemType(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	handler := NewLinkTypeHandler(tdb.GetDatabase())

	// Check if there are system link types in the database
	rows, err := tdb.Query("SELECT id FROM link_types WHERE is_system = true LIMIT 1")
	if err != nil {
		t.Fatalf("Failed to query system link types: %v", err)
	}
	defer rows.Close()

	var systemID int
	if !rows.Next() {
		// Create a system link type manually for testing
		_, err := tdb.Exec(`
			INSERT INTO link_types (name, forward_label, reverse_label, is_system, active, created_at, updated_at)
			VALUES ('System Type', 'sys forward', 'sys reverse', true, true, datetime('now'), datetime('now'))
		`)
		if err != nil {
			t.Fatalf("Failed to create system link type: %v", err)
		}
		err = tdb.QueryRow("SELECT id FROM link_types WHERE name = 'System Type'").Scan(&systemID)
		if err != nil {
			t.Fatalf("Failed to get system link type ID: %v", err)
		}
	} else {
		rows.Scan(&systemID)
	}

	// Try to delete system link type
	req := testutils.CreateJSONRequest(t, "DELETE", "/api/link-types/"+testutils.IntToString(systemID), nil)
	req.SetPathValue("id", testutils.IntToString(systemID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.Delete, req, nil)

	rr.AssertStatusCode(http.StatusForbidden)
	if !strings.Contains(rr.Body.String(), "Cannot delete system link types") {
		t.Errorf("Expected 'Cannot delete system link types', got %s", rr.Body.String())
	}
}
