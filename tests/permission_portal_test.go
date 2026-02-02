package tests

import (
	"fmt"
	"net/http"
	"testing"
)

// TestPortalCustomer_CannotAccessInternalEndpoints verifies that a portal session token
// cannot be used to access internal API endpoints. Portal tokens are stored in
// portal_customer_sessions (not api_tokens), so RequireAuth rejects them.
func TestPortalCustomer_CannotAccessInternalEndpoints(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	// Create workspace and item for testing
	workspaceID, _ := CreateTestWorkspace(t, server, "Portal Boundary Test", shortKey("PBT"))
	itemID := CreateTestItem(t, server, workspaceID, "Test Item")

	// Create portal customer with session
	_, portalToken := CreatePortalCustomerWithSession(t, server, "Portal User", "portal@test.com")

	// All internal endpoints should reject portal tokens with 401
	tests := []struct {
		name     string
		method   string
		endpoint string
	}{
		{"GET /items", http.MethodGet, "/items"},
		{"GET /items/{id}", http.MethodGet, fmt.Sprintf("/items/%d", itemID)},
		{"POST /items", http.MethodPost, "/items"},
		{"PUT /items/{id}", http.MethodPut, fmt.Sprintf("/items/%d", itemID)},
		{"DELETE /items/{id}", http.MethodDelete, fmt.Sprintf("/items/%d", itemID)},
		{"GET /workspaces", http.MethodGet, "/workspaces"},
		{"GET /workspaces/{id}", http.MethodGet, fmt.Sprintf("/workspaces/%d", workspaceID)},
		{"GET /users", http.MethodGet, "/users"},
		{"GET /permissions", http.MethodGet, "/permissions"},
		{"GET /channels", http.MethodGet, "/channels"},
		{"GET /configuration-sets", http.MethodGet, "/configuration-sets"},
		{"GET /custom-fields", http.MethodGet, "/custom-fields"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := MakePortalRequest(t, server, portalToken, tc.method, tc.endpoint, nil)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized for portal token on %s %s, got %d",
					tc.method, tc.endpoint, resp.StatusCode)
			}
		})
	}
}

// TestUnauthenticated_CannotAccessInternalEndpoints verifies that requests with no
// authentication are rejected from internal endpoints, while public portal endpoints
// remain accessible.
func TestUnauthenticated_CannotAccessInternalEndpoints(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	workspaceID, _ := CreateTestWorkspace(t, server, "Unauth Boundary Test", shortKey("UBT"))
	portalSlug := SetupPortalChannel(t, server, workspaceID)

	// Internal endpoints should reject unauthenticated requests
	internalTests := []struct {
		name     string
		method   string
		endpoint string
	}{
		{"GET /items", http.MethodGet, "/items"},
		{"POST /items", http.MethodPost, "/items"},
		{"GET /workspaces", http.MethodGet, "/workspaces"},
		{"GET /users", http.MethodGet, "/users"},
		{"GET /permissions", http.MethodGet, "/permissions"},
		{"GET /channels", http.MethodGet, "/channels"},
		{"GET /configuration-sets", http.MethodGet, "/configuration-sets"},
	}

	for _, tc := range internalTests {
		t.Run("Rejected/"+tc.name, func(t *testing.T) {
			resp := MakeUnauthenticatedRequest(t, server, tc.method, tc.endpoint, nil)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized for unauthenticated request on %s %s, got %d",
					tc.method, tc.endpoint, resp.StatusCode)
			}
		})
	}

	// Public portal endpoint should be accessible without auth
	t.Run("Allowed/GET /portal/{slug}", func(t *testing.T) {
		resp := MakeUnauthenticatedRequest(t, server, http.MethodGet, fmt.Sprintf("/portal/%s", portalSlug), nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for public portal endpoint, got %d", resp.StatusCode)
		}
	})
}

// TestPortalCustomer_IDOR_Isolation verifies that portal customer A cannot access
// portal customer B's requests. The portal returns 404 (not 403) to prevent enumeration.
func TestPortalCustomer_IDOR_Isolation(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	workspaceID, _ := CreateTestWorkspace(t, server, "IDOR Test Workspace", shortKey("IDOR"))
	portalSlug := SetupPortalChannel(t, server, workspaceID)

	// Create two portal customers with sessions
	_, tokenA := CreatePortalCustomerWithSession(t, server, "Customer A", "customerA@test.com")
	_, tokenB := CreatePortalCustomerWithSession(t, server, "Customer B", "customerB@test.com")

	// Each customer submits a request (using their token for authentication)
	itemIDA := SubmitPortalRequest(t, server, portalSlug, tokenA, "Request from Customer A")
	itemIDB := SubmitPortalRequest(t, server, portalSlug, tokenB, "Request from Customer B")

	t.Logf("Customer A item: %d, Customer B item: %d", itemIDA, itemIDB)

	// Customer A can see own requests
	t.Run("CustomerA_CanSeeOwnRequests", func(t *testing.T) {
		resp := MakePortalRequest(t, server, tokenA, http.MethodGet,
			fmt.Sprintf("/portal/%s/my-requests", portalSlug), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	// Customer A can see own request detail
	t.Run("CustomerA_CanSeeOwnRequestDetail", func(t *testing.T) {
		resp := MakePortalRequest(t, server, tokenA, http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemIDA), nil)
		defer resp.Body.Close()
		AssertStatusCode(t, resp, http.StatusOK)
	})

	// Customer A CANNOT see Customer B's request detail (expects 404 to prevent enumeration)
	t.Run("CustomerA_CannotSeeCustomerB_RequestDetail", func(t *testing.T) {
		resp := MakePortalRequest(t, server, tokenA, http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemIDB), nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found (IDOR protection), got %d", resp.StatusCode)
		}
	})

	// Customer A CANNOT see Customer B's comments (expects 404)
	t.Run("CustomerA_CannotSeeCustomerB_Comments", func(t *testing.T) {
		resp := MakePortalRequest(t, server, tokenA, http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d/comments", portalSlug, itemIDB), nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found (IDOR protection), got %d", resp.StatusCode)
		}
	})

	// Bidirectional: Customer B CANNOT see Customer A's request detail
	t.Run("CustomerB_CannotSeeCustomerA_RequestDetail", func(t *testing.T) {
		resp := MakePortalRequest(t, server, tokenB, http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemIDA), nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found (IDOR protection), got %d", resp.StatusCode)
		}
	})

	// Bidirectional: Customer B CANNOT see Customer A's comments
	t.Run("CustomerB_CannotSeeCustomerA_Comments", func(t *testing.T) {
		resp := MakePortalRequest(t, server, tokenB, http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d/comments", portalSlug, itemIDA), nil)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found (IDOR protection), got %d", resp.StatusCode)
		}
	})
}

// TestInternalUser_CannotImpersonatePortalCustomer verifies that an internal user's
// bearer token is rejected by portal request-tracking endpoints when the user has
// no linked portal customer.
func TestInternalUser_CannotImpersonatePortalCustomer(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server)

	workspaceID, _ := CreateTestWorkspace(t, server, "Impersonation Test", shortKey("IMP"))
	portalSlug := SetupPortalChannel(t, server, workspaceID)

	// Create portal customer and submit a request
	_, portalToken := CreatePortalCustomerWithSession(t, server, "Real Customer", "real@test.com")
	itemID := SubmitPortalRequest(t, server, portalSlug, portalToken, "Real customer request")

	// Create an internal user (no linked portal customer)
	_, username, password := CreateTestUserWithCredentials(t, server, "internaluser", "internal@test.com")
	internalToken := CreateBearerTokenForUser(t, server, username, password)

	// Internal bearer token should fail on portal request-tracking endpoints
	// because the internal user has no linked portal customer
	portalEndpoints := []struct {
		name     string
		method   string
		endpoint string
	}{
		{"GET /portal/{slug}/my-requests", http.MethodGet,
			fmt.Sprintf("/portal/%s/my-requests", portalSlug)},
		{"GET /portal/{slug}/requests/{itemId}", http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemID)},
		{"GET /portal/{slug}/requests/{itemId}/comments", http.MethodGet,
			fmt.Sprintf("/portal/%s/requests/%d/comments", portalSlug, itemID)},
		{"POST /portal/{slug}/requests/{itemId}/comments", http.MethodPost,
			fmt.Sprintf("/portal/%s/requests/%d/comments", portalSlug, itemID)},
	}

	for _, tc := range portalEndpoints {
		t.Run(tc.name, func(t *testing.T) {
			var body interface{}
			if tc.method == http.MethodPost {
				body = map[string]string{"content": "test comment"}
			}

			resp := MakeAuthRequestWithToken(t, server, internalToken, tc.method, tc.endpoint, body)
			defer resp.Body.Close()

			// The internal user's bearer token passes OptionalAuth (sets user context),
			// but getPortalCustomerID falls back to internal session lookup which requires
			// a session cookie (not bearer token). The bearer token path through
			// GetPortalSessionFromRequest will extract the token, but ValidatePortalSession
			// will fail because the token is an API token, not a portal session token.
			// Expected: 401 Unauthorized
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized for internal token on portal endpoint %s, got %d",
					tc.endpoint, resp.StatusCode)
			}
		})
	}
}
