// Package webauthn provides WebAuthn configuration and passkey authentication support.
package webauthn

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Config holds the WebAuthn configuration
type Config struct {
	RPID          string   // Relying Party ID (domain)
	RPName        string   // Display name
	RPOrigins     []string // Allowed origins
	Debug         bool     // Debug mode
	webAuthn      *webauthn.WebAuthn
	isDevelopment bool
}

// NewConfig creates a new WebAuthn configuration
func NewConfig(rpID, rpName string, origins []string, isDev bool, allowedHosts, port string, enableHTTPS, useProxy bool) (*Config, error) {
	c := &Config{
		RPID:          rpID,
		RPName:        rpName,
		RPOrigins:     origins,
		isDevelopment: isDev,
		Debug:         isDev,
	}

	// Dev-mode RPID override: production-mode RPID/RPName are pre-resolved by
	// config.Load (env → hostname fallback for RPID, default "Windshift" for
	// RPName), so this package no longer reads env vars itself.
	if c.RPID == "" && c.isDevelopment {
		c.RPID = "localhost"
	}
	if c.RPID == "" {
		return nil, fmt.Errorf("no RP ID provided and not in development mode (config.Load should have resolved this)")
	}
	if c.RPName == "" {
		c.RPName = "Windshift"
	}

	// If no origins provided, derive from configuration
	if len(c.RPOrigins) == 0 {
		if c.isDevelopment {
			// Development mode: Allow both http and https with common ports
			c.RPOrigins = []string{
				fmt.Sprintf("http://%s", c.RPID),
				fmt.Sprintf("http://%s:8080", c.RPID),
				fmt.Sprintf("http://%s:3000", c.RPID),
				fmt.Sprintf("http://%s:5555", c.RPID), // Vite dev server
				fmt.Sprintf("http://%s:5173", c.RPID), // Vite alternate port
				fmt.Sprintf("https://%s", c.RPID),
				"http://localhost",
				"http://localhost:8080",
				"http://localhost:3000",
				"http://localhost:5555", // Vite dev server
				"http://localhost:5173", // Vite alternate port
				"https://localhost",
			}
		} else {
			// Production mode: Infer origins from allowed-hosts
			if allowedHosts == "" {
				return nil, fmt.Errorf("no allowed hosts configured for WebAuthn origin inference")
			}

			// Determine scheme based on TLS configuration or proxy mode
			scheme := "http"
			if enableHTTPS || useProxy {
				scheme = "https"
			}

			// Determine standard port based on scheme
			standardPort := "80"
			if scheme == "https" {
				standardPort = "443"
			}

			// Parse allowed hosts and generate origins
			hosts := strings.Split(allowedHosts, ",")
			c.RPOrigins = make([]string, 0, len(hosts)*2)

			for _, host := range hosts {
				host = strings.TrimSpace(host)
				if host == "" {
					continue
				}

				// Add origin with explicit port and standard port
				c.RPOrigins = append(c.RPOrigins,
					fmt.Sprintf("%s://%s:%s", scheme, host, port),
					fmt.Sprintf("%s://%s:%s", scheme, host, standardPort),
				)
			}

			if len(c.RPOrigins) == 0 {
				return nil, fmt.Errorf("no valid hosts found in allowed-hosts configuration")
			}
		}
	}

	// Validate origins
	for _, origin := range c.RPOrigins {
		if _, err := url.Parse(origin); err != nil {
			return nil, fmt.Errorf("invalid origin %s: %w", origin, err)
		}
	}

	// Create WebAuthn config
	wconfig := &webauthn.Config{
		RPDisplayName: c.RPName,
		RPID:          c.RPID,
		RPOrigins:     c.RPOrigins,
		// Set reasonable defaults
		AttestationPreference: protocol.PreferNoAttestation, // Don't require attestation by default
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform,                        // Prefer platform authenticators (passkeys)
			RequireResidentKey:      &[]bool{false}[0],                        // Don't require resident key
			ResidentKey:             protocol.ResidentKeyRequirementPreferred, // Prefer resident keys for passkeys
			UserVerification:        protocol.VerificationPreferred,           // Prefer user verification
		},
		Debug: c.Debug,
	}

	// Create WebAuthn instance
	wa, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn instance: %w", err)
	}

	c.webAuthn = wa
	return c, nil
}

// WebAuthn returns the underlying WebAuthn instance
func (c *Config) WebAuthn() *webauthn.WebAuthn {
	return c.webAuthn
}

// ConfigForDiscoverableCredentials returns a WebAuthn config for passwordless login
func (c *Config) ConfigForDiscoverableCredentials() (*webauthn.WebAuthn, error) {
	// Create a new config with resident key required for passwordless
	wconfig := &webauthn.Config{
		RPDisplayName:         c.RPName,
		RPID:                  c.RPID,
		RPOrigins:             c.RPOrigins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform,
			RequireResidentKey:      &[]bool{true}[0], // Require resident key for passwordless
			ResidentKey:             protocol.ResidentKeyRequirementRequired,
			UserVerification:        protocol.VerificationRequired, // Require user verification for passwordless
		},
		Debug: c.Debug,
	}

	return webauthn.New(wconfig)
}
