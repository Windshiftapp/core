package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

var (
	ErrOIDCDiscoveryFailed = errors.New("OIDC discovery failed")
	ErrOIDCTokenExchange   = errors.New("OIDC token exchange failed")
	ErrOIDCInvalidIDToken  = errors.New("invalid OIDC ID token")
	ErrOIDCMissingClaims   = errors.New("required OIDC claims missing")
)

// OIDCClaims represents the extracted claims from an OIDC ID token
type OIDCClaims struct {
	Subject               string                 `json:"sub"`
	Email                 string                 `json:"email"`
	EmailVerified         bool                   `json:"email_verified"`
	EmailVerifiedProvided bool                   `json:"email_verified_provided"` // True if IdP included email_verified in claims
	Name                  string                 `json:"name"`
	GivenName             string                 `json:"given_name"`
	FamilyName            string                 `json:"family_name"`
	Username              string                 `json:"preferred_username"`
	Picture               string                 `json:"picture"`
	Raw                   map[string]interface{} `json:"raw"` // All claims for debugging
}

// OIDCService handles OIDC authentication flows using zitadel/oidc library
type OIDCService struct {
	cookieKey []byte // 32-byte key for cookie encryption
}

// NewOIDCService creates a new OIDC service
// cookieKey should be a 32-byte key for secure cookie encryption
func NewOIDCService(cookieKey []byte) *OIDCService {
	if len(cookieKey) < 32 {
		// Pad key if too short (should not happen in production)
		padded := make([]byte, 32)
		copy(padded, cookieKey)
		cookieKey = padded
	}
	return &OIDCService{
		cookieKey: cookieKey[:32],
	}
}

// CreateRelyingParty creates a new OIDC relying party for a provider
func (s *OIDCService) CreateRelyingParty(ctx context.Context, provider *SSOProvider, redirectURI string, clientSecret string) (rp.RelyingParty, error) {
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
		rp.WithVerifierOpts(
			rp.WithIssuedAtOffset(5 * time.Second), // Allow 5s clock skew
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
		return nil, fmt.Errorf("%w: %v", ErrOIDCDiscoveryFailed, err)
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
		Raw:     make(map[string]interface{}),
	}

	// Get all claims for debugging
	allClaims := idTokenClaims.Claims
	if allClaims != nil {
		for k, v := range allClaims {
			claims.Raw[k] = v
		}
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

	// Check if email_verified was explicitly provided by the IdP
	// The zitadel/oidc library returns a special type that defaults to false
	// We need to check the raw claims to know if it was actually provided
	if allClaims != nil {
		if _, exists := allClaims["email_verified"]; exists {
			claims.EmailVerifiedProvided = true
		}
	}
	claims.EmailVerified = bool(idTokenClaims.EmailVerified)

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
func getClaimString(claims map[string]interface{}, key string) (string, bool) {
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
