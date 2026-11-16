package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"windshift/internal/operatorcontrols"
	"windshift/internal/restapi"
)

// OperatorAdmission applies aggregate instance ceilings independently of users,
// tokens and workspaces. Health, metrics and static assets retain recovery access.
func OperatorAdmission(controls *operatorcontrols.Controls) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limits := controls.Limits()
		if limits.ConcurrentRequests == 0 && limits.ConcurrentStreams == 0 && limits.RequestsPerMinute == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/rest/") && !strings.HasPrefix(path, "/scim/") && path != "/mcp" {
				next.ServeHTTP(w, r)
				return
			}
			resource, ceiling := "requests", limits.ConcurrentRequests
			if isStreamingPath(path) {
				resource, ceiling = "streams", limits.ConcurrentStreams
			}
			release, err := controls.Acquire(r.Context(), resource, ceiling)
			if err == nil {
				defer release()
				err = controls.Consume(r.Context(), "requests", 1, limits.RequestsPerMinute, time.Minute)
			}
			if err != nil {
				status, code, message := http.StatusServiceUnavailable, "operator_budget_unavailable", "Operator budget admission is unavailable"
				var limited *operatorcontrols.LimitError
				if errors.As(err, &limited) {
					status, code, message = limited.Status(), "operator_limit_exceeded", limited.Error()
				}
				if status == http.StatusTooManyRequests {
					w.Header().Set("Retry-After", "60")
				}
				if strings.HasPrefix(path, "/api/v2/") || strings.HasPrefix(path, "/rest/api/v2/") {
					requestID := "req_" + uuid.NewString()
					w.Header().Set("X-Request-ID", requestID)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID})
				} else {
					restapi.RespondError(w, r, restapi.NewAPIError(status, code, message))
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
