package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"windshift/internal/utils"
)

// oidcHTTPTimeout caps every outbound call the relying party makes
// (discovery, JWKS fetch, token exchange).
const oidcHTTPTimeout = 15 * time.Second

// newSSRFSafeOIDCClient returns an *http.Client whose Transport can be set to
// refuse loopback / RFC1918 / link-local / CGNAT addresses by explicitly
// disabling --allow-local-connections. Used as a defense-in-depth layer
// beneath upfront IssuerURL validation: closes the validate-then-dial gap (DNS
// rebinding) and also covers the JWKS URL and token endpoint that the IdP
// advertises via discovery — those are not validated upfront because we don't
// see them until discovery runs.
func newSSRFSafeOIDCClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: utils.ConfigureHTTPTransport(&http.Transport{
			DialContext: utils.SafeNetDialer(timeout).DialContext,
		}),
	}
}

var (
	ErrOIDCDiscoveryFailed = errors.New("OIDC discovery failed")
	ErrOIDCTokenExchange   = errors.New("OIDC token exchange failed")
	ErrOIDCInvalidIDToken  = errors.New("invalid OIDC ID token")
	ErrOIDCMissingClaims   = errors.New("required OIDC claims missing")
)

// OIDCClaims represents the extracted claims from an OIDC ID token
type OIDCClaims struct {
	Subject               string         `json:"sub"`
	Email                 string         `json:"email"`
	EmailVerified         bool           `json:"email_verified"`
	EmailVerifiedProvided bool           `json:"email_verified_provided"` // True if IdP included email_verified in claims
	Name                  string         `json:"name"`
	GivenName             string         `json:"given_name"`
	FamilyName            string         `json:"family_name"`
	Username              string         `json:"preferred_username"`
	Picture               string         `json:"picture"`
	Raw                   map[string]any `json:"raw"` // All claims for debugging
}

// OIDCService handles OIDC authentication flows using zitadel/oidc library
type OIDCService struct {
	cookieKey  []byte // 32-byte key for cookie encryption
	httpClient *http.Client
}

// NewOIDCService creates a new OIDC service.
// cookieKey should be a 32-byte key for secure cookie encryption. Discovery /
// JWKS / token calls can block private/loopback destinations when the global
// --allow-local-connections switch is explicitly disabled.
func NewOIDCService(cookieKey []byte) *OIDCService {
	if len(cookieKey) < 32 {
		// Pad key if too short (should not happen in production)
		padded := make([]byte, 32)
		copy(padded, cookieKey)
		cookieKey = padded
	}
	return &OIDCService{
		cookieKey:  cookieKey[:32],
		httpClient: newSSRFSafeOIDCClient(oidcHTTPTimeout),
	}
}

// CreateRelyingParty creates a new OIDC relying party for a provider
func (s *OIDCService) CreateRelyingParty(ctx context.Context, provider *SSOProvider, redirectURI, clientSecret string) (rp.RelyingParty, error) {
	if provider.ProviderType != ProviderTypeOIDC {
		return nil, fmt.Errorf("provider type is not OIDC: %s", provider.ProviderType)
	}

	if provider.IssuerURL == "" {
		return nil, errors.New("issuer URL is required")
	}

	if provider.ClientID == "" {
		return nil, errors.New("client ID is required")
	}

	// Parse scopes
	scopes := strings.Fields(provider.Scopes)
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, oidc.ScopeEmail, oidc.ScopeProfile}
	}

	// Create cookie handler for state management
	cookieHandler := httphelper.NewCookieHandler(s.cookieKey, s.cookieKey)

	// Create options
	options := []rp.Option{
		rp.WithCookieHandler(cookieHandler),
		rp.WithHTTPClient(s.httpClient),
		rp.WithVerifierOpts(
			rp.WithIssuedAtOffset(5*time.Second), // Allow 5s clock skew
			// Pin accepted ID-token signing algorithms. Defense-in-depth against
			// algorithm-confusion attacks if an IdP advertises (or is tricked
			// into advertising) `none` or an HMAC alg via discovery. Asymmetric
			// only — no `none`, no `HS*`.
			rp.WithSupportedSigningAlgorithms(
				"RS256", "RS384", "RS512",
				"ES256", "ES384", "ES512",
				"PS256", "PS384", "PS512",
			),
		),
		rp.WithPKCE(cookieHandler), // Use PKCE for enhanced security
	}

	// Create relying party with OIDC discovery
	relyingParty, err := rp.NewRelyingPartyOIDC(
		ctx,
		provider.IssuerURL,
		provider.ClientID,
		clientSecret,
		redirectURI,
		scopes,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCDiscoveryFailed, err)
	}

	return relyingParty, nil
}

// GetAuthURLHandler returns an HTTP handler that redirects to the OIDC provider
// The state function should return a unique state string for each request
func (s *OIDCService) GetAuthURLHandler(relyingParty rp.RelyingParty, stateFn func() string) http.HandlerFunc {
	return rp.AuthURLHandler(stateFn, relyingParty)
}

// CodeExchangeCallback is the callback type for handling successful authentication
type CodeExchangeCallback = rp.CodeExchangeCallback[*oidc.IDTokenClaims]

// GetCodeExchangeHandler returns an HTTP handler that processes the OIDC callback
func (s *OIDCService) GetCodeExchangeHandler(relyingParty rp.RelyingParty, callback CodeExchangeCallback) http.HandlerFunc {
	return rp.CodeExchangeHandler[*oidc.IDTokenClaims](callback, relyingParty)
}

// ExtractClaims extracts user claims from OIDC tokens using the attribute mapping
func (s *OIDCService) ExtractClaims(tokens *oidc.Tokens[*oidc.IDTokenClaims], attributeMap *AttributeMap) (*OIDCClaims, error) {
	idTokenClaims := tokens.IDTokenClaims
	if idTokenClaims == nil {
		return nil, ErrOIDCInvalidIDToken
	}

	claims := &OIDCClaims{
		Subject: idTokenClaims.Subject,
		Raw:     make(map[string]any),
	}

	// Get all claims for debugging
	allClaims := idTokenClaims.Claims
	for k, v := range allClaims {
		claims.Raw[k] = v
	}

	// Extract email
	if attributeMap != nil && attributeMap.Email != "" {
		if email, ok := getClaimString(allClaims, attributeMap.Email); ok {
			claims.Email = email
		}
	}
	// Fallback to standard claim
	if claims.Email == "" {
		claims.Email = idTokenClaims.Email
	}

	// Check if email_verified was explicitly provided by the IdP.
	// The claim name is configurable via the attribute mapping; it falls back
	// to the standard OIDC "email_verified" claim. The zitadel/oidc library
	// returns a special type that defaults to false, so we inspect the raw
	// claims to know whether the IdP actually sent a value.
	emailVerifiedClaim := "email_verified"
	if attributeMap != nil && attributeMap.EmailVerified != "" {
		emailVerifiedClaim = attributeMap.EmailVerified
	}
	if emailVerifiedValue, exists := allClaims[emailVerifiedClaim]; exists {
		claims.EmailVerifiedProvided = true
		claims.EmailVerified = claimToBool(emailVerifiedValue)
	} else if emailVerifiedClaim == "email_verified" {
		claims.EmailVerified = bool(idTokenClaims.EmailVerified)
	}

	// Extract name fields using attribute mapping or standard claims
	if attributeMap != nil {
		if attributeMap.Name != "" {
			if name, ok := getClaimString(allClaims, attributeMap.Name); ok {
				claims.Name = name
			}
		}
		if attributeMap.GivenName != "" {
			if givenName, ok := getClaimString(allClaims, attributeMap.GivenName); ok {
				claims.GivenName = givenName
			}
		}
		if attributeMap.FamilyName != "" {
			if familyName, ok := getClaimString(allClaims, attributeMap.FamilyName); ok {
				claims.FamilyName = familyName
			}
		}
		if attributeMap.Username != "" {
			if username, ok := getClaimString(allClaims, attributeMap.Username); ok {
				claims.Username = username
			}
		}
	}

	// Picture from claims
	if picture, ok := getClaimString(allClaims, "picture"); ok {
		claims.Picture = picture
	}

	// Fallback to standard claims from the token
	if claims.Name == "" && idTokenClaims.Name != "" {
		claims.Name = idTokenClaims.Name
	}
	if claims.GivenName == "" && idTokenClaims.GivenName != "" {
		claims.GivenName = idTokenClaims.GivenName
	}
	if claims.FamilyName == "" && idTokenClaims.FamilyName != "" {
		claims.FamilyName = idTokenClaims.FamilyName
	}
	if claims.Username == "" && idTokenClaims.PreferredUsername != "" {
		claims.Username = idTokenClaims.PreferredUsername
	}
	if claims.Picture == "" && idTokenClaims.Picture != "" {
		claims.Picture = idTokenClaims.Picture
	}

	// Validate required claims
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: subject (sub) claim is missing", ErrOIDCMissingClaims)
	}

	return claims, nil
}

// TestConnection tests the OIDC provider connection by performing discovery
func (s *OIDCService) TestConnection(ctx context.Context, provider *SSOProvider, clientSecret string) error {
	// Create a timeout context for testing
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use a dummy redirect URI for testing
	dummyRedirectURI := "https://example.com/callback"

	_, err := s.CreateRelyingParty(ctx, provider, dummyRedirectURI, clientSecret)
	if err != nil {
		return err
	}

	return nil
}

// getClaimString extracts a string value from claims map
func getClaimString(claims map[string]any, key string) (string, bool) {
	if claims == nil {
		return "", false
	}
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str, true
		}
	}
	return "", false
}

// claimToBool coerces a raw claim value to a bool. IdPs normally send a JSON
// boolean for email_verified, but some send the string "true"/"false".
func claimToBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		return err == nil && b
	default:
		return false
	}
}
