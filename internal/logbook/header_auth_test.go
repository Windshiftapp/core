package logbook

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"windshift/internal/logbookauth"
)

const testSecret = "test-secret-dont-use-in-prod"

// signedRequest builds a request with a valid signature for the given claims.
// Fields default to a well-formed admin user — override by mutating the
// returned claims before calling sign.
func signedRequest(t *testing.T, secret string, tsOffset time.Duration, overrides func(*logbookauth.Claims)) *http.Request {
	t.Helper()
	claims := logbookauth.Claims{
		Timestamp: time.Now().Add(tsOffset).Unix(),
		Method:    http.MethodGet,
		Path:      "/api/logbook/buckets",
		Nonce:     fmt.Sprintf("nonce-%d", time.Now().UnixNano()),
		UserID:    "42",
		Email:     "user@example.com",
		FirstName: "Ada",
		LastName:  "Lovelace",
		IsAdmin:   "true",
		GroupIDs:  "1,2,3",
	}
	if overrides != nil {
		overrides(&claims)
	}
	req := httptest.NewRequest(claims.Method, claims.Path, nil)
	req.Header.Set("X-Logbook-User-ID", claims.UserID)
	req.Header.Set("X-Logbook-User-Email", claims.Email)
	req.Header.Set("X-Logbook-User-First-Name", claims.FirstName)
	req.Header.Set("X-Logbook-User-Last-Name", claims.LastName)
	req.Header.Set("X-Logbook-Is-Admin", claims.IsAdmin)
	req.Header.Set("X-Logbook-Group-IDs", claims.GroupIDs)
	req.Header.Set(logbookauth.HeaderTimestamp, strconv.FormatInt(claims.Timestamp, 10))
	req.Header.Set(logbookauth.HeaderNonce, claims.Nonce)
	req.Header.Set(logbookauth.HeaderSignature, logbookauth.Sign(secret, claims))
	return req
}

func runMiddleware(t *testing.T, secret string, req *http.Request) (*httptest.ResponseRecorder, *LogbookUser) {
	t.Helper()
	return runMiddlewareWithCache(t, secret, newNonceCache(logbookauth.MaxSkew, nonceCacheSize), req)
}

func runMiddlewareWithCache(t *testing.T, secret string, cache *nonceCache, req *http.Request) (*httptest.ResponseRecorder, *LogbookUser) {
	t.Helper()
	var captured *LogbookUser
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetLogbookUser(r)
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	headerAuthMiddlewareWithCache(secret, cache, inner).ServeHTTP(rec, req)
	return rec, captured
}

func TestHeaderAuth_ValidSignatureAccepted(t *testing.T) {
	rec, user := runMiddleware(t, testSecret, signedRequest(t, testSecret, 0, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body=%s", rec.Code, rec.Body.String())
	}
	if user == nil || user.ID != 42 || !user.IsAdmin {
		t.Fatalf("user not populated correctly: %+v", user)
	}
	if len(user.GroupIDs) != 3 {
		t.Fatalf("want 3 group ids, got %v", user.GroupIDs)
	}
}

func TestHeaderAuth_MissingSignatureRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	req.Header.Del(logbookauth.HeaderSignature)
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_MissingTimestampRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	req.Header.Del(logbookauth.HeaderTimestamp)
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_MissingNonceRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	req.Header.Del(logbookauth.HeaderNonce)
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_WrongSecretRejected(t *testing.T) {
	// Request is signed with testSecret but the middleware runs with a
	// different secret — must reject.
	req := signedRequest(t, testSecret, 0, nil)
	rec, _ := runMiddleware(t, "different-secret", req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_TamperedHeaderRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	// Elevate to admin=false → true by flipping header after signing.
	// Claims were signed as admin=true; change email to something unsigned.
	req.Header.Set("X-Logbook-User-Email", "attacker@evil.example")
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_StaleTimestampRejected(t *testing.T) {
	req := signedRequest(t, testSecret, -(logbookauth.MaxSkew + 10*time.Second), nil)
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_FutureTimestampRejected(t *testing.T) {
	req := signedRequest(t, testSecret, logbookauth.MaxSkew+10*time.Second, nil)
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_InvalidUserIDRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, func(c *logbookauth.Claims) {
		c.UserID = "-1"
	})
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_NonHexSignatureRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	req.Header.Set(logbookauth.HeaderSignature, "v2=not-hex")
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_WrongVersionPrefixRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	// Replace current version prefix with "v1=" — verifier must refuse any
	// version other than the current one, including old schemes.
	sig := req.Header.Get(logbookauth.HeaderSignature)
	req.Header.Set(logbookauth.HeaderSignature, "v1="+sig[len(logbookauth.SignatureVersion+"="):])
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_TamperedPathRejected(t *testing.T) {
	// Sign one path, send the same headers to a different path. Because path
	// is part of the canonical, the signature must not verify.
	req := signedRequest(t, testSecret, 0, nil)
	req.URL.Path = "/api/logbook/documents/attacker-chose"
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_TamperedMethodRejected(t *testing.T) {
	req := signedRequest(t, testSecret, 0, nil)
	req.Method = http.MethodDelete
	rec, _ := runMiddleware(t, testSecret, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHeaderAuth_NonceReplayRejected(t *testing.T) {
	cache := newNonceCache(logbookauth.MaxSkew, nonceCacheSize)
	req := signedRequest(t, testSecret, 0, nil)

	rec1, _ := runMiddlewareWithCache(t, testSecret, cache, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request should succeed, got %d", rec1.Code)
	}

	// Second call with identical headers (same nonce) must be rejected.
	rec2, _ := runMiddlewareWithCache(t, testSecret, cache, signedRequest(t, testSecret, 0, func(c *logbookauth.Claims) {
		// Keep everything identical to the first request so the signature
		// verifies; only the nonce-cache check can catch this.
		*c = logbookauth.Claims{
			Timestamp: c.Timestamp, Method: http.MethodGet, Path: "/api/logbook/buckets",
			Nonce: req.Header.Get(logbookauth.HeaderNonce),
			UserID: "42", Email: "user@example.com",
			FirstName: "Ada", LastName: "Lovelace",
			IsAdmin: "true", GroupIDs: "1,2,3",
		}
	}))
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("nonce replay should be rejected, got %d", rec2.Code)
	}
}

func TestHeaderAuth_GroupIDsCapped(t *testing.T) {
	var b []byte
	for i := 1; i <= maxGroupIDs+50; i++ {
		if i > 1 {
			b = append(b, ',')
		}
		b = append(b, []byte(strconv.Itoa(i))...)
	}
	req := signedRequest(t, testSecret, 0, func(c *logbookauth.Claims) {
		c.GroupIDs = string(b)
	})
	_, user := runMiddleware(t, testSecret, req)
	if user == nil {
		t.Fatal("user should be populated")
	}
	if len(user.GroupIDs) != maxGroupIDs {
		t.Fatalf("want %d group IDs after cap, got %d", maxGroupIDs, len(user.GroupIDs))
	}
}

// Documents the format failure mode we care about most: if signing and
// verification ever disagree on the canonical string, everything breaks.
func TestCanonical_DeterministicAcrossCalls(t *testing.T) {
	claims := logbookauth.Claims{
		Timestamp: 1700000000,
		Method:    http.MethodGet,
		Path:      "/api/logbook/buckets",
		Nonce:     "abc",
		UserID:    "1",
		Email:     "a@b.c",
		IsAdmin:   "false",
	}
	a := logbookauth.Canonical(claims)
	b := logbookauth.Canonical(claims)
	if a != b {
		t.Fatalf("canonical not deterministic:\n%q\nvs\n%q", a, b)
	}
	// Regression guard: spot-check version prefix and newline layout.
	want := fmt.Sprintf("%s\n1700000000\nGET\n/api/logbook/buckets\nabc\n1\na@b.c\n\n\nfalse\n", logbookauth.SignatureVersion)
	if a != want {
		t.Fatalf("canonical format changed — update SignatureVersion before changing the format.\ngot:  %q\nwant: %q", a, want)
	}
}
