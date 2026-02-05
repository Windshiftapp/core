package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"windshift/internal/restapi"
)

// RequestID middleware adds a unique request ID to each request
// If X-Request-ID header is provided, it will be used; otherwise, a new one is generated
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Set response header
		w.Header().Set("X-Request-ID", requestID)

		// Add to context
		ctx := context.WithValue(r.Context(), restapi.ContextKeyRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateRequestID creates a new unique request ID
func generateRequestID() string {
	bytes := make([]byte, 12)
	_, _ = rand.Read(bytes)
	return "req_" + hex.EncodeToString(bytes)
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(restapi.ContextKeyRequestID).(string); ok {
		return id
	}
	return ""
}
