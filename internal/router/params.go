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

// PathInt64 extracts a path parameter and parses it as an int64.
// Returns an error if the parameter is missing or not a valid integer.
func PathInt64(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

// PathString extracts a path parameter as a string.
// Returns empty string if the parameter is not set.
func PathString(r *http.Request, name string) string {
	return r.PathValue(name)
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

// RequireNumericParam creates middleware that validates a specific path parameter is numeric.
// Returns 400 Bad Request if the parameter is not a valid integer.
func RequireNumericParam(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := PathInt(r, param); err != nil {
				http.Error(w, "Invalid "+param+": must be numeric", http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireNumericParams creates middleware that validates multiple path parameters are numeric.
// Returns 400 Bad Request if any parameter is not a valid integer.
func RequireNumericParams(params ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, param := range params {
				if _, err := PathInt(r, param); err != nil {
					http.Error(w, "Invalid "+param+": must be numeric", http.StatusBadRequest)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
