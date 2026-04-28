package router

import (
	"net/http"
	"strconv"
)

// PathInt extracts a path parameter and parses it as an integer.
// Returns an error if the parameter is missing or not a valid integer.
func PathInt(r *http.Request, name string) (int, error) {
	return strconv.Atoi(r.PathValue(name))
}

// RequireNumericID is middleware that validates the {id} path parameter is numeric.
// Returns 400 Bad Request if the ID is not a valid integer.
func RequireNumericID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := PathInt(r, "id"); err != nil {
			http.Error(w, "Invalid ID: must be numeric", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}
