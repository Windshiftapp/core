package v2

import (
	"net/http"
	"strings"
)

// NewCORS returns the v2 CORS policy as error-returning middleware.
func NewCORS(allowedOrigins []string, allowAny, credentialed bool) Middleware {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(origin)] = true
	}
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return next(w, r)
			}
			if !allowAny && !allowed[strings.ToLower(origin)] {
				apiErr := newError(http.StatusForbidden, "cors_origin_not_allowed", "Origin is not allowed")
				apiErr.Details = map[string]any{"origin": origin}
				return apiErr
			}

			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if credentialed {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if r.Method == http.MethodOptions {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			return next(w, r)
		}
	}
}
