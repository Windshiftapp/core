package handlers

import "windshift/internal/models"

// GetUserSchema returns the SCIM User schema definition
func GetUserSchema() models.SCIMSchema {
	return models.SCIMSchema{
		Schemas:     []string{models.SCIMSchemaSchema},
		ID:          models.SCIMSchemaUser,
		Name:        "User",
		Description: "User Account",
		Attributes: []models.SCIMSchemaAttribute{
			{
				Name:        "userName",
				Type:        "string",
				MultiValued: false,
				Description: "Unique identifier for the User, typically used by the user to directly authenticate to the service provider.",
				Required:    true,
				CaseExact:   false,
				Mutability:  "readWrite",
				Returned:    "default",
				Uniqueness:  "server",
			},
			{
				Name:        "name",
				Type:        "complex",
				MultiValued: false,
				Description: "The components of the user's real name.",
				Required:    false,
				Mutability:  "readWrite",
				Returned:    "default",
				SubAttributes: []models.SCIMSchemaAttribute{
					{
						Name:        "formatted",
						Type:        "string",
						MultiValued: false,
						Description: "The full name, including all middle names, titles, and suffixes as appropriate, formatted for display.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
					{
						Name:        "familyName",
						Type:        "string",
						MultiValued: false,
						Description: "The family name of the User, or last name in most Western languages.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
					{
						Name:        "givenName",
						Type:        "string",
						MultiValued: false,
						Description: "The given name of the User, or first name in most Western languages.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
					{
						Name:        "middleName",
						Type:        "string",
						MultiValued: false,
						Description: "The middle name(s) of the User.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
					{
						Name:        "honorificPrefix",
						Type:        "string",
						MultiValued: false,
						Description: "The honorific prefix(es) of the User, or title in most Western languages.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
					{
						Name:        "honorificSuffix",
						Type:        "string",
						MultiValued: false,
						Description: "The honorific suffix(es) of the User, or suffix in most Western languages.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
				},
			},
			{
				Name:        "displayName",
				Type:        "string",
				MultiValued: false,
				Description: "The name of the User, suitable for display to end-users.",
				Required:    false,
				Mutability:  "readWrite",
				Returned:    "default",
			},
			{
				Name:        "emails",
				Type:        "complex",
				MultiValued: true,
				Description: "Email addresses for the user.",
				Required:    false,
				Mutability:  "readWrite",
				Returned:    "default",
				SubAttributes: []models.SCIMSchemaAttribute{
					{
						Name:        "value",
						Type:        "string",
						MultiValued: false,
						Description: "Email address value.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
					{
						Name:            "type",
						Type:            "string",
						MultiValued:     false,
						Description:     "A label indicating the attribute's function, e.g., 'work' or 'home'.",
						Required:        false,
						Mutability:      "readWrite",
						Returned:        "default",
						CanonicalValues: []string{"work", "home", "other"},
					},
					{
						Name:        "primary",
						Type:        "boolean",
						MultiValued: false,
						Description: "A Boolean value indicating the 'primary' or preferred attribute value for this attribute.",
						Required:    false,
						Mutability:  "readWrite",
						Returned:    "default",
					},
				},
			},
			{
				Name:        "active",
				Type:        "boolean",
				MultiValued: false,
				Description: "A Boolean value indicating the User's administrative status.",
				Required:    false,
				Mutability:  "readWrite",
				Returned:    "default",
			},
			{
				Name:        "externalId",
				Type:        "string",
				MultiValued: false,
				Description: "A String that is an identifier for the resource as defined by the provisioning client.",
				Required:    false,
				CaseExact:   true,
				Mutability:  "readWrite",
				Returned:    "default",
			},
		},
	}
}

// GetGroupSchema returns the SCIM Group schema definition
func GetGroupSchema() models.SCIMSchema {
	return models.SCIMSchema{
		Schemas:     []string{models.SCIMSchemaSchema},
		ID:          models.SCIMSchemaGroup,
		Name:        "Group",
		Description: "Group",
		Attributes: []models.SCIMSchemaAttribute{
			{
				Name:        "displayName",
				Type:        "string",
				MultiValued: false,
				Description: "A human-readable name for the Group.",
				Required:    true,
				CaseExact:   false,
				Mutability:  "readWrite",
				Returned:    "default",
				Uniqueness:  "none",
			},
			{
				Name:        "members",
				Type:        "complex",
				MultiValued: true,
				Description: "A list of members of the Group.",
				Required:    false,
				Mutability:  "readWrite",
				Returned:    "default",
				SubAttributes: []models.SCIMSchemaAttribute{
					{
						Name:           "value",
						Type:           "string",
						MultiValued:    false,
						Description:    "Identifier of the member of this Group.",
						Required:       false,
						Mutability:     "immutable",
						Returned:       "default",
						ReferenceTypes: []string{"User"},
					},
					{
						Name:           "$ref",
						Type:           "reference",
						MultiValued:    false,
						Description:    "The URI of the corresponding 'User' resource to which the member belongs.",
						Required:       false,
						Mutability:     "immutable",
						Returned:       "default",
						ReferenceTypes: []string{"User"},
					},
					{
						Name:        "display",
						Type:        "string",
						MultiValued: false,
						Description: "A human-readable name for the member.",
						Required:    false,
						Mutability:  "readOnly",
						Returned:    "default",
					},
				},
			},
			{
				Name:        "externalId",
				Type:        "string",
				MultiValued: false,
				Description: "A String that is an identifier for the resource as defined by the provisioning client.",
				Required:    false,
				CaseExact:   true,
				Mutability:  "readWrite",
				Returned:    "default",
			},
		},
	}
}

// GetUserResourceType returns the SCIM User resource type definition
func GetUserResourceType(baseURL string) models.SCIMResourceType {
	return models.SCIMResourceType{
		Schemas:     []string{models.SCIMSchemaResourceType},
		ID:          "User",
		Name:        "User",
		Description: "User Account",
		Endpoint:    "/scim/v2/Users",
		Schema:      models.SCIMSchemaUser,
		Meta: &models.SCIMMeta{
			ResourceType: "ResourceType",
			Location:     baseURL + "/scim/v2/ResourceTypes/User",
		},
	}
}

// GetGroupResourceType returns the SCIM Group resource type definition
func GetGroupResourceType(baseURL string) models.SCIMResourceType {
	return models.SCIMResourceType{
		Schemas:     []string{models.SCIMSchemaResourceType},
		ID:          "Group",
		Name:        "Group",
		Description: "Group",
		Endpoint:    "/scim/v2/Groups",
		Schema:      models.SCIMSchemaGroup,
		Meta: &models.SCIMMeta{
			ResourceType: "ResourceType",
			Location:     baseURL + "/scim/v2/ResourceTypes/Group",
		},
	}
}

// GetServiceProviderConfig returns the SCIM service provider configuration
func GetServiceProviderConfig(baseURL string) models.SCIMServiceProviderConfig {
	return models.SCIMServiceProviderConfig{
		Schemas:          []string{models.SCIMSchemaServiceProviderConfig},
		DocumentationURI: "",
		Patch: models.SCIMSupported{
			Supported: true,
		},
		Bulk: models.SCIMBulkConfig{
			Supported:      false,
			MaxOperations:  0,
			MaxPayloadSize: 0,
		},
		Filter: models.SCIMFilterConfig{
			Supported:  true,
			MaxResults: 200,
		},
		ChangePassword: models.SCIMSupported{
			Supported: false, // SCIM users don't have passwords
		},
		Sort: models.SCIMSupported{
			Supported: false,
		},
		Etag: models.SCIMSupported{
			Supported: false,
		},
		AuthenticationSchemes: []models.SCIMAuthScheme{
			{
				Type:        "oauthbearertoken",
				Name:        "OAuth Bearer Token",
				Description: "Authentication scheme using the OAuth Bearer Token Standard",
				SpecURI:     "http://www.rfc-editor.org/info/rfc6750",
				Primary:     true,
			},
		},
		Meta: &models.SCIMMeta{
			ResourceType: "ServiceProviderConfig",
			Location:     baseURL + "/scim/v2/ServiceProviderConfig",
		},
	}
}
