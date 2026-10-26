package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/validation"
)

// IsBlankSubmittedField reports whether a value submitted in a portal/form
// payload should be treated as "no value" by required-field validation. JSON
// unmarshalling produces []any / map[string]any for empty
// arrays and objects respectively, and the old `== nil || == ""` check let
// those slip through, allowing required multiselect/object fields to be
// satisfied by `[]` or `{}`. Scalars `false` and `0` (and `0.0`) are NOT
// blank — they're legitimate values.
func IsBlankSubmittedField(value any) bool {
	if value == nil {
		return true
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	default:
		return false
	}
}

// RequestTypeValidationResult contains the result of request type field validation.
type RequestTypeValidationResult struct {
	ItemTypeID *int
	// WorkspaceID is the request type's own target workspace, when set. It is
	// the source of truth for submission routing: callers create the item in
	// this workspace, falling back to the channel's first configured workspace
	// only when it is nil. Nullable in the schema, so a request type may not
	// pin a workspace.
	WorkspaceID        *int
	VirtualFieldValues map[string]any
	CustomFieldValues  map[string]any
	// TitleFieldInForm is true when the request type's field config includes
	// the default "title" field — meaning the submitter saw a title input on
	// the form. Callers that need a title (every item create) use this to
	// decide between trusting submission.Title vs. rendering a title template.
	TitleFieldInForm bool
}

type virtualRequestField struct {
	fieldType string
	options   string
	required  bool
}

// IsPublicFormFillableCustomFieldType keeps public forms to values that can be
// entered without loading options from authenticated internal APIs.
func IsPublicFormFillableCustomFieldType(fieldType string) bool {
	switch models.CanonicalCustomFieldType(fieldType) {
	case "text", "textarea", "select", "multiselect", "number", "date",
		models.CustomFieldTypeBoolean, "email", "url":
		return true
	default:
		return false
	}
}

func normalizeVirtualFieldValue(fieldID string, field virtualRequestField, raw any) (any, error) {
	switch field.fieldType {
	case "text":
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("field %s must be text", fieldID)
		}
		normalized := sanitize.PlainTextField.Sanitize(value)
		if field.required && IsBlankSubmittedField(normalized) {
			return nil, fmt.Errorf("field %s is required", fieldID)
		}
		return normalized, nil
	case "textarea":
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("field %s must be text", fieldID)
		}
		normalized := sanitize.RichText.Sanitize(value)
		if field.required && IsBlankSubmittedField(normalized) {
			return nil, fmt.Errorf("field %s is required", fieldID)
		}
		return normalized, nil
	case "checkbox":
		if raw == nil {
			return nil, nil
		}
		return validation.ValidateCheckboxValue(fieldID, raw)
	case "select":
		if IsBlankSubmittedField(raw) {
			return raw, nil
		}
		var configured []any
		if field.options == "" {
			return nil, fmt.Errorf("field %s has no configured options", fieldID)
		}
		if err := json.Unmarshal([]byte(field.options), &configured); err != nil {
			return nil, fmt.Errorf("field %s has invalid options: %w", fieldID, err)
		}
		for _, option := range configured {
			value := option
			if object, ok := option.(map[string]any); ok {
				configuredValue, exists := object["value"]
				if !exists {
					continue
				}
				value = configuredValue
			}
			if reflect.DeepEqual(value, raw) {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("field %s contains an invalid option", fieldID)
	default:
		return nil, fmt.Errorf("field %s has unsupported virtual type %q", fieldID, field.fieldType)
	}
}

// AllowedCreateScreenCustomFieldIdentifiers resolves the custom fields that
// may be used when creating an item of itemTypeID in workspaceID. Public form
// schemas and submissions both use this list so stale or forged field rows do
// not expose or write fields outside the effective create screen.
func AllowedCreateScreenCustomFieldIdentifiers(db database.Database, workspaceID, itemTypeID int) (map[string]struct{}, error) {
	allowed := make(map[string]struct{})
	if workspaceID <= 0 || itemTypeID <= 0 {
		return allowed, nil
	}
	itemTypeAllowed, err := IsItemTypeAllowedInWorkspace(db, workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if !itemTypeAllowed {
		return allowed, nil
	}

	screenRepo := repository.NewScreenRepository(db)
	createScreenID, err := screenRepo.GetCreateScreenID(workspaceID, itemTypeID)
	if err != nil || createScreenID == nil {
		return allowed, err
	}
	fields, err := screenRepo.ListFields(*createScreenID)
	if err != nil {
		return nil, err
	}
	for _, field := range fields {
		if field.FieldType == "custom" && field.FieldIdentifier != "" && field.FieldName != "" &&
			IsPublicFormFillableCustomFieldType(field.CustomFieldType) {
			allowed[field.FieldIdentifier] = struct{}{}
		}
	}
	return allowed, nil
}

func allowedRequestTypeCustomFieldIdentifiers(ctx context.Context, db database.Database, requestTypeID int) (map[string]struct{}, error) {
	var itemTypeID int
	var workspaceID sql.NullInt64
	var channelType, direction, configJSON string
	err := db.QueryRowContext(ctx, `
		SELECT rt.item_type_id, rt.workspace_id, c.type, c.direction, COALESCE(c.config, '{}')
		FROM request_types rt
		JOIN channels c ON c.id = rt.channel_id
		WHERE rt.id = ? AND rt.is_active = true
	`, requestTypeID).Scan(&itemTypeID, &workspaceID, &channelType, &direction, &configJSON)
	if err != nil {
		return nil, err
	}

	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("parse request type channel config: %w", err)
	}
	var servedWorkspaceIDs []int
	if direction == "inbound" {
		switch channelType {
		case "portal":
			servedWorkspaceIDs = config.PortalWorkspaceIDs
		case "form":
			servedWorkspaceIDs = config.FormWorkspaceIDs
		}
	}
	// A request type pins its workspace at creation; an unpinned row is not
	// routable and exposes no custom fields.
	if len(servedWorkspaceIDs) == 0 || !workspaceID.Valid {
		return map[string]struct{}{}, nil
	}
	effectiveWorkspaceID := int(workspaceID.Int64)
	served := false
	for _, candidate := range servedWorkspaceIDs {
		if candidate == effectiveWorkspaceID {
			served = true
			break
		}
	}
	if !served {
		return map[string]struct{}{}, nil
	}
	return AllowedCreateScreenCustomFieldIdentifiers(db, effectiveWorkspaceID, itemTypeID)
}

// ValidateAndSeparateRequestFields validates request type fields and separates virtual from custom fields.
func ValidateAndSeparateRequestFields(ctx context.Context, db database.Database, requestTypeID *int, title, description string, customFields map[string]any) (*RequestTypeValidationResult, error) {
	result := &RequestTypeValidationResult{}

	if requestTypeID == nil {
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		return result, nil
	}

	var rtID int
	var rtName string
	var itemTypeID int
	var workspaceID sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT id, name, item_type_id, workspace_id FROM request_types WHERE id = ? AND is_active = true`, *requestTypeID).Scan(
		&rtID, &rtName, &itemTypeID, &workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request type (ID: %d): %w", *requestTypeID, err)
	}
	result.ItemTypeID = &itemTypeID
	if workspaceID.Valid {
		wsID := int(workspaceID.Int64)
		result.WorkspaceID = &wsID
	}
	allowedCustomFieldIDs, err := allowedRequestTypeCustomFieldIdentifiers(ctx, db, *requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("resolve request type create-screen fields: %w", err)
	}

	virtualFields := make(map[string]virtualRequestField)
	configuredCustomFieldIDs := make(map[string]bool)
	rows, err := db.QueryContext(ctx, `
		SELECT rtf.field_identifier, rtf.field_type, rtf.is_required,
		       COALESCE(rtf.virtual_field_type, ''), COALESCE(rtf.virtual_field_options, ''),
		       COALESCE(cfd.field_type, '')
		FROM request_type_fields rtf
		LEFT JOIN custom_field_definitions cfd
		  ON rtf.field_type = 'custom' AND CAST(cfd.id AS TEXT) = rtf.field_identifier
		WHERE rtf.request_type_id = ?
		ORDER BY rtf.display_order
	`, *requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load request type fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var fieldID, fieldType, virtualFieldType, virtualFieldOptions, customFieldType string
		var isRequired bool
		if err := rows.Scan(&fieldID, &fieldType, &isRequired, &virtualFieldType, &virtualFieldOptions, &customFieldType); err != nil {
			return nil, fmt.Errorf("scan request type field: %w", err)
		}
		if fieldType == "custom" {
			if _, allowed := allowedCustomFieldIDs[fieldID]; !allowed {
				continue
			}
		}

		switch fieldType {
		case "virtual":
			virtualFields[fieldID] = virtualRequestField{
				fieldType: virtualFieldType,
				options:   virtualFieldOptions,
				required:  isRequired,
			}
		case "custom":
			configuredCustomFieldIDs[fieldID] = true
		}

		// Title is always required when shown on the form, regardless of the
		// admin-set is_required flag — items.title is NOT NULL and the
		// portal's title-template fallback only applies when the field is
		// hidden entirely.
		if fieldType == "default" && fieldID == "title" {
			result.TitleFieldInForm = true
			if title == "" {
				return nil, fmt.Errorf("title is required")
			}
		}

		if isRequired {
			switch fieldType {
			case "default":
				// Description is sanitized with sanitize.Comment, which does not
				// trim, so the blank check must trim explicitly to match title
				// and custom/virtual fields.
				if fieldID == "description" && IsBlankSubmittedField(description) {
					return nil, fmt.Errorf("description is required")
				}
			case "custom":
				if models.IsBooleanCustomFieldType(customFieldType) {
					continue
				}
				if customFields == nil || IsBlankSubmittedField(customFields[fieldID]) {
					return nil, fmt.Errorf("field %s is required", fieldID)
				}
			case "virtual":
				if virtualFieldType == "checkbox" {
					continue
				}
				if customFields == nil || IsBlankSubmittedField(customFields[fieldID]) {
					return nil, fmt.Errorf("field %s is required", fieldID)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request type fields: %w", err)
	}

	// Partition submitted fields. Keys that are neither configured custom fields
	// nor virtual fields are dropped silently — a 400 would act as an oracle
	// telling probers which field IDs exist on the request type.
	result.VirtualFieldValues = make(map[string]any)
	result.CustomFieldValues = make(map[string]any)
	for fieldID, value := range customFields {
		if virtualField, ok := virtualFields[fieldID]; ok {
			normalized, err := normalizeVirtualFieldValue(fieldID, virtualField, value)
			if err != nil {
				return nil, err
			}
			result.VirtualFieldValues[fieldID] = normalized
		} else if configuredCustomFieldIDs[fieldID] {
			if IsBlankSubmittedField(value) {
				continue
			}
			result.CustomFieldValues[fieldID] = value
		}
	}
	if err := validation.ValidateAndNormalizeCustomFieldValues(db, result.CustomFieldValues); err != nil {
		return nil, err
	}

	return result, nil
}
