package email

import (
	"context"
	"fmt"

	"windshift/internal/models"
)

// GenericProvider implements basic auth IMAP for self-hosted servers
type GenericProvider struct {
	IMAPHost       string
	IMAPPort       int
	IMAPEncryption string // "ssl", "tls", "starttls", "none"
}

// NewGenericProvider creates a new generic IMAP provider
func NewGenericProvider(host string, port int, encryption string) *GenericProvider {
	if port == 0 {
		port = 993
	}
	if encryption == "" {
		encryption = "ssl"
	}
	return &GenericProvider{
		IMAPHost:       host,
		IMAPPort:       port,
		IMAPEncryption: encryption,
	}
}

// GetType returns the provider type identifier
func (p *GenericProvider) GetType() string {
	return models.EmailProviderTypeGeneric
}

// GetIMAPServer returns the configured IMAP server details
func (p *GenericProvider) GetIMAPServer(config *models.ChannelConfig) (string, int) { //nolint:gocritic // unnamedResult
	// For generic provider, use config from channel if available
	host := config.IMAPHost
	port := config.IMAPPort
	if host == "" {
		host = p.IMAPHost
	}
	if port == 0 {
		port = p.IMAPPort
	}
	return host, port
}

// GetEncryption returns the IMAP encryption setting
func (p *GenericProvider) GetEncryption(config *models.ChannelConfig) string {
	enc := config.IMAPEncryption
	if enc == "" {
		enc = p.IMAPEncryption
	}
	if enc == "" {
		enc = "ssl"
	}
	return enc
}

// Connect establishes an IMAP connection using basic authentication
func (p *GenericProvider) Connect(ctx context.Context, config *models.ChannelConfig) (*Client, error) {
	if config.IMAPUsername == "" {
		return nil, fmt.Errorf("no IMAP username configured")
	}
	if config.IMAPPassword == "" {
		return nil, fmt.Errorf("no IMAP password configured")
	}

	host, port := p.GetIMAPServer(config)
	if host == "" {
		return nil, fmt.Errorf("no IMAP host configured")
	}

	encryption := p.GetEncryption(config)

	client, err := Connect(ConnectOptions{
		Host:       host,
		Port:       port,
		Encryption: encryption,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	if err := client.AuthenticateBasic(config.IMAPUsername, config.IMAPPassword); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("basic authentication failed: %w", err)
	}

	return client, nil
}

// TestConnection tests if the IMAP connection can be established
func (p *GenericProvider) TestConnection(ctx context.Context, config *models.ChannelConfig) error {
	client, err := p.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return nil
}
