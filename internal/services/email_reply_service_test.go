package services_test

import (
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/services"
	"windshift/internal/smtp"
	"windshift/internal/testutils/factory"
)

// mockSMTPSender implements services.ThreadedEmailSender for testing.
type mockSMTPSender struct {
	configured bool
	sent       []smtp.ThreadedEmailParams
	sendErr    error
}

func (m *mockSMTPSender) IsSMTPConfigured() bool { return m.configured }
func (m *mockSMTPSender) SendThreadedEmail(p smtp.ThreadedEmailParams) error {
	m.sent = append(m.sent, p)
	return m.sendErr
}

// emailReplyTestEnv holds common test data for email reply service tests.
type emailReplyTestEnv struct {
	UserID           int
	WorkspaceID      int
	ItemID           int
	ChannelID        int64
	PortalCustomerID int64
}

// setupEmailReplyTestEnv creates a full environment for email reply tests:
// user, workspace, item, email channel, SMTP channel, portal customer,
// and links the item to the email channel and portal customer.
func setupEmailReplyTestEnv(t *testing.T, db database.Database) emailReplyTestEnv {
	t.Helper()
	f := factory.NewTestFactory(db)
	env, err := f.CreateFullTestEnv()
	if err != nil {
		t.Fatalf("Failed to create test env: %v", err)
	}

	// Create email channel (inbound)
	res, err := db.Exec(`
		INSERT INTO channels (name, type, direction, status)
		VALUES ('Email', 'email', 'inbound', 'enabled')
	`)
	if err != nil {
		t.Fatalf("Failed to create email channel: %v", err)
	}
	emailChannelID, _ := res.LastInsertId()

	// Create SMTP channel (outbound) with config
	_, err = db.Exec(`
		INSERT INTO channels (name, type, direction, status, config)
		VALUES ('SMTP', 'smtp', 'outbound', 'enabled', '{"smtp_host":"smtp.test.com","smtp_port":587,"smtp_from_email":"noreply@test.com","smtp_from_name":"Test App"}')
	`)
	if err != nil {
		t.Fatalf("Failed to create SMTP channel: %v", err)
	}

	// Create portal customer
	res, err = db.Exec(`
		INSERT INTO portal_customers (name, email)
		VALUES ('Customer One', 'customer@example.com')
	`)
	if err != nil {
		t.Fatalf("Failed to create portal customer: %v", err)
	}
	portalCustomerID, _ := res.LastInsertId()

	// Link item to email channel and portal customer
	_, err = db.Exec(`
		UPDATE items SET channel_id = ?, creator_portal_customer_id = ? WHERE id = ?
	`, emailChannelID, portalCustomerID, env.ItemID)
	if err != nil {
		t.Fatalf("Failed to update item: %v", err)
	}

	return emailReplyTestEnv{
		UserID:           env.UserID,
		WorkspaceID:      env.WorkspaceID,
		ItemID:           env.ItemID,
		ChannelID:        emailChannelID,
		PortalCustomerID: portalCustomerID,
	}
}

// insertTrackingRecord inserts an email_message_tracking row for testing.
func insertTrackingRecord(t *testing.T, db database.Database, channelID int64, messageID, subject string, itemID int) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO email_message_tracking (channel_id, message_id, from_email, from_name, subject, item_id, direction, processed_at)
		VALUES (?, ?, 'customer@example.com', 'Customer One', ?, ?, 'inbound', CURRENT_TIMESTAMP)
	`, channelID, messageID, subject, itemID)
	if err != nil {
		t.Fatalf("Failed to insert tracking record: %v", err)
	}
}

func TestEmailReplyService_SkipsPrivateComment(t *testing.T) {
	db := createCommentTestDB(t)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    1,
		AuthorID:  1,
		IsPrivate: true,
		Content:   "private note",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for private comment")
	}
}

func TestEmailReplyService_SkipsPortalCustomerComment(t *testing.T) {
	db := createCommentTestDB(t)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	pcID := 42
	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID:        1,
		ItemID:           1,
		AuthorID:         0,
		PortalCustomerID: &pcID,
		Content:          "customer reply",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for portal customer comment")
	}
}

func TestEmailReplyService_SkipsZeroAuthor(t *testing.T) {
	db := createCommentTestDB(t)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    1,
		AuthorID:  0,
		Content:   "no author",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for zero author")
	}
}

func TestEmailReplyService_SkipsWhenSMTPNotConfigured(t *testing.T) {
	db := createCommentTestDB(t)
	mock := &mockSMTPSender{configured: false}
	svc := services.NewEmailReplyService(db, mock)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    1,
		AuthorID:  1,
		Content:   "test",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent when SMTP not configured")
	}
}

func TestEmailReplyService_SkipsNonExistentItem(t *testing.T) {
	db := createCommentTestDB(t)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    99999,
		AuthorID:  1,
		Content:   "test",
	})
	if err != nil {
		t.Fatalf("Expected nil error for non-existent item, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for non-existent item")
	}
}

func TestEmailReplyService_SkipsItemWithoutChannel(t *testing.T) {
	db := createCommentTestDB(t)
	env := setupCommentTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	// Item has no channel_id set — should skip
	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    env.ItemID,
		AuthorID:  env.UserID,
		Content:   "test",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for item without channel")
	}
}

func TestEmailReplyService_SkipsItemWithoutPortalCustomer(t *testing.T) {
	db := createCommentTestDB(t)
	env := setupCommentTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	// Create an email channel and assign it to the item, but don't set portal customer
	res, err := db.Exec(`
		INSERT INTO channels (name, type, direction, status)
		VALUES ('Email', 'email', 'inbound', 'enabled')
	`)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}
	chID, _ := res.LastInsertId()
	_, err = db.Exec("UPDATE items SET channel_id = ? WHERE id = ?", chID, env.ItemID)
	if err != nil {
		t.Fatalf("Failed to update item: %v", err)
	}

	err = svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    env.ItemID,
		AuthorID:  env.UserID,
		Content:   "test",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for item without portal customer")
	}
}

func TestEmailReplyService_SkipsNonEmailChannel(t *testing.T) {
	db := createCommentTestDB(t)
	env := setupCommentTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	// Create a portal channel (not email type)
	res, err := db.Exec(`
		INSERT INTO channels (name, type, direction, status)
		VALUES ('Portal', 'portal', 'inbound', 'enabled')
	`)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}
	chID, _ := res.LastInsertId()

	// Create portal customer
	res2, err := db.Exec(`
		INSERT INTO portal_customers (name, email)
		VALUES ('Customer', 'cust@example.com')
	`)
	if err != nil {
		t.Fatalf("Failed to create portal customer: %v", err)
	}
	pcID, _ := res2.LastInsertId()

	_, err = db.Exec("UPDATE items SET channel_id = ?, creator_portal_customer_id = ? WHERE id = ?", chID, pcID, env.ItemID)
	if err != nil {
		t.Fatalf("Failed to update item: %v", err)
	}

	err = svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    env.ItemID,
		AuthorID:  env.UserID,
		Content:   "test",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent for non-email channel")
	}
}

func TestEmailReplyService_SkipsWhenNoTrackingRecords(t *testing.T) {
	db := createCommentTestDB(t)
	tenv := setupEmailReplyTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	// No tracking records inserted — should skip
	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 1,
		ItemID:    tenv.ItemID,
		AuthorID:  tenv.UserID,
		Content:   "test",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 0 {
		t.Error("Expected no email sent when no tracking records exist")
	}
}

func TestEmailReplyService_SendsEmailForInternalUserComment(t *testing.T) {
	db := createCommentTestDB(t)
	tenv := setupEmailReplyTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	insertTrackingRecord(t, db, tenv.ChannelID, "<orig-msg-1@example.com>", "Help with login", tenv.ItemID)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 100,
		ItemID:    tenv.ItemID,
		AuthorID:  tenv.UserID,
		Content:   "We are looking into this.",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 1 {
		t.Fatalf("Expected 1 email sent, got %d", len(mock.sent))
	}

	sent := mock.sent[0]

	// Verify recipient
	if sent.ToEmail != "customer@example.com" {
		t.Errorf("Expected ToEmail 'customer@example.com', got '%s'", sent.ToEmail)
	}
	if sent.ToName != "Customer One" {
		t.Errorf("Expected ToName 'Customer One', got '%s'", sent.ToName)
	}

	// Verify subject starts with Re:
	if !strings.HasPrefix(sent.Subject, "Re:") {
		t.Errorf("Expected subject to start with 'Re:', got '%s'", sent.Subject)
	}
	if !strings.Contains(sent.Subject, "Help with login") {
		t.Errorf("Expected subject to contain original subject, got '%s'", sent.Subject)
	}

	// Verify threading headers
	if sent.InReplyTo != "<orig-msg-1@example.com>" {
		t.Errorf("Expected InReplyTo '<orig-msg-1@example.com>', got '%s'", sent.InReplyTo)
	}
	if len(sent.References) != 1 || sent.References[0] != "<orig-msg-1@example.com>" {
		t.Errorf("Expected References to contain original message ID, got %v", sent.References)
	}

	// Verify Message-ID format
	if !strings.HasPrefix(sent.MessageID, "<ws-comment-100@") {
		t.Errorf("Expected MessageID to start with '<ws-comment-100@', got '%s'", sent.MessageID)
	}
	if !strings.HasSuffix(sent.MessageID, ">") {
		t.Errorf("Expected MessageID to end with '>', got '%s'", sent.MessageID)
	}

	// Verify body content
	if !strings.Contains(sent.HTMLBody, "We are looking into this.") {
		t.Error("Expected HTML body to contain comment content")
	}
	if !strings.Contains(sent.TextBody, "We are looking into this.") {
		t.Error("Expected text body to contain comment content")
	}
}

func TestEmailReplyService_RecordsOutboundTracking(t *testing.T) {
	db := createCommentTestDB(t)
	tenv := setupEmailReplyTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	insertTrackingRecord(t, db, tenv.ChannelID, "<orig@example.com>", "Test subject", tenv.ItemID)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 200,
		ItemID:    tenv.ItemID,
		AuthorID:  tenv.UserID,
		Content:   "Reply content",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}

	// Verify outbound tracking record was inserted
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM email_message_tracking
		WHERE item_id = ? AND direction = 'outbound' AND comment_id = 200
	`, tenv.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query tracking: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 outbound tracking record, got %d", count)
	}

	// Verify the outbound Message-ID format
	var msgID string
	err = db.QueryRow(`
		SELECT message_id FROM email_message_tracking
		WHERE item_id = ? AND direction = 'outbound' AND comment_id = 200
	`, tenv.ItemID).Scan(&msgID)
	if err != nil {
		t.Fatalf("Failed to query outbound message_id: %v", err)
	}
	if !strings.HasPrefix(msgID, "<ws-comment-200@") {
		t.Errorf("Expected outbound message_id to start with '<ws-comment-200@', got '%s'", msgID)
	}
}

func TestEmailReplyService_ThreadingWithMultipleMessages(t *testing.T) {
	db := createCommentTestDB(t)
	tenv := setupEmailReplyTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	// Insert 3 tracking records
	insertTrackingRecord(t, db, tenv.ChannelID, "<msg-1@example.com>", "Original subject", tenv.ItemID)
	insertTrackingRecord(t, db, tenv.ChannelID, "<msg-2@example.com>", "", tenv.ItemID)
	insertTrackingRecord(t, db, tenv.ChannelID, "<msg-3@example.com>", "", tenv.ItemID)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 300,
		ItemID:    tenv.ItemID,
		AuthorID:  tenv.UserID,
		Content:   "Threaded reply",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 1 {
		t.Fatalf("Expected 1 email, got %d", len(mock.sent))
	}

	sent := mock.sent[0]

	// In-Reply-To should be the most recent message
	if sent.InReplyTo != "<msg-3@example.com>" {
		t.Errorf("Expected InReplyTo '<msg-3@example.com>', got '%s'", sent.InReplyTo)
	}

	// References should contain all 3 message IDs
	if len(sent.References) != 3 {
		t.Fatalf("Expected 3 references, got %d: %v", len(sent.References), sent.References)
	}
	expected := []string{"<msg-1@example.com>", "<msg-2@example.com>", "<msg-3@example.com>"}
	for i, ref := range sent.References {
		if ref != expected[i] {
			t.Errorf("References[%d]: expected '%s', got '%s'", i, expected[i], ref)
		}
	}

	// Subject should use the first tracking record's subject
	if sent.Subject != "Re: Original subject" {
		t.Errorf("Expected subject 'Re: Original subject', got '%s'", sent.Subject)
	}
}

func TestEmailReplyService_SubjectAlreadyHasRe(t *testing.T) {
	db := createCommentTestDB(t)
	tenv := setupEmailReplyTestEnv(t, db)
	mock := &mockSMTPSender{configured: true}
	svc := services.NewEmailReplyService(db, mock)

	insertTrackingRecord(t, db, tenv.ChannelID, "<msg@example.com>", "Re: Already replied", tenv.ItemID)

	err := svc.HandleCommentCreated(services.HandleCommentParams{
		CommentID: 400,
		ItemID:    tenv.ItemID,
		AuthorID:  tenv.UserID,
		Content:   "Another reply",
	})
	if err != nil {
		t.Fatalf("Expected nil error, got: %v", err)
	}
	if len(mock.sent) != 1 {
		t.Fatalf("Expected 1 email, got %d", len(mock.sent))
	}

	// Subject should NOT get double Re:
	if sent := mock.sent[0]; sent.Subject != "Re: Already replied" {
		t.Errorf("Expected subject 'Re: Already replied' (no double Re:), got '%s'", sent.Subject)
	}
}
