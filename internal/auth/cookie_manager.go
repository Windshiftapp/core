package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/securecookie"
	"golang.org/x/crypto/hkdf"

	"windshift/internal/utils"
)

// cookieManager provides shared cookie handling and secure-request detection
// infrastructure. Both SessionManager and PortalSessionManager embed it.
type cookieManager struct {
	secureCookie      *securecookie.SecureCookie
	useSecure         bool     // Whether to set Secure flag on cookies (true for HTTPS, false for HTTP)
	useProxy          bool     // Whether proxy mode is enabled
	additionalProxies []net.IP // Additional proxy IPs beyond private ranges
}

// newCookieManager creates a cookieManager with HKDF-derived (or random) keys.
// hashInfo and blockInfo are the HKDF info strings used to derive cookie keys,
// allowing different managers to use distinct key material from the same secret.
// last review: ser, 210426
func newCookieManager(useSecureCookies, useProxy bool, additionalProxies []string, cookieSecret, hashInfo, blockInfo string) cookieManager {
	var hashKey, blockKey []byte
	if cookieSecret != "" {
		hashKey = deriveKey(cookieSecret, hashInfo, 64)
		blockKey = deriveKey(cookieSecret, blockInfo, 32)
	} else {
		hashKey = generateSecureKey(64)  // 512-bit key for HMAC
		blockKey = generateSecureKey(32) // 256-bit key for encryption
	}

	// Parse additional proxy IPs (beyond auto-trusted private ranges)
	var additionalIPs []net.IP
	for _, proxyStr := range additionalProxies {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalIPs = append(additionalIPs, ip)
		}
	}

	return cookieManager{
		secureCookie:      securecookie.New(hashKey, blockKey),
		useSecure:         useSecureCookies,
		useProxy:          useProxy,
		additionalProxies: additionalIPs,
	}
}

// generateSecureKey creates a cryptographically secure random key.
// last review: ser, 210426
func generateSecureKey(length int) []byte {
	key := make([]byte, length)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("Failed to generate secure key: %v", err))
	}
	return key
}

// deriveKey deterministically derives a key of the given length from a secret
// using HKDF-SHA256. This allows cookie encryption keys to be stable across
// process restarts when the same secret is provided.
// last review: ser, 210426
func deriveKey(secret, info string, length int) []byte {
	r := hkdf.New(sha256.New, []byte(secret), nil, []byte(info))
	key := make([]byte, length)
	if _, err := io.ReadFull(r, key); err != nil {
		panic(fmt.Sprintf("failed to derive key: %v", err))
	}
	return key
}

// generateSessionToken creates a cryptographically secure session token (32-byte hex).
// last review: ser, 210426
func generateSessionToken() (string, error) {
	bytes := make([]byte, SessionTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// isSecureRequest checks if the request is over HTTPS (either direct or via trusted proxy).
// last review: ser, 210426, NOTE: verbose debug logging
func (cm *cookieManager) isSecureRequest(r *http.Request) bool {
	// Check if request came via HTTPS directly
	if r.TLS != nil {
		slog.Debug("secure request check: TLS present", slog.String("component", "sso"), slog.Bool("result", true))
		return true
	}

	// Check if local HTTPS is enabled
	if cm.useSecure {
		slog.Debug("secure request check: useSecure enabled", slog.String("component", "sso"), slog.Bool("result", true))
		return true
	}

	// Extract direct client IP (not from headers)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	clientIP := net.ParseIP(host)
	if clientIP == nil {
		slog.Debug("secure request check: failed to parse IP", slog.String("component", "sso"), slog.String("remote_addr", host), slog.Bool("result", false)) //nolint:gosec // G706: logging parsed host from r.RemoteAddr
		return false
	}

	// Only trust X-Forwarded-Proto if request comes from a trusted proxy
	isTrusted := utils.IsTrustedProxy(clientIP, cm.useProxy, cm.additionalProxies)
	proto := r.Header.Get("X-Forwarded-Proto")
	if isTrusted {
		result := proto == "https"
		slog.Debug("secure request check: trusted proxy", //nolint:gosec // G706: structured slog with named fields prevents log injection
			slog.String("component", "sso"),
			slog.String("ip", clientIP.String()),
			slog.String("x_forwarded_proto", proto),
			slog.Bool("result", result))
		return result
	}

	slog.Debug("secure request check: untrusted proxy", //nolint:gosec // G706: structured slog with named fields prevents log injection
		slog.String("component", "sso"),
		slog.String("ip", clientIP.String()),
		slog.String("x_forwarded_proto", proto),
		slog.Bool("result", false))
	return false
}

// setSessionCookie encodes and sets a named session cookie.
// last review: ser, 210426, NOTE: verbose debug logging again
func (cm *cookieManager) setSessionCookie(w http.ResponseWriter, r *http.Request, cookieName, token string, maxAge int) error {
	encoded, err := cm.secureCookie.Encode(cookieName, token)
	if err != nil {
		return fmt.Errorf("failed to encode session cookie: %w", err)
	}

	useSecure := cm.isSecureRequest(r)

	slog.Debug("setting session cookie", //nolint:gosec // G706: structured slog with named fields prevents log injection
		slog.String("component", "sso"),
		slog.String("remote_addr", r.RemoteAddr),
		slog.Bool("tls", r.TLS != nil),
		slog.String("x_forwarded_proto", r.Header.Get("X-Forwarded-Proto")),
		slog.Bool("use_secure", useSecure))

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   useSecure, // codeql[go/cookie-secure-not-set] Secure flag is dynamically set via isSecureRequest (TLS / trusted proxy detection)
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, cookie)
	slog.Debug("session cookie set", slog.String("component", "sso"), slog.String("name", cookieName), slog.Bool("secure", useSecure)) //nolint:gosec // G706: structured slog
	return nil
}

// getSessionFromCookie decodes a session token from a named cookie.
// last review: ser, 210426
func (cm *cookieManager) getSessionFromCookie(r *http.Request, cookieName string) (string, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", err
	}

	var token string
	err = cm.secureCookie.Decode(cookieName, cookie.Value, &token)
	if err != nil {
		return "", fmt.Errorf("failed to decode session cookie: %w", err)
	}

	return token, nil
}

// clearSessionCookie removes a named session cookie.
// last review: ser, 210426
func (cm *cookieManager) clearSessionCookie(w http.ResponseWriter, r *http.Request, cookieName string) {
	useSecure := cm.isSecureRequest(r)

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   useSecure, // codeql[go/cookie-secure-not-set] Secure flag is dynamically set via isSecureRequest (TLS / trusted proxy detection)
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// getSessionFromRequest extracts a session token from cookie or Authorization bearer header.
// last review: ser, 210426
func (cm *cookieManager) getSessionFromRequest(r *http.Request, cookieName string) (string, error) {
	// Try cookie first
	token, err := cm.getSessionFromCookie(r, cookieName)
	if err == nil {
		return token, nil
	}

	// Try Authorization header as fallback
	auth := r.Header.Get("Authorization")
	if auth != "" && len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:], nil
	}

	return "", errors.New("no session token found")
}
