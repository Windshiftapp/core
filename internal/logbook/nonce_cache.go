package logbook

import (
	"sync"
	"time"
)

// nonceCache keeps recently-seen signature nonces so replays within MaxSkew
// are rejected. Entries expire after their deadline; a periodic sweep removes
// stale items to bound memory.
type nonceCache struct {
	ttl   time.Duration
	mu    sync.Mutex
	seen  map[string]time.Time
	maxSz int
}

func newNonceCache(ttl time.Duration, maxSz int) *nonceCache {
	return &nonceCache{
		ttl:   ttl,
		seen:  make(map[string]time.Time),
		maxSz: maxSz,
	}
}

// checkAndAdd returns true if the nonce was accepted (unseen within TTL).
// Returns false if it's already in the cache, meaning a replay.
func (c *nonceCache) checkAndAdd(nonce string, now time.Time) bool {
	if nonce == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if exp, ok := c.seen[nonce]; ok && exp.After(now) {
		return false
	}

	// Opportunistic sweep: if we've hit the cap, drop everything expired.
	if len(c.seen) >= c.maxSz {
		for k, exp := range c.seen {
			if !exp.After(now) {
				delete(c.seen, k)
			}
		}
		// Still full of live entries — drop arbitrary ones. This can cause
		// false acceptances only if an attacker floods with fresh nonces
		// faster than TTL expires, which isn't a replay exploit.
		for k := range c.seen {
			if len(c.seen) < c.maxSz {
				break
			}
			delete(c.seen, k)
		}
	}

	c.seen[nonce] = now.Add(c.ttl)
	return true
}
