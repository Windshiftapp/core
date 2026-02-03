package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"windshift/internal/models"
)

// respondJSON sends a JSON response with the given status code
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// respondJSONOK sends a JSON response with 200 OK
func respondJSONOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
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
		json.NewEncoder(w).Encode(response)
	} else {
		json.NewEncoder(w).Encode(data)
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
