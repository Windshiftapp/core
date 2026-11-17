package llm

import (
	"net/http"
	"strings"
	"time"

	"windshift/internal/utils"
)

// joinProviderPath appends an API path to an admin-configured provider base URL.
//
// Many OpenAI-compatible tools document their base URL as .../v1, while the
// built-in Windshift provider definitions keep the host/root URL and store the
// /v1/... suffix in ChatPath/ModelsEndpoint. Accept both forms so a custom
// endpoint entered as http://localhost:11434/v1 does not become
// http://localhost:11434/v1/v1/chat/completions.
func joinProviderPath(baseURL, apiPath string) string {
	base := strings.TrimRight(baseURL, "/")
	path := "/" + strings.TrimLeft(apiPath, "/")
	if base == "" {
		return path
	}
	path = dropDuplicateVersionPrefix(base, path)
	return base + path
}

// dropDuplicateVersionPrefix removes a leading version segment ("/v1",
// "/v1beta", …) from apiPath when the base URL already carries that same
// version as one of its own path segments.
//
// The built-in provider definitions keep the version inside the API path
// (/v1/chat/completions, /v1/models), which is correct for a host-root base URL
// (https://api.deepseek.com) and for a version-root base URL
// (https://api.deepseek.com/v1). It is wrong for an API root that continues
// after the version: joining https://gateway.example.com/v1/unified-model with
// /v1/models produced .../v1/unified-model/v1/models, which the upstream
// rejects because the extra segment reads as a resource id. A base URL whose
// path has no version segment at all (a reverse-proxy prefix such as
// https://gateway.example.com/openai) keeps the version, preserving the
// prefix-then-/v1 layout.
func dropDuplicateVersionPrefix(baseURL, apiPath string) string {
	segment, _, _ := strings.Cut(strings.TrimPrefix(apiPath, "/"), "/")
	if !isVersionPathSegment(segment) {
		return apiPath
	}
	for _, baseSegment := range strings.Split(baseURL, "/") {
		if baseSegment == segment {
			return strings.TrimPrefix(apiPath, "/"+segment)
		}
	}
	return apiPath
}

// isVersionPathSegment reports whether path segment looks like an API version
// ("v1", "v4", "v1beta") rather than a resource name, so only version prefixes
// are ever de-duplicated.
func isVersionPathSegment(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' || segment[1] < '0' || segment[1] > '9' {
		return false
	}
	for _, r := range segment[2:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

// newAdminConfiguredHTTPClient returns a client for URLs configured by a system
// administrator. Local/internal endpoints are reachable by default; operators
// can restore loopback/private/metadata blocking with
// --allow-local-connections=false.
func newAdminConfiguredHTTPClient(timeout time.Duration) *http.Client {
	if timeout == 0 {
		timeout = DefaultRequestTimeout
	}
	return utils.NewSSRFSafeHTTPClient(timeout)
}
