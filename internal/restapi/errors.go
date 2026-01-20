package restapi

import (
	"encoding/json"
	"net/http"
)

// Error codes for the public API
const (
	// Authentication errors
	ErrCodeUnauthorized           = "UNAUTHORIZED"
	ErrCodeInvalidToken           = "INVALID_TOKEN"
	ErrCodeTokenExpired           = "TOKEN_EXPIRED"
	ErrCodeInsufficientPermission = "INSUFFICIENT_PERMISSION"

	// Validation errors
	ErrCodeValidationFailed = "VALIDATION_FAILED"
	ErrCodeInvalidInput     = "INVALID_INPUT"
	ErrCodeMissingField     = "MISSING_FIELD"

	// Resource errors
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeItemNotFound      = "ITEM_NOT_FOUND"
	ErrCodeWorkspaceNotFound = "WORKSPACE_NOT_FOUND"
	ErrCodeUserNotFound      = "USER_NOT_FOUND"
	ErrCodeConflict          = "CONFLICT"
	ErrCodeAlreadyExists     = "ALREADY_EXISTS"

	// Rate limiting
	ErrCodeRateLimited = "RATE_LIMITED"

	// Server errors
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// ErrorResponse represents a structured API error response
type ErrorResponse struct {
	Error     string      `json:"error"`                // Human-readable message
	Code      string      `json:"code"`                 // Machine-readable error code
	RequestID string      `json:"request_id,omitempty"` // Request correlation ID
	Details   interface{} `json:"details,omitempty"`    // Additional error details
}

// APIError represents an error with HTTP status code
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    interface{}
}

func (e *APIError) Error() string {
	return e.Message
}

// NewAPIError creates a new API error
func NewAPIError(statusCode int, code, message string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

// WithDetails adds details to an API error
func (e *APIError) WithDetails(details interface{}) *APIError {
	e.Details = details
	return e
}

// Common errors
var (
	ErrUnauthorized           = NewAPIError(http.StatusUnauthorized, ErrCodeUnauthorized, "Authentication required")
	ErrInvalidToken           = NewAPIError(http.StatusUnauthorized, ErrCodeInvalidToken, "Invalid or malformed token")
	ErrTokenExpired           = NewAPIError(http.StatusUnauthorized, ErrCodeTokenExpired, "Token has expired")
	ErrInsufficientPermission = NewAPIError(http.StatusForbidden, ErrCodeInsufficientPermission, "Insufficient permissions")
	ErrNotFound               = NewAPIError(http.StatusNotFound, ErrCodeNotFound, "Resource not found")
	ErrItemNotFound           = NewAPIError(http.StatusNotFound, ErrCodeItemNotFound, "Item not found")
	ErrWorkspaceNotFound      = NewAPIError(http.StatusNotFound, ErrCodeWorkspaceNotFound, "Workspace not found")
	ErrUserNotFound           = NewAPIError(http.StatusNotFound, ErrCodeUserNotFound, "User not found")
	ErrValidationFailed       = NewAPIError(http.StatusBadRequest, ErrCodeValidationFailed, "Validation failed")
	ErrInvalidInput           = NewAPIError(http.StatusBadRequest, ErrCodeInvalidInput, "Invalid input")
	ErrRateLimited            = NewAPIError(http.StatusTooManyRequests, ErrCodeRateLimited, "Rate limit exceeded")
	ErrInternalError          = NewAPIError(http.StatusInternalServerError, ErrCodeInternalError, "Internal server error")
)

// RespondError writes an error response to the client
func RespondError(w http.ResponseWriter, r *http.Request, err *APIError) {
	requestID, _ := r.Context().Value(ContextKeyRequestID).(string)

	response := ErrorResponse{
		Error:     err.Message,
		Code:      err.Code,
		RequestID: requestID,
		Details:   err.Details,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// RespondErrorWithMessage writes an error response with a custom message
func RespondErrorWithMessage(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	RespondError(w, r, NewAPIError(statusCode, code, message))
}
