package v2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"windshift/internal/contextkeys"
)

// Handler is the v2 transport boundary. Failures are written only by Adapt.
type Handler func(http.ResponseWriter, *http.Request) error

// Middleware decorates a v2 handler without taking ownership of error output.
type Middleware func(Handler) Handler

// Error is a stable v2 wire error.
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
	Headers http.Header
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.cause }

func newError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func internalError(err error) *Error {
	return &Error{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "An unexpected error occurred",
		cause:   err,
	}
}

type errorBody struct {
	Error     errorContent `json:"error"`
	RequestID string       `json:"request_id"`
}

type errorContent struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Adapt converts one v2 handler chain to net/http and owns all error output.
func Adapt(handler Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), contextkeys.RequestID, requestID)

		if err := handler(w, r.WithContext(ctx)); err != nil {
			writeError(w, requestID, err)
		}
	})
}

func writeError(w http.ResponseWriter, requestID string, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = internalError(err)
	}
	if apiErr.Status < 400 || apiErr.Status > 599 {
		apiErr = internalError(err)
	}
	if apiErr.Status >= 500 && apiErr.cause != nil {
		slog.Error("v2 request failed", "request_id", requestID, "error", apiErr.cause)
	}
	for name, values := range apiErr.Headers {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(errorBody{
		Error: errorContent{
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Details: apiErr.Details,
		},
		RequestID: requestID,
	})
}

func generateRequestID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "req_unavailable"
	}
	return "req_" + hex.EncodeToString(value)
}
