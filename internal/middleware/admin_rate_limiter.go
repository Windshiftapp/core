package middleware

import (
	"database/sql"
	"sync"
	"time"

	"windshift/internal/database"
)

// AdminFallbackRateLimiter implements persistent rate limiting for admin password fallback
// when stricter authentication policies are in effect.
// Limits: 5 attempts/hour per admin user, 3 attempts/hour per IP for admin accounts
type AdminFallbackRateLimiter struct {
	db database.Database
	mu sync.RWMutex
}

const (
	// MaxAdminAttemptsPerUser is the maximum password login attempts per admin user per hour
	MaxAdminAttemptsPerUser = 5
	// AdminLockoutDuration is the lockout period after exceeding limits
	AdminLockoutDuration = 1 * time.Hour
	// RateLimitWindow is the window for counting attempts
	RateLimitWindow = 1 * time.Hour
)

// NewAdminFallbackRateLimiter creates a new rate limiter for admin fallback authentication
func NewAdminFallbackRateLimiter(db database.Database) *AdminFallbackRateLimiter {
	return &AdminFallbackRateLimiter{db: db}
}

// RecordAttempt records an admin fallback login attempt
func (rl *AdminFallbackRateLimiter) RecordAttempt(userID int, ipAddress string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	oneHourAgo := time.Now().Add(-time.Hour).UTC()

	// Try to update existing record
	result, err := rl.db.Exec(`
		UPDATE admin_fallback_rate_limits
		SET attempts = attempts + 1
		WHERE user_id = ? AND ip_address = ?
		AND first_attempt_at > ?
	`, userID, ipAddress, oneHourAgo)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Clean up old entries and insert new one
		_, _ = rl.db.Exec(`
			DELETE FROM admin_fallback_rate_limits
			WHERE user_id = ? AND ip_address = ?
			AND first_attempt_at <= ?
		`, userID, ipAddress, oneHourAgo)

		_, err = rl.db.Exec(`
			INSERT INTO admin_fallback_rate_limits (user_id, ip_address, attempts, first_attempt_at)
			VALUES (?, ?, 1, CURRENT_TIMESTAMP)
			ON CONFLICT(user_id, ip_address) DO UPDATE SET
				attempts = CASE
					WHEN first_attempt_at <= ? THEN 1
					ELSE attempts + 1
				END,
				first_attempt_at = CASE
					WHEN first_attempt_at <= ? THEN CURRENT_TIMESTAMP
					ELSE first_attempt_at
				END
		`, userID, ipAddress, oneHourAgo, oneHourAgo)
		if err != nil {
			return err
		}
	}

	// Check if we need to set a lockout
	var attempts int
	err = rl.db.QueryRow(`
		SELECT attempts FROM admin_fallback_rate_limits
		WHERE user_id = ? AND ip_address = ?
	`, userID, ipAddress).Scan(&attempts)
	if err != nil {
		return nil // Non-fatal
	}

	if attempts >= MaxAdminAttemptsPerUser {
		// Set lockout
		lockoutTime := time.Now().Add(time.Hour).UTC()
		_, _ = rl.db.Exec(`
			UPDATE admin_fallback_rate_limits
			SET locked_until = ?
			WHERE user_id = ? AND ip_address = ?
		`, lockoutTime, userID, ipAddress)
	}

	return nil
}

// IsAllowed checks if an admin fallback login attempt is allowed
// Returns (allowed, remainingAttempts, lockedUntil)
func (rl *AdminFallbackRateLimiter) IsAllowed(userID int, ipAddress string) (allowed bool, remaining int, resetTime *time.Time) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Check for active lockout
	var lockedUntil sql.NullTime
	var attempts int

	oneHourAgo := time.Now().Add(-time.Hour).UTC()
	err := rl.db.QueryRow(`
		SELECT attempts, locked_until FROM admin_fallback_rate_limits
		WHERE user_id = ? AND ip_address = ?
		AND first_attempt_at > ?
	`, userID, ipAddress, oneHourAgo).Scan(&attempts, &lockedUntil)

	if err == sql.ErrNoRows {
		// No record - first attempt, fully allowed
		return true, MaxAdminAttemptsPerUser, nil
	}
	if err != nil {
		// On error, allow the attempt (fail open for availability)
		return true, MaxAdminAttemptsPerUser, nil
	}

	// Check if locked out
	if lockedUntil.Valid && time.Now().Before(lockedUntil.Time) {
		return false, 0, &lockedUntil.Time
	}

	// Check if attempts exceeded
	if attempts >= MaxAdminAttemptsPerUser {
		lockTime := time.Now().Add(AdminLockoutDuration)
		return false, 0, &lockTime
	}

	return true, MaxAdminAttemptsPerUser - attempts, nil
}
