package handlers

import (
	"log/slog"
	"net/http"

	"windshift/internal/restapi"
)

// Error response helpers for legacy handlers
// These provide a migration path from http.Error() to structured JSON responses

// respondError writes a structured JSON error response
func respondError(w http.ResponseWriter, r *http.Request, err *restapi.APIError) {
	restapi.RespondError(w, r, err)
}

// respondUnauthorized writes a 401 Unauthorized JSON response
func respondUnauthorized(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrUnauthorized)
}

// respondForbidden writes a 403 Forbidden JSON response
func respondForbidden(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrInsufficientPermission)
}

// respondAdminRequired writes a 403 Forbidden JSON response for admin-only endpoints
func respondAdminRequired(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrAdminRequired)
}

// respondNotFound writes a 404 Not Found JSON response with the resource type
func respondNotFound(w http.ResponseWriter, r *http.Request, resourceType string) {
	var err *restapi.APIError
	switch resourceType {
	case "item":
		err = restapi.ErrItemNotFound
	case "workspace":
		err = restapi.ErrWorkspaceNotFound
	case "user":
		err = restapi.ErrUserNotFound
	case "channel":
		err = restapi.ErrChannelNotFound
	case "test_case":
		err = restapi.ErrTestCaseNotFound
	case "test_run":
		err = restapi.ErrTestRunNotFound
	case "test_run_template":
		err = restapi.ErrTestRunTemplateNotFound
	case "test_folder":
		err = restapi.ErrTestFolderNotFound
	case "test_set":
		err = restapi.ErrTestSetNotFound
	case "portal":
		err = restapi.ErrPortalNotFound
	case "screen":
		err = restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Screen not found")
	case "asset":
		err = restapi.ErrAssetNotFound
	case "attachment":
		err = restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Attachment not found")
	case "file":
		err = restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found")
	case "thumbnail":
		err = restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Thumbnail not found")
	default:
		err = restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, resourceType+" not found")
	}
	restapi.RespondError(w, r, err)
}

// respondInvalidID writes a 400 Bad Request JSON response for invalid ID parameters
func respondInvalidID(w http.ResponseWriter, r *http.Request, paramName string) {
	err := restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid "+paramName)
	restapi.RespondError(w, r, err)
}

// respondValidationError writes a 400 Bad Request JSON response with a custom validation message
func respondValidationError(w http.ResponseWriter, r *http.Request, message string) {
	err := restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, message)
	restapi.RespondError(w, r, err)
}

// respondInternalError logs the error and writes a 500 Internal Server Error JSON response
// The actual error message is logged but not exposed to the client
func respondInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error",
		slog.Any("error", err),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
	)
	restapi.RespondError(w, r, restapi.ErrInternalError)
}

// respondBadRequest writes a 400 Bad Request JSON response with a custom message
func respondBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	err := restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, message)
	restapi.RespondError(w, r, err)
}

// respondConflict writes a 409 Conflict JSON response
func respondConflict(w http.ResponseWriter, r *http.Request, message string) {
	err := restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, message)
	restapi.RespondError(w, r, err)
}

// respondTooManyRequests writes a 429 Too Many Requests JSON response
func respondTooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	err := restapi.NewAPIError(http.StatusTooManyRequests, restapi.ErrCodeRateLimited, message)
	restapi.RespondError(w, r, err)
}

// respondGone writes a 410 Gone JSON response
func respondGone(w http.ResponseWriter, r *http.Request, message string) {
	err := restapi.NewAPIError(http.StatusGone, "GONE", message)
	restapi.RespondError(w, r, err)
}

// respondServiceUnavailable writes a 503 Service Unavailable JSON response
func respondServiceUnavailable(w http.ResponseWriter, r *http.Request, message string) {
	err := restapi.NewAPIError(http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable, message)
	restapi.RespondError(w, r, err)
}
