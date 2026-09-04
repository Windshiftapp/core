package v2

import (
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/cql"
	"windshift/internal/models"
)

type qlCompletionCatalogDTO struct {
	Fields           []qlCompletionFieldDTO `json:"fields"`
	LogicalOperators []string               `json:"logical_operators"`
}

type qlCompletionFieldDTO struct {
	Name      string                 `json:"name"`
	Label     string                 `json:"label"`
	Aliases   []string               `json:"aliases"`
	ValueType string                 `json:"value_type"`
	Operators []string               `json:"operators"`
	ValueHelp *qlCompletionValueHelp `json:"value_help,omitempty"`
	Values    []qlCompletionValueDTO `json:"values,omitempty"`
}

type qlCompletionValueDTO struct {
	Value any    `json:"value"`
	Label string `json:"label"`
}

type qlCompletionValueHelp struct {
	Endpoint    string   `json:"endpoint"`
	APIVersion  string   `json:"api_version"`
	Paginated   bool     `json:"paginated"`
	ValueField  string   `json:"value_field"`
	LabelFields []string `json:"label_fields"`
}

func queryLanguageCompletionCatalog(reader configurationReader) readOperation[qlCompletionCatalogDTO] {
	return func(*http.Request) (qlCompletionCatalogDTO, error) {
		definitions, err := reader.ListCustomFields()
		if err != nil {
			return qlCompletionCatalogDTO{}, internalError(err)
		}

		fields := cql.ItemCompletionFields()
		result := make([]qlCompletionFieldDTO, 0, len(fields)+len(definitions))
		for _, field := range fields {
			result = append(result, completionFieldDTO(field))
		}
		for _, field := range definitions {
			item, err := customCompletionFieldDTO(field.ID, field.Name, field.FieldType, field.Options)
			if err != nil {
				return qlCompletionCatalogDTO{}, internalError(fmt.Errorf("custom field %d options: %w", field.ID, err))
			}
			result = append(result, item)
		}

		return qlCompletionCatalogDTO{
			Fields:           result,
			LogicalOperators: []string{"AND", "OR"},
		}, nil
	}
}

func completionFieldDTO(field cql.CompletionField) qlCompletionFieldDTO {
	aliases := field.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	result := qlCompletionFieldDTO{
		Name: field.Name, Label: field.Name, Aliases: aliases,
		ValueType: field.ValueType, Operators: field.Operators,
	}
	result.ValueHelp = completionValueHelp(field.ValueSource, field.ValueType, field.ValueField)
	if field.ValueType == "boolean" {
		result.Values = []qlCompletionValueDTO{{Value: true, Label: "true"}, {Value: false, Label: "false"}}
	}
	return result
}

func customCompletionFieldDTO(id int, name, fieldType, rawOptions string) (qlCompletionFieldDTO, error) {
	valueType, operators := cql.CustomFieldCompletion(strings.ToLower(fieldType))
	result := qlCompletionFieldDTO{
		Name: fmt.Sprintf("cfid_%d", id), Label: name, ValueType: valueType, Operators: operators,
		Aliases: []string{"cf_" + name, "custom." + name},
	}

	switch strings.ToLower(fieldType) {
	case "select", "multiselect":
		options, err := models.ParseSelectOptions(rawOptions)
		if err != nil {
			return qlCompletionFieldDTO{}, err
		}
		result.Values = make([]qlCompletionValueDTO, len(options.Items))
		for i, option := range options.Items {
			result.Values[i] = qlCompletionValueDTO{Value: option.ID, Label: option.Label}
		}
	case "milestone":
		result.ValueType = "number"
		result.ValueHelp = completionValueHelp(cql.CompletionValuesMilestones, "number", "")
	case "iteration":
		result.ValueType = "number"
		result.ValueHelp = completionValueHelp(cql.CompletionValuesIterations, "number", "")
	case "user":
		result.ValueType = "number"
		result.ValueHelp = completionValueHelp(cql.CompletionValuesUsers, "number", "")
	}

	return result, nil
}

func completionValueHelp(source cql.CompletionValueSource, valueType, valueFieldOverride string) *qlCompletionValueHelp {
	valueField := "name"
	if valueType == "number" {
		valueField = "id"
	}
	if valueFieldOverride != "" {
		valueField = valueFieldOverride
	}

	switch source {
	case cql.CompletionValuesWorkspaces:
		return &qlCompletionValueHelp{Endpoint: "/workspaces", APIVersion: "v2", Paginated: true, ValueField: valueField, LabelFields: []string{"name", "key"}}
	case cql.CompletionValuesStatuses:
		return &qlCompletionValueHelp{Endpoint: "/statuses", APIVersion: "v2", ValueField: valueField, LabelFields: []string{"display_name", "name"}}
	case cql.CompletionValuesStatusCategories:
		return &qlCompletionValueHelp{Endpoint: "/status-categories", APIVersion: "v2", ValueField: valueField, LabelFields: []string{"display_name", "name"}}
	case cql.CompletionValuesPriorities:
		return &qlCompletionValueHelp{Endpoint: "/priorities", APIVersion: "v2", ValueField: valueField, LabelFields: []string{"display_name", "name"}}
	case cql.CompletionValuesUsers:
		return &qlCompletionValueHelp{Endpoint: "/users", APIVersion: "v2", Paginated: true, ValueField: "id", LabelFields: []string{"full_name", "username"}}
	case cql.CompletionValuesMilestones:
		return &qlCompletionValueHelp{Endpoint: "/milestones", APIVersion: "v2", Paginated: true, ValueField: valueField, LabelFields: []string{"name"}}
	case cql.CompletionValuesIterations:
		return &qlCompletionValueHelp{Endpoint: "/iterations", APIVersion: "v2", Paginated: true, ValueField: valueField, LabelFields: []string{"name"}}
	case cql.CompletionValuesProjects:
		return &qlCompletionValueHelp{Endpoint: "/projects", APIVersion: "v1", ValueField: valueField, LabelFields: []string{"name"}}
	case cql.CompletionValuesItemTypes:
		return &qlCompletionValueHelp{Endpoint: "/item-types", APIVersion: "v2", ValueField: valueField, LabelFields: []string{"display_name", "name"}}
	default:
		return nil
	}
}
