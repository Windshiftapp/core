package middleware

import (
	"windshift/internal/ratelimit"
	"windshift/internal/utils"
)

// NewRateLimiter builds the v1 API limiter on the shared rate-limit engine.
// Authenticated requests are keyed by token ID; the unauthenticated discovery
// routes fall back to the client IP.
func NewRateLimiter(requestsPerMinute int, ipExtractor *utils.IPExtractor) *ratelimit.Limiter {
	return ratelimit.New(
		float64(requestsPerMinute)/60.0,
		requestsPerMinute,
		ratelimit.ByTokenOrIP(ipExtractor),
	)
}
