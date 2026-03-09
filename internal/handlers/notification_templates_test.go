//go:build test

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"windshift/internal/models"
	"windshift/internal/testutils"
)

func TestNotificationTemplateHandler_Create_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	template := models.NotificationTemplate{
		Name:         "Item Created Template",
		TemplateType: "notification_type",
		Subject:      "New item: {{item.title}}",
		Content:      "<p>A new item was created</p>",
		Description:  "Template for item creation notifications",
		IsActive:     true,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.NotificationTemplate
	rr.AssertJSONResponse(&response)

	if response.ID == 0 {
		t.Error("Expected template to have an ID")
	}
	if response.Name != template.Name {
		t.Errorf("Expected name %q, got %q", template.Name, response.Name)
	}
	if response.TemplateType != "notification_type" {
		t.Errorf("Expected template_type 'notification_type', got %q", response.TemplateType)
	}
}

func TestNotificationTemplateHandler_Create_AllTypes(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	types := []string{"header", "footer", "notification_type"}

	for _, tp := range types {
		t.Run(tp, func(t *testing.T) {
			template := models.NotificationTemplate{
				Name:         "Template " + tp,
				TemplateType: tp,
				Content:      "<p>Content for " + tp + "</p>",
				IsActive:     true,
			}

			req := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req, nil)

			rr.AssertStatusCode(http.StatusCreated)

			var response models.NotificationTemplate
			rr.AssertJSONResponse(&response)

			if response.TemplateType != tp {
				t.Errorf("Expected template_type %q, got %q", tp, response.TemplateType)
			}
		})
	}
}

func TestNotificationTemplateHandler_Create_ValidationErrors(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	tests := []struct {
		name        string
		template    models.NotificationTemplate
		expectedErr string
	}{
		{
			name:        "Missing name",
			template:    models.NotificationTemplate{TemplateType: "header", Content: "content"},
			expectedErr: "Name, template_type, and content are required",
		},
		{
			name:        "Missing template_type",
			template:    models.NotificationTemplate{Name: "Test", Content: "content"},
			expectedErr: "Name, template_type, and content are required",
		},
		{
			name:        "Missing content",
			template:    models.NotificationTemplate{Name: "Test", TemplateType: "header"},
			expectedErr: "Name, template_type, and content are required",
		},
		{
			name:        "Invalid template_type",
			template:    models.NotificationTemplate{Name: "Test", TemplateType: "invalid", Content: "content"},
			expectedErr: "Invalid template_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", tt.template)
			rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req, nil)

			rr.AssertStatusCode(http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.expectedErr) {
				t.Errorf("Expected body to contain %q, got %q", tt.expectedErr, rr.Body.String())
			}
		})
	}
}

func TestNotificationTemplateHandler_Create_DuplicateName(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	template := models.NotificationTemplate{
		Name:         "Unique Template",
		TemplateType: "header",
		Content:      "content",
		IsActive:     true,
	}

	// Create first
	req := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req, nil)
	rr.AssertStatusCode(http.StatusCreated)

	// Try to create duplicate
	req2 := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req2, nil)

	rr2.AssertStatusCode(http.StatusConflict)
}

func TestNotificationTemplateHandler_GetAll_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	// Count pre-existing templates from DB initialization
	baseReq := testutils.CreateJSONRequest(t, "GET", "/api/notification-templates", nil)
	baseRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetAllTemplates, baseReq, nil)
	var baseTemplates []models.NotificationTemplate
	baseRR.AssertJSONResponse(&baseTemplates)
	baseCount := len(baseTemplates)

	// Create templates
	for i := 0; i < 3; i++ {
		template := models.NotificationTemplate{
			Name:         "Custom Template " + testutils.IntToString(i+1),
			TemplateType: "notification_type",
			Content:      "Content " + testutils.IntToString(i+1),
			IsActive:     true,
		}
		req := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-templates", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAllTemplates, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var templates []models.NotificationTemplate
	rr.AssertJSONResponse(&templates)

	expected := baseCount + 3
	if len(templates) != expected {
		t.Errorf("Expected %d templates, got %d", expected, len(templates))
	}
}

func TestNotificationTemplateHandler_GetAll_FilterByType(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	// Create templates of different types
	templates := []models.NotificationTemplate{
		{Name: "My Header", TemplateType: "header", Content: "header content", IsActive: true},
		{Name: "My Footer", TemplateType: "footer", Content: "footer content", IsActive: true},
		{Name: "My Notif", TemplateType: "notification_type", Content: "notif content", IsActive: true},
	}

	for _, tmpl := range templates {
		req := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", tmpl)
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req, nil)
	}

	// Filter by footer type - check that filtering works
	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-templates?type=footer", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetAllTemplates, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var filtered []models.NotificationTemplate
	rr.AssertJSONResponse(&filtered)

	if len(filtered) == 0 {
		t.Error("Expected at least 1 footer template")
	}
	for _, tmpl := range filtered {
		if tmpl.TemplateType != "footer" {
			t.Errorf("Expected all templates to have type 'footer', got %q", tmpl.TemplateType)
		}
	}
}

func TestNotificationTemplateHandler_Get_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	template := models.NotificationTemplate{
		Name:         "Get Test Template",
		TemplateType: "footer",
		Content:      "Footer content",
		Description:  "A footer template",
		IsActive:     true,
	}

	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, createReq, nil)

	var created models.NotificationTemplate
	createRR.AssertJSONResponse(&created)

	// Get the template
	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-templates/"+testutils.IntToString(created.ID), nil)
	req.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetTemplate, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.NotificationTemplate
	rr.AssertJSONResponse(&response)

	if response.ID != created.ID {
		t.Errorf("Expected ID %d, got %d", created.ID, response.ID)
	}
	if response.Name != template.Name {
		t.Errorf("Expected name %q, got %q", template.Name, response.Name)
	}
}

func TestNotificationTemplateHandler_Get_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "GET", "/api/notification-templates/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetTemplate, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationTemplateHandler_Update_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	// Create a template
	template := models.NotificationTemplate{
		Name:         "Original",
		TemplateType: "header",
		Content:      "Original content",
		IsActive:     true,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, createReq, nil)

	var created models.NotificationTemplate
	createRR.AssertJSONResponse(&created)

	// Update
	updated := models.NotificationTemplate{
		Name:         "Updated",
		TemplateType: "header",
		Content:      "Updated content",
		Description:  "Now with description",
		IsActive:     false,
	}
	updateReq := testutils.CreateJSONRequest(t, "PUT", "/api/notification-templates/"+testutils.IntToString(created.ID), updated)
	updateReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateTemplate, updateReq, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response models.NotificationTemplate
	rr.AssertJSONResponse(&response)

	if response.Name != "Updated" {
		t.Errorf("Expected name 'Updated', got %q", response.Name)
	}
	if response.Content != "Updated content" {
		t.Errorf("Expected content 'Updated content', got %q", response.Content)
	}
}

func TestNotificationTemplateHandler_Update_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	template := models.NotificationTemplate{
		Name:         "Test",
		TemplateType: "header",
		Content:      "Content",
	}
	req := testutils.CreateJSONRequest(t, "PUT", "/api/notification-templates/99999", template)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateTemplate, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationTemplateHandler_Update_DuplicateName(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	// Create two templates
	t1 := models.NotificationTemplate{Name: "First", TemplateType: "header", Content: "c1", IsActive: true}
	t2 := models.NotificationTemplate{Name: "Second", TemplateType: "header", Content: "c2", IsActive: true}

	req1 := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", t1)
	testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req1, nil)

	req2 := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", t2)
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, req2, nil)

	var created2 models.NotificationTemplate
	rr2.AssertJSONResponse(&created2)

	// Try to rename second to match first
	rename := models.NotificationTemplate{Name: "First", TemplateType: "header", Content: "c2"}
	renameReq := testutils.CreateJSONRequest(t, "PUT", "/api/notification-templates/"+testutils.IntToString(created2.ID), rename)
	renameReq.SetPathValue("id", testutils.IntToString(created2.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.UpdateTemplate, renameReq, nil)

	rr.AssertStatusCode(http.StatusConflict)
}

func TestNotificationTemplateHandler_Delete_Success(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	// Create a template
	template := models.NotificationTemplate{
		Name:         "To Delete",
		TemplateType: "header",
		Content:      "Will be deleted",
		IsActive:     true,
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notification-templates", template)
	createRR := testutils.ExecuteAuthenticatedRequest(t, handler.CreateTemplate, createReq, nil)

	var created models.NotificationTemplate
	createRR.AssertJSONResponse(&created)

	// Delete
	deleteReq := testutils.CreateJSONRequest(t, "DELETE", "/api/notification-templates/"+testutils.IntToString(created.ID), nil)
	deleteReq.SetPathValue("id", testutils.IntToString(created.ID))
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteTemplate, deleteReq, nil)

	rr.AssertStatusCode(http.StatusNoContent)

	// Verify it's gone
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/notification-templates/"+testutils.IntToString(created.ID), nil)
	getReq.SetPathValue("id", testutils.IntToString(created.ID))
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetTemplate, getReq, nil)

	getRR.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationTemplateHandler_Delete_NotFound(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	req := testutils.CreateJSONRequest(t, "DELETE", "/api/notification-templates/99999", nil)
	req.SetPathValue("id", "99999")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.DeleteTemplate, req, nil)

	rr.AssertStatusCode(http.StatusNotFound)
}

func TestNotificationTemplateHandler_InvalidID_Scenarios(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()

	tdb.SeedTestData(t)
	handler := NewNotificationTemplateHandlerWithPool(tdb.GetDatabase())

	tests := []struct {
		name    string
		method  string
		handler testutils.TestHandler
	}{
		{"Get invalid ID", "GET", handler.GetTemplate},
		{"Update invalid ID", "PUT", handler.UpdateTemplate},
		{"Delete invalid ID", "DELETE", handler.DeleteTemplate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := testutils.CreateJSONRequest(t, tt.method, "/api/notification-templates/abc",
				models.NotificationTemplate{Name: "test", TemplateType: "header", Content: "c"})
			req.SetPathValue("id", "abc")
			rr := testutils.ExecuteAuthenticatedRequest(t, tt.handler, req, nil)
			rr.AssertStatusCode(http.StatusBadRequest)
		})
	}
}
