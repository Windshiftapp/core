package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"windshift/internal/restapi"
	"windshift/internal/utils"
)

// idleEvictAfter is how long a per-token bucket can sit unused before the
// background sweeper drops it. Without this, the limiters map grows for the
// process lifetime as new token IDs (e.g. short-lived CI tokens) arrive.
const idleEvictAfter = 10 * time.Minute

// sweepInterval is how often the eviction sweep runs.
const sweepInterval = 5 * time.Minute

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	limiters sync.Map // token_id -> *tokenBucket
	rate     int      // requests per window
	window   time.Duration

	// ipExtractor resolves the client IP for unauthenticated requests with
	// trusted-proxy handling. When nil, forwarded headers are ignored.
	ipExtractor *utils.IPExtractor

	sweepTicker *time.Ticker
	stopChan    chan struct{}
}

type tokenBucket struct {
	tokens    int
	lastReset time.Time
	// lastSeen is the most recent request time; the sweeper uses it to drop
	// buckets that no caller has touched within idleEvictAfter.
	lastSeen time.Time
	mu       sync.Mutex
}

// NewRateLimiter creates a new rate limiter
// requestsPerMinute specifies the maximum requests allowed per minute per token
// ipExtractor resolves the client IP for the unauthenticated IP-keyed fallback;
// nil ignores forwarded headers and uses RemoteAddr.
func NewRateLimiter(requestsPerMinute int, ipExtractor *utils.IPExtractor) *RateLimiter {
	rl := &RateLimiter{
		rate:        requestsPerMinute,
		window:      time.Minute,
		ipExtractor: ipExtractor,
		sweepTicker: time.NewTicker(sweepInterval),
		stopChan:    make(chan struct{}),
	}
	go rl.sweepLoop()
	return rl
}

// sweepLoop drops idle buckets on a fixed cadence. Exits when stopChan is
// closed (via Stop) so the goroutine doesn't outlive the limiter.
func (rl *RateLimiter) sweepLoop() {
	for {
		select {
		case <-rl.sweepTicker.C:
			rl.evictIdle(time.Now())
		case <-rl.stopChan:
			return
		}
	}
}

// evictIdle removes buckets whose lastSeen is older than idleEvictAfter.
// Split out for direct testing.
func (rl *RateLimiter) evictIdle(now time.Time) {
	rl.limiters.Range(func(key, val any) bool {
		b, ok := val.(*tokenBucket)
		if !ok {
			return true
		}
		b.mu.Lock()
		idle := now.Sub(b.lastSeen) > idleEvictAfter
		b.mu.Unlock()
		if idle {
			rl.limiters.Delete(key)
		}
		return true
	})
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get token ID from context (set by auth middleware)
		tokenID := rl.getTokenID(r)
		if tokenID == "" {
			// No token ID means unauthenticated - use IP address
			tokenID = "ip:" + rl.getClientIP(r)
		}

		bucket := rl.getBucket(tokenID)
		allowed, remaining, resetTime := bucket.allow(rl.rate, rl.window)

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.rate))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			w.Header().Set("Retry-After", strconv.FormatInt(int64(time.Until(resetTime).Seconds()), 10))
			restapi.RespondError(w, r, restapi.ErrRateLimited)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) getTokenID(r *http.Request) string {
	apiToken := GetAPIToken(r.Context())
	if apiToken != nil {
		return strconv.Itoa(apiToken.ID)
	}
	return ""
}

func (rl *RateLimiter) getBucket(tokenID string) *tokenBucket {
	if existing, ok := rl.limiters.Load(tokenID); ok {
		return existing.(*tokenBucket) //nolint:errcheck // type assertion is safe, we only store *tokenBucket
	}

	now := time.Now()
	bucket := &tokenBucket{
		tokens:    rl.rate,
		lastReset: now,
		lastSeen:  now,
	}
	actual, _ := rl.limiters.LoadOrStore(tokenID, bucket)
	return actual.(*tokenBucket) //nolint:errcheck // type assertion is safe, we only store *tokenBucket
}

func (tb *tokenBucket) allow(rate int, window time.Duration) (allowed bool, remaining int, resetTime time.Time) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	tb.lastSeen = now
	resetTime = tb.lastReset.Add(window)

	// Reset if window has passed
	if now.After(resetTime) {
		tb.tokens = rate
		tb.lastReset = now
		resetTime = now.Add(window)
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true, tb.tokens, resetTime
	}

	return false, 0, resetTime
}

func (rl *RateLimiter) getClientIP(r *http.Request) string {
	if rl.ipExtractor != nil {
		return rl.ipExtractor.GetClientIP(r)
	}
	return utils.ExtractClientIP(r, false, nil)
}
