package middleware

import (
	"net/http"
	"strings"
	"sync"

	"windshift/internal/models"
)

// UserConcurrencyLimiter caps each authenticated user's simultaneous work to
// prevent one client from exhausting the shared database pool. Unauthenticated
// and streaming requests are governed elsewhere.
type UserConcurrencyLimiter struct {
	mu       sync.Mutex
	inFlight map[int]int // userID -> current in-flight count
	limit    int
}

// NewUserConcurrencyLimiter caps each user; a non-positive limit disables it.
func NewUserConcurrencyLimiter(limit int) *UserConcurrencyLimiter {
	return &UserConcurrencyLimiter{inFlight: make(map[int]int), limit: limit}
}

// Limit returns 429 once a user reaches the in-flight request cap.
func (l *UserConcurrencyLimiter) Limit(next http.Handler) http.Handler {
	if l == nil || l.limit <= 0 || e2eRateLimitsDisabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(ContextKeyUser).(*models.User)
		if !ok || user == nil || isStreamingPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		release, ok := l.Acquire(user.ID)
		if !ok {
			// Do not queue requests that would add pool pressure.
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too many concurrent requests. Please retry shortly.", http.StatusTooManyRequests)
			return
		}
		defer release()
		next.ServeHTTP(w, r)
	})
}

// Acquire reserves one per-user slot and returns its release function.
func (l *UserConcurrencyLimiter) Acquire(userID int) (func(), bool) {
	if l == nil || l.limit <= 0 || e2eRateLimitsDisabled() {
		return func() {}, true
	}
	if !l.acquire(userID) {
		return nil, false
	}
	return func() { l.release(userID) }, true
}

func (l *UserConcurrencyLimiter) acquire(userID int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight[userID] >= l.limit {
		return false
	}
	l.inFlight[userID]++
	return true
}

func (l *UserConcurrencyLimiter) release(userID int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight[userID] <= 1 {
		delete(l.inFlight, userID)
		return
	}
	l.inFlight[userID]--
}

// isStreamingPath excludes long-lived streams from the request cap.
func isStreamingPath(path string) bool {
	return strings.HasSuffix(path, "/events") || strings.Contains(path, "/ai/")
}
