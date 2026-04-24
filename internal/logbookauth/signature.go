// Package logbookauth implements the shared signing scheme used to authenticate
// requests flowing from the windshift main server into the logbook sidecar.
//
// The main server proxy injects trusted X-Logbook-* headers (user identity,
// admin flag, group memberships) and signs them with HMAC-SHA256 using the
// shared SSO_SECRET. The signature binds the identity claims to the HTTP
// method, path, and a per-request nonce, so a captured signature cannot be
// replayed against a different endpoint or reused for the same one. The
// logbook sidecar recomputes the signature before trusting any header value.
package logbookauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// MaxSkew is the maximum allowed difference between the signed timestamp and
// the verifier's wall clock. Requests outside this window are rejected to
// limit replay opportunities.
const MaxSkew = 5 * time.Minute

// SignatureVersion is the prefix of the X-Logbook-Signature value. Bumped to
// v2 when method+path+nonce were added to the canonical. Verifiers refuse any
// other version prefix.
const SignatureVersion = "v2"

// Header names for the signing metadata. Values live in the existing
// X-Logbook-* headers already set by the proxy.
const (
	HeaderTimestamp = "X-Logbook-Timestamp"
	HeaderSignature = "X-Logbook-Signature"
	HeaderNonce     = "X-Logbook-Nonce"
)

// Claims captures the identity + request fields signed on each request.
// Method and Path bind the signature to one endpoint; Nonce prevents replay
// against that same endpoint within the MaxSkew window.
type Claims struct {
	Timestamp int64
	Method    string
	Path      string
	Nonce     string
	UserID    string
	Email     string
	FirstName string
	LastName  string
	IsAdmin   string
	GroupIDs  string
}

// Canonical returns the exact byte sequence that gets HMAC'd. Both signer and
// verifier must build this identically — if you add a field here, bump
// SignatureVersion.
func Canonical(c Claims) string {
	var b strings.Builder
	b.WriteString(SignatureVersion)
	b.WriteByte('\n')
	b.WriteString(strconv.FormatInt(c.Timestamp, 10))
	b.WriteByte('\n')
	b.WriteString(c.Method)
	b.WriteByte('\n')
	b.WriteString(c.Path)
	b.WriteByte('\n')
	b.WriteString(c.Nonce)
	b.WriteByte('\n')
	b.WriteString(c.UserID)
	b.WriteByte('\n')
	b.WriteString(c.Email)
	b.WriteByte('\n')
	b.WriteString(c.FirstName)
	b.WriteByte('\n')
	b.WriteString(c.LastName)
	b.WriteByte('\n')
	b.WriteString(c.IsAdmin)
	b.WriteByte('\n')
	b.WriteString(c.GroupIDs)
	return b.String()
}

// Sign returns the X-Logbook-Signature header value for the given claims.
func Sign(secret string, c Claims) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(Canonical(c)))
	return SignatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature header against the expected HMAC of the claims.
// Returns true only on a version-prefixed, constant-time match.
func Verify(secret, signatureHeader string, c Claims) bool {
	prefix := SignatureVersion + "="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return false
	}
	got, err := hex.DecodeString(signatureHeader[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(Canonical(c)))
	return hmac.Equal(got, mac.Sum(nil))
}

// TimestampFresh reports whether a signed timestamp is within MaxSkew of now.
func TimestampFresh(ts int64, now time.Time) bool {
	delta := now.Unix() - ts
	if delta < 0 {
		delta = -delta
	}
	return delta <= int64(MaxSkew/time.Second)
}
