package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"windshift/internal/jira"
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

// respondJiraUpstreamError uses stable 502 codes. It must not return 401,
// which fetchAPI interprets as a Windshift-session expiry rather than a Jira
// reconnection problem.
func respondJiraUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	code := "JIRA_UPSTREAM_ERROR"
	message := "Jira request failed."
	switch {
	case errors.Is(err, jira.ErrInvalidCredentials):
		code = "JIRA_AUTH_FAILED"
		message = "Jira authentication failed — the saved token may be expired or revoked. Reconnect this Jira connection to continue."
	case errors.Is(err, jira.ErrForbidden):
		code = "JIRA_FORBIDDEN"
		message = "Jira denied the request. Check that the user the token belongs to has access to these projects."
	case errors.Is(err, jira.ErrRateLimited):
		code = "JIRA_RATE_LIMITED"
		message = "Jira rate limit hit — wait a moment and try again."
	}
	slog.Warn("jira upstream error",
		slog.Any("error", err),
		slog.String("code", code),
		slog.String("path", r.URL.Path),
	)
	restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadGateway, code, message))
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

// respondRequestTooLarge writes a 413 Request Entity Too Large JSON response.
// Used when a request body exceeds a handler's http.MaxBytesReader cap.
func respondRequestTooLarge(w http.ResponseWriter, r *http.Request) {
	err := restapi.NewAPIError(http.StatusRequestEntityTooLarge, restapi.ErrCodeRequestTooLarge, "Request body too large")
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

// respondUpgradeRequired writes a 426 Upgrade Required JSON response. Used by
// the llm-proxy broker when a coding-agent client presents a missing or
// mismatched protocol version, so an out-of-step agent fails loudly with a
// diagnostic signal instead of misparsing the response (WI-921). The caller
// advertises the supported version in the X-Protocol-Version response header.
func respondUpgradeRequired(w http.ResponseWriter, r *http.Request) {
	err := restapi.NewAPIError(http.StatusUpgradeRequired, "PROTOCOL_VERSION_MISMATCH",
		"coding-agent protocol version is out of date; the agent image must match the server version")
	restapi.RespondError(w, r, err)
}
