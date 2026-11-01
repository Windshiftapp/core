// Package ratelimit provides the single rate-limiting engine used by every
// HTTP surface (the core /api routes, /rest/api/v1, and the v2 APIs).
//
// The engine is a token bucket (golang.org/x/time/rate) stored per key in a
// sharded map, so a limiter hit takes a short shard read lock plus the bucket's
// own lock and never serializes the whole process. Keys are fixed-size and
// comparable, so building one allocates nothing.
package ratelimit

import (
	"net/http"
	"net/netip"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// Kind distinguishes the source of a limiter key. Two keys with different
// kinds never collide even if their IDs match.
type Kind uint8

const (
	KindUser Kind = iota + 1
	KindToken
	KindIP
)

// Key is a fixed-size comparable limiter key. Constructing one does not
// allocate, and neither does using it as a map key.
type Key struct {
	Kind Kind
	ID   uint64
	Addr netip.Addr
}

// UserKey keys a limiter on an authenticated user.
func UserKey(id int) Key { return Key{Kind: KindUser, ID: uint64(id)} } //nolint:gosec // G115: user ids are positive

// TokenKey keys a limiter on an API token.
func TokenKey(id int) Key { return Key{Kind: KindToken, ID: uint64(id)} } //nolint:gosec // G115: token ids are positive

// IPKey keys a limiter on a client IP address.
func IPKey(addr netip.Addr) Key { return Key{Kind: KindIP, Addr: addr} }

// KeyFunc derives the limiter key for a request. Returning ok=false skips
// limiting entirely (for example an unauthenticated request on a user-keyed
// limiter when IP limiting is disabled).
type KeyFunc func(*http.Request) (Key, bool)

// ByUser keys authenticated requests by user ID and skips requests without a
// user.
func ByUser() KeyFunc {
	return func(r *http.Request) (Key, bool) {
		user, ok := r.Context().Value(contextkeys.User).(*models.User)
		if !ok || user == nil {
			return Key{}, false
		}
		return UserKey(user.ID), true
	}
}

// ByToken keys authenticated requests by API token ID and skips requests
// without a token.
func ByToken() KeyFunc {
	return func(r *http.Request) (Key, bool) {
		token, ok := r.Context().Value(contextkeys.APIToken).(*models.APIToken)
		if !ok || token == nil {
			return Key{}, false
		}
		return TokenKey(token.ID), true
	}
}

// ByIP keys every request by client IP using the trusted-proxy extractor.
func ByIP(extractor *utils.IPExtractor) KeyFunc {
	return func(r *http.Request) (Key, bool) {
		return ipKey(extractor, r)
	}
}

// ByUserOrIP keys authenticated requests by user ID and everything else by IP.
func ByUserOrIP(extractor *utils.IPExtractor) KeyFunc {
	user := ByUser()
	return func(r *http.Request) (Key, bool) {
		if key, ok := user(r); ok {
			return key, true
		}
		return ipKey(extractor, r)
	}
}

// ByTokenOrIP keys authenticated requests by token ID and everything else by
// IP. Used by the v1 API, whose unauthenticated discovery routes still need a
// limit.
func ByTokenOrIP(extractor *utils.IPExtractor) KeyFunc {
	token := ByToken()
	return func(r *http.Request) (Key, bool) {
		if key, ok := token(r); ok {
			return key, true
		}
		return ipKey(extractor, r)
	}
}

func ipKey(extractor *utils.IPExtractor, r *http.Request) (Key, bool) {
	raw := clientIPString(extractor, r)
	if raw == "" {
		return Key{}, false
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		// An unparseable peer still gets a stable key so limits cannot be
		// bypassed by rotating garbage RemoteAddr values.
		return Key{Kind: KindIP, ID: fnv64(raw)}, true
	}
	return IPKey(addr.Unmap()), true
}

func clientIPString(extractor *utils.IPExtractor, r *http.Request) string {
	if extractor != nil {
		return extractor.GetClientIP(r)
	}
	return utils.ExtractClientIP(r, false, nil)
}

func fnv64(s string) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint64(s[i])) * prime
	}
	return h
}

func (k Key) hash() uint64 {
	const prime = 1099511628211
	h := fnv64("ratelimit-key")
	h = (h ^ uint64(k.Kind)) * prime
	h = (h ^ k.ID) * prime
	if k.Kind == KindIP {
		addr := k.Addr.As16()
		for _, b := range addr {
			h = (h ^ uint64(b)) * prime
		}
	}
	return h
}
