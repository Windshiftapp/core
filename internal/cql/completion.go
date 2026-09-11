package cql

import "slices"

// CompletionValueSource identifies a catalog that can provide values for a QL field.
type CompletionValueSource string

const (
	CompletionValuesNone             CompletionValueSource = ""
	CompletionValuesWorkspaces       CompletionValueSource = "workspaces"
	CompletionValuesStatuses         CompletionValueSource = "statuses"
	CompletionValuesStatusCategories CompletionValueSource = "status_categories"
	CompletionValuesPriorities       CompletionValueSource = "priorities"
	CompletionValuesUsers            CompletionValueSource = "users"
	CompletionValuesMilestones       CompletionValueSource = "milestones"
	CompletionValuesIterations       CompletionValueSource = "iterations"
	CompletionValuesProjects         CompletionValueSource = "projects"
	CompletionValuesItemTypes        CompletionValueSource = "item_types"
	CompletionValuesLabels           CompletionValueSource = "labels"
)

// CompletionField describes one canonical field offered by QL editors.
// Generator-only compatibility aliases stay in the generator and are not
// presented as separate choices.
type CompletionField struct {
	Name        string
	Aliases     []string
	ValueType   string
	ValueSource CompletionValueSource
	ValueField  string
	Operators   []string
}

var (
	equalityOperators = []string{"=", "!=", "IN", "NOT IN", "IS NULL", "IS NOT NULL"}
	orderedOperators  = []string{"=", "!=", "<", "<=", ">", ">=", "IN", "NOT IN", "IS NULL", "IS NOT NULL"}
	textOperators     = []string{"=", "!=", "~", "IN", "NOT IN", "IS NULL", "IS NOT NULL"}
	booleanOperators  = []string{"=", "!=", "IS NULL", "IS NOT NULL"}

	itemCompletionFields = []CompletionField{
		{Name: "workspace", ValueType: "string", ValueSource: CompletionValuesWorkspaces, Operators: equalityOperators},
		{Name: "workspaceKey", ValueType: "string", ValueSource: CompletionValuesWorkspaces, ValueField: "key", Operators: equalityOperators},
		{Name: "workspaceId", Aliases: []string{"workspace_id"}, ValueType: "number", ValueSource: CompletionValuesWorkspaces, Operators: orderedOperators},
		{Name: "status", ValueType: "string", ValueSource: CompletionValuesStatuses, Operators: equalityOperators},
		{Name: "statusId", Aliases: []string{"status_id"}, ValueType: "number", ValueSource: CompletionValuesStatuses, Operators: orderedOperators},
		{Name: "statusCategory", Aliases: []string{"status_category"}, ValueType: "string", ValueSource: CompletionValuesStatusCategories, Operators: equalityOperators},
		{Name: "statusCompleted", Aliases: []string{"status_completed"}, ValueType: "boolean", Operators: booleanOperators},
		{Name: "priority", ValueType: "string", ValueSource: CompletionValuesPriorities, Operators: equalityOperators},
		{Name: "priorityId", Aliases: []string{"priority_id"}, ValueType: "number", ValueSource: CompletionValuesPriorities, Operators: orderedOperators},
		{Name: "title", ValueType: "string", Operators: textOperators},
		{Name: "description", ValueType: "string", Operators: textOperators},
		{Name: "createdAt", Aliases: []string{"created", "created_at"}, ValueType: "date", Operators: orderedOperators},
		{Name: "updatedAt", Aliases: []string{"updated", "updated_at"}, ValueType: "date", Operators: orderedOperators},
		{Name: "completed_at", ValueType: "date", Operators: orderedOperators},
		{Name: "dueDate", Aliases: []string{"due_date", "due-date"}, ValueType: "date", Operators: orderedOperators},
		{Name: "assignee", Aliases: []string{"assigneeId", "assignee_id"}, ValueType: "number", ValueSource: CompletionValuesUsers, Operators: equalityOperators},
		{Name: "creator", Aliases: []string{"creatorId", "creator_id"}, ValueType: "number", ValueSource: CompletionValuesUsers, Operators: equalityOperators},
		{Name: "reporter", Aliases: []string{"reporterId", "reporter_id"}, ValueType: "number", ValueSource: CompletionValuesUsers, Operators: equalityOperators},
		{Name: "milestone", ValueType: "string", ValueSource: CompletionValuesMilestones, Operators: textOperators},
		{Name: "milestoneId", Aliases: []string{"milestone_id"}, ValueType: "number", ValueSource: CompletionValuesMilestones, Operators: equalityOperators},
		{Name: "milestoneName", ValueType: "string", ValueSource: CompletionValuesMilestones, Operators: textOperators},
		{Name: "iteration", ValueType: "string", ValueSource: CompletionValuesIterations, Operators: equalityOperators},
		{Name: "iterationId", Aliases: []string{"iteration_id"}, ValueType: "number", ValueSource: CompletionValuesIterations, Operators: orderedOperators},
		{Name: "iterationName", ValueType: "string", ValueSource: CompletionValuesIterations, Operators: equalityOperators},
		{Name: "project", ValueType: "string", ValueSource: CompletionValuesProjects, Operators: equalityOperators},
		{Name: "projectId", Aliases: []string{"project_id"}, ValueType: "number", ValueSource: CompletionValuesProjects, Operators: orderedOperators},
		{Name: "projectName", ValueType: "string", ValueSource: CompletionValuesProjects, Operators: equalityOperators},
		{Name: "timeProject", Aliases: []string{"timeProjectId", "time_project_id"}, ValueType: "string", Operators: equalityOperators},
		{Name: "inheritProject", Aliases: []string{"inherit_project"}, ValueType: "boolean", Operators: booleanOperators},
		{Name: "itemType", ValueType: "string", ValueSource: CompletionValuesItemTypes, Operators: equalityOperators},
		{Name: "itemTypeId", Aliases: []string{"item_type_id"}, ValueType: "number", ValueSource: CompletionValuesItemTypes, Operators: orderedOperators},
		{Name: "itemTypeName", ValueType: "string", ValueSource: CompletionValuesItemTypes, Operators: equalityOperators},
		{Name: "type", ValueType: "string", ValueSource: CompletionValuesItemTypes, Operators: equalityOperators},
		{Name: "parent", Aliases: []string{"parentId", "parent_id"}, ValueType: "number", Operators: equalityOperators},
		{Name: "isTask", Aliases: []string{"is_task"}, ValueType: "boolean", Operators: booleanOperators},
		{Name: "rank", ValueType: "number", Operators: orderedOperators},
		{Name: "id", ValueType: "number", Operators: orderedOperators},
		{Name: "key", ValueType: "string", Operators: equalityOperators},
		{Name: "label", Aliases: []string{"labels"}, ValueType: "string", ValueSource: CompletionValuesLabels, Operators: textOperators},
	}
)

// ItemCompletionFields returns a copy of the canonical work-item QL catalog.
func ItemCompletionFields() []CompletionField {
	fields := make([]CompletionField, len(itemCompletionFields))
	for i, field := range itemCompletionFields {
		field.Aliases = slices.Clone(field.Aliases)
		field.Operators = slices.Clone(field.Operators)
		fields[i] = field
	}
	return fields
}

// CustomFieldCompletion returns completion metadata for a custom-field type.
func CustomFieldCompletion(fieldType string) (valueType string, operators []string) {
	switch fieldType {
	case "number":
		return "number", slices.Clone(orderedOperators)
	case "checkbox", "boolean":
		return "boolean", slices.Clone(booleanOperators)
	case "date":
		return "date", slices.Clone(orderedOperators)
	case "text", "textarea":
		return "string", slices.Clone(textOperators)
	default:
		return "string", slices.Clone(equalityOperators)
	}
}
