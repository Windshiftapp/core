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

// GetClientIP extracts the client IP with proxy validation. Only trusts
// X-Forwarded-For/X-Real-IP headers if the request comes from a trusted proxy.
func (e *IPExtractor) GetClientIP(r *http.Request) string {
	return ExtractClientIP(r, e.useProxy, e.additionalProxies)
}

// ExtractClientIP returns the client IP for r, trusting X-Forwarded-For and
// X-Real-IP only when the request's immediate peer (RemoteAddr) is a trusted
// proxy. This is the canonical extractor; every surface must use it so the IP
// stored at login, the IP compared by session validation, the IP used for rate
// limiting, and the IP written to audit logs all agree.
//
// Forwarded chains are walked right-to-left: each proxy appends the address it
// saw, so entries written by the proxies closest to us are the most
// trustworthy and the leftmost entries are the most likely to be
// client-supplied. Hops that are themselves trusted proxies are skipped, and
// the first untrusted hop is the real client. When every hop is a trusted proxy
// there is no attributable client, so the socket peer is used instead of a
// client-supplied value. A client therefore cannot rotate its own identity by
// injecting trusted-range addresses into the forwarded headers.
func ExtractClientIP(r *http.Request, useProxy bool, additionalProxies []net.IP) string {
	remoteAddr := remoteHost(r.RemoteAddr)
	peer := net.ParseIP(remoteAddr)
	if peer == nil {
		return remoteAddr // Return as-is if parsing fails
	}

	if !IsTrustedProxy(peer, useProxy, additionalProxies) {
		return remoteAddr
	}

	isTrusted := func(ip net.IP) bool {
		return IsTrustedProxy(ip, useProxy, additionalProxies)
	}
	if clientIP := ForwardedClientIP(r.Header.Get("X-Forwarded-For"), isTrusted); clientIP != "" {
		return clientIP
	}

	// X-Real-IP is a single hop. Trust it only when it is not itself a trusted
	// proxy, mirroring the X-Forwarded-For rule; otherwise fall back to the peer.
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if ip := net.ParseIP(realIP); isValidForwardedIP(ip) && !isTrusted(ip) {
			return realIP
		}
	}

	// Fall back to direct connection IP
	return remoteAddr
}

// ForwardedClientIP returns the client IP encoded in an X-Forwarded-For header.
// It walks the chain right-to-left, skipping hops for which isTrusted reports a
// trusted proxy, and returns the first untrusted hop. Malformed, unspecified,
// and multicast entries are ignored. Returns "" when the header holds no usable
// address or when every hop is a trusted proxy, in which case the header cannot
// attribute a client and the caller should fall back to the socket peer.
func ForwardedClientIP(forwarded string, isTrusted func(net.IP) bool) string {
	if strings.TrimSpace(forwarded) == "" {
		return ""
	}

	parts := strings.Split(forwarded, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ipStr := strings.TrimSpace(parts[i])
		ip := net.ParseIP(ipStr)
		if !isValidForwardedIP(ip) {
			continue
		}
		if !isTrusted(ip) {
			return ipStr
		}
	}
	return ""
}

// remoteHost strips the port from a RemoteAddr, handling IPv6 brackets. The
// bare-host fallback covers the rare case where RemoteAddr has no port at all.
func remoteHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// isValidForwardedIP reports whether a forwarded address is usable as a client
// IP: parseable, specified, and not multicast.
func isValidForwardedIP(ip net.IP) bool {
	return ip != nil && !ip.IsUnspecified() && !ip.IsMulticast()
}

// IsPrivateIP checks if an IP is a private/internal address.
//
// It is encoding-safe. Go's net.IP predicates only normalize native IPv4 and
// IPv4-mapped IPv6 (::ffff:a.b.c.d) via To4(); they return false for the other
// transitional forms that embed an IPv4 — IPv4-compatible (::a.b.c.d), 6to4
// (2002:aabb:ccdd::/16), and the NAT64 well-known prefix (64:ff9b::/96). Without
// unwrapping those, a loopback/RFC1918/link-local target can be smuggled past
// the check as e.g. ::127.0.0.1, 2002:0a00:0001:: or 64:ff9b::a9fe:a9fe and then
// routed to the embedded IPv4 at dial time. So we also re-check any embedded
// IPv4 against the private ranges.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if isPrivateRange(ip) {
		return true
	}
	if v4 := embeddedIPv4(ip); v4 != nil {
		return isPrivateRange(v4)
	}
	return false
}

func isPrivateRange(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// embeddedIPv4 returns the IPv4 address embedded in an IPv6 address for the
// transitional forms whose net.IP predicates do NOT already normalize via To4():
// IPv4-compatible (::a.b.c.d), 6to4 (2002::/16) and the NAT64 well-known prefix
// (64:ff9b::/96). It returns nil for native IPv4, IPv4-mapped (::ffff:a.b.c.d,
// already covered by To4), :: and ::1, and any address embedding no IPv4. Range
// checks are re-run on the result so an encoded private/loopback target can't
// slip through. Shared by IsPrivateIP and IsBlockedSSRFAddr.
func embeddedIPv4(ip net.IP) net.IP {
	if ip.To4() != nil {
		// Native IPv4 or IPv4-mapped — the standard predicates already see it.
		return nil
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return nil
	}
	switch {
	case ip16[0] == 0x20 && ip16[1] == 0x02:
		// 6to4 2002:WWXX:YYZZ::/16 embeds W.X.Y.Z at bytes 2..5.
		return net.IPv4(ip16[2], ip16[3], ip16[4], ip16[5]).To4()
	case ip16[0] == 0x00 && ip16[1] == 0x64 && ip16[2] == 0xff && ip16[3] == 0x9b && allZero(ip16[4:12]):
		// NAT64 64:ff9b::/96 embeds the IPv4 in the low 32 bits.
		return net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15]).To4()
	case allZero(ip16[0:12]) && (!allZero(ip16[12:15]) || ip16[15] > 1):
		// IPv4-compatible ::a.b.c.d (deprecated); excludes :: and ::1.
		return net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15]).To4()
	}
	return nil
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
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

// GetClientIP extracts the client IP address from request context.
//
// Prefer the proxy-validated client IP stored by auth middleware. When called
// outside the API middleware chain, this falls back only to RemoteAddr; it must
// not trust X-Forwarded-For/X-Real-IP because there is no trusted-proxy check
// on this deprecated helper.
//
// Deprecated: Prefer IPExtractor.GetClientIP for new code, or rely on the
// middleware-populated context value for audit logging.
func GetClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(contextkeys.ClientIP).(string); ok && ip != "" {
		return ip
	}

	// Fall back to RemoteAddr. SplitHostPort handles "[::1]:5678" correctly;
	// LastIndex(":") would leave the IPv6 brackets in place.
	return remoteHost(r.RemoteAddr)
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

// IsMutationMethod reports whether an HTTP method changes state. Denial
// auditing uses this to avoid flooding the log with high-volume reads.
func IsMutationMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}
