package objecttranslation

import (
	"errors"
	"fmt"
	"slices"
)

const (
	FieldName        = "name"
	FieldDescription = "description"

	SourceSystem   = "system"
	SourceInstance = "instance"
)

var (
	ErrUnsupportedObjectType = errors.New("unsupported translation object type")
	ErrUnsupportedField      = errors.New("unsupported translation field")
	ErrInvalidLocale         = errors.New("invalid translation locale")
	ErrInvalidValue          = errors.New("invalid translation value")
	ErrObjectNotFound        = errors.New("translated object not found")
	ErrTranslationNotFound   = errors.New("translation not found")
)

type objectSpec struct {
	objectType string
	table      string
	fields     []string
}

var registry = []objectSpec{
	{objectType: "configuration_set", table: "configuration_sets", fields: []string{FieldName, FieldDescription}},
	{objectType: "hierarchy_level", table: "hierarchy_levels", fields: []string{FieldName, FieldDescription}},
	{objectType: "item_type", table: "item_types", fields: []string{FieldName, FieldDescription}},
	{objectType: "link_type", table: "link_types", fields: []string{FieldName, FieldDescription}},
	{objectType: "notification_setting", table: "notification_settings", fields: []string{FieldName, FieldDescription}},
	{objectType: "priority", table: "priorities", fields: []string{FieldName, FieldDescription}},
	{objectType: "screen", table: "screens", fields: []string{FieldName, FieldDescription}},
	{objectType: "status", table: "statuses", fields: []string{FieldName, FieldDescription}},
	{objectType: "status_category", table: "status_categories", fields: []string{FieldName, FieldDescription}},
	{objectType: "theme", table: "themes", fields: []string{FieldName, FieldDescription}},
	{objectType: "workflow", table: "workflows", fields: []string{FieldName, FieldDescription}},
	{objectType: "workspace_role", table: "workspace_roles", fields: []string{FieldName, FieldDescription}},
}

// ObjectDefinition describes one configurable object accepted by translation writes.
type ObjectDefinition struct {
	ObjectType string   `json:"object_type"`
	Fields     []string `json:"fields"`
}

// Definitions returns the stable public translation registry.
func Definitions() []ObjectDefinition {
	definitions := make([]ObjectDefinition, 0, len(registry))
	for _, spec := range registry {
		definitions = append(definitions, ObjectDefinition{
			ObjectType: spec.objectType,
			Fields:     slices.Clone(spec.fields),
		})
	}
	return definitions
}

func lookupSpec(objectType, field string) (objectSpec, error) {
	for _, spec := range registry {
		if spec.objectType != objectType {
			continue
		}
		if !slices.Contains(spec.fields, field) {
			return objectSpec{}, fmt.Errorf("%w: %s.%s", ErrUnsupportedField, objectType, field)
		}
		return spec, nil
	}
	return objectSpec{}, fmt.Errorf("%w: %s", ErrUnsupportedObjectType, objectType)
}

func lookupObjectSpec(objectType string) (objectSpec, error) {
	for _, spec := range registry {
		if spec.objectType == objectType {
			return spec, nil
		}
	}
	return objectSpec{}, fmt.Errorf("%w: %s", ErrUnsupportedObjectType, objectType)
}
