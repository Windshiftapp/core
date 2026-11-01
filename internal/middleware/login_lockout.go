package middleware

import (
	"fmt"
	"sync"
	"time"
)

// LoginLockout tracks failed password attempts per client IP and applies a
// progressive lockout. It is deliberately separate from the rate limiter: a
// lockout is stateful failed-attempt history, not a request rate.
type LoginLockout struct {
	mu        sync.Mutex
	attempts  map[string]*failureTracker
	lastSweep time.Time
}

type failureTracker struct {
	count       int
	lockedUntil time.Time
	lastFailed  time.Time
}

const (
	loginLockoutRetention  = 30 * time.Minute
	loginLockoutSweepEvery = 5 * time.Minute
)

// NewLoginLockout creates an empty lockout store.
func NewLoginLockout() *LoginLockout {
	return &LoginLockout{attempts: make(map[string]*failureTracker)}
}

// RecordFailedLogin records a failed login attempt and applies progressive
// lockout thresholds.
func (l *LoginLockout) RecordFailedLogin(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweepLocked(now)
	tracker, exists := l.attempts[ip]
	if !exists {
		tracker = &failureTracker{}
		l.attempts[ip] = tracker
	}

	tracker.count++
	tracker.lastFailed = now

	switch {
	case tracker.count >= 10:
		tracker.lockedUntil = now.Add(15 * time.Minute)
	case tracker.count >= 5:
		tracker.lockedUntil = now.Add(5 * time.Minute)
	case tracker.count >= 3:
		tracker.lockedUntil = now.Add(1 * time.Minute)
	}
}

// sweepLocked drops stale trackers so the map does not grow with distinct IPs.
// It runs at most every loginLockoutSweepEvery, from the failure path, so the
// O(n) scan is amortized. Caller holds the lock.
func (l *LoginLockout) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < loginLockoutSweepEvery {
		return
	}
	l.lastSweep = now
	for ip, tracker := range l.attempts {
		if now.Sub(tracker.lastFailed) > loginLockoutRetention {
			delete(l.attempts, ip)
		}
	}
}

// RecordSuccessfulLogin clears failed login attempts for an IP.
func (l *LoginLockout) RecordSuccessfulLogin(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// IsLockedOut reports whether an IP is currently locked out and for how long.
func (l *LoginLockout) IsLockedOut(ip string) (bool, time.Duration) {
	if l == nil {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	tracker, exists := l.attempts[ip]
	if !exists {
		return false, 0
	}
	now := time.Now()
	if now.Before(tracker.lockedUntil) {
		return true, tracker.lockedUntil.Sub(now)
	}
	return false, 0
}

// GetFailedAttemptCount returns the number of failed attempts for an IP.
func (l *LoginLockout) GetFailedAttemptCount(ip string) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	tracker, exists := l.attempts[ip]
	if !exists {
		return 0
	}
	return tracker.count
}

// FormatLockoutDuration formats a duration for user-friendly display.
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
