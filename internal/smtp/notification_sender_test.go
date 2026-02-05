package smtp

import (
	"strings"
	"testing"
)

func TestBuildThreadedMimeMessage_ContainsHeaders(t *testing.T) {
	s := &NotificationSMTPSender{}
	params := ThreadedEmailParams{
		ToEmail:    "to@example.com",
		ToName:     "Recipient",
		Subject:    "Re: Test",
		HTMLBody:   "<p>Hello</p>",
		TextBody:   "Hello",
		MessageID:  "<msg-123@example.com>",
		InReplyTo:  "<orig-456@example.com>",
		References: []string{"<orig-456@example.com>", "<reply-789@example.com>"},
	}

	msg := s.buildThreadedMimeMessage("from@example.com", "Sender", params)

	if !strings.Contains(msg, "Message-ID: <msg-123@example.com>") {
		t.Error("Expected Message-ID header in output")
	}
	if !strings.Contains(msg, "In-Reply-To: <orig-456@example.com>") {
		t.Error("Expected In-Reply-To header in output")
	}
	if !strings.Contains(msg, "References: <orig-456@example.com> <reply-789@example.com>") {
		t.Error("Expected References header with both message IDs")
	}
	if !strings.Contains(msg, "Subject: Re: Test") {
		t.Error("Expected Subject header in output")
	}
}

func TestBuildThreadedMimeMessage_OmitsEmptyHeaders(t *testing.T) {
	s := &NotificationSMTPSender{}
	params := ThreadedEmailParams{
		ToEmail:  "to@example.com",
		Subject:  "Test",
		HTMLBody: "<p>Hi</p>",
		TextBody: "Hi",
		// MessageID, InReplyTo, References all empty
	}

	msg := s.buildThreadedMimeMessage("from@example.com", "", params)

	if strings.Contains(msg, "Message-ID:") {
		t.Error("Expected no Message-ID header when MessageID is empty")
	}
	if strings.Contains(msg, "In-Reply-To:") {
		t.Error("Expected no In-Reply-To header when InReplyTo is empty")
	}
	if strings.Contains(msg, "References:") {
		t.Error("Expected no References header when References is empty")
	}
}

func TestBuildThreadedMimeMessage_MultipartStructure(t *testing.T) {
	s := &NotificationSMTPSender{}
	params := ThreadedEmailParams{
		ToEmail:  "to@example.com",
		Subject:  "Test",
		HTMLBody: "<p>HTML content</p>",
		TextBody: "Plain text content",
	}

	msg := s.buildThreadedMimeMessage("from@example.com", "Sender", params)

	if !strings.Contains(msg, "Content-Type: multipart/alternative") {
		t.Error("Expected multipart/alternative content type")
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8") {
		t.Error("Expected text/plain part")
	}
	if !strings.Contains(msg, "Content-Type: text/html; charset=UTF-8") {
		t.Error("Expected text/html part")
	}
	if !strings.Contains(msg, "Plain text content") {
		t.Error("Expected text body content in message")
	}
	if !strings.Contains(msg, "<p>HTML content</p>") {
		t.Error("Expected HTML body content in message")
	}
}

func TestBuildThreadedMimeMessage_FromName(t *testing.T) {
	s := &NotificationSMTPSender{}
	params := ThreadedEmailParams{
		ToEmail:  "to@example.com",
		ToName:   "Recipient Name",
		Subject:  "Test",
		HTMLBody: "<p>Hi</p>",
		TextBody: "Hi",
	}

	t.Run("WithFromName", func(t *testing.T) {
		msg := s.buildThreadedMimeMessage("from@example.com", "Sender Name", params)
		if !strings.Contains(msg, "From: Sender Name <from@example.com>") {
			t.Error("Expected From header with display name")
		}
	})

	t.Run("WithoutFromName", func(t *testing.T) {
		msg := s.buildThreadedMimeMessage("from@example.com", "", params)
		if !strings.Contains(msg, "From: from@example.com") {
			t.Error("Expected From header with email only")
		}
		if strings.Contains(msg, "From:  <from@example.com>") {
			t.Error("Should not have empty name in From header")
		}
	})

	t.Run("ToWithName", func(t *testing.T) {
		msg := s.buildThreadedMimeMessage("from@example.com", "", params)
		if !strings.Contains(msg, "To: Recipient Name <to@example.com>") {
			t.Error("Expected To header with display name")
		}
	})

	t.Run("ToWithoutName", func(t *testing.T) {
		paramsNoName := params
		paramsNoName.ToName = ""
		msg := s.buildThreadedMimeMessage("from@example.com", "", paramsNoName)
		if !strings.Contains(msg, "To: to@example.com") {
			t.Error("Expected To header with email only")
		}
	})
}
