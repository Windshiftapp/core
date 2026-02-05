package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestPortalSecurity(t *testing.T) {
	server, _ := StartTestServer(t, "sqlite")
	CreateBearerToken(t, server) // Admin token for setup

	// Setup shared resources
	timestamp := time.Now().Unix()
	workspaceID, _ := CreateTestWorkspace(t, server, "Portal Security Workspace", shortKey("SEC"))
	portalSlug := SetupPortalChannel(t, server, workspaceID)

	t.Run("PortalTokenCannotAccessInternalAPIs", func(t *testing.T) {
		// 1. Create a portal session
		_, portalToken := CreatePortalCustomerWithSession(t, server, "Hacker Portal User", "hacker@example.com")

		// 2. Try to access internal items API
		endpoint := "/items"
		resp := MakePortalRequest(t, server, portalToken, http.MethodGet, endpoint, nil)
		defer resp.Body.Close()

		// 3. Verify it is rejected
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Portal token was allowed to access internal API! Expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("PortalCustomerCannotAccessOtherCustomersRequests", func(t *testing.T) {
		// 1. Create Customer A and their request
		emailA := fmt.Sprintf("customerA-%d@example.com", timestamp)
		customerIDA, tokenA := CreatePortalCustomerWithSession(t, server, "Customer A", emailA)
		itemID_A := SubmitPortalRequest(t, server, portalSlug, tokenA, "Request A")

		// Verify ownership of Item A
		// We can't check DB directly easily from here without duplicating logic,
		// but we can check if Customer A can see it.
		respVerify := MakePortalRequest(t, server, tokenA, http.MethodGet, fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemID_A), nil)
		defer respVerify.Body.Close()
		if respVerify.StatusCode != http.StatusOK {
			t.Fatalf("Setup failed: Customer A cannot see their own request. Status: %d. CustomerID: %d, ItemID: %d", respVerify.StatusCode, customerIDA, itemID_A)
		}

		// 2. Create Customer B
		emailB := fmt.Sprintf("customerB-%d@example.com", timestamp)
		_, tokenB := CreatePortalCustomerWithSession(t, server, "Customer B", emailB)

		// 3. Customer B tries to access Item A
		resp := MakePortalRequest(t, server, tokenB, http.MethodGet, fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemID_A), nil)
		defer resp.Body.Close()

		// 4. Verify it is rejected (404 Not Found is expected to hide existence)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Customer B could access Customer A's request! Expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("PortalCustomerCanEditOwnRequest", func(t *testing.T) {
		t.Skip("Feature not yet implemented - portal request edit endpoint doesn't exist")
	})
}
