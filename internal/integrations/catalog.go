package integrations

import "windshift/internal/models"

// CredentialOwner describes who owns the authorization established for an
// integration provider. User-owned credentials are stored per user, while
// system-owned credentials back an administrator-managed connection.
type CredentialOwner string

const (
	CredentialOwnerUser   CredentialOwner = "user"
	CredentialOwnerSystem CredentialOwner = "system"
)

// ProviderCapabilities declares which parts of the shared integration core a
// provider can safely use. Provider-specific synchronization remains outside
// this catalog.
type ProviderCapabilities struct {
	Type               models.IntegrationProviderType
	CredentialOwner    CredentialOwner
	AdminProviderCRUD  bool
	OAuth              bool
	GenericItemLinks   bool
	GenericItemRefresh bool
}

var providerCatalog = map[models.IntegrationProviderType]ProviderCapabilities{
	models.IntegrationProviderNotion: {
		Type:               models.IntegrationProviderNotion,
		CredentialOwner:    CredentialOwnerUser,
		AdminProviderCRUD:  true,
		OAuth:              true,
		GenericItemLinks:   true,
		GenericItemRefresh: true,
	},
	models.IntegrationProviderTodoist: {
		Type:              models.IntegrationProviderTodoist,
		CredentialOwner:   CredentialOwnerUser,
		AdminProviderCRUD: true,
		OAuth:             true,
		GenericItemLinks:  true,
	},
	models.IntegrationProviderZammad: {
		Type:            models.IntegrationProviderZammad,
		CredentialOwner: CredentialOwnerSystem,
		OAuth:           true,
	},
}

// Capabilities returns the declared capabilities for a known provider type.
// Unknown types fail closed.
func Capabilities(providerType models.IntegrationProviderType) (ProviderCapabilities, bool) {
	capabilities, ok := providerCatalog[providerType]
	return capabilities, ok
}

// IsKnown reports whether a provider type is registered with the integration
// core.
func IsKnown(providerType models.IntegrationProviderType) bool {
	_, ok := Capabilities(providerType)
	return ok
}

// SupportsAdminProviderCRUD reports whether the generic provider settings API
// owns the provider row.
func SupportsAdminProviderCRUD(providerType models.IntegrationProviderType) bool {
	capabilities, ok := Capabilities(providerType)
	return ok && capabilities.AdminProviderCRUD
}

// SupportsUserOAuth reports whether the shared user connection flow owns the
// provider's OAuth token.
func SupportsUserOAuth(providerType models.IntegrationProviderType) bool {
	capabilities, ok := Capabilities(providerType)
	return ok && capabilities.OAuth && capabilities.CredentialOwner == CredentialOwnerUser
}

// SupportsGenericItemLinks reports whether callers may create and delete this
// provider's links through the generic item-link endpoints.
func SupportsGenericItemLinks(providerType models.IntegrationProviderType) bool {
	capabilities, ok := Capabilities(providerType)
	return ok && capabilities.GenericItemLinks
}

// SupportsGenericItemRefresh reports whether the generic refresh endpoint has
// an implementation for the provider.
func SupportsGenericItemRefresh(providerType models.IntegrationProviderType) bool {
	capabilities, ok := Capabilities(providerType)
	return ok && capabilities.GenericItemRefresh
}
