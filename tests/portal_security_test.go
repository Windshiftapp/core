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
		itemID_A := SubmitPortalRequest(t, server, portalSlug, emailA, "Request A")
		
		// Force update the creator_portal_customer_id to ensure the link is correct 
		// (SubmitPortalRequest uses anonymous submission which might create a NEW customer if email doesn't match perfectly,
		// but CreatePortalCustomerWithSession creates one. We need to ensure they match or are linked.
		// Actually, SubmitPortalRequest by default does anonymous submission. 
		// Let's verify that SubmitPortalRequest finds the existing customer by email.)
		
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
		// 1. Create Customer and Request
		email := fmt.Sprintf("editor-%d@example.com", timestamp)
		_, token := CreatePortalCustomerWithSession(t, server, "Editor Customer", email)
		itemID := SubmitPortalRequest(t, server, portalSlug, email, "Original Title")

		// 2. Prepare update data
		updateData := map[string]interface{}{
			"title": "Updated Title",
			"description": "Updated Description",
		}

		// 3. Try to update (PUT)
		resp := MakePortalRequest(t, server, token, http.MethodPut, fmt.Sprintf("/portal/%s/requests/%d", portalSlug, itemID), updateData)
		defer resp.Body.Close()

		// 4. Verify success
		// NOTE: This is expected to FAIL until implementation is complete
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Failed to update own request. Expected 200, got %d. (This confirms feature is missing or broken)", resp.StatusCode)
		}
	})
}
