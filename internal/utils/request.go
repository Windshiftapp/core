package utils

import (
	"net"
	"net/http"
	"strings"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
)

// IPExtractor extracts client IP addresses with proxy validation
type IPExtractor struct {
	useProxy          bool
	additionalProxies []net.IP
}

// NewIPExtractor creates a new IP extractor with proxy configuration
func NewIPExtractor(useProxy bool, additionalProxies []string) *IPExtractor {
	var additionalIPs []net.IP
	for _, proxyStr := range additionalProxies {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalIPs = append(additionalIPs, ip)
		}
	}
	return &IPExtractor{
		useProxy:          useProxy,
		additionalProxies: additionalIPs,
	}
}

// GetClientIP extracts the client IP with proxy validation
// Only trusts X-Forwarded-For/X-Real-IP headers if the request comes from a trusted proxy
func (e *IPExtractor) GetClientIP(r *http.Request) string {
	// Get the immediate client IP (could be proxy)
	remoteAddr := r.RemoteAddr
	if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
		remoteAddr = remoteAddr[:colonIndex]
	}

	clientIP := net.ParseIP(remoteAddr)
	if clientIP == nil {
		return remoteAddr // Return as-is if parsing fails
	}

	// Only trust proxy headers if the request comes from a trusted proxy
	if e.isTrustedProxy(clientIP) {
		// Check X-Forwarded-For header (for proxies)
		forwarded := r.Header.Get("X-Forwarded-For")
		if forwarded != "" {
			// Validate and extract the first (original client) IP
			ips := strings.Split(forwarded, ",")
			for _, ipStr := range ips {
				ipStr = strings.TrimSpace(ipStr)
				if ip := net.ParseIP(ipStr); ip != nil && e.isValidClientIP(ip) {
					return ipStr
				}
			}
		}

		// Check X-Real-IP header
		realIP := r.Header.Get("X-Real-IP")
		if realIP != "" {
			if ip := net.ParseIP(realIP); ip != nil && e.isValidClientIP(ip) {
				return realIP
			}
		}
	}

	// Fall back to direct connection IP
	return remoteAddr
}

// IsPrivateIP checks if an IP is a private/internal address
func IsPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// IsTrustedProxy checks if an IP is a trusted proxy (private IP or in additional list)
// This is the canonical implementation used throughout the codebase.
// Parameters:
//   - ip: the IP address to check
//   - useProxy: whether proxy mode is enabled (if false, always returns false)
//   - additionalProxies: list of additional trusted proxy IPs beyond private ranges
func IsTrustedProxy(ip net.IP, useProxy bool, additionalProxies []net.IP) bool {
	if !useProxy {
		return false // Proxy mode disabled - trust nothing
	}
	if IsPrivateIP(ip) {
		return true
	}
	for _, trustedIP := range additionalProxies {
		if ip.Equal(trustedIP) {
			return true
		}
	}
	return false
}

// isTrustedProxy checks if an IP is a trusted proxy (method wrapper for IPExtractor)
func (e *IPExtractor) isTrustedProxy(ip net.IP) bool {
	return IsTrustedProxy(ip, e.useProxy, e.additionalProxies)
}

// isValidClientIP validates that an IP is valid for a client
func (e *IPExtractor) isValidClientIP(ip net.IP) bool {
	return ip != nil && !ip.IsUnspecified()
}

// GetClientIP extracts the client IP address from request headers.
//
// Deprecated: Use IPExtractor.GetClientIP for secure proxy-aware extraction.
// This function blindly trusts proxy headers and should only be used when
// proxy validation is not required (e.g., internal services).
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP if multiple are present
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	remoteAddr := r.RemoteAddr
	// Remove port if present
	if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
		remoteAddr = remoteAddr[:colonIndex]
	}
	return remoteAddr
}

// GetCurrentUser retrieves the authenticated user from request context
// Returns nil if no user is authenticated
func GetCurrentUser(r *http.Request) *models.User {
	userVal := r.Context().Value(contextkeys.User)
	if userVal == nil {
		return nil
	}

	user, ok := userVal.(*models.User)
	if !ok {
		return nil
	}

	return user
}
