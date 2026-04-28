package utils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

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
func IsBlockedSSRFAddr(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
		if cgnat != nil && cgnat.Contains(v4) {
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
			if IsBlockedSSRFAddr(ip) {
				return fmt.Errorf("%w: %s (%s)", ErrBlockedSSRFAddr, ip.String(), network)
			}
			return nil
		},
	}
}
