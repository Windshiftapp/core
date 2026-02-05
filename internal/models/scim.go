package models

import "time"

// SCIM 2.0 Schema URIs
const (
	SCIMSchemaUser                  = "urn:ietf:params:scim:schemas:core:2.0:User"
	SCIMSchemaGroup                 = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SCIMSchemaListResponse          = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SCIMSchemaError                 = "urn:ietf:params:scim:api:messages:2.0:Error"
	SCIMSchemaPatchOp               = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SCIMSchemaServiceProviderConfig = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SCIMSchemaResourceType          = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	SCIMSchemaSchema                = "urn:ietf:params:scim:schemas:core:2.0:Schema"
)

// =============================================================================
// SCIM Token Models
// =============================================================================

// SCIMToken represents a dedicated SCIM authentication token
type SCIMToken struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"-"` // Never expose hash
	TokenPrefix string     `json:"token_prefix"`
	IsActive    bool       `json:"is_active"`
	CreatedBy   *int       `json:"created_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Joined fields
	CreatedByName string `json:"created_by_name,omitempty"`
}

// SCIMTokenCreate represents a request to create a SCIM token
type SCIMTokenCreate struct {
	Name      string     `json:"name" validate:"required,min=1,max=100"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// SCIMTokenResponse is returned when a token is created (includes raw token once)
type SCIMTokenResponse struct {
	Token     string    `json:"token"` // Only returned on creation
	SCIMToken SCIMToken `json:"scim_token"`
}

// =============================================================================
// SCIM 2.0 Resource Models (RFC 7643)
// =============================================================================

// SCIMMeta represents SCIM resource metadata
type SCIMMeta struct {
	ResourceType string     `json:"resourceType"`
	Created      *time.Time `json:"created,omitempty"`
	LastModified *time.Time `json:"lastModified,omitempty"`
	Location     string     `json:"location,omitempty"`
	Version      string     `json:"version,omitempty"`
}

// SCIMName represents a user's name in SCIM format
type SCIMName struct {
	Formatted       string `json:"formatted,omitempty"`
	FamilyName      string `json:"familyName,omitempty"`
	GivenName       string `json:"givenName,omitempty"`
	MiddleName      string `json:"middleName,omitempty"`
	HonorificPrefix string `json:"honorificPrefix,omitempty"`
	HonorificSuffix string `json:"honorificSuffix,omitempty"`
}

// SCIMEmail represents an email address in SCIM format
type SCIMEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// SCIMUser represents a SCIM 2.0 User resource
type SCIMUser struct {
	Schemas     []string    `json:"schemas"`
	ID          string      `json:"id,omitempty"`
	ExternalID  string      `json:"externalId,omitempty"`
	UserName    string      `json:"userName"`
	Name        SCIMName    `json:"name,omitempty"`
	DisplayName string      `json:"displayName,omitempty"`
	Emails      []SCIMEmail `json:"emails,omitempty"`
	Active      bool        `json:"active"`
	Meta        *SCIMMeta   `json:"meta,omitempty"`
}

// SCIMGroupMember represents a member reference in a SCIM Group
type SCIMGroupMember struct {
	Value   string `json:"value"`             // User ID
	Ref     string `json:"$ref,omitempty"`    // URI reference to user
	Display string `json:"display,omitempty"` // Display name
}

// SCIMGroup represents a SCIM 2.0 Group resource
type SCIMGroup struct {
	Schemas     []string          `json:"schemas"`
	ID          string            `json:"id,omitempty"`
	ExternalID  string            `json:"externalId,omitempty"`
	DisplayName string            `json:"displayName"`
	Members     []SCIMGroupMember `json:"members,omitempty"`
	Meta        *SCIMMeta         `json:"meta,omitempty"`
}

// SCIMListResponse represents a SCIM 2.0 list response
type SCIMListResponse struct {
	Schemas      []string      `json:"schemas"`
	TotalResults int           `json:"totalResults"`
	StartIndex   int           `json:"startIndex"`
	ItemsPerPage int           `json:"itemsPerPage"`
	Resources    []interface{} `json:"Resources"`
}

// =============================================================================
// SCIM Error Response (RFC 7644 Section 3.12)
// =============================================================================

// SCIMError represents a SCIM error response
type SCIMError struct {
	Schemas  []string `json:"schemas"`
	Detail   string   `json:"detail"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
}

// NewSCIMError creates a new SCIM error response
func NewSCIMError(status int, detail, scimType string) *SCIMError {
	return &SCIMError{
		Schemas:  []string{SCIMSchemaError},
		Detail:   detail,
		Status:   string(rune('0'+status/100)) + string(rune('0'+(status/10)%10)) + string(rune('0'+status%10)),
		ScimType: scimType,
	}
}

// =============================================================================
// SCIM Patch Operation (RFC 7644 Section 3.5.2)
// =============================================================================

// SCIMPatchOp represents a single SCIM patch operation
type SCIMPatchOp struct {
	Op    string      `json:"op"`             // add, remove, replace
	Path  string      `json:"path,omitempty"` // Attribute path (optional for add/replace at root)
	Value interface{} `json:"value,omitempty"`
}

// SCIMPatchRequest represents a SCIM PATCH request
type SCIMPatchRequest struct {
	Schemas    []string      `json:"schemas"`
	Operations []SCIMPatchOp `json:"Operations"`
}

// =============================================================================
// SCIM Service Provider Config (RFC 7643 Section 5)
// =============================================================================

// SCIMSupported represents a boolean support indicator
type SCIMSupported struct {
	Supported bool `json:"supported"`
}

// SCIMBulkConfig represents bulk operation configuration
type SCIMBulkConfig struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

// SCIMFilterConfig represents filter configuration
type SCIMFilterConfig struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

// SCIMAuthScheme represents an authentication scheme
type SCIMAuthScheme struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	SpecURI          string `json:"specUri,omitempty"`
	DocumentationURI string `json:"documentationUri,omitempty"`
	Primary          bool   `json:"primary,omitempty"`
}

// SCIMServiceProviderConfig represents the service provider configuration
type SCIMServiceProviderConfig struct {
	Schemas               []string         `json:"schemas"`
	DocumentationURI      string           `json:"documentationUri,omitempty"`
	Patch                 SCIMSupported    `json:"patch"`
	Bulk                  SCIMBulkConfig   `json:"bulk"`
	Filter                SCIMFilterConfig `json:"filter"`
	ChangePassword        SCIMSupported    `json:"changePassword"`
	Sort                  SCIMSupported    `json:"sort"`
	Etag                  SCIMSupported    `json:"etag"`
	AuthenticationSchemes []SCIMAuthScheme `json:"authenticationSchemes"`
	Meta                  *SCIMMeta        `json:"meta,omitempty"`
}

// SCIMSchemaExtension represents a schema extension
type SCIMSchemaExtension struct {
	Schema   string `json:"schema"`
	Required bool   `json:"required"`
}

// SCIMResourceType represents a SCIM resource type definition
type SCIMResourceType struct {
	Schemas          []string              `json:"schemas"`
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	Endpoint         string                `json:"endpoint"`
	Schema           string                `json:"schema"`
	SchemaExtensions []SCIMSchemaExtension `json:"schemaExtensions,omitempty"`
	Meta             *SCIMMeta             `json:"meta,omitempty"`
}

// =============================================================================
// SCIM Schema Definition (RFC 7643 Section 7)
// =============================================================================

// SCIMSchemaAttribute represents a schema attribute definition
type SCIMSchemaAttribute struct {
	Name            string                `json:"name"`
	Type            string                `json:"type"` // string, boolean, decimal, integer, dateTime, reference, complex
	MultiValued     bool                  `json:"multiValued"`
	Description     string                `json:"description,omitempty"`
	Required        bool                  `json:"required"`
	CaseExact       bool                  `json:"caseExact,omitempty"`
	Mutability      string                `json:"mutability,omitempty"`      // readOnly, readWrite, immutable, writeOnly
	Returned        string                `json:"returned,omitempty"`        // always, never, default, request
	Uniqueness      string                `json:"uniqueness,omitempty"`      // none, server, global
	SubAttributes   []SCIMSchemaAttribute `json:"subAttributes,omitempty"`   // For complex types
	ReferenceTypes  []string              `json:"referenceTypes,omitempty"`  // For reference types
	CanonicalValues []string              `json:"canonicalValues,omitempty"` // Suggested values
}

// SCIMSchema represents a SCIM schema definition
type SCIMSchema struct {
	Schemas     []string              `json:"schemas"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Attributes  []SCIMSchemaAttribute `json:"attributes"`
	Meta        *SCIMMeta             `json:"meta,omitempty"`
}
