package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

type nativeSSOBinding struct {
	challenge string
	state     string
}

func validNativeSSOValue(value string) bool {
	if len(value) != base64.RawURLEncoding.EncodedLen(sha256.Size) {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (b nativeSSOBinding) valid() bool {
	return validNativeSSOValue(b.challenge) && validNativeSSOValue(b.state)
}

// Persist the challenge and app state with the server-side OIDC state.
func (b nativeSSOBinding) redirectRecord() string {
	return nativeRedirectURI + "?" + (url.Values{"challenge": {b.challenge}, "state": {b.state}}).Encode()
}

func parseNativeSSOBinding(record string) (nativeSSOBinding, bool) {
	raw, ok := strings.CutPrefix(record, nativeRedirectURI+"?")
	if !ok {
		return nativeSSOBinding{}, false
	}
	values, err := url.ParseQuery(raw)
	b := nativeSSOBinding{challenge: values.Get("challenge"), state: values.Get("state")}
	if err != nil || !b.valid() || record != b.redirectRecord() {
		return nativeSSOBinding{}, false
	}
	return b, true
}

// Namespace bound lookups so an unbound exchange cannot redeem them.
func nativeAuthLookupKey(code, challenge string) string {
	if challenge == "" {
		return code
	}
	digest := sha256.Sum256([]byte(code + ":" + challenge))
	return "s256:" + base64.RawURLEncoding.EncodeToString(digest[:])
}
