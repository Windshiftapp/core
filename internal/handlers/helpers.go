package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// respondJSON sends a JSON response with the given status code
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// respondJSONOK sends a JSON response with 200 OK
func respondJSONOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// respondJSONCreated sends a JSON response with 201 Created
func respondJSONCreated(w http.ResponseWriter, data interface{}) {
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
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return v, false
	}
	return v, true
}

// respondJSONWithWarnings sends a JSON response with warnings if any exist
// If there are warnings, the response is wrapped in {"data": ..., "warnings": [...]}
// If there are no warnings, the response is sent as-is for backward compatibility
func respondJSONWithWarnings(w http.ResponseWriter, statusCode int, data interface{}, warnings []models.APIWarning) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if len(warnings) > 0 {
		response := map[string]interface{}{
			"data":     data,
			"warnings": warnings,
		}
		_ = json.NewEncoder(w).Encode(response)
	} else {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondJSONOKWithWarnings sends 200 OK with optional warnings
func respondJSONOKWithWarnings(w http.ResponseWriter, data interface{}, warnings []models.APIWarning) {
	respondJSONWithWarnings(w, http.StatusOK, data, warnings)
}

// respondJSONCreatedWithWarnings sends 201 Created with optional warnings
func respondJSONCreatedWithWarnings(w http.ResponseWriter, data interface{}, warnings []models.APIWarning) {
	respondJSONWithWarnings(w, http.StatusCreated, data, warnings)
}

// createCacheWarning creates a standardized cache invalidation warning
func createCacheWarning(cacheType string, err error, context string) models.APIWarning {
	return models.APIWarning{
		Code:    "cache_invalidation_failed",
		Message: fmt.Sprintf("Failed to invalidate %s cache: %v", cacheType, err),
		Context: context,
	}
}

// logAudit logs a successful resource action audit event.
func logAudit(db database.Database, r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string) {
	_ = logger.LogAudit(db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Success:      true,
	})
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

// addDateRangeFilter appends date_from/date_to query parameter filters to a SQL query.
// Expects dates in "2006-01-02" format. Compares against a Unix timestamp column.
func addDateRangeFilter(r *http.Request, query *string, args *[]interface{}, column string) {
	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		t, err := time.Parse("2006-01-02", dateFrom)
		if err == nil {
			*query += " AND " + column + " >= ?"
			*args = append(*args, t.Unix())
		}
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		t, err := time.Parse("2006-01-02", dateTo)
		if err == nil {
			*query += " AND " + column + " <= ?"
			*args = append(*args, t.Add(24*time.Hour-time.Second).Unix())
		}
	}
}
