//go:build test

package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"windshift/internal/models"
	"windshift/internal/testutils"
	"windshift/internal/testutils/mocks"
)

// errorNotificationService is a mock that returns errors from ForceRefreshCache
type errorNotificationService struct{}

func (e *errorNotificationService) ForceRefreshCache() error {
	return fmt.Errorf("cache refresh failed")
}

func setupNotificationHandler(t *testing.T) (*NotificationHandler, *testutils.TestDB) {
	tdb := testutils.CreateTestDB(t, true)
	tdb.SeedTestData(t)

	manager, err := NewNotificationManager(tdb.GetDatabase())
	if err != nil {
		tdb.Close()
		t.Fatalf("Failed to create notification manager: %v", err)
	}
	t.Cleanup(func() { manager.Stop() })

	service := mocks.CreateMockNotificationService()
	handler := NewNotificationHandler(manager, service)
	return handler, tdb
}

// --- GetNotifications ---

func TestNotificationHandler_GetNotifications_Unauthenticated(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	req := testutils.CreateJSONRequest(t, "GET", "/api/notifications", nil)
	rr := testutils.ExecuteRequest(t, handler.GetNotifications, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}

func TestNotificationHandler_GetNotifications_Empty(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	req := testutils.CreateJSONRequest(t, "GET", "/api/notifications", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var notifications []models.Notification
	rr.AssertJSONResponse(&notifications)

	if len(notifications) != 0 {
		t.Errorf("Expected 0 notifications, got %d", len(notifications))
	}
}

func TestNotificationHandler_GetNotifications_WithData(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	// Create notifications via POST
	for i := 0; i < 3; i++ {
		notification := models.Notification{
			UserID:  1,
			Title:   fmt.Sprintf("Test Notification %d", i+1),
			Message: fmt.Sprintf("Message %d", i+1),
			Type:    "info",
		}
		createReq := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, createReq, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/notifications", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var notifications []models.Notification
	rr.AssertJSONResponse(&notifications)

	if len(notifications) != 3 {
		t.Errorf("Expected 3 notifications, got %d", len(notifications))
	}
}

func TestNotificationHandler_GetNotifications_CustomPagination(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	// Create 5 notifications
	for i := 0; i < 5; i++ {
		notification := models.Notification{
			UserID:  1,
			Title:   fmt.Sprintf("Notification %d", i+1),
			Message: fmt.Sprintf("Message %d", i+1),
			Type:    "info",
		}
		createReq := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, createReq, nil)
	}

	req := testutils.CreateJSONRequest(t, "GET", "/api/notifications?limit=2&offset=1", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, req, nil)

	rr.AssertStatusCode(http.StatusOK)

	var notifications []models.Notification
	rr.AssertJSONResponse(&notifications)

	if len(notifications) != 2 {
		t.Errorf("Expected 2 notifications with limit=2&offset=1, got %d", len(notifications))
	}
}

func TestNotificationHandler_GetNotifications_PaginationBounds(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	// Create 2 notifications
	for i := 0; i < 2; i++ {
		notification := models.Notification{
			UserID:  1,
			Title:   fmt.Sprintf("Notification %d", i+1),
			Message: fmt.Sprintf("Message %d", i+1),
			Type:    "info",
		}
		createReq := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
		testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, createReq, nil)
	}

	// High offset returns empty
	req := testutils.CreateJSONRequest(t, "GET", "/api/notifications?offset=100", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, req, nil)
	rr.AssertStatusCode(http.StatusOK)

	var notifications []models.Notification
	rr.AssertJSONResponse(&notifications)
	if len(notifications) != 0 {
		t.Errorf("Expected 0 notifications with high offset, got %d", len(notifications))
	}

	// Invalid limit/offset uses defaults
	req2 := testutils.CreateJSONRequest(t, "GET", "/api/notifications?limit=abc&offset=xyz", nil)
	rr2 := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, req2, nil)
	rr2.AssertStatusCode(http.StatusOK)

	var notifications2 []models.Notification
	rr2.AssertJSONResponse(&notifications2)
	if len(notifications2) != 2 {
		t.Errorf("Expected 2 notifications with invalid params (defaults), got %d", len(notifications2))
	}
}

// --- CreateNotification ---

func TestNotificationHandler_CreateNotification_Success(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	notification := models.Notification{
		UserID:  1,
		Title:   "New Assignment",
		Message: "You have been assigned to task #42",
		Type:    "assignment",
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, req, nil)

	rr.AssertStatusCode(http.StatusCreated).
		AssertContentType("application/json")

	var response models.Notification
	rr.AssertJSONResponse(&response)

	if response.Title != notification.Title {
		t.Errorf("Expected title %q, got %q", notification.Title, response.Title)
	}
	if response.Message != notification.Message {
		t.Errorf("Expected message %q, got %q", notification.Message, response.Message)
	}
	if response.Timestamp.IsZero() {
		t.Error("Expected timestamp to be auto-set")
	}
}

func TestNotificationHandler_CreateNotification_WithTimestamp(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	customTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	notification := models.Notification{
		UserID:    1,
		Title:     "Scheduled Notification",
		Message:   "This has a custom timestamp",
		Type:      "info",
		Timestamp: customTime,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.Notification
	rr.AssertJSONResponse(&response)

	if !response.Timestamp.Equal(customTime) {
		t.Errorf("Expected timestamp %v, got %v", customTime, response.Timestamp)
	}
}

func TestNotificationHandler_CreateNotification_InvalidJSON(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications", nil)
	req.Body = http.NoBody
	// Write invalid JSON
	req, _ = http.NewRequest("POST", "/api/notifications", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req = testutils.WithAuthContext(req, nil)

	rr := testutils.ExecuteRequest(t, handler.CreateNotification, req)

	rr.AssertStatusCode(http.StatusBadRequest)
}

func TestNotificationHandler_CreateNotification_AllFields(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	notification := models.Notification{
		UserID:    1,
		Title:     "Full Notification",
		Message:   "All fields populated",
		Type:      "comment",
		Avatar:    "JD",
		ActionURL: "/items/42",
		Metadata:  `{"item_id":42,"workspace":"TEST"}`,
	}

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, req, nil)

	rr.AssertStatusCode(http.StatusCreated)

	var response models.Notification
	rr.AssertJSONResponse(&response)

	if response.Avatar != notification.Avatar {
		t.Errorf("Expected avatar %q, got %q", notification.Avatar, response.Avatar)
	}
	if response.ActionURL != notification.ActionURL {
		t.Errorf("Expected action_url %q, got %q", notification.ActionURL, response.ActionURL)
	}
	if response.Metadata != notification.Metadata {
		t.Errorf("Expected metadata %q, got %q", notification.Metadata, response.Metadata)
	}
}

// --- MarkNotificationAsRead ---

func TestNotificationHandler_MarkNotificationAsRead_Success(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	// Create a notification
	notification := models.Notification{
		UserID:  1,
		Title:   "Unread Notification",
		Message: "This should be marked as read",
		Type:    "info",
	}
	createReq := testutils.CreateJSONRequest(t, "POST", "/api/notifications", notification)
	testutils.ExecuteAuthenticatedRequest(t, handler.CreateNotification, createReq, nil)

	// Get the notification ID from cache (CreateNotification returns the pre-AddNotification copy with ID=0,
	// so we need to fetch from GET to get the real cache ID)
	getReq := testutils.CreateJSONRequest(t, "GET", "/api/notifications", nil)
	getRR := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, getReq, nil)

	var notifications []models.Notification
	getRR.AssertJSONResponse(&notifications)

	if len(notifications) == 0 {
		t.Fatal("Expected at least 1 notification")
	}
	notifID := notifications[0].ID

	// Mark as read
	markReq := testutils.CreateJSONRequest(t, "PATCH", fmt.Sprintf("/api/notifications/%d/read", notifID), nil)
	markReq.SetPathValue("id", testutils.IntToString(notifID))
	markRR := testutils.ExecuteAuthenticatedRequest(t, handler.MarkNotificationAsRead, markReq, nil)

	markRR.AssertStatusCode(http.StatusOK)

	// Verify via GET that it's read
	getReq2 := testutils.CreateJSONRequest(t, "GET", "/api/notifications", nil)
	getRR2 := testutils.ExecuteAuthenticatedRequest(t, handler.GetNotifications, getReq2, nil)

	var updated []models.Notification
	getRR2.AssertJSONResponse(&updated)

	found := false
	for _, n := range updated {
		if n.ID == notifID {
			found = true
			if !n.Read {
				t.Error("Expected notification to be marked as read")
			}
		}
	}
	if !found {
		t.Error("Notification not found in GET response after marking as read")
	}
}

func TestNotificationHandler_MarkNotificationAsRead_Unauthenticated(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	req := testutils.CreateJSONRequest(t, "PATCH", "/api/notifications/1/read", nil)
	req.SetPathValue("id", "1")
	rr := testutils.ExecuteRequest(t, handler.MarkNotificationAsRead, req)

	rr.AssertStatusCode(http.StatusUnauthorized)
}

func TestNotificationHandler_MarkNotificationAsRead_InvalidID(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	req := testutils.CreateJSONRequest(t, "PATCH", "/api/notifications/abc/read", nil)
	req.SetPathValue("id", "abc")
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.MarkNotificationAsRead, req, nil)

	rr.AssertStatusCode(http.StatusBadRequest)
}

// --- RefreshCache ---

func TestNotificationHandler_RefreshCache_Success(t *testing.T) {
	handler, tdb := setupNotificationHandler(t)
	defer tdb.Close()

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications/refresh-cache", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.RefreshCache, req, nil)

	rr.AssertStatusCode(http.StatusOK).
		AssertContentType("application/json")

	var response map[string]string
	rr.AssertJSONResponse(&response)

	if !strings.Contains(response["message"], "refreshed successfully") {
		t.Errorf("Expected success message, got %q", response["message"])
	}
}

func TestNotificationHandler_RefreshCache_NoService(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	manager, err := NewNotificationManager(tdb.GetDatabase())
	if err != nil {
		t.Fatalf("Failed to create notification manager: %v", err)
	}
	defer manager.Stop()

	// Create handler with nil service
	handler := NewNotificationHandler(manager, nil)

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications/refresh-cache", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.RefreshCache, req, nil)

	rr.AssertStatusCode(http.StatusInternalServerError)
}

func TestNotificationHandler_RefreshCache_ServiceError(t *testing.T) {
	tdb := testutils.CreateTestDB(t, true)
	defer tdb.Close()
	tdb.SeedTestData(t)

	manager, err := NewNotificationManager(tdb.GetDatabase())
	if err != nil {
		t.Fatalf("Failed to create notification manager: %v", err)
	}
	defer manager.Stop()

	// Create handler with error-returning service
	handler := NewNotificationHandler(manager, &errorNotificationService{})

	req := testutils.CreateJSONRequest(t, "POST", "/api/notifications/refresh-cache", nil)
	rr := testutils.ExecuteAuthenticatedRequest(t, handler.RefreshCache, req, nil)

	rr.AssertStatusCode(http.StatusInternalServerError)
}
