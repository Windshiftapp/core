package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"windshift/internal/restapi"
)

// Recovery returns middleware that recovers from panics and returns a structured error response.
// It logs the panic with stack trace for debugging purposes.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with stack trace
				slog.Error("panic recovered",
					slog.Any("error", err),
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
					slog.String("stack", string(debug.Stack())),
				)

				// Return structured JSON error response
				restapi.RespondError(w, r, restapi.ErrInternalError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
