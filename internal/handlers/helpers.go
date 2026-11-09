package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
)

const emailOAuthStateRandomBytes = 32

// newEmailOAuthState binds an otherwise-random OAuth state to the exact raw
// channel config present when the flow started. The state row still provides
// one-time authenticity; the suffix lets callbacks reject an old browser tab
// after another admin changes the channel while the user is at the provider.
func newEmailOAuthState(configJSON string) (string, error) {
	random := make([]byte, emailOAuthStateRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(configJSON))
	return hex.EncodeToString(random) + "." + hex.EncodeToString(digest[:]), nil
}

func emailOAuthStateMatchesConfig(state, configJSON string) bool {
	const randomHexLen = emailOAuthStateRandomBytes * 2
	const digestHexLen = sha256.Size * 2
	if len(state) != randomHexLen+1+digestHexLen || state[randomHexLen] != '.' {
		return false
	}
	expected, err := hex.DecodeString(state[randomHexLen+1:])
	if err != nil || len(expected) != sha256.Size {
		return false
	}
	actual := sha256.Sum256([]byte(configJSON))
	return subtle.ConstantTimeCompare(expected, actual[:]) == 1
}

// rowScanner abstracts sql.Row and sql.Rows for Scan.
type rowScanner interface {
	Scan(dest ...any) error
}

// respondJSON sends a JSON response with the given status code
func respondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(restapi.NormalizeEmptySlice(data))
}

// respondJSONOK sends a JSON response with 200 OK
func respondJSONOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(restapi.NormalizeEmptySlice(data))
}

// respondJSONCreated sends a JSON response with 201 Created
func respondJSONCreated(w http.ResponseWriter, data any) {
	respondJSON(w, http.StatusCreated, data)
}

// parseIDParam extracts and parses an integer ID from URL parameters
func parseIDParam(r *http.Request, paramName string) (int, error) {
	return strconv.Atoi(r.PathValue(paramName))
}

// requireIDParam parses ID and writes error response if invalid, returns 0 and false on error
func requireIDParam(w http.ResponseWriter, r *http.Request, paramName string) (int, bool) {
	id, err := parseIDParam(r, paramName)
	if err != nil {
		respondInvalidID(w, r, paramName)
		return 0, false
	}
	return id, true
}

// decodeJSON decodes a JSON request body into a value of type T.
// Returns the decoded value and true on success, or zero value and false on error
// (error response already written).
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := restapi.DecodeJSONBody(w, r, &v); err != nil {
		if isRequestBodyTooLarge(err) {
			respondRequestTooLarge(w, r)
			return v, false
		}
		respondBadRequest(w, r, "Invalid request body")
		return v, false
	}
	return v, true
}

// decodeJSONWithFields decodes an object and records which fields were sent.
// Update handlers use the field map to distinguish omission from zero values.
func decodeJSONWithFields[T any](w http.ResponseWriter, r *http.Request) (value T, fields map[string]json.RawMessage, ok bool) {
	body, err := restapi.ReadJSONBody(w, r)
	if err != nil {
		if isRequestBodyTooLarge(err) {
			respondRequestTooLarge(w, r)
			return value, nil, false
		}
		respondBadRequest(w, r, "Invalid request body")
		return value, nil, false
	}

	if err := json.Unmarshal(body, &fields); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return value, nil, false
	}
	if fields == nil {
		respondBadRequest(w, r, "Invalid request body")
		return value, nil, false
	}
	if err := json.Unmarshal(body, &value); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return value, nil, false
	}
	return value, fields, true
}

func newJSONDecoder(w http.ResponseWriter, r *http.Request) *json.Decoder {
	return restapi.NewJSONDecoder(w, r)
}

// isRequestBodyTooLarge reports whether err is the error http.MaxBytesReader
// returns once a request body exceeds its cap. Used to distinguish an
// oversized body (413) from otherwise-malformed input (400) after a decode.
func isRequestBodyTooLarge(err error) bool {
	return restapi.IsRequestBodyTooLarge(err)
}

// decodeOptionalJSON decodes a JSON request body into a value of type T when one
// is present. A nil or empty body is treated as success and v is left zero.
// Malformed JSON writes a 400 and returns false. Use this for endpoints whose
// body is genuinely optional; use decodeJSON when the body is required.
func decodeOptionalJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if r.Body == nil {
		return v, true
	}
	err := restapi.DecodeJSONBody(w, r, &v)
	if err == nil || errors.Is(err, io.EOF) {
		return v, true
	}
	if isRequestBodyTooLarge(err) {
		respondRequestTooLarge(w, r)
		return v, false
	}
	respondBadRequest(w, r, "Invalid request body")
	return v, false
}

// requireWorkspaceIDParam resolves a workspace path parameter that may be a numeric ID or a workspace key.
// Returns the numeric workspace ID and true on success, or 0 and false if resolution fails (error already written).
func requireWorkspaceIDParam(w http.ResponseWriter, r *http.Request, cache *WorkspaceKeyCache, paramName string) (int, bool) {
	raw := r.PathValue(paramName)
	if raw == "" {
		respondBadRequest(w, r, "Workspace ID or key is required")
		return 0, false
	}
	id, ok := cache.Resolve(raw)
	if !ok {
		respondNotFound(w, r, "workspace")
		return 0, false
	}
	return id, true
}

// respondJSONWithWarnings sends a JSON response with warnings if any exist
// If there are warnings, the response is wrapped in {"data": ..., "warnings": [...]}
// If there are no warnings, the response is sent as-is for backward compatibility
func respondJSONWithWarnings(w http.ResponseWriter, statusCode int, data any, warnings []models.APIWarning) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	data = restapi.NormalizeEmptySlice(data)
	if len(warnings) > 0 {
		response := map[string]any{
			"data":     data,
			"warnings": warnings,
		}
		_ = json.NewEncoder(w).Encode(response)
	} else {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondJSONOKWithWarnings sends 200 OK with optional warnings
func respondJSONOKWithWarnings(w http.ResponseWriter, data any, warnings []models.APIWarning) {
	respondJSONWithWarnings(w, http.StatusOK, data, warnings)
}

// respondJSONCreatedWithWarnings sends 201 Created with optional warnings
func respondJSONCreatedWithWarnings(w http.ResponseWriter, data any, warnings []models.APIWarning) {
	respondJSONWithWarnings(w, http.StatusCreated, data, warnings)
}

// createCacheWarning creates a standardized cache invalidation warning
func createCacheWarning(cacheType string, err error, ctx string) models.APIWarning {
	return models.APIWarning{
		Code:    "cache_invalidation_failed",
		Message: fmt.Sprintf("Failed to invalidate %s cache: %v", cacheType, err),
		Context: ctx,
	}
}

// logAudit logs a successful resource action audit event.
func logAudit(db database.Database, r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string) {
	_ = logger.LogAudit(db, logger.NewRequestAuditEvent(r, user, actionType, resourceType, resourceID, resourceName, nil))
}

// logAuditWithDetails logs a successful resource action audit event with extra
// structured details (serialized to JSON in the audit log row).
func logAuditWithDetails(db database.Database, r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any) {
	_ = logger.LogAudit(db, logger.NewRequestAuditEvent(r, user, actionType, resourceType, resourceID, resourceName, details))
}

// deserializeIntArray converts a JSON string pointer to a slice of ints.
// Returns nil if the string is nil or empty or the JSON is invalid.
func deserializeIntArray(s *string) []int {
	if s == nil || *s == "" {
		return nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(*s), &ids); err != nil {
		return nil
	}
	return ids
}

// channelResult contains a resolved channel and its parsed config.
// Used by both PortalHandler and FormHandler to avoid duplicating the
// query-channels-by-slug lookup pattern.
type channelResult struct {
	channel models.Channel
	config  models.ChannelConfig
}

// findChannelBySlug resolves through channels.public_slug's unique partial
// index, then verifies that the derived column still matches the JSON config.
// This is the single implementation behind PortalHandler and FormHandler.
func findChannelBySlug(ctx context.Context, db database.Database, channelType, slug string, slugFromConfig func(*models.ChannelConfig) string) (*channelResult, error) {
	candidate, err := repository.NewChannelRepository(db).FindEnabledByPublicSlug(ctx, channelType, slug)
	if err != nil {
		return nil, err
	}
	if slugFromConfig(&candidate.Config) != slug {
		return nil, fmt.Errorf("%s channel not found", channelType)
	}
	return &channelResult{channel: candidate.Channel, config: candidate.Config}, nil
}

// visibilityInput holds the decoded visibility update request.
type visibilityInput struct {
	GroupIDs []int `json:"group_ids"`
	OrgIDs   []int `json:"org_ids"`
}

// PaginationParams holds parsed pagination values.
type PaginationParams struct {
	Page   int
	Limit  int
	Offset int
}

// parseOffsetPagination extracts limit/offset from query params.
func parseOffsetPagination(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	offset = 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxLimit {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

// boolValue reports an optional boolean, defaulting to false when the field
// was omitted.
func boolValue(v *bool) bool {
	return v != nil && *v
}
