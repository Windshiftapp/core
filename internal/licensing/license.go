// Package licensing verifies Windshift Pro plugin licenses.
//
// A license is a compact signed token: base64url(JSON payload) + "." +
// base64(Ed25519 signature over the raw payload bytes). The payload binds a
// plugin to one installation via the install's instance ID, so a bundle zip
// is useless on any host other than the one the license names. The portal
// holds the license private key and issues tokens; core only ever sees the
// public key (PLUGIN_LICENSE_PUBKEY) and verifies locally — no phone-home.
package licensing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// License binds a plugin to a specific installation.
type License struct {
	Plugin     string     `json:"plugin"`
	InstanceID string     `json:"instance_id"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// Verification errors. ErrorsIs-able so callers can surface precise reasons.
var (
	ErrLicenseMalformed    = errors.New("license token is malformed")
	ErrLicenseBadSignature = errors.New("license signature is invalid")
	ErrLicenseWrongPlugin  = errors.New("license was issued for a different plugin")
	ErrLicenseWrongHost    = errors.New("license was issued for a different instance")
	ErrLicenseExpired      = errors.New("license has expired")
)

// Sign encodes the license and signs the raw payload bytes. The portal is the
// production signer; this exists for tooling and tests.
func Sign(license License, privateKey ed25519.PrivateKey) (string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", errors.New("invalid ed25519 private key size")
	}
	payload, err := json.Marshal(license)
	if err != nil {
		return "", fmt.Errorf("encode license payload: %w", err)
	}
	signature := ed25519.Sign(privateKey, payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.StdEncoding.EncodeToString(signature), nil
}

// Verify checks the token signature against publicKey and returns the decoded
// license. Name/instance/expiry checks are the caller's job (see Validate).
func Verify(token string, publicKey ed25519.PublicKey) (License, error) {
	var license License
	if len(publicKey) != ed25519.PublicKeySize {
		return license, errors.New("invalid ed25519 public key size")
	}

	payloadPart, sigPart, ok := splitToken(token)
	if !ok {
		return license, fmt.Errorf("%w: expected payload.signature", ErrLicenseMalformed)
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return license, fmt.Errorf("%w: payload is not base64url", ErrLicenseMalformed)
	}
	signature, err := base64.StdEncoding.DecodeString(sigPart)
	if err != nil {
		return license, fmt.Errorf("%w: signature is not base64", ErrLicenseMalformed)
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return license, ErrLicenseBadSignature
	}

	if err := json.Unmarshal(payload, &license); err != nil {
		return license, fmt.Errorf("%w: payload is not a license", ErrLicenseMalformed)
	}
	return license, nil
}

// Validate checks a verified license against the expectations of this
// installation: it must name pluginName, carry instanceID, and not be expired.
func Validate(license License, pluginName, instanceID string, now time.Time) error {
	if license.Plugin != pluginName {
		return fmt.Errorf("%w: license names %q", ErrLicenseWrongPlugin, license.Plugin)
	}
	if license.InstanceID != instanceID {
		return ErrLicenseWrongHost
	}
	if license.ExpiresAt != nil && now.After(*license.ExpiresAt) {
		return ErrLicenseExpired
	}
	return nil
}

// MakeVerifier returns a LicenseVerifier that verifies and validates tokens
// for the given plugin and instance. It is the shape the plugin manager and
// upload handler consume; nil disables enforcement.
func MakeVerifier(publicKey ed25519.PublicKey, instanceID string) func(pluginName string, license []byte) error {
	now := time.Now
	return func(pluginName string, licenseBytes []byte) error {
		if len(licenseBytes) == 0 {
			return ErrLicenseMalformed
		}
		license, err := Verify(string(licenseBytes), publicKey)
		if err != nil {
			return err
		}
		return Validate(license, pluginName, instanceID, now())
	}
}

func splitToken(token string) (payload, signature string, ok bool) {
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			return token[:i], token[i+1:], true
		}
	}
	return "", "", false
}
