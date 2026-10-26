package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

const (
	maxPublicFormFields  = 100
	maxVirtualFieldItems = 100
)

var publicFormFieldIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type publicFormFieldSchema struct {
	Identifier          string
	FieldType           string
	DisplayOrder        int
	StepNumber          int
	VirtualFieldType    *string
	VirtualFieldOptions *string
}

type virtualFieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// availableCreateFields is the canonical create-screen lookup for both
// request-type and asset-report form schemas. Keeping validation and the
// management picker on the same result prevents a direct API caller from
// binding a custom field the target create screen does not expose or the
// public form cannot render.
func availableCreateFields(screenRepo *repository.ScreenRepository, workspaceID *int, itemTypeID int) ([]AvailableField, error) {
	fields := []AvailableField{
		{Identifier: "title", Name: "Title", Type: "default"},
		{Identifier: "description", Name: "Description", Type: "default"},
	}
	if workspaceID == nil || itemTypeID <= 0 {
		return fields, nil
	}

	createScreenID, err := screenRepo.GetCreateScreenID(*workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if createScreenID == nil {
		return fields, nil
	}
	screenFields, err := screenRepo.ListFields(*createScreenID)
	if err != nil {
		return nil, err
	}
	for _, field := range screenFields {
		if field.FieldType != "custom" || field.FieldIdentifier == "" || field.FieldName == "" {
			continue
		}
		if !services.IsPublicFormFillableCustomFieldType(field.CustomFieldType) {
			continue
		}
		fields = append(fields, AvailableField{
			Identifier: field.FieldIdentifier,
			Name:       field.FieldName,
			Type:       "custom",
			FieldType:  field.CustomFieldType,
		})
	}
	return fields, nil
}

func validatePublicFormFieldSchema(fields []publicFormFieldSchema, available []AvailableField) error {
	if len(fields) > maxPublicFormFields {
		return fmt.Errorf("a form may contain at most %d fields", maxPublicFormFields)
	}

	allowedCustom := make(map[string]struct{})
	for _, field := range available {
		if field.Type == "custom" {
			allowedCustom[field.Identifier] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.Identifier == "" || !publicFormFieldIdentifierPattern.MatchString(field.Identifier) {
			return fmt.Errorf("field identifier %q is invalid", field.Identifier)
		}
		if utf8.RuneCountInString(field.Identifier) > sanitize.ShortIdentifierMaxRunes {
			return fmt.Errorf("field identifier %q is too long", field.Identifier)
		}
		if _, duplicate := seen[field.Identifier]; duplicate {
			return fmt.Errorf("field identifier %q is duplicated", field.Identifier)
		}
		seen[field.Identifier] = struct{}{}
		if field.DisplayOrder < 0 || field.StepNumber < 0 {
			return fmt.Errorf("field %q has an invalid order or step", field.Identifier)
		}

		switch field.FieldType {
		case "default":
			if field.Identifier != "title" && field.Identifier != "description" {
				return fmt.Errorf("default field %q is not supported", field.Identifier)
			}
		case "custom":
			if _, allowed := allowedCustom[field.Identifier]; !allowed {
				return fmt.Errorf("custom field %q is not available on the target create screen", field.Identifier)
			}
		case "virtual":
			if err := validateVirtualField(field); err != nil {
				return err
			}
		default:
			return fmt.Errorf("field %q has unsupported type %q", field.Identifier, field.FieldType)
		}
	}
	return nil
}

func validateVirtualField(field publicFormFieldSchema) error {
	if field.VirtualFieldType == nil {
		return fmt.Errorf("virtual field %q is missing its input type", field.Identifier)
	}
	fieldType := strings.TrimSpace(*field.VirtualFieldType)
	switch fieldType {
	case "text", "textarea", "checkbox":
		return nil
	case "select":
	default:
		return fmt.Errorf("virtual field %q has unsupported input type %q", field.Identifier, fieldType)
	}
	if field.VirtualFieldOptions == nil || strings.TrimSpace(*field.VirtualFieldOptions) == "" {
		return fmt.Errorf("virtual select field %q must define options", field.Identifier)
	}
	var options []virtualFieldOption
	if err := json.Unmarshal([]byte(*field.VirtualFieldOptions), &options); err != nil {
		return fmt.Errorf("virtual select field %q has invalid options", field.Identifier)
	}
	if len(options) == 0 || len(options) > maxVirtualFieldItems {
		return fmt.Errorf("virtual select field %q must define between 1 and %d options", field.Identifier, maxVirtualFieldItems)
	}
	values := make(map[string]struct{}, len(options))
	for _, option := range options {
		if strings.TrimSpace(option.Value) == "" || strings.TrimSpace(option.Label) == "" {
			return fmt.Errorf("virtual select field %q has a blank option", field.Identifier)
		}
		if utf8.RuneCountInString(option.Value) > sanitize.PlainTextFieldMaxRunes || utf8.RuneCountInString(option.Label) > sanitize.PlainTextFieldMaxRunes {
			return fmt.Errorf("virtual select field %q has an option that is too long", field.Identifier)
		}
		if _, duplicate := values[option.Value]; duplicate {
			return fmt.Errorf("virtual select field %q has duplicate option values", field.Identifier)
		}
		values[option.Value] = struct{}{}
	}
	return nil
}
