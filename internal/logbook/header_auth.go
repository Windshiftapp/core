package logbook

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
)

// logbookUserKey is the context key for LogbookUser.
type logbookUserKey struct{}

// LogbookUser represents the authenticated user as provided by the main server
// proxy via X-Logbook-* headers.
type LogbookUser struct {
	ID        int
	Email     string
	FirstName string
	LastName  string
	IsAdmin   bool
	GroupIDs  []int
}

// GetLogbookUser retrieves the LogbookUser from the request context.
func GetLogbookUser(r *http.Request) *LogbookUser {
	val := r.Context().Value(logbookUserKey{})
	if val == nil {
		return nil
	}
	u, _ := val.(*LogbookUser)
	return u
}

// headerAuthMiddleware reads trusted X-Logbook-* headers injected by the
// main server proxy and places both a LogbookUser and a *models.User into
// the request context so existing helpers (utils.GetCurrentUser) keep working.
func headerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userIDStr := r.Header.Get("X-Logbook-User-ID")
		if userIDStr == "" {
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		userID, err := strconv.Atoi(userIDStr)
		if err != nil || userID <= 0 {
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		isAdmin := r.Header.Get("X-Logbook-Is-Admin") == "true"

		var groupIDs []int
		if gids := r.Header.Get("X-Logbook-Group-IDs"); gids != "" {
			for _, s := range strings.Split(gids, ",") {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				gid, err := strconv.Atoi(s)
				if err == nil && gid > 0 {
					groupIDs = append(groupIDs, gid)
				}
			}
		}

		lbUser := &LogbookUser{
			ID:        userID,
			Email:     r.Header.Get("X-Logbook-User-Email"),
			FirstName: r.Header.Get("X-Logbook-User-First-Name"),
			LastName:  r.Header.Get("X-Logbook-User-Last-Name"),
			IsAdmin:   isAdmin,
			GroupIDs:  groupIDs,
		}

		// Build a minimal *models.User so utils.GetCurrentUser still works
		modelUser := &models.User{
			ID:            userID,
			Email:         lbUser.Email,
			FirstName:     lbUser.FirstName,
			LastName:      lbUser.LastName,
			IsSystemAdmin: isAdmin,
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, logbookUserKey{}, lbUser)
		ctx = context.WithValue(ctx, contextkeys.User, modelUser)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
