//go:build test

package testutils

import (
	"net/http"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
)

// MockNotificationService is a mock notification service for testing.
// It records emitted events for verification in tests.
// Uses interface{} for the event parameter to avoid import cycles.
type MockNotificationService struct {
	Events []interface{}
}

// EmitEvent implements the notification service interface.
// Accepts interface{} to avoid import cycles with services package.
func (m *MockNotificationService) EmitEvent(event interface{}) {
	m.Events = append(m.Events, event)
}

// ForceRefreshCache implements the notification service interface for configuration set handlers
func (m *MockNotificationService) ForceRefreshCache() error {
	return nil
}

// Reset clears all recorded events
func (m *MockNotificationService) Reset() {
	m.Events = nil
}

// LastEvent returns the most recently emitted event, or nil if none
func (m *MockNotificationService) LastEvent() interface{} {
	if len(m.Events) == 0 {
		return nil
	}
	return m.Events[len(m.Events)-1]
}

// EventCount returns the number of events emitted
func (m *MockNotificationService) EventCount() int {
	return len(m.Events)
}

// CreateMockNotificationService creates a mock notification service for handlers
func CreateMockNotificationService() *MockNotificationService {
	return &MockNotificationService{}
}

// MockAuthMiddleware is a no-op auth middleware for testing
type MockAuthMiddleware struct {
	SetupCompleted bool
}

// MarkSetupCompleted implements the AuthMiddleware interface
func (m *MockAuthMiddleware) MarkSetupCompleted() {
	m.SetupCompleted = true
}

// CreateMockAuthMiddleware creates a mock auth middleware for setup handler tests
func CreateMockAuthMiddleware() *MockAuthMiddleware {
	return &MockAuthMiddleware{}
}

// PermissionService interface for testing - avoids circular import
type PermissionService interface {
	Close()
}

// ActivityTracker interface for testing - avoids circular import
type ActivityTracker interface {
	Close()
}

// createPermissionService is a variable that can be set by the services package for testing
var createPermissionService func(db database.Database) (PermissionService, error)

// createActivityTracker is a variable that can be set by the services package for testing
var createActivityTracker func(db database.Database) (ActivityTracker, error)

// SetServiceFactories allows the services package to provide factory functions
func SetServiceFactories(permFactory func(db database.Database) (PermissionService, error), actFactory func(db database.Database) (ActivityTracker, error)) {
	createPermissionService = permFactory
	createActivityTracker = actFactory
}

// MockSessionManager is a test double for auth.SessionManager
type MockSessionManager struct {
	CreateSessionFunc    func(userID int, clientIP, userAgent string, rememberMe bool) (*auth.Session, error)
	SetSessionCookieFunc func(w http.ResponseWriter, r *http.Request, token string, rememberMe bool) error
}

// CreateSession implements the SessionCreator interface
func (m *MockSessionManager) CreateSession(userID int, clientIP, userAgent string, rememberMe bool) (*auth.Session, error) {
	if m.CreateSessionFunc != nil {
		return m.CreateSessionFunc(userID, clientIP, userAgent, rememberMe)
	}
	return &auth.Session{
		ID:        1,
		UserID:    userID,
		Token:     "test-session-token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		IPAddress: clientIP,
		UserAgent: userAgent,
		IsActive:  true,
		CreatedAt: time.Now(),
	}, nil
}

// SetSessionCookie implements the SessionCreator interface
func (m *MockSessionManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, rememberMe bool) error {
	if m.SetSessionCookieFunc != nil {
		return m.SetSessionCookieFunc(w, r, token, rememberMe)
	}
	return nil
}

// CreateMockSessionManager returns a default mock session manager
func CreateMockSessionManager() *MockSessionManager {
	return &MockSessionManager{}
}
