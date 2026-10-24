package llm

import (
	"net/http"

	"github.com/google/uuid"
)

// NewSessionID returns a fresh, unique conversation/session identifier with an
// optional human-readable prefix. Callers use it to give a logical conversation
// one stable id; the header transport uses it as the per-request fallback.
func NewSessionID(prefix string) string {
	id := uuid.New().String()
	if prefix == "" {
		return id
	}
	return prefix + "-" + id
}

// providerHeaderTransport adds provider-mandated identifying headers to every
// outbound generation request. It sits above the SDK transport so it applies to
// all three wire protocols (Chat Completions, Responses, Anthropic Messages)
// and to retries of the same request.
type providerHeaderTransport struct {
	next          http.RoundTripper
	userAgent     string
	sessionHeader string
}

func (t *providerHeaderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.userAgent != "" {
		// Override the SDK's generic User-Agent: the provider asked the client
		// to identify itself rather than impersonate an SDK or HTTP library.
		request.Header.Set("User-Agent", t.userAgent)
	}
	if t.sessionHeader != "" && request.Header.Get(t.sessionHeader) == "" {
		// Prefer the caller's conversation id. Fall back to a fresh id so a
		// provider that requires the header never sees a request without one;
		// setting it on the request keeps it stable across retries.
		session := SessionIDFromContext(request.Context())
		if session == "" {
			session = NewSessionID("")
		}
		request.Header.Set(t.sessionHeader, session)
	}
	return t.next.RoundTrip(request)
}
