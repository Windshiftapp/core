package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"

	"windshift/internal/licensing"
	"windshift/internal/plugins"
)

// resolvePluginLicenseVerifier decides plugin license enforcement at startup.
//
// An empty public key means enforcement is intentionally off, and every plugin
// contributes its capabilities. A configured key must yield a working verifier
// bound to this install's instance identity: a malformed or wrong-sized key, or
// a missing identity, is an error the caller must turn into a startup failure.
// Enforcement never silently degrades to off, because enforcement-off grants
// every plugin its capabilities.
func resolvePluginLicenseVerifier(licensePubKey, instanceID string, instanceErr error) (plugins.LicenseVerifier, error) {
	if licensePubKey == "" {
		return nil, nil
	}

	publicKey, err := base64.StdEncoding.DecodeString(licensePubKey)
	if err != nil {
		return nil, fmt.Errorf("PLUGIN_LICENSE_PUBKEY is not valid base64: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("PLUGIN_LICENSE_PUBKEY must decode to a %d-byte Ed25519 public key, got %d bytes",
			ed25519.PublicKeySize, len(publicKey))
	}
	if instanceErr != nil {
		return nil, fmt.Errorf("PLUGIN_LICENSE_PUBKEY is configured but the instance identity is unavailable: %w", instanceErr)
	}

	return licensing.MakeVerifier(ed25519.PublicKey(publicKey), instanceID), nil
}
