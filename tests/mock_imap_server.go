package tests

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// MockIMAPServer wraps an in-memory IMAP server for testing
type MockIMAPServer struct {
	server    *imapserver.Server
	memServer *imapmemserver.Server
	listener  net.Listener
	port      int
	users     map[string]*imapmemserver.User
	mu        sync.Mutex
	t         *testing.T
}

// MockEmail represents an email to add to the mock server
type MockEmail struct {
	From        string
	To          []string
	Subject     string
	Body        string
	MessageID   string
	InReplyTo   string
	References  []string
	Date        time.Time
	Attachments []MockAttachment
}

// MockAttachment represents an email attachment
type MockAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// StartMockIMAPServer creates and starts a mock IMAP server on a random port.
// The server is automatically cleaned up when the test completes.
func StartMockIMAPServer(t *testing.T) *MockIMAPServer {
	t.Helper()

	// Create in-memory backend
	memServer := imapmemserver.New()

	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	// Create IMAP server
	server := imapserver.New(&imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memServer.NewSession(), nil, nil
		},
		Caps: imap.CapSet{
			imap.CapIMAP4rev1: {},
			imap.CapUIDPlus:   {},
		},
		InsecureAuth: true, // Allow plain text auth for testing
	})

	mock := &MockIMAPServer{
		server:    server,
		memServer: memServer,
		listener:  listener,
		port:      port,
		users:     make(map[string]*imapmemserver.User),
		t:         t,
	}

	// Start serving in background
	go func() {
		if err := server.Serve(listener); err != nil {
			// Ignore errors after test cleanup
			if !mock.isClosed() {
				t.Logf("IMAP server error: %v", err)
			}
		}
	}()

	// Register cleanup
	t.Cleanup(func() {
		mock.Close()
	})

	t.Logf("Mock IMAP server started on port %d", port)
	return mock
}

// isClosed is a helper to check if the server is closed
func (m *MockIMAPServer) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.server == nil
}

// Port returns the port the server is listening on
func (m *MockIMAPServer) Port() int {
	return m.port
}

// Host returns the host address for the server
func (m *MockIMAPServer) Host() string {
	return "127.0.0.1"
}

// AddUser creates a user with the given credentials and an INBOX mailbox
func (m *MockIMAPServer) AddUser(username, password string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user := imapmemserver.NewUser(username, password)

	// Create INBOX mailbox
	if err := user.Create("INBOX", nil); err != nil {
		m.t.Fatalf("Failed to create INBOX for user %s: %v", username, err)
	}

	m.memServer.AddUser(user)
	m.users[username] = user

	m.t.Logf("Added user %s to mock IMAP server", username)
}

// AddEmail adds an email to a user's mailbox
func (m *MockIMAPServer) AddEmail(username, mailbox string, email MockEmail) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[username]
	if !ok {
		m.t.Fatalf("User %s not found", username)
	}

	// Build RFC 5322 formatted email
	rawEmail := m.buildRawEmail(email)

	// Append to mailbox using the User's Append method
	_, err := user.Append(mailbox, &literalReader{bytes.NewReader(rawEmail), int64(len(rawEmail))}, &imap.AppendOptions{
		Time: email.Date,
	})
	if err != nil {
		m.t.Fatalf("Failed to append email to mailbox %s: %v", mailbox, err)
	}

	m.t.Logf("Added email '%s' to %s/%s", email.Subject, username, mailbox)
}

// literalReader wraps a reader to implement imap.LiteralReader
type literalReader struct {
	io.Reader
	size int64
}

func (r *literalReader) Size() int64 {
	return r.size
}

// buildRawEmail builds an RFC 5322 formatted email
func (m *MockIMAPServer) buildRawEmail(email MockEmail) []byte {
	var buf bytes.Buffer

	// Set default date
	date := email.Date
	if date.IsZero() {
		date = time.Now()
	}

	// Write headers
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", date.Format(time.RFC1123Z)))
	buf.WriteString(fmt.Sprintf("From: %s\r\n", email.From))

	if len(email.To) > 0 {
		for _, to := range email.To {
			buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
		}
	}

	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", email.Subject))

	if email.MessageID != "" {
		buf.WriteString(fmt.Sprintf("Message-ID: <%s>\r\n", email.MessageID))
	} else {
		// Generate a default message ID
		buf.WriteString(fmt.Sprintf("Message-ID: <%d.test@mock.imap>\r\n", time.Now().UnixNano()))
	}

	if email.InReplyTo != "" {
		buf.WriteString(fmt.Sprintf("In-Reply-To: <%s>\r\n", email.InReplyTo))
	}

	if len(email.References) > 0 {
		refs := ""
		for _, ref := range email.References {
			if refs != "" {
				refs += " "
			}
			refs += "<" + ref + ">"
		}
		buf.WriteString(fmt.Sprintf("References: %s\r\n", refs))
	}

	// Handle attachments or plain body
	if len(email.Attachments) > 0 {
		boundary := fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
		buf.WriteString(fmt.Sprintf("MIME-Version: 1.0\r\n"))
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
		buf.WriteString("\r\n")

		// Body part
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(email.Body)
		buf.WriteString("\r\n")

		// Attachment parts
		for _, att := range email.Attachments {
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			buf.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.ContentType, att.Filename))
			buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", att.Filename))
			buf.WriteString("Content-Transfer-Encoding: base64\r\n")
			buf.WriteString("\r\n")
			// For simplicity, just write raw bytes (real implementation would base64 encode)
			buf.Write(att.Data)
			buf.WriteString("\r\n")
		}

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		// Simple text body
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(email.Body)
		buf.WriteString("\r\n")
	}

	return buf.Bytes()
}

// Close shuts down the mock IMAP server
func (m *MockIMAPServer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server == nil {
		return nil
	}

	err := m.server.Close()
	m.server = nil
	m.t.Logf("Mock IMAP server closed")
	return err
}
