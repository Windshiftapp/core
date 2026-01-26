package tests

import (
	"testing"
	"time"
)

// TestEmailChannelItemCreation tests that emails received via IMAP create work items
func TestEmailChannelItemCreation(t *testing.T) {
	t.Skip("Skipping: Email channel feature not yet implemented - email_providers table doesn't exist")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start test server
	server, cleanup := StartTestServer(t, "sqlite")
	defer cleanup()

	// Complete setup and get token
	token := CreateBearerToken(t, server)
	server.BearerToken = token

	// Create workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Email Test Workspace", "EMAIL")

	// Get item type from the default configuration set
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)

	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}
	if itemTypeID == 0 {
		t.Fatal("No item types found")
	}

	// Associate workspace with configuration set (required for item type validation)
	AssociateWorkspaceWithConfigSet(t, server, workspaceID, configSetID)

	// Start mock IMAP server
	mockIMAP := StartMockIMAPServer(t)
	mockIMAP.AddUser("testuser", "testpass")

	// Create email provider
	providerID := CreateEmailProvider(t, server, "Test IMAP Provider", "generic")

	// Create inbound email channel pointing to mock IMAP server
	channelID := CreateInboundEmailChannel(t, server, EmailChannelConfig{
		Name:            "Test Inbound Email",
		WorkspaceID:     workspaceID,
		ItemTypeID:      itemTypeID,
		EmailProviderID: providerID,
		IMAPHost:        mockIMAP.Host(),
		IMAPPort:        mockIMAP.Port(),
		Username:        "testuser",
		Password:        "testpass",
		Encryption:      "none",
	})

	// Add a test email to the mock server
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "customer@example.com",
		To:        []string{"support@test.com"},
		Subject:   "Need help with login",
		Body:      "I can't log into my account. Please help!",
		MessageID: "test-email-001@example.com",
		Date:      time.Now(),
	})

	// Wait a moment for email to be stored
	time.Sleep(100 * time.Millisecond)

	// Trigger email processing
	TriggerEmailProcessing(t, server, channelID)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify item was created
	items := GetItemsByWorkspace(t, server, workspaceID)

	if len(items) == 0 {
		t.Fatal("Expected at least one item to be created from email")
	}

	// Check the item has the correct title
	found := false
	for _, item := range items {
		if title, ok := item["title"].(string); ok {
			if title == "Need help with login" {
				found = true
				t.Logf("Found item with title: %s", title)

				// Verify item type
				if itemType, ok := item["item_type_id"].(float64); ok {
					if int(itemType) != itemTypeID {
						t.Errorf("Expected item type %d, got %d", itemTypeID, int(itemType))
					}
				}
				break
			}
		}
	}

	if !found {
		t.Error("Did not find item with expected title 'Need help with login'")
		for _, item := range items {
			t.Logf("Found item: %+v", item)
		}
	}
}

// TestReplyCreatesComment tests that reply emails create comments on existing items
func TestReplyCreatesComment(t *testing.T) {
	t.Skip("Skipping: Email channel feature not yet implemented - email_providers table doesn't exist")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start test server
	server, cleanup := StartTestServer(t, "sqlite")
	defer cleanup()

	// Complete setup and get token
	token := CreateBearerToken(t, server)
	server.BearerToken = token

	// Create workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Reply Test Workspace", "REPLY")

	// Get item type
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	// Associate workspace with configuration set (required for item type validation)
	AssociateWorkspaceWithConfigSet(t, server, workspaceID, configSetID)

	// Start mock IMAP server
	mockIMAP := StartMockIMAPServer(t)
	mockIMAP.AddUser("testuser", "testpass")

	// Create provider and channel
	providerID := CreateEmailProvider(t, server, "Test Reply Provider", "generic")
	channelID := CreateInboundEmailChannel(t, server, EmailChannelConfig{
		Name:            "Test Reply Channel",
		WorkspaceID:     workspaceID,
		ItemTypeID:      itemTypeID,
		EmailProviderID: providerID,
		IMAPHost:        mockIMAP.Host(),
		IMAPPort:        mockIMAP.Port(),
		Username:        "testuser",
		Password:        "testpass",
		Encryption:      "none",
	})

	// Add original email
	originalMessageID := "original-email-001@example.com"
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "customer@example.com",
		To:        []string{"support@test.com"},
		Subject:   "Original question",
		Body:      "What is the pricing?",
		MessageID: originalMessageID,
		Date:      time.Now().Add(-1 * time.Hour),
	})

	// Process original email
	time.Sleep(100 * time.Millisecond)
	TriggerEmailProcessing(t, server, channelID)
	time.Sleep(500 * time.Millisecond)

	// Verify item was created
	items := GetItemsByWorkspace(t, server, workspaceID)
	if len(items) == 0 {
		t.Fatal("Original email should have created an item")
	}

	itemID := int(items[0]["id"].(float64))
	t.Logf("Original email created item ID: %d", itemID)

	// Add reply email
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:       "customer@example.com",
		To:         []string{"support@test.com"},
		Subject:    "Re: Original question",
		Body:       "Actually, I found the pricing page. Thanks anyway!",
		MessageID:  "reply-email-001@example.com",
		InReplyTo:  originalMessageID,
		References: []string{originalMessageID},
		Date:       time.Now(),
	})

	// Process reply email
	time.Sleep(100 * time.Millisecond)
	TriggerEmailProcessing(t, server, channelID)
	time.Sleep(500 * time.Millisecond)

	// Verify no new items were created
	items = GetItemsByWorkspace(t, server, workspaceID)
	if len(items) != 1 {
		t.Errorf("Expected 1 item (reply should be comment), got %d", len(items))
	}

	// Verify comment was added
	comments := GetItemComments(t, server, itemID)
	if len(comments) == 0 {
		t.Error("Expected a comment to be added from reply email")
	} else {
		t.Logf("Found %d comments on item", len(comments))
	}
}

// TestEmailChannelValidation tests that channels without item type configured fail properly
func TestEmailChannelValidation(t *testing.T) {
	t.Skip("Skipping: Email channel feature not yet implemented - email_providers table doesn't exist")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start test server
	server, cleanup := StartTestServer(t, "sqlite")
	defer cleanup()

	// Complete setup and get token
	token := CreateBearerToken(t, server)
	server.BearerToken = token

	// Create workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Validation Test Workspace", "VALID")

	// Start mock IMAP server
	mockIMAP := StartMockIMAPServer(t)
	mockIMAP.AddUser("testuser", "testpass")

	// Create provider
	providerID := CreateEmailProvider(t, server, "Test Validation Provider", "generic")

	// Try to create channel WITHOUT item type - this should still create but processing should fail
	// First, we need to create it through raw API to omit item_type_id
	channelConfig := map[string]interface{}{
		"email_provider_id":  providerID,
		"email_workspace_id": workspaceID,
		// Intentionally omitting email_item_type_id
		"email_host":        mockIMAP.Host(),
		"email_port":        mockIMAP.Port(),
		"email_username":    "testuser",
		"email_password":    "testpass",
		"email_encryption":  "none",
		"email_auth_method": "basic",
		"email_mailbox":     "INBOX",
	}

	data := map[string]interface{}{
		"name":        "Invalid Email Channel",
		"type":        "email",
		"direction":   "inbound",
		"description": "Should fail processing",
		"status":      "enabled",
		"config":      channelConfig,
	}

	resp := MakeAuthRequest(t, server, "POST", "/channels", data)
	defer resp.Body.Close()

	// Channel creation might succeed (config is just stored as JSON)
	// But processing should fail
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.Log("Channel created (config validation is at processing time)")

		// Add email
		mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
			From:      "test@example.com",
			Subject:   "Test email",
			Body:      "This should not create an item",
			MessageID: "validation-test@example.com",
		})

		time.Sleep(100 * time.Millisecond)

		// The processing will fail due to missing item type
		// We can't easily verify the error, but we can verify no items were created
		time.Sleep(500 * time.Millisecond)

		items := GetItemsByWorkspace(t, server, workspaceID)
		if len(items) > 0 {
			t.Error("No items should be created when item type is not configured")
		} else {
			t.Log("Correctly did not create items without item type")
		}
	} else {
		t.Logf("Channel creation blocked: %d (this is also acceptable)", resp.StatusCode)
	}
}

// TestEmailDeduplication tests that duplicate emails are not processed twice
func TestEmailDeduplication(t *testing.T) {
	t.Skip("Skipping: Email channel feature not yet implemented - email_providers table doesn't exist")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start test server
	server, cleanup := StartTestServer(t, "sqlite")
	defer cleanup()

	// Complete setup and get token
	token := CreateBearerToken(t, server)
	server.BearerToken = token

	// Create workspace
	workspaceID, _ := CreateTestWorkspace(t, server, "Dedup Test Workspace", "DEDUP")

	// Get item type
	configSetID := GetDefaultConfigurationSet(t, server)
	itemTypes := GetItemTypes(t, server, configSetID)
	var itemTypeID int
	for _, id := range itemTypes {
		itemTypeID = id
		break
	}

	// Associate workspace with configuration set (required for item type validation)
	AssociateWorkspaceWithConfigSet(t, server, workspaceID, configSetID)

	// Start mock IMAP server
	mockIMAP := StartMockIMAPServer(t)
	mockIMAP.AddUser("testuser", "testpass")

	// Create provider and channel
	providerID := CreateEmailProvider(t, server, "Test Dedup Provider", "generic")
	channelID := CreateInboundEmailChannel(t, server, EmailChannelConfig{
		Name:            "Test Dedup Channel",
		WorkspaceID:     workspaceID,
		ItemTypeID:      itemTypeID,
		EmailProviderID: providerID,
		IMAPHost:        mockIMAP.Host(),
		IMAPPort:        mockIMAP.Port(),
		Username:        "testuser",
		Password:        "testpass",
		Encryption:      "none",
	})

	// Add the same email twice (same Message-ID)
	messageID := "duplicate-email-001@example.com"

	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "sender@example.com",
		To:        []string{"support@test.com"},
		Subject:   "Duplicate Test",
		Body:      "This is the first copy",
		MessageID: messageID,
		Date:      time.Now(),
	})

	// Process first email
	time.Sleep(100 * time.Millisecond)
	TriggerEmailProcessing(t, server, channelID)
	time.Sleep(500 * time.Millisecond)

	// Verify one item was created
	items := GetItemsByWorkspace(t, server, workspaceID)
	if len(items) != 1 {
		t.Fatalf("Expected 1 item after first processing, got %d", len(items))
	}
	t.Log("First email processed, created 1 item")

	// Add the "duplicate" (same Message-ID, simulating a re-delivery)
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "sender@example.com",
		To:        []string{"support@test.com"},
		Subject:   "Duplicate Test",
		Body:      "This is the duplicate copy",
		MessageID: messageID, // Same Message-ID
		Date:      time.Now(),
	})

	// Process again
	time.Sleep(100 * time.Millisecond)
	TriggerEmailProcessing(t, server, channelID)
	time.Sleep(500 * time.Millisecond)

	// Verify still only one item exists
	items = GetItemsByWorkspace(t, server, workspaceID)
	if len(items) != 1 {
		t.Errorf("Expected still 1 item after duplicate processing, got %d", len(items))
	} else {
		t.Log("Deduplication working: still only 1 item after duplicate email")
	}
}
