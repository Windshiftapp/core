package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"windshift/internal/utils"

	"golang.org/x/time/rate"
)

// RateLimiter implements token bucket rate limiting per IP address
type RateLimiter struct {
	visitors          map[string]*visitor
	failedAttempts    map[string]*failureTracker
	mu                sync.RWMutex
	rate              rate.Limit // Requests per second
	burst             int        // Burst size
	cleanupTicker     *time.Ticker
	useProxy          bool     // Whether proxy mode is enabled
	additionalProxies []net.IP // Additional trusted proxy IPs beyond private ranges
}

// visitor represents a single IP's rate limiter
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// failureTracker tracks failed login attempts per IP
type failureTracker struct {
	count       int
	lockedUntil time.Time
	lastFailed  time.Time
}

// NewRateLimiter creates a new rate limiter with specified rate and burst
// rps: requests per second (e.g., 5.0/60.0 = 5 per minute)
// burst: maximum burst size
// useProxy: whether to trust proxy headers (X-Forwarded-For) from trusted proxies
// additionalProxies: list of additional trusted proxy IPs (beyond auto-trusted private ranges)
func NewRateLimiter(rps float64, burst int, useProxy bool, additionalProxies []string) *RateLimiter {
	// Parse additional proxy IPs
	var additionalIPs []net.IP
	for _, proxyStr := range additionalProxies {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalIPs = append(additionalIPs, ip)
		}
	}

	rl := &RateLimiter{
		visitors:          make(map[string]*visitor),
		failedAttempts:    make(map[string]*failureTracker),
		rate:              rate.Limit(rps),
		burst:             burst,
		cleanupTicker:     time.NewTicker(5 * time.Minute),
		useProxy:          useProxy,
		additionalProxies: additionalIPs,
	}

	// Start background cleanup goroutine
	go rl.startCleanupLoop()

	return rl
}

// Limit is the middleware function that enforces rate limiting
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.getClientIP(r)
		limiter := rl.getVisitor(ip)

		if !limiter.Allow() {
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RecordFailedLogin records a failed login attempt and applies progressive lockout
func (rl *RateLimiter) RecordFailedLogin(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	tracker, exists := rl.failedAttempts[ip]
	if !exists {
		tracker = &failureTracker{}
		rl.failedAttempts[ip] = tracker
	}

	tracker.count++
	tracker.lastFailed = time.Now()

	// Progressive lockout based on failure count
	switch {
	case tracker.count >= 10:
		// 10+ failures: 15 minute lockout
		tracker.lockedUntil = time.Now().Add(15 * time.Minute)
	case tracker.count >= 5:
		// 5-9 failures: 5 minute lockout
		tracker.lockedUntil = time.Now().Add(5 * time.Minute)
	case tracker.count >= 3:
		// 3-4 failures: 1 minute lockout
		tracker.lockedUntil = time.Now().Add(1 * time.Minute)
	}
}

// RecordSuccessfulLogin clears failed login attempts for an IP
func (rl *RateLimiter) RecordSuccessfulLogin(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.failedAttempts, ip)
}

// IsLockedOut checks if an IP is currently locked out due to failed attempts
// Returns (isLocked, remainingDuration)
func (rl *RateLimiter) IsLockedOut(ip string) (bool, time.Duration) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	tracker, exists := rl.failedAttempts[ip]
	if !exists {
		return false, 0
	}

	now := time.Now()
	if now.Before(tracker.lockedUntil) {
		remaining := tracker.lockedUntil.Sub(now)
		return true, remaining
	}

	return false, 0
}

// getVisitor returns the rate limiter for a specific IP
func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = &visitor{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	// Update last seen time
	v.lastSeen = time.Now()
	return v.limiter
}

// startCleanupLoop runs periodic cleanup of old visitors and failures
func (rl *RateLimiter) startCleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanupOldEntries()
		}
	}
}

// cleanupOldEntries removes inactive visitors and expired failures
func (rl *RateLimiter) cleanupOldEntries() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Remove visitors not seen in 10 minutes
	for ip, v := range rl.visitors {
		if now.Sub(v.lastSeen) > 10*time.Minute {
			delete(rl.visitors, ip)
		}
	}

	// Remove failure trackers older than 30 minutes
	for ip, tracker := range rl.failedAttempts {
		if now.Sub(tracker.lastFailed) > 30*time.Minute {
			delete(rl.failedAttempts, ip)
		}
	}
}

// Stop stops the cleanup ticker
func (rl *RateLimiter) Stop() {
	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
}

// getClientIP extracts the client IP from request headers with proxy validation
func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Get the immediate client IP (could be proxy)
	remoteAddr := r.RemoteAddr
	// Remove port if present
	if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
		remoteAddr = remoteAddr[:colonIndex]
	}

	clientIP := net.ParseIP(remoteAddr)
	if clientIP == nil {
		return remoteAddr // Return as-is if parsing fails
	}

	// Only trust proxy headers if the request comes from a trusted proxy
	if utils.IsTrustedProxy(clientIP, rl.useProxy, rl.additionalProxies) {
		// Check X-Forwarded-For header (for proxies)
		forwarded := r.Header.Get("X-Forwarded-For")
		if forwarded != "" {
			// Take the first (original client) IP
			ips := strings.Split(forwarded, ",")
			firstIP := strings.TrimSpace(ips[0])
			if firstIP != "" {
				return firstIP
			}
		}

		// Check X-Real-IP header
		realIP := r.Header.Get("X-Real-IP")
		if realIP != "" {
			return realIP
		}
	}

	// Fall back to direct connection IP
	return remoteAddr
}

// GetFailedAttemptCount returns the number of failed attempts for an IP (for testing/monitoring)
func (rl *RateLimiter) GetFailedAttemptCount(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	tracker, exists := rl.failedAttempts[ip]
	if !exists {
		return 0
	}
	return tracker.count
}

// FormatLockoutDuration formats a duration for user-friendly display
func FormatLockoutDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm%ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
