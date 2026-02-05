//go:build test

package email_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/models"
	"windshift/internal/testutils"
	"windshift/internal/testutils/factory"
)

// --- StripSignature unit tests (pure function, no DB) ---

func TestStripSignature_ExplicitDelimiter(t *testing.T) {
	input := "Hello\n\n--\nJohn Smith\nVP Engineering"
	got := email.StripSignature(input)
	want := "Hello"
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_ExplicitDelimiterWithSpace(t *testing.T) {
	input := "Hello\n\n-- \nJohn Smith"
	got := email.StripSignature(input)
	want := "Hello"
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_RegardsSignOff(t *testing.T) {
	input := "Please review.\n\nBest regards,\nJohn Smith\njohn@acme.com\n+1 555-123-4567"
	got := email.StripSignature(input)
	want := "Please review."
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_ThanksSignOff(t *testing.T) {
	input := "Fixed the bug.\n\nThanks,\nBob\n\nBob Johnson\nbob@co.com"
	got := email.StripSignature(input)
	want := "Fixed the bug."
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_CheersSignOff(t *testing.T) {
	input := "Done!\n\nCheers,\nJane"
	got := email.StripSignature(input)
	want := "Done!"
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_FooterSentFromIPhone(t *testing.T) {
	input := "Quick update.\n\nSent from my iPhone"
	got := email.StripSignature(input)
	want := "Quick update."
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_NoSignature(t *testing.T) {
	input := "Just a plain message."
	got := email.StripSignature(input)
	if got != input {
		t.Errorf("StripSignature() = %q, want %q (unchanged)", got, input)
	}
}

func TestStripSignature_ThanksInBodyNotStripped(t *testing.T) {
	input := "Thanks for helping with the deploy. It works."
	got := email.StripSignature(input)
	if got != input {
		t.Errorf("StripSignature() = %q, want %q (unchanged)", got, input)
	}
}

func TestStripSignature_EmptyInput(t *testing.T) {
	got := email.StripSignature("")
	if got != "" {
		t.Errorf("StripSignature(\"\") = %q, want \"\"", got)
	}
}

func TestStripSignature_PipeDelimitedSignature(t *testing.T) {
	input := "Text\n\n--\nJane | PM\nCo | www.co.com"
	got := email.StripSignature(input)
	want := "Text"
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_MultiParagraphNoSignature(t *testing.T) {
	input := "First paragraph with some context about the project.\n\n" +
		"Second paragraph explaining the technical details.\n\n" +
		"Third paragraph with the conclusion and next steps.\n\n" +
		"Fourth paragraph with additional notes for the team."
	got := email.StripSignature(input)
	if got != input {
		t.Errorf("StripSignature() should not modify multi-paragraph text without signature")
	}
}

func TestStripSignature_SincerelySignOff(t *testing.T) {
	input := "Please find attached.\n\nSincerely,\nAlice\nalice@corp.com"
	got := email.StripSignature(input)
	want := "Please find attached."
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

func TestStripSignature_ThankYouSignOff(t *testing.T) {
	input := "I've updated the code.\n\nThank you,\nDave"
	got := email.StripSignature(input)
	want := "I've updated the code."
	if got != want {
		t.Errorf("StripSignature() = %q, want %q", got, want)
	}
}

// --- Processor integration tests ---

// processorTestEnv holds the test environment for processor integration tests.
type processorTestEnv struct {
	db             database.Database
	processor      *email.Processor
	workspaceID    int
	userID         int
	channelID      int
	itemTypeID     int
	config         *models.ChannelConfig
	attachmentPath string
}

// setupProcessorTestEnv creates a full test environment for processor integration tests.
func setupProcessorTestEnv(t *testing.T, withAttachments bool) *processorTestEnv {
	t.Helper()

	tdb := testutils.CreateTestDB(t, true)
	t.Cleanup(func() { tdb.Close() })
	db := tdb.GetDatabase()

	f := factory.NewTestFactory(db)
	userID, workspaceID, err := f.CreateUserAndWorkspace()
	if err != nil {
		t.Fatalf("Failed to create user and workspace: %v", err)
	}

	// Get default item type (seeded by DB init)
	var itemTypeID int
	err = tdb.QueryRow(`SELECT id FROM item_types WHERE is_default = true LIMIT 1`).Scan(&itemTypeID)
	if err != nil {
		t.Fatalf("Failed to find default item type: %v", err)
	}

	// Create email channel
	configJSON, _ := json.Marshal(models.ChannelConfig{
		EmailWorkspaceID: workspaceID,
		EmailItemTypeID:  &itemTypeID,
	})
	result, err := tdb.Exec(`
		INSERT INTO channels (name, type, direction, status, config, created_at, updated_at)
		VALUES ('Test Email', 'imap', 'inbound', 'enabled', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, string(configJSON))
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}
	channelIDInt64, _ := result.LastInsertId()
	channelID := int(channelIDInt64)

	// Create email channel state
	_, err = tdb.Exec(`
		INSERT INTO email_channel_state (channel_id, last_uid)
		VALUES (?, 0)
	`, channelID)
	if err != nil {
		t.Fatalf("Failed to create email channel state: %v", err)
	}

	var attachmentPath string
	if withAttachments {
		attachmentPath = t.TempDir()
	}

	processor := email.NewProcessor(db, attachmentPath)

	config := &models.ChannelConfig{
		EmailWorkspaceID: workspaceID,
		EmailItemTypeID:  &itemTypeID,
	}

	return &processorTestEnv{
		db:             db,
		processor:      processor,
		workspaceID:    workspaceID,
		userID:         userID,
		channelID:      channelID,
		itemTypeID:     itemTypeID,
		config:         config,
		attachmentPath: attachmentPath,
	}
}

func newTestEmail(messageID, subject, body string) *email.ParsedEmail {
	return &email.ParsedEmail{
		UID:       1,
		MessageID: messageID,
		Subject:   subject,
		PlainBody: body,
		From: email.EmailAddress{
			Name:    "Test Sender",
			Address: "sender@example.com",
		},
		To: []email.EmailAddress{
			{Name: "Support", Address: "support@example.com"},
		},
		Date: time.Now(),
	}
}

func newTestReplyEmail(messageID, inReplyTo, body string) *email.ParsedEmail {
	e := newTestEmail(messageID, "Re: Test Subject", body)
	e.InReplyTo = inReplyTo
	e.References = []string{inReplyTo}
	return e
}

func TestProcessor_NewItem_StripsSignature(t *testing.T) {
	env := setupProcessorTestEnv(t, false)
	ctx := context.Background()

	pe := newTestEmail(
		"<sig-test-1@example.com>",
		"Bug Report",
		"The login button is broken.\n\nBest regards,\nJohn Smith\njohn@acme.com",
	)

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}
	if result.Action != email.ActionItemCreated {
		t.Fatalf("Expected ActionItemCreated, got %s", result.Action)
	}

	// Verify the item description has signature stripped
	var description string
	err = env.db.QueryRow(`SELECT description FROM items WHERE id = ?`, *result.ItemID).Scan(&description)
	if err != nil {
		t.Fatalf("Failed to query item: %v", err)
	}
	expected := "The login button is broken."
	if description != expected {
		t.Errorf("Item description = %q, want %q", description, expected)
	}
}

func TestProcessor_Reply_StripsQuotesAndSignature(t *testing.T) {
	env := setupProcessorTestEnv(t, false)
	ctx := context.Background()

	// First create an original item
	original := newTestEmail("<original-1@example.com>", "Help Request", "I need help.")
	origResult, err := env.processor.ProcessEmail(ctx, original, env.channelID, env.config)
	if err != nil {
		t.Fatalf("Failed to create original item: %v", err)
	}

	// Now process a reply with both quoted text and signature
	reply := newTestReplyEmail(
		"<reply-1@example.com>",
		"<original-1@example.com>",
		"Here is the fix.\n\n> I need help.\n\nThanks,\nBob\nbob@co.com",
	)

	result, err := env.processor.ProcessEmail(ctx, reply, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}
	if result.Action != email.ActionCommentAdded {
		t.Fatalf("Expected ActionCommentAdded, got %s", result.Action)
	}
	if *result.ItemID != *origResult.ItemID {
		t.Fatalf("Comment should be on original item %d, got %d", *origResult.ItemID, *result.ItemID)
	}

	// Verify comment content has quotes and signature stripped
	var content string
	err = env.db.QueryRow(`SELECT content FROM comments WHERE id = ?`, *result.CommentID).Scan(&content)
	if err != nil {
		t.Fatalf("Failed to query comment: %v", err)
	}
	expected := "Here is the fix."
	if content != expected {
		t.Errorf("Comment content = %q, want %q", content, expected)
	}
}

func TestProcessor_NewItem_WithAttachments(t *testing.T) {
	env := setupProcessorTestEnv(t, true)
	ctx := context.Background()

	pe := newTestEmail("<att-1@example.com>", "With Attachment", "See attached.")
	pe.Attachments = []email.Attachment{
		{
			Filename:    "report.pdf",
			ContentType: "application/pdf",
			Size:        1024,
			Data:        make([]byte, 1024),
		},
	}

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}

	// Verify attachment record in DB with correct columns
	var filename, origFilename, filePath, mimeType string
	var fileSize int64
	err = env.db.QueryRow(`
		SELECT filename, original_filename, file_path, mime_type, file_size
		FROM attachments WHERE item_id = ?
	`, *result.ItemID).Scan(&filename, &origFilename, &filePath, &mimeType, &fileSize)
	if err != nil {
		t.Fatalf("Failed to query attachment: %v", err)
	}

	if origFilename != "report.pdf" {
		t.Errorf("original_filename = %q, want %q", origFilename, "report.pdf")
	}
	if mimeType != "application/pdf" {
		t.Errorf("mime_type = %q, want %q", mimeType, "application/pdf")
	}
	if fileSize != 1024 {
		t.Errorf("file_size = %d, want %d", fileSize, 1024)
	}
	if filePath == "" {
		t.Error("file_path should not be empty")
	}

	// Verify file exists on disk
	fullPath := filepath.Join(env.attachmentPath, filePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("Attachment file should exist at %s", fullPath)
	}
}

func TestProcessor_Reply_WithAttachments(t *testing.T) {
	env := setupProcessorTestEnv(t, true)
	ctx := context.Background()

	// Create original item
	original := newTestEmail("<orig-att-1@example.com>", "Original", "Original body.")
	origResult, err := env.processor.ProcessEmail(ctx, original, env.channelID, env.config)
	if err != nil {
		t.Fatalf("Failed to create original: %v", err)
	}

	// Reply with attachment
	reply := newTestReplyEmail("<reply-att-1@example.com>", "<orig-att-1@example.com>", "Here is the screenshot.")
	reply.Attachments = []email.Attachment{
		{
			Filename:    "screenshot.png",
			ContentType: "image/png",
			Size:        512,
			Data:        make([]byte, 512),
		},
	}

	_, err = env.processor.ProcessEmail(ctx, reply, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}

	// Attachment should be linked to the original item
	var count int
	err = env.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ?`, *origResult.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 attachment on item %d, got %d", *origResult.ItemID, count)
	}
}

func TestProcessor_Attachments_DisabledSilently(t *testing.T) {
	env := setupProcessorTestEnv(t, false) // no attachment path
	ctx := context.Background()

	pe := newTestEmail("<disabled-att-1@example.com>", "With Attachment", "See attached.")
	pe.Attachments = []email.Attachment{
		{
			Filename:    "report.pdf",
			ContentType: "application/pdf",
			Size:        1024,
			Data:        make([]byte, 1024),
		},
	}

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}
	if result.Action != email.ActionItemCreated {
		t.Fatalf("Expected ActionItemCreated, got %s", result.Action)
	}

	// No attachment records should exist
	var count int
	err = env.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 attachments (disabled), got %d", count)
	}
}

func TestProcessor_Attachments_DisabledViaSetting(t *testing.T) {
	env := setupProcessorTestEnv(t, true) // has attachment path, but setting disabled
	ctx := context.Background()

	// Insert attachment_settings with enabled=false
	_, err := env.db.Exec(`
		INSERT INTO attachment_settings (max_file_size, allowed_mime_types, attachment_path, enabled)
		VALUES (52428800, '', ?, false)
	`, env.attachmentPath)
	if err != nil {
		t.Fatalf("Failed to insert attachment_settings: %v", err)
	}

	pe := newTestEmail("<setting-disabled-1@example.com>", "With Attachment", "See attached.")
	pe.Attachments = []email.Attachment{
		{
			Filename:    "report.pdf",
			ContentType: "application/pdf",
			Size:        1024,
			Data:        make([]byte, 1024),
		},
	}

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}

	var count int
	err = env.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 attachments (setting disabled), got %d", count)
	}
}

func TestProcessor_Attachments_MimeAllowlist(t *testing.T) {
	env := setupProcessorTestEnv(t, true)
	ctx := context.Background()

	// Only allow image/ MIME types
	allowedJSON, _ := json.Marshal([]string{"image/"})
	_, err := env.db.Exec(`
		INSERT INTO attachment_settings (max_file_size, allowed_mime_types, attachment_path, enabled)
		VALUES (52428800, ?, ?, true)
	`, string(allowedJSON), env.attachmentPath)
	if err != nil {
		t.Fatalf("Failed to insert attachment_settings: %v", err)
	}

	pe := newTestEmail("<mime-test-1@example.com>", "Mixed Attachments", "See attached.")
	pe.Attachments = []email.Attachment{
		{
			Filename:    "photo.png",
			ContentType: "image/png",
			Size:        512,
			Data:        make([]byte, 512),
		},
		{
			Filename:    "document.pdf",
			ContentType: "application/pdf",
			Size:        1024,
			Data:        make([]byte, 1024),
		},
	}

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}

	// Only the image should be saved
	var count int
	err = env.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 attachment (image only), got %d", count)
	}

	// Verify it's the image
	var origFilename string
	err = env.db.QueryRow(`SELECT original_filename FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&origFilename)
	if err != nil {
		t.Fatalf("Failed to query attachment: %v", err)
	}
	if origFilename != "photo.png" {
		t.Errorf("Expected saved attachment to be photo.png, got %s", origFilename)
	}
}

func TestProcessor_Attachments_MaxFileSizeEnforced(t *testing.T) {
	env := setupProcessorTestEnv(t, true)
	ctx := context.Background()

	// Set max_file_size to 100 bytes
	_, err := env.db.Exec(`
		INSERT INTO attachment_settings (max_file_size, allowed_mime_types, attachment_path, enabled)
		VALUES (100, '', ?, true)
	`, env.attachmentPath)
	if err != nil {
		t.Fatalf("Failed to insert attachment_settings: %v", err)
	}

	pe := newTestEmail("<size-test-1@example.com>", "Size Test", "See attached.")
	pe.Attachments = []email.Attachment{
		{
			Filename:    "small.txt",
			ContentType: "text/plain",
			Size:        50,
			Data:        make([]byte, 50),
		},
		{
			Filename:    "large.txt",
			ContentType: "text/plain",
			Size:        200,
			Data:        make([]byte, 200),
		},
	}

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}

	// Only the small file should be saved
	var count int
	err = env.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 attachment (small only), got %d", count)
	}

	var origFilename string
	err = env.db.QueryRow(`SELECT original_filename FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&origFilename)
	if err != nil {
		t.Fatalf("Failed to query attachment: %v", err)
	}
	if origFilename != "small.txt" {
		t.Errorf("Expected saved attachment to be small.txt, got %s", origFilename)
	}
}

func TestProcessor_MultipleAttachments(t *testing.T) {
	env := setupProcessorTestEnv(t, true)
	ctx := context.Background()

	pe := newTestEmail("<multi-att-1@example.com>", "Multiple Attachments", "Three files attached.")
	pe.Attachments = []email.Attachment{
		{Filename: "a.txt", ContentType: "text/plain", Size: 10, Data: []byte("aaaaaaaaaa")},
		{Filename: "b.png", ContentType: "image/png", Size: 20, Data: make([]byte, 20)},
		{Filename: "c.pdf", ContentType: "application/pdf", Size: 30, Data: make([]byte, 30)},
	}

	result, err := env.processor.ProcessEmail(ctx, pe, env.channelID, env.config)
	if err != nil {
		t.Fatalf("ProcessEmail failed: %v", err)
	}

	var count int
	err = env.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ?`, *result.ItemID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count attachments: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 attachments, got %d", count)
	}

	// Verify all files exist on disk
	dir := filepath.Join(env.attachmentPath, "items", fmt.Sprintf("%d", *result.ItemID))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read attachment directory: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 files in attachment directory, got %d", len(entries))
	}
}
