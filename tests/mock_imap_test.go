package tests

import (
	"testing"
	"time"

	"windshift/internal/email"
	"windshift/internal/models"
)

// TestMockIMAPServerIncrementalFetch tests that we can fetch only new messages after the last UID
func TestMockIMAPServerIncrementalFetch(t *testing.T) {
	// Start mock server
	mockIMAP := StartMockIMAPServer(t)
	mockIMAP.AddUser("testuser", "testpass")

	// Create generic provider and connect
	provider := email.NewGenericProvider(mockIMAP.Host(), mockIMAP.Port(), "none")
	config := &models.ChannelConfig{
		IMAPHost:       mockIMAP.Host(),
		IMAPPort:       mockIMAP.Port(),
		IMAPUsername:   "testuser",
		IMAPPassword:   "testpass",
		IMAPEncryption: "none",
	}

	client, err := provider.Connect(nil, config)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Add first email
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "sender@example.com",
		Subject:   "First Email",
		Body:      "First body",
		MessageID: "first@example.com",
		Date:      time.Now().Add(-1 * time.Hour),
	})

	// Fetch all messages (since_uid = 0)
	messages1, err := client.FetchMessages("INBOX", 0, 10)
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}
	if len(messages1) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages1))
	}
	firstUID := messages1[0].UID
	t.Logf("First email UID: %d", firstUID)

	// Add second email
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "sender@example.com",
		Subject:   "Second Email",
		Body:      "Second body",
		MessageID: "second@example.com",
		Date:      time.Now(),
	})

	// Fetch only new messages (since_uid = first UID)
	messages2, err := client.FetchMessages("INBOX", firstUID, 10)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}
	t.Logf("Fetched %d new messages after UID %d", len(messages2), firstUID)

	if len(messages2) != 1 {
		t.Errorf("Expected 1 new message, got %d", len(messages2))
		// Fetch all to see what we have
		allMessages, _ := client.FetchMessages("INBOX", 0, 10)
		t.Logf("All messages (%d):", len(allMessages))
		for _, m := range allMessages {
			t.Logf("  UID %d: %s", m.UID, m.Envelope.Subject)
		}
	} else {
		t.Logf("Second email UID: %d, Subject: %s", messages2[0].UID, messages2[0].Envelope.Subject)
		if messages2[0].Envelope.Subject != "Second Email" {
			t.Errorf("Expected 'Second Email', got %s", messages2[0].Envelope.Subject)
		}
	}
}

// TestMockIMAPServerConnection tests that we can connect to and fetch from the mock IMAP server
func TestMockIMAPServerConnection(t *testing.T) {
	// Start mock server
	mockIMAP := StartMockIMAPServer(t)
	mockIMAP.AddUser("testuser", "testpass")

	// Add an email
	mockIMAP.AddEmail("testuser", "INBOX", MockEmail{
		From:      "sender@example.com",
		To:        []string{"recipient@example.com"},
		Subject:   "Test Subject",
		Body:      "Test body content",
		MessageID: "test-123@example.com",
		Date:      time.Now(),
	})

	// Create generic provider
	provider := email.NewGenericProvider(mockIMAP.Host(), mockIMAP.Port(), "none")

	// Test connection with channel config
	config := &models.ChannelConfig{
		IMAPHost:       mockIMAP.Host(),
		IMAPPort:       mockIMAP.Port(),
		IMAPUsername:   "testuser",
		IMAPPassword:   "testpass",
		IMAPEncryption: "none",
	}

	// Connect
	client, err := provider.Connect(nil, config)
	if err != nil {
		t.Fatalf("Failed to connect to mock IMAP server: %v", err)
	}
	defer client.Close()

	t.Log("Connected to mock IMAP server successfully")

	// Try to fetch messages
	messages, err := client.FetchMessages("INBOX", 0, 10)
	if err != nil {
		t.Fatalf("Failed to fetch messages: %v", err)
	}

	t.Logf("Fetched %d messages", len(messages))

	if len(messages) == 0 {
		t.Error("Expected at least one message")
	} else {
		msg := messages[0]
		t.Logf("Message UID: %d", msg.UID)
		if msg.Envelope != nil {
			t.Logf("Subject: %s", msg.Envelope.Subject)
		}
	}
}
