package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"windshift/internal/auth"
	"windshift/internal/repository"
)

const oauthRegistrationMaxBytes = 64 << 10

// configureOAuthServer derives every advertised endpoint from the externally
// visible issuer. BASE_URL already includes the configured context path.
func (h *OAuthHandler) configureOAuthServer(cfg OAuthServerConfig) {
	h.issuerURL = strings.TrimRight(strings.TrimSpace(cfg.IssuerURL), "/")
	if h.issuerURL == "" || !cfg.MCPEnabled {
		return
	}
	h.mcpResourceURI = h.issuerURL + "/mcp"
	h.mcpMetadataURI = h.issuerURL + "/.well-known/oauth-protected-resource/mcp"
}

// MCPResourceURI is the RFC 8707 audience used for MCP access tokens.
func (h *OAuthHandler) MCPResourceURI() string { return h.mcpResourceURI }

// MCPProtectedResourceMetadataURI is advertised in Bearer challenges.
func (h *OAuthHandler) MCPProtectedResourceMetadataURI() string { return h.mcpMetadataURI }

// MCPDiscoveryEnabled reports whether the OAuth handler has a canonical MCP
// audience. Routes use it to avoid advertising an unavailable resource.
func (h *OAuthHandler) MCPDiscoveryEnabled() bool {
	return h.mcpResourceURI != "" && h.mcpMetadataURI != ""
}

// ProtectedResourceMetadata implements RFC 9728 discovery for /mcp.
func (h *OAuthHandler) ProtectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthMetadata(w, map[string]any{
		"resource":                 h.mcpResourceURI,
		"resource_name":            "Windshift MCP",
		"authorization_servers":    []string{h.issuerURL},
		"scopes_supported":         nonAdminOAuthScopes(),
		"bearer_methods_supported": []string{"header"},
	})
}

// AuthorizationServerMetadata implements RFC 8414 discovery for Windshift's
// authorization-code server.
func (h *OAuthHandler) AuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthMetadata(w, map[string]any{
		"issuer":                                         h.issuerURL,
		"authorization_endpoint":                         h.issuerURL + "/oauth/authorize",
		"token_endpoint":                                 h.issuerURL + "/api/oauth/token",
		"registration_endpoint":                          h.issuerURL + "/api/oauth/register",
		"scopes_supported":                               nonAdminOAuthScopes(),
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported":          []string{"none", "client_secret_post"},
		"code_challenge_methods_supported":               []string{"S256"},
		"authorization_response_iss_parameter_supported": false,
	})
}

type dynamicClientRegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

type dynamicClientRegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

// RegisterDynamicClient implements a constrained RFC 7591 registration
// endpoint for public OAuth clients. It creates public clients only:
// authorization-code plus refresh-token grants, S256 PKCE, and HTTPS or
// loopback callbacks. Registration does not select a protected-resource
// audience; clients that need one (such as MCP clients) bind it on the
// authorization request via RFC 8707's resource parameter.
func (h *OAuthHandler) RegisterDynamicClient(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, oauthRegistrationMaxBytes)
	var req dynamicClientRegistrationRequest
	decoder := newJSONDecoder(w, r)
	if err := decoder.Decode(&req); err != nil {
		writeDynamicRegistrationError(w, http.StatusBadRequest, "invalid JSON registration request")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeDynamicRegistrationError(w, http.StatusBadRequest, "registration request must contain one JSON object")
		return
	}

	if err := validateDynamicRegistration(&req); err != nil {
		writeDynamicRegistrationError(w, http.StatusBadRequest, err.Error())
		return
	}

	clientID, err := generateOAuthClientID()
	if err != nil {
		writeDynamicRegistrationError(w, http.StatusInternalServerError, "could not generate client identifier")
		return
	}
	displayName := strings.TrimSpace(req.ClientName)
	if displayName == "" {
		displayName = "OAuth client"
	}
	slug := "oauth-" + strings.TrimPrefix(clientID, "wsoc_")[:16]
	redirectsJSON, _ := json.Marshal(req.RedirectURIs)
	// allowed_scopes caps what this client may ever hold — the full non-admin
	// catalog, so a client that wants a destructive scope can still step up to
	// it through an incremental authorization. The registration response
	// advertises the smaller DefaultAgentScopes instead, so a client that
	// simply echoes back what it was offered lands on the same capabilities a
	// `ws` CLI token gets rather than asking for delete scopes it won't use.
	allowedScopes := nonAdminOAuthScopes()
	scopesJSON, _ := json.Marshal(allowedScopes)

	err = repository.NewOAuthClientRepository(h.db).CreateDynamicPublicClient(
		r.Context(), slug, displayName, clientID, string(redirectsJSON), string(scopesJSON), "",
	)
	if err != nil {
		if errors.Is(err, repository.ErrOAuthClientRegistrationCapacity) {
			writeOAuthJSON(w, http.StatusTooManyRequests, map[string]string{
				"error":             "temporarily_unavailable",
				"error_description": "OAuth client registration capacity reached; contact an administrator",
			})
			return
		}
		if errors.Is(err, repository.ErrDuplicateEntry) {
			writeDynamicRegistrationError(w, http.StatusConflict, "client identifier collision")
			return
		}
		writeDynamicRegistrationError(w, http.StatusInternalServerError, "could not register client")
		return
	}

	writeOAuthJSON(w, http.StatusCreated, dynamicClientRegistrationResponse{
		ClientID:                clientID,
		ClientIDIssuedAt:        time.Now().Unix(),
		ClientName:              displayName,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   strings.Join(auth.DefaultAgentScopes, " "),
	})
}

func validateDynamicRegistration(req *dynamicClientRegistrationRequest) error {
	if req.TokenEndpointAuthMethod == "" {
		req.TokenEndpointAuthMethod = "none"
	}
	if req.TokenEndpointAuthMethod != "none" {
		return fmt.Errorf("token_endpoint_auth_method must be 'none'")
	}
	if !validDynamicGrantTypes(req.GrantTypes) {
		return fmt.Errorf("grant_types must contain authorization_code and may contain refresh_token")
	}
	if !sameStringSetOrDefault(req.ResponseTypes, []string{"code"}) {
		return fmt.Errorf("response_types must contain code only")
	}
	if err := validateDynamicRedirectURIs(req.RedirectURIs); err != nil {
		return err
	}
	if len(req.ClientName) > 128 {
		return fmt.Errorf("client_name must be at most 128 characters")
	}
	for _, r := range req.ClientName {
		if unicode.IsControl(r) {
			return fmt.Errorf("client_name must not contain control characters")
		}
	}
	requested := splitOAuthScopes(req.Scope)
	if len(requested) > 0 {
		if err := auth.ValidateScopes(requested); err != nil {
			return err
		}
		for _, scope := range requested {
			if auth.IsAdminScope(scope) {
				return fmt.Errorf("admin scopes cannot be registered dynamically")
			}
		}
	}
	return nil
}

func validDynamicGrantTypes(grantTypes []string) bool {
	if len(grantTypes) == 0 {
		return true
	}
	seenAuthorizationCode := false
	for _, grantType := range grantTypes {
		switch grantType {
		case "authorization_code":
			seenAuthorizationCode = true
		case "refresh_token":
		default:
			return false
		}
	}
	return seenAuthorizationCode
}

func validateDynamicRedirectURIs(uris []string) error {
	if err := validateRedirectURIs(uris); err != nil {
		return err
	}
	for _, raw := range uris {
		u, _ := url.Parse(raw)
		if u.User != nil {
			return fmt.Errorf("redirect_uris must not contain user information")
		}
		switch strings.ToLower(u.Scheme) {
		case "https":
			continue
		case "http":
			host := u.Hostname()
			if host == "localhost" || isLoopbackIP(host) {
				continue
			}
			return fmt.Errorf("http redirect_uris are allowed only for loopback hosts")
		default:
			return fmt.Errorf("redirect_uris must use https or an http loopback host")
		}
	}
	return nil
}

func isLoopbackIP(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func sameStringSetOrDefault(got, want []string) bool {
	if len(got) == 0 {
		return true
	}
	if len(got) != len(want) {
		return false
	}
	set := make(map[string]struct{}, len(got))
	for _, value := range got {
		set[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func nonAdminOAuthScopes() []string {
	return auth.NonAdminScopes()
}

func (h *OAuthHandler) validateOAuthResource(client *oauthClientRow, raw string) (string, error) {
	if raw == "" {
		if client.ResourceURI != "" {
			return "", fmt.Errorf("resource is required for this client")
		}
		return "", nil
	}
	resource, err := canonicalOAuthResource(raw)
	if err != nil {
		return "", err
	}
	expected := client.ResourceURI
	if expected == "" {
		expected = h.mcpResourceURI
	}
	if expected == "" || resource != expected {
		return "", fmt.Errorf("resource is not registered for this client")
	}
	return resource, nil
}

func canonicalOAuthResource(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("resource must be an absolute URI")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("resource must use http or https")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("resource must not contain user information, a query, or a fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

func matchStoredOAuthResource(stored, requested string) (string, error) {
	if stored == "" {
		if requested != "" {
			return "", fmt.Errorf("resource does not match the authorization grant")
		}
		return "", nil
	}
	if requested == "" {
		return stored, nil
	}
	canonical, err := canonicalOAuthResource(requested)
	if err != nil || canonical != stored {
		return "", fmt.Errorf("resource does not match the authorization grant")
	}
	return stored, nil
}

func writeOAuthMetadata(w http.ResponseWriter, body any) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeOAuthJSON(w, http.StatusOK, body)
}

func writeOAuthJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeDynamicRegistrationError(w http.ResponseWriter, status int, description string) {
	writeOAuthJSON(w, status, map[string]string{
		"error":             "invalid_client_metadata",
		"error_description": description,
	})
}
