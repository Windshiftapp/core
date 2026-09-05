package v2

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"windshift/internal/cql"
	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/services"
)

const defaultQLCompletionValueLimit = 50

var errUnsupportedQLCompletionValueSource = errors.New("unsupported query language completion value source")

type qlCompletionCatalogDTO struct {
	Fields           []qlCompletionFieldDTO `json:"fields"`
	LogicalOperators []string               `json:"logical_operators"`
}

type qlCompletionFieldDTO struct {
	Name        string                 `json:"name"`
	Label       string                 `json:"label"`
	Description string                 `json:"description,omitempty"`
	Aliases     []string               `json:"aliases"`
	FieldType   string                 `json:"field_type,omitempty"`
	ValueType   string                 `json:"value_type"`
	Operators   []string               `json:"operators"`
	ValueHelp   *qlCompletionValueHelp `json:"value_help,omitempty"`
	Values      []qlCompletionValueDTO `json:"values,omitempty"`
}

type qlCompletionValueDTO struct {
	Value       any    `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	ID          int    `json:"-"`
	Name        string `json:"-"`
	DisplayName string `json:"-"`
	searchTerms []string
}

type qlCompletionValueHelp struct {
	Source     cql.CompletionValueSource `json:"source"`
	ValueField string                    `json:"value_field"`
}

type qlCompletionValueReader interface {
	Load(context.Context, int, cql.CompletionValueSource, string) ([]qlCompletionValueDTO, error)
}

type queryLanguageValueLoader struct {
	configuration configurationReader
	statuses      statusReader
	catalog       catalogReader
	planning      planningApplication
	timeProjects  timeProjectApplication
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
			item, err := customCompletionFieldDTO(field.ID, field.Name, field.Description, field.FieldType, field.Options)
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

func queryLanguageCompletionValues(reader qlCompletionValueReader, localizer objectLocalizer) readOperation[[]qlCompletionValueDTO] {
	return func(r *http.Request) ([]qlCompletionValueDTO, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		source := cql.CompletionValueSource(strings.TrimSpace(r.URL.Query().Get("source")))
		valueField := strings.TrimSpace(r.URL.Query().Get("value_field"))
		if source == cql.CompletionValuesNone || valueField == "" {
			return nil, newError(http.StatusBadRequest, "invalid_request", "source and value_field are required")
		}
		limit, err := qlCompletionValueLimit(r.URL.Query().Get("limit"))
		if err != nil {
			return nil, err
		}

		values, err := reader.Load(r.Context(), user.ID, source, valueField)
		if errors.Is(err, errUnsupportedQLCompletionValueSource) {
			return nil, newError(http.StatusBadRequest, "invalid_request", err.Error())
		}
		if err != nil {
			return nil, internalError(err)
		}
		if err := localizeQLCompletionValues(r, localizer, source, values); err != nil {
			return nil, err
		}
		return filterQLCompletionValues(values, r.URL.Query().Get("q"), limit), nil
	}
}

func (l queryLanguageValueLoader) Load(_ context.Context, userID int, source cql.CompletionValueSource, valueField string) ([]qlCompletionValueDTO, error) {
	if !validQLCompletionValueField(source, valueField) {
		return nil, fmt.Errorf("%w: %s does not support %s", errUnsupportedQLCompletionValueSource, source, valueField)
	}

	switch source {
	case cql.CompletionValuesWorkspaces:
		workspaces, err := l.visibleWorkspaces(userID)
		if err != nil {
			return nil, err
		}
		values := make([]qlCompletionValueDTO, len(workspaces))
		for i, workspace := range workspaces {
			values[i] = qlCompletionValue(workspace.ID, workspace.Name, workspace.Key, valueField, workspace.Name, workspace.Key)
		}
		return values, nil
	case cql.CompletionValuesStatuses:
		rows, err := l.statuses.ListStatuses()
		if err != nil {
			return nil, fmt.Errorf("list statuses for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name)
		}
		return values, nil
	case cql.CompletionValuesStatusCategories:
		rows, err := l.statuses.ListCategories()
		if err != nil {
			return nil, fmt.Errorf("list status categories for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name)
		}
		return values, nil
	case cql.CompletionValuesPriorities:
		rows, err := l.configuration.ListPriorities()
		if err != nil {
			return nil, fmt.Errorf("list priorities for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name)
		}
		return values, nil
	case cql.CompletionValuesItemTypes:
		rows, err := l.configuration.ListItemTypes()
		if err != nil {
			return nil, fmt.Errorf("list item types for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name)
		}
		return values, nil
	case cql.CompletionValuesUsers:
		rows, _, err := l.catalog.ListUsers(userID, services.CatalogPageParams{Limit: math.MaxInt, Sort: "full_name"})
		if err != nil {
			return nil, fmt.Errorf("list users for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			label := strings.TrimSpace(row.FullName)
			if label == "" {
				label = row.Username
			}
			values[i] = qlCompletionValue(row.ID, row.Username, "", valueField, label, row.Username)
		}
		return values, nil
	case cql.CompletionValuesLabels:
		workspaces, err := l.visibleWorkspaces(userID)
		if err != nil {
			return nil, err
		}
		if len(workspaces) == 0 {
			return []qlCompletionValueDTO{}, nil
		}
		rows, err := l.catalog.ListLabels(userID, workspaces[0].ID)
		if err != nil {
			return nil, fmt.Errorf("list labels for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name)
		}
		return values, nil
	case cql.CompletionValuesMilestones, cql.CompletionValuesIterations:
		return l.loadPlanningValues(userID, source, valueField)
	case cql.CompletionValuesProjects:
		rows, err := l.timeProjects.List(userID, "")
		if err != nil {
			return nil, fmt.Errorf("list projects for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name, row.CustomerName)
			values[i].Description = row.CustomerName
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedQLCompletionValueSource, source)
	}
}

func (l queryLanguageValueLoader) visibleWorkspaces(userID int) ([]models.Workspace, error) {
	rows, _, err := l.catalog.ListWorkspaces(userID, services.CatalogPageParams{Limit: math.MaxInt, Sort: "name"})
	if err != nil {
		return nil, fmt.Errorf("list workspaces for query completion: %w", err)
	}
	return rows, nil
}

func (l queryLanguageValueLoader) loadPlanningValues(userID int, source cql.CompletionValueSource, valueField string) ([]qlCompletionValueDTO, error) {
	workspaces, err := l.visibleWorkspaces(userID)
	if err != nil {
		return nil, err
	}
	workspaceIDs := make([]int, len(workspaces))
	for i, workspace := range workspaces {
		workspaceIDs[i] = workspace.ID
	}

	if source == cql.CompletionValuesMilestones {
		rows, _, err := l.planning.ListMilestones(userID, services.MilestoneListParams{
			Limit: math.MaxInt, WorkspaceIDs: workspaceIDs, IncludeGlobal: true, SortBy: "name",
		})
		if err != nil {
			return nil, fmt.Errorf("list milestones for query completion: %w", err)
		}
		values := make([]qlCompletionValueDTO, len(rows))
		for i, row := range rows {
			values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name, row.WorkspaceName)
			values[i].Description = row.WorkspaceName
		}
		return values, nil
	}

	rows, _, err := l.planning.ListIterations(userID, services.IterationListParams{
		Limit: math.MaxInt, WorkspaceIDs: workspaceIDs, IncludeGlobal: true, SortBy: "name",
	})
	if err != nil {
		return nil, fmt.Errorf("list iterations for query completion: %w", err)
	}
	values := make([]qlCompletionValueDTO, len(rows))
	for i, row := range rows {
		values[i] = qlCompletionValue(row.ID, row.Name, "", valueField, row.Name, row.WorkspaceName)
		values[i].Description = row.WorkspaceName
	}
	return values, nil
}

func qlCompletionValue(id int, name, key, valueField string, searchTerms ...string) qlCompletionValueDTO {
	var value any = name
	switch valueField {
	case "id":
		value = id
	case "key":
		value = key
	}
	label := name
	if len(searchTerms) > 0 && strings.TrimSpace(searchTerms[0]) != "" {
		label = searchTerms[0]
	}
	return qlCompletionValueDTO{
		Value: value, Label: label, ID: id, Name: name, DisplayName: name,
		searchTerms: searchTerms,
	}
}

func localizeQLCompletionValues(r *http.Request, localizer objectLocalizer, source cql.CompletionValueSource, values []qlCompletionValueDTO) error {
	if localizer == nil || len(values) == 0 {
		return nil
	}
	objectType := ""
	switch source {
	case cql.CompletionValuesStatuses:
		objectType = "status"
	case cql.CompletionValuesStatusCategories:
		objectType = "status_category"
	case cql.CompletionValuesPriorities:
		objectType = "priority"
	case cql.CompletionValuesItemTypes:
		objectType = "item_type"
	default:
		return nil
	}
	if err := localizer.LocalizeResponse(r.Context(), objecttranslation.RequestLocale(r), objectType, &values); err != nil {
		return internalError(err)
	}
	for i := range values {
		if displayName := strings.TrimSpace(values[i].DisplayName); displayName != "" {
			values[i].searchTerms = append(values[i].searchTerms, displayName)
			values[i].Label = displayName
		}
	}
	return nil
}

func validQLCompletionValueField(source cql.CompletionValueSource, field string) bool {
	switch source {
	case cql.CompletionValuesWorkspaces:
		return field == "id" || field == "name" || field == "key"
	case cql.CompletionValuesStatuses, cql.CompletionValuesStatusCategories,
		cql.CompletionValuesPriorities, cql.CompletionValuesUsers,
		cql.CompletionValuesMilestones, cql.CompletionValuesIterations,
		cql.CompletionValuesProjects, cql.CompletionValuesItemTypes,
		cql.CompletionValuesLabels:
		return field == "id" || field == "name"
	default:
		return false
	}
}

func qlCompletionValueLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultQLCompletionValueLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, newError(http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
	}
	return limit, nil
}

func filterQLCompletionValues(values []qlCompletionValueDTO, query string, limit int) []qlCompletionValueDTO {
	query = strings.ToLower(strings.TrimSpace(query))
	values = slices.Clone(values)
	slices.SortStableFunc(values, func(left, right qlCompletionValueDTO) int {
		leftScore := qlCompletionMatchScore(left, query)
		rightScore := qlCompletionMatchScore(right, query)
		if leftScore != rightScore {
			return leftScore - rightScore
		}
		if order := strings.Compare(strings.ToLower(left.Label), strings.ToLower(right.Label)); order != 0 {
			return order
		}
		return strings.Compare(fmt.Sprint(left.Value), fmt.Sprint(right.Value))
	})

	result := make([]qlCompletionValueDTO, 0, min(limit, len(values)))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if qlCompletionMatchScore(value, query) == 2 {
			continue
		}
		key := fmt.Sprintf("%T:%v", value.Value, value.Value)
		if seen[key] {
			continue
		}
		seen[key] = true
		value.searchTerms = nil
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func qlCompletionMatchScore(value qlCompletionValueDTO, query string) int {
	if query == "" {
		return 0
	}
	candidates := append([]string{value.Label, fmt.Sprint(value.Value)}, value.searchTerms...)
	for _, candidate := range candidates {
		if strings.HasPrefix(strings.ToLower(candidate), query) {
			return 0
		}
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), query) {
			return 1
		}
	}
	return 2
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

func customCompletionFieldDTO(id int, name, description, fieldType, rawOptions string) (qlCompletionFieldDTO, error) {
	valueType, operators := cql.CustomFieldCompletion(strings.ToLower(fieldType))
	result := qlCompletionFieldDTO{
		Name: fmt.Sprintf("cfid_%d", id), Label: name, Description: description,
		FieldType: strings.ToLower(fieldType), ValueType: valueType, Operators: operators,
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
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesStatuses:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesStatusCategories:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesPriorities:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesUsers:
		return &qlCompletionValueHelp{Source: source, ValueField: "id"}
	case cql.CompletionValuesMilestones:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesIterations:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesProjects:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesItemTypes:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	case cql.CompletionValuesLabels:
		return &qlCompletionValueHelp{Source: source, ValueField: valueField}
	default:
		return nil
	}
}
