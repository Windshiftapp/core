package sso

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
)

var (
	ErrSAMLNotConfigured    = errors.New("SAML provider not fully configured")
	ErrSAMLInvalidCert      = errors.New("invalid IdP certificate")
	ErrSAMLAssertionInvalid = errors.New("SAML assertion validation failed")
)

// SAMLServiceProvider wraps the crewjam/saml SP for a given SSO provider.
type SAMLServiceProvider struct {
	SP       saml.ServiceProvider
	Provider *SSOProvider
}

// NewSAMLServiceProvider creates a SAML SP from an SSOProvider configuration.
// baseURL is the application base URL (e.g. "https://app.example.com").
func NewSAMLServiceProvider(provider *SSOProvider, baseURL string) (*SAMLServiceProvider, error) {
	if provider.ProviderType != ProviderTypeSAML {
		return nil, fmt.Errorf("provider %q is not a SAML provider", provider.Slug)
	}

	if provider.SAMLIdPSSOURL == "" && provider.SAMLIdPMetadataURL == "" {
		return nil, ErrSAMLNotConfigured
	}

	rootURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Parse entity ID
	entityID := provider.SAMLSPEntityID
	if entityID == "" {
		entityID = baseURL + "/api/sso/" + provider.Slug + "/saml/metadata"
	}
	entityIDURL, err := url.Parse(entityID)
	if err != nil {
		return nil, fmt.Errorf("invalid SP entity ID: %w", err)
	}

	// ACS URL
	acsURL, _ := url.Parse(baseURL + "/api/sso/" + provider.Slug + "/saml/acs")

	// Metadata URL
	metadataURL, _ := url.Parse(baseURL + "/api/sso/" + provider.Slug + "/saml/metadata")

	sp := saml.ServiceProvider{
		EntityID:          entityIDURL.String(),
		AcsURL:            *acsURL,
		MetadataURL:       *metadataURL,
		AllowIDPInitiated: true,
	}

	// If IdP metadata URL is provided, fetch and parse it
	if provider.SAMLIdPMetadataURL != "" {
		idpMetadata, fetchErr := fetchIDPMetadata(provider.SAMLIdPMetadataURL)
		if fetchErr != nil {
			slog.Warn("failed to fetch IdP metadata, falling back to manual config",
				"provider", provider.Slug, "error", fetchErr)
		} else {
			sp.IDPMetadata = idpMetadata
		}
	}

	// If no metadata was fetched, build from manual config
	if sp.IDPMetadata == nil {
		idpMetadata, buildErr := buildIDPMetadata(provider)
		if buildErr != nil {
			return nil, fmt.Errorf("failed to build IdP metadata: %w", buildErr)
		}
		sp.IDPMetadata = idpMetadata
	}

	// Set the root URL for the SP
	_ = rootURL // rootURL used for entity ID derivation above

	return &SAMLServiceProvider{
		SP:       sp,
		Provider: provider,
	}, nil
}

// Metadata returns the SP metadata XML document.
func (s *SAMLServiceProvider) Metadata() *saml.EntityDescriptor {
	return s.SP.Metadata()
}

// MakeAuthenticationRequest creates a SAML AuthnRequest and returns the redirect URL.
func (s *SAMLServiceProvider) MakeAuthenticationRequest(relayState string) (*url.URL, error) {
	authReq, err := s.SP.MakeAuthenticationRequest(
		s.SP.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AuthnRequest: %w", err)
	}

	redirectURL, err := authReq.Redirect(relayState, &s.SP)
	if err != nil {
		return nil, fmt.Errorf("failed to create redirect URL: %w", err)
	}

	return redirectURL, nil
}

// ParseResponse validates a SAML response and extracts the assertion attributes.
func (s *SAMLServiceProvider) ParseResponse(r *http.Request) (*SAMLAssertionInfo, error) {
	possibleRequestIDs := []string{} // For SP-initiated flow, we'd track request IDs
	assertion, err := s.SP.ParseResponse(r, possibleRequestIDs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSAMLAssertionInvalid, err)
	}

	info := &SAMLAssertionInfo{
		NameID:     assertion.Subject.NameID.Value,
		Attributes: make(map[string][]string),
	}

	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			var values []string
			for _, v := range attr.Values {
				values = append(values, v.Value)
			}
			// Map by both Name and FriendlyName
			info.Attributes[attr.Name] = values
			if attr.FriendlyName != "" {
				info.Attributes[attr.FriendlyName] = values
			}
		}
	}

	return info, nil
}

// SAMLAssertionInfo contains the parsed information from a SAML assertion.
type SAMLAssertionInfo struct {
	NameID     string              // Subject NameID
	Attributes map[string][]string // Attribute name -> values
}

// GetAttribute returns the first value for a given attribute name.
func (a *SAMLAssertionInfo) GetAttribute(name string) string {
	if vals, ok := a.Attributes[name]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// fetchIDPMetadata fetches and parses IdP metadata from a URL.
func fetchIDPMetadata(metadataURL string) (*saml.EntityDescriptor, error) {
	mdURL, err := url.Parse(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata URL: %w", err)
	}

	metadata, err := samlsp.FetchMetadata(
		context.TODO(),
		http.DefaultClient,
		*mdURL,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata from %s: %w", metadataURL, err)
	}

	return metadata, nil
}

// buildIDPMetadata constructs IdP metadata from manual configuration.
func buildIDPMetadata(provider *SSOProvider) (*saml.EntityDescriptor, error) {
	if provider.SAMLIdPSSOURL == "" {
		return nil, fmt.Errorf("IdP SSO URL is required when metadata URL is not provided")
	}

	ssoURL, err := url.Parse(provider.SAMLIdPSSOURL)
	if err != nil {
		return nil, fmt.Errorf("invalid IdP SSO URL: %w", err)
	}

	descriptor := &saml.EntityDescriptor{
		EntityID: provider.SAMLIdPSSOURL,
		IDPSSODescriptors: []saml.IDPSSODescriptor{
			{
				SingleSignOnServices: []saml.Endpoint{
					{
						Binding:  saml.HTTPRedirectBinding,
						Location: ssoURL.String(),
					},
					{
						Binding:  saml.HTTPPostBinding,
						Location: ssoURL.String(),
					},
				},
			},
		},
	}

	// Parse and add IdP certificate if provided
	if provider.SAMLIdPCertificate != "" {
		cert, parseErr := parsePEMCertificate(provider.SAMLIdPCertificate)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrSAMLInvalidCert, parseErr)
		}

		keyDescriptor := saml.KeyDescriptor{
			Use: "signing",
			KeyInfo: saml.KeyInfo{
				X509Data: saml.X509Data{
					X509Certificates: []saml.X509Certificate{
						{Data: encodeCertDER(cert)},
					},
				},
			},
		}
		descriptor.IDPSSODescriptors[0].KeyDescriptors = []saml.KeyDescriptor{keyDescriptor}
	}

	return descriptor, nil
}

// parsePEMCertificate parses a PEM-encoded X.509 certificate.
func parsePEMCertificate(pemData string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		// Try parsing as raw base64 DER
		cert, err := x509.ParseCertificate([]byte(pemData))
		if err != nil {
			return nil, fmt.Errorf("failed to decode PEM or DER certificate: %w", err)
		}
		return cert, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	return cert, nil
}

// encodeCertDER returns the base64-encoded DER bytes of a certificate.
func encodeCertDER(cert *x509.Certificate) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}))
}

// ValidateSAMLProviderConfig checks if a SAML provider has minimum required configuration.
func ValidateSAMLProviderConfig(provider *SSOProvider) error {
	if provider.SAMLIdPMetadataURL == "" && provider.SAMLIdPSSOURL == "" {
		return fmt.Errorf("either IdP metadata URL or IdP SSO URL must be provided")
	}
	if provider.SAMLIdPSSOURL != "" && provider.SAMLIdPCertificate == "" {
		return fmt.Errorf("IdP certificate is required when configuring manually (without metadata URL)")
	}
	return nil
}

// Ensure SAMLServiceProvider key types are compatible at compile time
var _ xml.Marshaler = (*saml.EntityDescriptor)(nil)
var _ *rsa.PrivateKey // referenced for potential future SP signing
