// Package portalwebauthn provides WebAuthn registration and authentication
// scoped to portal customers. It mirrors internal/webauthn but operates on a
// separate set of database tables so portal credentials are isolated from
// internal-user credentials.
package portalwebauthn

import (
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	intwebauthn "windshift/internal/webauthn"
)

// Config wraps a go-webauthn instance configured for the portal flow.
//
// Discoverable (passwordless) login requires resident-key credentials. The
// internal config (internal/webauthn/config.go) sets ResidentKey=Preferred,
// which would let an authenticator silently fall back to a non-resident
// credential the customer can't sign in with later. Portal credentials must
// be resident, so we build a dedicated *webauthn.WebAuthn here.
type Config struct {
	webAuthn *webauthn.WebAuthn
}

// NewConfig builds a portal-flavored WebAuthn instance from the shared
// relying-party settings used by the internal webauthn config.
func NewConfig(shared *intwebauthn.Config) (*Config, error) {
	if shared == nil {
		return nil, fmt.Errorf("shared webauthn config is required")
	}

	requireResident := true
	wconfig := &webauthn.Config{
		RPDisplayName:         shared.RPName,
		RPID:                  shared.RPID,
		RPOrigins:             shared.RPOrigins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform,
			RequireResidentKey:      &requireResident,
			ResidentKey:             protocol.ResidentKeyRequirementRequired,
			UserVerification:        protocol.VerificationRequired,
		},
		Debug: shared.Debug,
	}

	wa, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create portal webauthn instance: %w", err)
	}
	return &Config{webAuthn: wa}, nil
}

// WebAuthn returns the underlying go-webauthn instance.
func (c *Config) WebAuthn() *webauthn.WebAuthn { return c.webAuthn }
