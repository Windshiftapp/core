package restapi

import (
	"encoding/json"
	"net/http"
)

// RespondJSON writes a JSON response with the given status code
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// RespondOK writes a 200 OK JSON response
func RespondOK(w http.ResponseWriter, data interface{}) {
	RespondJSON(w, http.StatusOK, data)
}

// RespondCreated writes a 201 Created JSON response
func RespondCreated(w http.ResponseWriter, data interface{}) {
	RespondJSON(w, http.StatusCreated, data)
}

// RespondNoContent writes a 204 No Content response
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// RespondPaginated writes a paginated JSON response
func RespondPaginated(w http.ResponseWriter, data interface{}, pagination PaginationMeta) {
	RespondOK(w, NewPaginatedResponse(data, pagination))
}
