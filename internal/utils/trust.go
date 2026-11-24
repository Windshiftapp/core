package utils

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// PeerObservationKind classifies a private peer that is not a configured
// trusted proxy.
type PeerObservationKind string

const (
	// PeerKindPrivateClient is a private-address client that reached Windshift
	// directly without forwarding headers. It is correctly identified as the
	// client.
	PeerKindPrivateClient PeerObservationKind = "private_client"
	// PeerKindUnlistedProxy is a peer that sent forwarding headers but is not a
	// configured proxy. Under strict trust the peer is treated as the client and
	// those headers are ignored; if it really is a reverse proxy, its clients
	// collapse to this address and lose Secure-cookie/HSTS detection. The peer
	// can have a public or a private address.
	PeerKindUnlistedProxy PeerObservationKind = "unlisted_proxy"
)

// TrustPolicy decides which peers may supply forwarded headers and resolves the
// client IP for a request. One policy is shared by client-IP extraction, the
// cookie Secure/HSTS decision, and audit attribution so those surfaces agree.
type TrustPolicy struct {
	// UseProxy enables forwarded-header trust at all. When false, the socket
	// peer is always the client.
	UseProxy bool
	// Strict trusts only loopback and Additional. When false, every private,
	// loopback, or link-local peer is trusted (the pre-0.9.8 legacy behavior).
	Strict bool
	// Additional holds explicitly trusted proxy addresses. They are trusted in
	// both modes. Only entries that parsed as a literal IP are present.
	Additional []net.IP
	// InvalidAdditional holds the raw ADDITIONAL_PROXIES entries that were not
	// literal IP addresses and were therefore ignored. Admin diagnostics reports
	// them so an operator does not believe an unparsed token is trusted.
	InvalidAdditional []string
	// Observer, when non-nil, records private peers that are not configured
	// proxies so administrators can see whether the deployment is affected and
	// whether a proxy is missing from Additional.
	Observer *TrustObserver
}

// NewTrustPolicy builds a policy, parsing the additional proxy strings. Invalid
// entries are kept in InvalidAdditional rather than silently dropped so the
// admin diagnostic can report the effective allowlist.
func NewTrustPolicy(useProxy, strict bool, additionalProxies []string, observer *TrustObserver) TrustPolicy {
	policy := TrustPolicy{UseProxy: useProxy, Strict: strict, Observer: observer}
	for _, proxyStr := range additionalProxies {
		trimmed := strings.TrimSpace(proxyStr)
		if trimmed == "" {
			continue
		}
		if ip := net.ParseIP(trimmed); ip != nil {
			policy.Additional = append(policy.Additional, ip)
			continue
		}
		policy.InvalidAdditional = append(policy.InvalidAdditional, trimmed)
	}
	return policy
}

// ConfiguredProxies returns the effective parsed allowlist as canonical
// address strings. Tokens that did not parse are not included; they are
// reported separately as InvalidAdditional.
func (p TrustPolicy) ConfiguredProxies() []string {
	out := make([]string, 0, len(p.Additional))
	for _, ip := range p.Additional {
		out = append(out, ip.String())
	}
	return out
}

// PeerIP returns the socket peer address of r, handling IPv6 brackets. It is
// the address that must be checked before trusting forwarded headers, shared by
// client-IP extraction, cookie Secure/HSTS detection, the SSO redirect check,
// and security headers so they all agree. Returns nil when the peer cannot be
// parsed.
func PeerIP(r *http.Request) net.IP {
	return net.ParseIP(remoteHost(r.RemoteAddr))
}

// IsTrusted reports whether ip may supply forwarded headers.
//
// Loopback is always trusted in proxy mode: a request from 127.0.0.1/::1 comes
// from a local process on the same host, which can already read the process
// memory and configuration. Strict mode removes the broader private-range
// exception, so an unlisted LAN or pod address is the client.
func (p TrustPolicy) IsTrusted(ip net.IP) bool {
	if !p.UseProxy || ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	if !p.Strict && IsPrivateIP(ip) {
		return true
	}
	return p.isConfiguredProxy(ip)
}

// isConfiguredProxy reports whether ip is explicitly listed in Additional.
func (p TrustPolicy) isConfiguredProxy(ip net.IP) bool {
	for _, trusted := range p.Additional {
		if ip.Equal(trusted) {
			return true
		}
	}
	return false
}

// PeerObservation is one distinct private peer seen by the resolver.
type PeerObservation struct {
	IP        string              `json:"ip"`
	Kind      PeerObservationKind `json:"kind"`
	Count     uint64              `json:"count"`
	FirstSeen time.Time           `json:"first_seen"`
	LastSeen  time.Time           `json:"last_seen"`
}

// TrustObserver records bounded, process-local observations of peers that are
// not configured proxies. It stores only the socket peer address, never a
// client-supplied header value. One observer is shared by every TrustPolicy
// derived from the running configuration.
type TrustObserver struct {
	mu       sync.Mutex
	observed map[string]*PeerObservation
	max      int
}

// NewTrustObserver creates an observer bounded to a small number of distinct
// peers so a flood of source addresses cannot grow memory without limit.
func NewTrustObserver() *TrustObserver {
	return &TrustObserver{observed: make(map[string]*PeerObservation), max: 64}
}

// Record notes one request from a private, non-proxy peer.
func (o *TrustObserver) Record(ip string, kind PeerObservationKind) {
	if o == nil {
		return
	}
	now := time.Now().UTC()
	o.mu.Lock()
	defer o.mu.Unlock()
	if obs, ok := o.observed[ip]; ok {
		// A peer can send forwarding headers on one request and not another;
		// the presence of any forwarding header is the actionable signal, so
		// unlisted_proxy wins over private_client.
		if kind == PeerKindUnlistedProxy {
			obs.Kind = kind
		}
		obs.Count++
		obs.LastSeen = now
		return
	}
	if len(o.observed) >= o.max {
		var oldestKey string
		var oldest time.Time
		for key, obs := range o.observed {
			if oldestKey == "" || obs.LastSeen.Before(oldest) {
				oldestKey, oldest = key, obs.LastSeen
			}
		}
		delete(o.observed, oldestKey)
	}
	o.observed[ip] = &PeerObservation{IP: ip, Kind: kind, Count: 1, FirstSeen: now, LastSeen: now}
}

// Snapshot returns a stable copy of the observations, most recently seen first.
func (o *TrustObserver) Snapshot() []PeerObservation {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]PeerObservation, 0, len(o.observed))
	for _, obs := range o.observed {
		out = append(out, *obs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}
