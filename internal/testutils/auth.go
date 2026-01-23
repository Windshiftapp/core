//go:build test

package testutils

import (
	"context"
	"net/http"
	"testing"
	"time"
	"windshift/internal/contextkeys"
	"windshift/internal/models"
)

// DefaultTestUser returns a standard test user as *models.User
func DefaultTestUser() *models.User {
	return &models.User{
		ID:        1,
		Email:     "test@example.com",
		Username:  "testuser",
		FirstName: "Test",
		LastName:  "User",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// TestUserWithID returns a test user with a specific ID
func TestUserWithID(id int) *models.User {
	return &models.User{
		ID:        id,
		Email:     "test@example.com",
		Username:  "testuser",
		FirstName: "Test",
		LastName:  "User",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// WithAuthContext adds authenticated user to request context
// Uses contextkeys.User to match utils.GetCurrentUser
func WithAuthContext(r *http.Request, user *models.User) *http.Request {
	if user == nil {
		user = DefaultTestUser()
	}
	ctx := context.WithValue(r.Context(), contextkeys.User, user)
	ctx = context.WithValue(ctx, contextkeys.AuthMethod, "test")
	ctx = context.WithValue(ctx, contextkeys.CSRFExempt, true)
	return r.WithContext(ctx)
}

// ExecuteAuthenticatedRequest executes a request with auth context
func ExecuteAuthenticatedRequest(t *testing.T, handler TestHandler, req *http.Request, user *models.User) *ResponseRecorder {
	return ExecuteRequest(t, handler, WithAuthContext(req, user))
}

// CreateAuthenticatedJSONRequest creates a JSON request with auth context
func CreateAuthenticatedJSONRequest(t *testing.T, method, url string, body interface{}, user *models.User) *http.Request {
	req := CreateJSONRequest(t, method, url, body)
	return WithAuthContext(req, user)
}
