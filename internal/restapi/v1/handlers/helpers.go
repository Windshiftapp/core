package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
)

// requireAuth extracts the authenticated user from the request context.
// Returns nil and writes a 401 response if not authenticated.
func requireAuth(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return nil, false
	}
	return user, true
}

// parsePathID parses an integer path parameter from the request.
// Returns 0 and writes a 400 response if the parameter is not a valid integer.
func parsePathID(w http.ResponseWriter, r *http.Request, param, label string) (int, bool) { //nolint:unparam // param is "id" today but kept for flexibility
	id, err := strconv.Atoi(r.PathValue(param))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid "+label))
		return 0, false
	}
	return id, true
}

// decodeBody decodes the JSON request body into v. Writes 400 on error.
func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return err
	}
	return nil
}
