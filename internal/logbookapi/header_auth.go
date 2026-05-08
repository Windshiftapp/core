package logbookapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/contextkeys"
	"windshift/internal/logbookauth"
	"windshift/internal/models"
)

// maxGroupIDs caps how many comma-separated group IDs the middleware will
// parse from the signed header. The HMAC already authenticates the value, but
// an unbounded list would let legitimate users in many groups drive
// long IN (...) queries. Chosen well above any realistic membership count.
const maxGroupIDs = 512

// nonceCacheSize bounds the in-memory replay cache. At 512 concurrent
// in-flight requests per ~MaxSkew (5 min) the cache is comfortably oversized.
const nonceCacheSize = 16384

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

// headerAuthMiddlewareWithCache reads trusted X-Logbook-* headers injected by
// the main server proxy and places both a LogbookUser and a *models.User into
// the request context so existing helpers (utils.GetCurrentUser) keep working.
//
// The sharedSecret is the HMAC key shared with the main server (SSO_SECRET).
// Requests without a valid, fresh HMAC signature are rejected before any
// header value is trusted. The signature covers the HTTP method, path, and a
// per-request nonce, so a captured signature cannot be replayed against a
// different endpoint or reused for the same one within the skew window. The
// nonces cache is passed in so routes mounted under one server share the same
// replay protection.
func headerAuthMiddlewareWithCache(sharedSecret string, nonces *nonceCache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tsStr := r.Header.Get(logbookauth.HeaderTimestamp)
		sig := r.Header.Get(logbookauth.HeaderSignature)
		nonce := r.Header.Get(logbookauth.HeaderNonce)
		if tsStr == "" || sig == "" || nonce == "" {
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}
		now := time.Now()
		if !logbookauth.TimestampFresh(ts, now) {
			slog.Warn("rejecting logbook request with stale signature timestamp",
				"path", r.URL.Path, "skew_seconds", now.Unix()-ts)
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		claims := logbookauth.Claims{
			Timestamp: ts,
			Method:    r.Method,
			Path:      r.URL.Path,
			Nonce:     nonce,
			UserID:    r.Header.Get("X-Logbook-User-ID"),
			Email:     r.Header.Get("X-Logbook-User-Email"),
			FirstName: r.Header.Get("X-Logbook-User-First-Name"),
			LastName:  r.Header.Get("X-Logbook-User-Last-Name"),
			IsAdmin:   r.Header.Get("X-Logbook-Is-Admin"),
			GroupIDs:  r.Header.Get("X-Logbook-Group-IDs"),
		}
		if !logbookauth.Verify(sharedSecret, sig, claims) {
			slog.Warn("rejecting logbook request with invalid signature", "path", r.URL.Path)
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		// Signature verified — now check the nonce is unused. Order matters:
		// only accept a nonce once we've proved the request was authorized,
		// otherwise an unauthenticated attacker could fill the cache.
		if !nonces.checkAndAdd(nonce, now) {
			slog.Warn("rejecting logbook request with replayed nonce", "path", r.URL.Path)
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		userID, err := strconv.Atoi(claims.UserID)
		if err != nil || userID <= 0 {
			http.Error(w, `{"error":"Unauthorized","code":"UNAUTHORIZED"}`, http.StatusUnauthorized)
			return
		}

		isAdmin := claims.IsAdmin == "true"

		var groupIDs []int
		if claims.GroupIDs != "" {
			for _, s := range strings.Split(claims.GroupIDs, ",") {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				gid, err := strconv.Atoi(s)
				if err == nil && gid > 0 {
					groupIDs = append(groupIDs, gid)
					if len(groupIDs) >= maxGroupIDs {
						break
					}
				}
			}
		}

		lbUser := &LogbookUser{
			ID:        userID,
			Email:     claims.Email,
			FirstName: claims.FirstName,
			LastName:  claims.LastName,
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
