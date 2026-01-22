//go:build test

// Package testutils provides unified test utilities for the windshift application.
//
// This package consolidates test helpers that were previously scattered across
// multiple packages (internal/database/testutils.go and internal/handlers/testutils/).
//
// # Database Testing
//
// Use CreateTestDB to create an initialized test database:
//
//	tdb := testutils.CreateTestDB(t, true) // true for in-memory
//	defer tdb.Close()
//	data := tdb.SeedTestData(t)
//
// Use CreateFreshDB for testing database initialization:
//
//	tdb := testutils.CreateFreshDB(t, true)
//	defer tdb.Close()
//
// # Mock Services
//
// MockNotificationService records events for verification:
//
//	notifSvc := testutils.CreateMockNotificationService()
//	// ... use in handler ...
//	if notifSvc.EventCount() != 1 {
//	    t.Error("expected one event")
//	}
//
// # HTTP Testing
//
// Use CreateJSONRequest and ExecuteRequest for handler tests:
//
//	req := testutils.CreateJSONRequest(t, "POST", "/items", body)
//	rr := testutils.ExecuteRequest(t, handler.Create, req)
//	rr.AssertStatusCode(http.StatusCreated)
//
// # Authentication
//
// Use WithAuthContext to add auth context to requests:
//
//	req := testutils.CreateJSONRequest(t, "GET", "/items", nil)
//	req = testutils.WithAuthContext(req, testutils.DefaultTestUser())
//
// Or use the helper that combines both:
//
//	rr := testutils.ExecuteAuthenticatedRequest(t, handler.List, req, nil)
package testutils
