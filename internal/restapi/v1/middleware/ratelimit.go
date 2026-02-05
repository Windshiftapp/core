package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"windshift/internal/restapi"
)

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	limiters sync.Map // token_id -> *tokenBucket
	rate     int      // requests per window
	window   time.Duration
}

type tokenBucket struct {
	tokens    int
	lastReset time.Time
	mu        sync.Mutex
}

// NewRateLimiter creates a new rate limiter
// requestsPerMinute specifies the maximum requests allowed per minute per token
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	return &RateLimiter{
		rate:   requestsPerMinute,
		window: time.Minute,
	}
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get token ID from context (set by auth middleware)
		tokenID := rl.getTokenID(r)
		if tokenID == "" {
			// No token ID means unauthenticated - use IP address
			tokenID = "ip:" + getClientIP(r)
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

	bucket := &tokenBucket{
		tokens:    rl.rate,
		lastReset: time.Now(),
	}
	actual, _ := rl.limiters.LoadOrStore(tokenID, bucket)
	return actual.(*tokenBucket) //nolint:errcheck // type assertion is safe, we only store *tokenBucket
}

func (tb *tokenBucket) allow(rate int, window time.Duration) (allowed bool, remaining int, resetTime time.Time) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
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

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to remote address
	return r.RemoteAddr
}
