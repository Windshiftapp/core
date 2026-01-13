package email

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// Client wraps an IMAP client connection
type Client struct {
	client *imapclient.Client
	host   string
	port   int
}

// ConnectOptions configures IMAP connection
type ConnectOptions struct {
	Host       string
	Port       int
	Encryption string // "ssl", "tls", "none"
	Timeout    time.Duration
}

// DefaultConnectOptions returns default connection options
func DefaultConnectOptions() ConnectOptions {
	return ConnectOptions{
		Port:       993,
		Encryption: "ssl",
		Timeout:    30 * time.Second,
	}
}

// Connect establishes an IMAP connection
func Connect(opts ConnectOptions) (*Client, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)

	var conn net.Conn
	var err error

	var client *imapclient.Client

	clientOpts := &imapclient.Options{
		WordDecoder: nil, // Use default
	}

	switch opts.Encryption {
	case "ssl", "tls":
		// Direct TLS connection (port 993)
		dialer := &net.Dialer{Timeout: opts.Timeout}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: opts.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
		}
		client = imapclient.New(conn, clientOpts)

	case "starttls":
		// Plain connection with STARTTLS upgrade (port 143)
		conn, err = net.DialTimeout("tcp", addr, opts.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
		}
		client, err = imapclient.NewStartTLS(conn, clientOpts)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("STARTTLS failed: %w", err)
		}

	default:
		// Plain connection (not recommended)
		conn, err = net.DialTimeout("tcp", addr, opts.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
		}
		client = imapclient.New(conn, clientOpts)
	}

	return &Client{
		client: client,
		host:   opts.Host,
		port:   opts.Port,
	}, nil
}

// AuthenticateBasic performs basic username/password authentication
func (c *Client) AuthenticateBasic(username, password string) error {
	if err := c.client.Login(username, password).Wait(); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	return nil
}

// AuthenticateXOAuth2 performs XOAUTH2 SASL authentication (for OAuth providers)
func (c *Client) AuthenticateXOAuth2(email, accessToken string) error {
	// Try OAUTHBEARER first (RFC 7628), then fall back to XOAUTH2
	// Both mechanisms are supported by the same sasl.NewOAuthBearerClient

	// Build XOAUTH2 SASL client (legacy but widely supported by O365/Gmail)
	xoauth2Client := newXOAuth2Client(email, accessToken)
	if err := c.client.Authenticate(xoauth2Client); err != nil {
		// If XOAUTH2 fails, try OAUTHBEARER
		saslClient := sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
			Username: email,
			Token:    accessToken,
		})
		if err2 := c.client.Authenticate(saslClient); err2 != nil {
			return fmt.Errorf("XOAUTH2 authentication failed: %w (OAUTHBEARER also failed: %v)", err, err2)
		}
		return nil
	}

	return nil
}

// SelectMailbox selects a mailbox and returns its status
func (c *Client) SelectMailbox(name string) (*imap.SelectData, error) {
	data, err := c.client.Select(name, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select mailbox %s: %w", name, err)
	}
	return data, nil
}

// FetchMessages fetches messages with UID greater than sinceUID
func (c *Client) FetchMessages(mailbox string, sinceUID uint32, batchSize int) ([]*FetchedMessage, error) {
	// Select mailbox
	_, err := c.SelectMailbox(mailbox)
	if err != nil {
		return nil, err
	}

	// Search for messages with UID > sinceUID
	var searchCriteria *imap.SearchCriteria
	if sinceUID > 0 {
		searchCriteria = &imap.SearchCriteria{
			UID: []imap.UIDSet{{
				imap.UIDRange{Start: imap.UID(sinceUID + 1), Stop: 0}, // 0 means * (max)
			}},
		}
	} else {
		// Fetch all messages
		searchCriteria = &imap.SearchCriteria{}
	}

	searchData, err := c.client.UIDSearch(searchCriteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(searchData.AllUIDs()) == 0 {
		return nil, nil
	}

	uids := searchData.AllUIDs()
	slog.Info("found messages to fetch", "count", len(uids), "since_uid", sinceUID)

	// Limit batch size
	if batchSize > 0 && len(uids) > batchSize {
		uids = uids[:batchSize]
	}

	// Build UID set
	uidSet := imap.UIDSet{}
	for _, uid := range uids {
		uidSet = append(uidSet, imap.UIDRange{Start: uid, Stop: uid})
	}

	// Fetch messages
	fetchOptions := &imap.FetchOptions{
		UID:      true,
		Envelope: true,
		Flags:    true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierHeader},
			{Specifier: imap.PartSpecifierText},
		},
	}

	fetchCmd := c.client.Fetch(uidSet, fetchOptions)
	defer fetchCmd.Close()

	var messages []*FetchedMessage
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		fetched, err := parseFetchedMessage(msg)
		if err != nil {
			slog.Error("failed to parse message", "error", err)
			continue
		}
		messages = append(messages, fetched)
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return messages, nil
}

// FetchedMessage represents a raw fetched IMAP message
type FetchedMessage struct {
	UID      uint32
	Envelope *imap.Envelope
	Flags    []imap.Flag
	Header   []byte
	Body     []byte
}

func parseFetchedMessage(msg *imapclient.FetchMessageData) (*FetchedMessage, error) {
	// Collect all fetch items into a buffer for easier access
	buf, err := msg.Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to collect message data: %w", err)
	}

	fetched := &FetchedMessage{
		UID:      uint32(buf.UID),
		Envelope: buf.Envelope,
		Flags:    buf.Flags,
	}

	// Extract header and body from body sections
	for _, section := range buf.BodySection {
		if section.Section.Specifier == imap.PartSpecifierHeader {
			fetched.Header = section.Bytes
		} else if section.Section.Specifier == imap.PartSpecifierText {
			fetched.Body = section.Bytes
		}
	}

	return fetched, nil
}

// MarkAsRead marks a message as read (adds \Seen flag)
func (c *Client) MarkAsRead(uid uint32) error {
	uidSet := imap.UIDSet{imap.UIDRange{Start: imap.UID(uid), Stop: imap.UID(uid)}}
	flags := []imap.Flag{imap.FlagSeen}

	storeCmd := c.client.Store(uidSet, &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: flags,
	}, nil)

	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("failed to mark message as read: %w", err)
	}
	return nil
}

// DeleteMessage marks a message for deletion (adds \Deleted flag)
func (c *Client) DeleteMessage(uid uint32) error {
	uidSet := imap.UIDSet{imap.UIDRange{Start: imap.UID(uid), Stop: imap.UID(uid)}}
	flags := []imap.Flag{imap.FlagDeleted}

	storeCmd := c.client.Store(uidSet, &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: flags,
	}, nil)

	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	return nil
}

// Expunge permanently removes messages marked for deletion
func (c *Client) Expunge() error {
	expungeCmd := c.client.Expunge()
	if err := expungeCmd.Close(); err != nil {
		return fmt.Errorf("expunge failed: %w", err)
	}
	return nil
}

// Close closes the IMAP connection
func (c *Client) Close() error {
	if c.client != nil {
		c.client.Logout().Wait()
		return c.client.Close()
	}
	return nil
}

// xoauth2Client implements SASL XOAUTH2 mechanism
type xoauth2Client struct {
	email       string
	accessToken string
}

func newXOAuth2Client(email, accessToken string) sasl.Client {
	return &xoauth2Client{
		email:       email,
		accessToken: accessToken,
	}
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	// XOAUTH2 initial response format:
	// user=<email>\x01auth=Bearer <token>\x01\x01
	authString := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", c.email, c.accessToken)
	return "XOAUTH2", []byte(base64.StdEncoding.EncodeToString([]byte(authString))), nil
}

func (c *xoauth2Client) Next(challenge []byte) (response []byte, err error) {
	// XOAUTH2 doesn't have a challenge-response flow
	return nil, nil
}
