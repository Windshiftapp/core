// Package ldap provides LDAP directory integration for user synchronization.
package ldap

import (
	"crypto/tls"
	"fmt"
	"log/slog"

	"windshift/internal/models"

	goldap "github.com/go-ldap/ldap/v3"
)

// LDAPUser represents a user entry found in LDAP.
type LDAPUser struct {
	DN          string
	UID         string
	Email       string
	FirstName   string
	LastName    string
	DisplayName string
}

// Client wraps an LDAP connection for searching users and groups.
type Client struct {
	conn   *goldap.Conn
	config *models.LDAPConfig
}

// NewClient creates a new LDAP client and establishes a connection.
func NewClient(config *models.LDAPConfig, bindPassword string) (*Client, error) {
	address := fmt.Sprintf("%s:%d", config.Host, config.Port)

	var conn *goldap.Conn
	var err error

	if config.UseSSL {
		// LDAPS (TLS from the start)
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.SkipTLSVerify, //nolint:gosec // Configurable for development environments
			ServerName:         config.Host,
		}
		conn, err = goldap.DialTLS("tcp", address, tlsConfig)
	} else {
		conn, err = goldap.Dial("tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}

	// STARTTLS if requested (and not already using SSL)
	if config.UseTLS && !config.UseSSL {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.SkipTLSVerify, //nolint:gosec // Configurable for development environments
			ServerName:         config.Host,
		}
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	// Bind with service account
	if err := conn.Bind(config.BindDN, bindPassword); err != nil {
		conn.Close()
		return nil, fmt.Errorf("LDAP bind failed: %w", err)
	}

	return &Client{conn: conn, config: config}, nil
}

// Close closes the LDAP connection.
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// TestConnection verifies the LDAP connection and bind credentials work.
func (c *Client) TestConnection() error {
	// Try a simple search to verify the connection works
	searchReq := goldap.NewSearchRequest(
		c.config.BaseDN,
		goldap.ScopeBaseObject,
		goldap.NeverDerefAliases,
		1, // size limit
		10, // time limit
		false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err := c.conn.Search(searchReq)
	if err != nil {
		return fmt.Errorf("LDAP search test failed: %w", err)
	}

	return nil
}

// SearchUsers searches for users in the LDAP directory.
func (c *Client) SearchUsers() ([]LDAPUser, error) {
	attrs := []string{
		"dn",
		c.config.AttrUsername,
		c.config.AttrEmail,
		c.config.AttrFirstName,
		c.config.AttrLastName,
		c.config.AttrDisplayName,
	}

	searchReq := goldap.NewSearchRequest(
		c.config.BaseDN,
		goldap.ScopeWholeSubtree,
		goldap.NeverDerefAliases,
		0,  // no size limit
		30, // 30 second timeout
		false,
		c.config.UserFilter,
		attrs,
		nil,
	)

	result, err := c.conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("LDAP user search failed: %w", err)
	}

	var users []LDAPUser
	for _, entry := range result.Entries {
		user := LDAPUser{
			DN:          entry.DN,
			UID:         entry.GetAttributeValue(c.config.AttrUsername),
			Email:       entry.GetAttributeValue(c.config.AttrEmail),
			FirstName:   entry.GetAttributeValue(c.config.AttrFirstName),
			LastName:    entry.GetAttributeValue(c.config.AttrLastName),
			DisplayName: entry.GetAttributeValue(c.config.AttrDisplayName),
		}

		// Skip entries without email (required for user creation)
		if user.Email == "" {
			slog.Debug("skipping LDAP user without email", "dn", entry.DN)
			continue
		}

		users = append(users, user)
	}

	return users, nil
}

// AuthenticateUser performs a bind-based authentication for a user.
// It searches for the user by username, then attempts to bind with the provided password.
func (c *Client) AuthenticateUser(username, password string) (*LDAPUser, error) {
	// Search for the user
	filter := fmt.Sprintf("(&%s(%s=%s))", c.config.UserFilter,
		goldap.EscapeFilter(c.config.AttrUsername),
		goldap.EscapeFilter(username))

	searchReq := goldap.NewSearchRequest(
		c.config.BaseDN,
		goldap.ScopeWholeSubtree,
		goldap.NeverDerefAliases,
		1,  // single result
		10, // 10 second timeout
		false,
		filter,
		[]string{
			"dn",
			c.config.AttrUsername,
			c.config.AttrEmail,
			c.config.AttrFirstName,
			c.config.AttrLastName,
			c.config.AttrDisplayName,
		},
		nil,
	)

	result, err := c.conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("LDAP user search failed: %w", err)
	}

	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("user not found in LDAP")
	}

	entry := result.Entries[0]

	// Attempt to bind as the user to verify password
	if err := c.conn.Bind(entry.DN, password); err != nil {
		return nil, fmt.Errorf("LDAP authentication failed: invalid credentials")
	}

	// Re-bind as service account for subsequent operations
	// (Note: caller should create a fresh client if needed)

	return &LDAPUser{
		DN:          entry.DN,
		UID:         entry.GetAttributeValue(c.config.AttrUsername),
		Email:       entry.GetAttributeValue(c.config.AttrEmail),
		FirstName:   entry.GetAttributeValue(c.config.AttrFirstName),
		LastName:    entry.GetAttributeValue(c.config.AttrLastName),
		DisplayName: entry.GetAttributeValue(c.config.AttrDisplayName),
	}, nil
}
