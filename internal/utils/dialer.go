package utils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"syscall"
	"time"
)

// blockLocalConnections stores the inverse of the public
// AllowLocalConnections setting so its zero value permits loopback/private
// destinations. Operators can explicitly set --allow-local-connections=false
// or ALLOW_LOCAL_CONNECTIONS=false to enable private-address SSRF blocking.
// The setting is process-wide and is applied once at startup before any
// request is served.
var blockLocalConnections atomic.Bool

// SetAllowLocalConnections sets the process-wide local-connection policy.
// Call once at startup from the resolved config.
func SetAllowLocalConnections(v bool) { blockLocalConnections.Store(!v) }

// AllowLocalConnections reports whether the global loopback/private override is
// enabled.
func AllowLocalConnections() bool { return !blockLocalConnections.Load() }

// ProxyFromEnvironmentUnlessBlocked returns the HTTP(S)_PROXY / NO_PROXY
// proxy for req, but only while local connections are allowed (the default).
// Use it as Transport.Proxy on dialer-backed SSRF-safe transports: a proxy
// resolves and dials the target itself, so the post-resolution blocklist
// cannot see the real destination. When --allow-local-connections=false it
// returns nil and the transport dials directly, preserving the SSRF guard.
func ProxyFromEnvironmentUnlessBlocked(req *http.Request) (*url.URL, error) {
	if !AllowLocalConnections() {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

// ErrBlockedSSRFAddr is returned by SafeNetDialer when the resolved IP
// for a connection is in a non-public range (loopback, RFC1918, link-local,
// CGNAT, multicast, unspecified). Callers can errors.Is(err, ErrBlockedSSRFAddr)
// to surface a clean error to admins instead of leaking dial details.
var ErrBlockedSSRFAddr = errors.New("dial host resolves to a blocked IP range")

// IsBlockedSSRFAddr reports whether ip belongs to a range that user-controlled
// dialing must never reach. It is the conservative superset of IsPrivateIP:
// also rejects unspecified, multicast, and CGNAT 100.64.0.0/10 (covers cloud
// metadata endpoints and other internal-only ranges that some
// IsPrivate-equivalents miss).
//
// deadcode-keep: called by core-tests/internal/utils/dialer_test.go
func IsBlockedSSRFAddr(ip net.IP) bool {
	return IsBlockedSSRFAddrWithAllowedCIDRs(ip, nil)
}

// IsBlockedSSRFAddrWithAllowedCIDRs is IsBlockedSSRFAddr plus an explicit
// operator allowlist for private / CGNAT networks. The allowlist intentionally
// cannot override loopback, unspecified, link-local, or multicast blocks:
// those ranges include local process surfaces and cloud metadata endpoints and
// should not be reachable by server-side, config-driven HTTP clients.
func IsBlockedSSRFAddrWithAllowedCIDRs(ip net.IP, allowedCIDRs []*net.IPNet) bool {
	if ip == nil {
		return true
	}
	// The process-wide setting supersedes per-endpoint CIDR allowlists. Local
	// destinations are allowed by default and blocked only when explicitly
	// configured.
	if AllowLocalConnections() {
		return false
	}
	if isAlwaysBlockedSSRFAddr(ip) {
		return true
	}
	if isPrivateOrCGNAT(ip) {
		return !ipInCIDRs(ip, allowedCIDRs)
	}
	// Encoding-safe: unwrap IPv4-compatible / 6to4 / NAT64 forms and re-check
	// the embedded IPv4, so e.g. ::127.0.0.1 or 2002:0a00:0001:: can't smuggle a
	// blocked target past the predicates above (which only normalize IPv4-mapped).
	if v4 := embeddedIPv4(ip); v4 != nil {
		if isAlwaysBlockedSSRFAddr(v4) {
			return true
		}
		if isPrivateOrCGNAT(v4) {
			return !ipInCIDRs(v4, allowedCIDRs)
		}
	}
	return false
}

func isAlwaysBlockedSSRFAddr(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

func isPrivateOrCGNAT(ip net.IP) bool {
	if ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
		return cgnat != nil && cgnat.Contains(v4)
	}
	return false
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr != nil && cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// SafeNetDialer returns a *net.Dialer that refuses to connect to non-public IPs.
// It uses ControlContext to inspect the resolved address *after* DNS resolution
// but *before* the TCP handshake, so the check is robust against DNS rebinding.
//
// Use this anywhere a config-driven host (SMTP server, IMAP server, webhook
// target, etc.) is dialed by the server: a malicious config setter could
// otherwise point the dialer at 127.0.0.1, 169.254.169.254 (cloud metadata),
// or RFC1918 internal services. The blocklist mirrors IsBlockedSSRFAddr above.
func SafeNetDialer(timeout time.Duration) *net.Dialer {
	return SafeNetDialerWithAllowedCIDRs(timeout, nil)
}

// SafeNetDialerWithAllowedCIDRs returns a SafeNetDialer that permits private /
// CGNAT destination IPs only when they fall inside allowedCIDRs. See
// IsBlockedSSRFAddrWithAllowedCIDRs for the ranges that remain blocked even
// when listed.
func SafeNetDialerWithAllowedCIDRs(timeout time.Duration, allowedCIDRs []*net.IPNet) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		ControlContext: func(_ context.Context, network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("invalid dial address %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("dial host %q did not resolve to an IP", host)
			}
			if IsBlockedSSRFAddrWithAllowedCIDRs(ip, allowedCIDRs) {
				return fmt.Errorf("%w: %s (%s)", ErrBlockedSSRFAddr, ip.String(), network)
			}
			return nil
		},
	}
}
