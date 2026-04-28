package jira

import (
	"fmt"
	"strings"
	"time"
)

// jiraTimestampLayouts covers the shapes Jira Cloud and Data Center actually
// emit for `created` / `updated` / comment / worklog timestamps. RFC3339Nano
// covers the modern Cloud format including `Z` suffix and colon-zone variants;
// the trailing two layouts cover historical 4-digit-zone serializations seen
// in Data Center and older Cloud responses.
var jiraTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000-0700",
	"2006-01-02T15:04:05.000Z0700",
}

// ParseJiraTimestamp parses a Jira timestamp string against the known layouts.
// Returns nil if the string is empty or matches no layout.
func ParseJiraTimestamp(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range jiraTimestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// WindshiftFieldType represents the field types supported by Windshift
type WindshiftFieldType string

const (
	FieldTypeText        WindshiftFieldType = "text"
	FieldTypeTextarea    WindshiftFieldType = "textarea"
	FieldTypeNumber      WindshiftFieldType = "number"
	FieldTypeSelect      WindshiftFieldType = "select"
	FieldTypeMultiselect WindshiftFieldType = "multiselect"
	FieldTypeDate        WindshiftFieldType = "date"
	FieldTypeUser        WindshiftFieldType = "user"
	FieldTypeUsers       WindshiftFieldType = "users" // Array of user IDs (multi-user picker)
	FieldTypeMilestone   WindshiftFieldType = "milestone"
	FieldTypeIteration   WindshiftFieldType = "iteration"
	FieldTypeAsset       WindshiftFieldType = "asset"
	FieldTypeUnmapped    WindshiftFieldType = "unmapped"
)

// FieldMappingSuggestion contains a suggested mapping for a Jira field
type FieldMappingSuggestion struct {
	JiraFieldID        string             `json:"jira_field_id"`
	JiraFieldName      string             `json:"jira_field_name"`
	JiraFieldType      string             `json:"jira_field_type"`
	WindshiftFieldType WindshiftFieldType `json:"windshift_field_type"`
	CanMap             bool               `json:"can_map"`
	Notes              string             `json:"notes,omitempty"`
	Options            []string           `json:"options,omitempty"` // For select fields
}

// jiraFieldTypeMap maps Jira field type keys to Windshift field types
var jiraFieldTypeMap = map[string]WindshiftFieldType{
	// Standard Jira field types (from schema.type)
	"string":    FieldTypeText,
	"text":      FieldTypeTextarea,
	"number":    FieldTypeNumber,
	"date":      FieldTypeDate,
	"datetime":  FieldTypeDate,
	"user":      FieldTypeUser,
	"array":     FieldTypeMultiselect, // Depends on items type
	"option":    FieldTypeSelect,
	"priority":  FieldTypeSelect, // Maps to Windshift priority
	"version":   FieldTypeMilestone,
	"project":   FieldTypeText,     // Project references become text
	"issuelink": FieldTypeUnmapped, // Handled separately as links

	// Custom field type keys (full plugin identifiers)
	"com.atlassian.jira.plugin.system.customfieldtypes:textfield":        FieldTypeText,
	"com.atlassian.jira.plugin.system.customfieldtypes:textarea":         FieldTypeTextarea,
	"com.atlassian.jira.plugin.system.customfieldtypes:float":            FieldTypeNumber,
	"com.atlassian.jira.plugin.system.customfieldtypes:numberfield":      FieldTypeNumber,
	"com.atlassian.jira.plugin.system.customfieldtypes:select":           FieldTypeSelect,
	"com.atlassian.jira.plugin.system.customfieldtypes:multiselect":      FieldTypeMultiselect,
	"com.atlassian.jira.plugin.system.customfieldtypes:radiobuttons":     FieldTypeSelect,
	"com.atlassian.jira.plugin.system.customfieldtypes:multicheckboxes":  FieldTypeMultiselect,
	"com.atlassian.jira.plugin.system.customfieldtypes:datepicker":       FieldTypeDate,
	"com.atlassian.jira.plugin.system.customfieldtypes:datetime":         FieldTypeDate,
	"com.atlassian.jira.plugin.system.customfieldtypes:url":              FieldTypeText,
	"com.atlassian.jira.plugin.system.customfieldtypes:userpicker":       FieldTypeUser,
	"com.atlassian.jira.plugin.system.customfieldtypes:multiuserpicker":  FieldTypeUsers, // Multi-user picker (array of user IDs)
	"com.atlassian.jira.plugin.system.customfieldtypes:grouppicker":      FieldTypeText,
	"com.atlassian.jira.plugin.system.customfieldtypes:multigrouppicker": FieldTypeMultiselect,
	"com.atlassian.jira.plugin.system.customfieldtypes:cascadingselect":  FieldTypeSelect,
	"com.atlassian.jira.plugin.system.customfieldtypes:labels":           FieldTypeMultiselect,
	"com.atlassian.jira.plugin.system.customfieldtypes:version":          FieldTypeMilestone,
	"com.atlassian.jira.plugin.system.customfieldtypes:multiversion":     FieldTypeMultiselect,
	"com.atlassian.jira.plugin.system.customfieldtypes:project":          FieldTypeText,
	"com.atlassian.jira.plugin.system.customfieldtypes:readonlyfield":    FieldTypeText,

	// Greenhopper (Jira Software) fields
	"com.pyxis.greenhopper.jira:gh-sprint":        FieldTypeIteration,
	"com.pyxis.greenhopper.jira:gh-epic-link":     FieldTypeText, // Parent link
	"com.pyxis.greenhopper.jira:gh-epic-label":    FieldTypeText,
	"com.pyxis.greenhopper.jira:gh-epic-status":   FieldTypeSelect,
	"com.pyxis.greenhopper.jira:gh-epic-color":    FieldTypeText,
	"com.pyxis.greenhopper.jira:jsw-story-points": FieldTypeNumber,
	"com.pyxis.greenhopper.jira:gh-lexo-rank":     FieldTypeUnmapped, // Internal ranking

	// Tempo and time tracking
	"com.atlassian.jira.ext.charting:timeinstatus":               FieldTypeUnmapped,
	"com.atlassian.jira.plugin.system.customfieldtypes:importid": FieldTypeText,

	// Service Management fields
	"com.atlassian.servicedesk:sd-request-participants":   FieldTypeMultiselect,
	"com.atlassian.servicedesk:vp-origin":                 FieldTypeText,
	"com.atlassian.servicedesk:sd-customer-organizations": FieldTypeMultiselect,

	// Assets/Insight fields
	"com.atlassian.jira.plugins.jira-servicedesk-cmdb-plugin:insight-object-field": FieldTypeAsset,
	"com.atlassian.jira.plugins.cmdb:cmdb-object-cftype":                           FieldTypeAsset,
}

// IsKnownFieldType returns true if the field's custom type is in our mapping table
// Fields with unknown types (e.g., third-party extensions) should be filtered out
func IsKnownFieldType(field JiraCustomField) bool {
	if field.Schema == nil || field.Schema.Custom == "" {
		return false
	}
	_, ok := jiraFieldTypeMap[field.Schema.Custom]
	return ok
}

// MapJiraFieldToWindshift analyzes a Jira custom field and suggests a Windshift mapping
func MapJiraFieldToWindshift(field JiraCustomField) FieldMappingSuggestion {
	suggestion := FieldMappingSuggestion{
		JiraFieldID:   field.ID,
		JiraFieldName: field.Name,
		CanMap:        true,
	}

	// Determine the field type key
	fieldTypeKey := ""
	if field.Schema != nil {
		if field.Schema.Custom != "" {
			fieldTypeKey = field.Schema.Custom
		} else {
			fieldTypeKey = field.Schema.Type
		}
		suggestion.JiraFieldType = fieldTypeKey
	} else {
		fieldTypeKey = field.FieldType
		suggestion.JiraFieldType = fieldTypeKey
	}

	// Look up in the mapping table
	if windshiftType, ok := jiraFieldTypeMap[fieldTypeKey]; ok {
		suggestion.WindshiftFieldType = windshiftType
		if windshiftType == FieldTypeUnmapped {
			suggestion.CanMap = false
			suggestion.Notes = "This field type cannot be directly mapped and will be skipped"
		}
		return suggestion
	}

	// Try to infer from schema type if custom key not found
	if field.Schema != nil {
		switch field.Schema.Type {
		case "string":
			suggestion.WindshiftFieldType = FieldTypeText
		case "number":
			suggestion.WindshiftFieldType = FieldTypeNumber
		case "date", "datetime":
			suggestion.WindshiftFieldType = FieldTypeDate
		case "user":
			suggestion.WindshiftFieldType = FieldTypeUser
		case "array":
			// Array type depends on items
			switch field.Schema.Items {
			case "option":
				suggestion.WindshiftFieldType = FieldTypeMultiselect
			case "user":
				suggestion.WindshiftFieldType = FieldTypeUsers
				suggestion.Notes = "Multi-user field will be stored as array of user IDs"
			case "string":
				suggestion.WindshiftFieldType = FieldTypeMultiselect
			default:
				suggestion.WindshiftFieldType = FieldTypeTextarea
				suggestion.Notes = "Complex array field will be stored as JSON text"
			}
		case "option":
			suggestion.WindshiftFieldType = FieldTypeSelect
		default:
			// Unknown type, default to text
			suggestion.WindshiftFieldType = FieldTypeText
			suggestion.Notes = "Unknown Jira field type, defaulting to text"
		}
		return suggestion
	}

	// Default fallback
	suggestion.WindshiftFieldType = FieldTypeText
	suggestion.Notes = "Could not determine field type, defaulting to text"
	return suggestion
}

// SuggestFieldMappings analyzes all custom fields and suggests mappings
func SuggestFieldMappings(fields []JiraCustomField) []FieldMappingSuggestion {
	suggestions := make([]FieldMappingSuggestion, 0, len(fields))
	for _, field := range fields {
		// Skip fields with unknown types (e.g., third-party extensions like ari:cloud:ecosystem::extension/...)
		if !IsKnownFieldType(field) {
			continue
		}
		suggestions = append(suggestions, MapJiraFieldToWindshift(field))
	}
	return suggestions
}

// StatusCategoryColorMap maps Jira status category colors to hex codes
var StatusCategoryColorMap = map[string]string{
	"blue-gray": "#6B7280", // gray-500
	"yellow":    "#F59E0B", // amber-500
	"green":     "#22C55E", // green-500
	"red":       "#EF4444", // red-500
	"blue":      "#3B82F6", // blue-500
}

// StatusCandidate represents a potential status mapping target
type StatusCandidate struct {
	ID          int
	Name        string
	CategoryID  int
	IsCompleted bool
}

// IssueTypeCandidate represents a potential item type mapping target
type IssueTypeCandidate struct {
	ID             int
	Name           string
	HierarchyLevel int
	Icon           string
	Color          string
}

// PriorityMapping maps common Jira priority names to suggested Windshift equivalents
var PriorityMapping = map[string]string{
	"highest":  "Critical",
	"high":     "High",
	"medium":   "Medium",
	"low":      "Low",
	"lowest":   "Low",
	"blocker":  "Critical",
	"critical": "Critical",
	"major":    "High",
	"minor":    "Low",
	"trivial":  "Low",
}

// SuggestPriorityMapping suggests a priority mapping based on name
func SuggestPriorityMapping(jiraPriorityName string) string {
	normalizedName := strings.ToLower(strings.TrimSpace(jiraPriorityName))
	if mapped, ok := PriorityMapping[normalizedName]; ok {
		return mapped
	}
	return "Medium" // Default
}

// MentionResolver maps a Jira accountID to the Windshift username that should
// be rendered for an `@mention`. Returning "" falls back to the mention's
// display text.
type MentionResolver func(accountID string) string

// ConvertADFToMarkdownWithUsers is the resolver-aware variant. The supplied
// MentionResolver is consulted for every `mention` node so the output uses
// Windshift's `@username` syntax — picked up later by MentionService and
// by the rendered comment view.
func ConvertADFToMarkdownWithUsers(adf interface{}, resolver MentionResolver) string {
	if adf == nil {
		return ""
	}
	if str, ok := adf.(string); ok {
		return str
	}
	adfMap, ok := adf.(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := adfMap["content"].([]interface{})
	if !ok {
		return ""
	}

	var result strings.Builder
	for _, node := range content {
		result.WriteString(convertADFNodeWithResolver(node, resolver))
	}
	return result.String()
}

// convertADFNodeWithResolver converts a single ADF node to Markdown,
// consulting the resolver (when non-nil) for `mention` nodes.
func convertADFNodeWithResolver(node interface{}, resolver MentionResolver) string {
	nodeMap, ok := node.(map[string]interface{})
	if !ok {
		return ""
	}

	nodeType, _ := nodeMap["type"].(string)

	switch nodeType {
	case "paragraph":
		return convertADFContentWithResolver(nodeMap, resolver) + "\n\n"
	case "heading":
		// Guard each step: a missing or non-map attrs would otherwise nil-deref,
		// and a non-float64 level would silently produce a heading with zero "#".
		attrs, _ := nodeMap["attrs"].(map[string]interface{})
		levelF, _ := attrs["level"].(float64)
		level := int(levelF)
		if level < 1 || level > 6 {
			level = 1
		}
		prefix := strings.Repeat("#", level) + " "
		return prefix + convertADFContentWithResolver(nodeMap, resolver) + "\n\n"
	case "bulletList":
		return convertADFListWithResolver(nodeMap, "- ", resolver)
	case "orderedList":
		return convertADFOrderedListWithResolver(nodeMap, resolver)
	case "codeBlock":
		lang := ""
		if attrs, ok := nodeMap["attrs"].(map[string]interface{}); ok {
			lang, _ = attrs["language"].(string)
		}
		return "```" + lang + "\n" + convertADFContentWithResolver(nodeMap, resolver) + "\n```\n\n"
	case "blockquote":
		lines := strings.Split(convertADFContentWithResolver(nodeMap, resolver), "\n")
		var quoted strings.Builder
		for _, line := range lines {
			quoted.WriteString("> " + line + "\n")
		}
		return quoted.String() + "\n"
	case "rule":
		return "---\n\n"
	case "text":
		text, _ := nodeMap["text"].(string)
		// Apply marks (bold, italic, etc.)
		if marks, ok := nodeMap["marks"].([]interface{}); ok {
			for _, mark := range marks {
				markMap, _ := mark.(map[string]interface{})
				markType, _ := markMap["type"].(string)
				switch markType {
				case "strong":
					text = "**" + text + "**"
				case "em":
					text = "*" + text + "*"
				case "code":
					text = "`" + text + "`"
				case "strike":
					text = "~~" + text + "~~"
				case "link":
					if attrs, ok := markMap["attrs"].(map[string]interface{}); ok {
						href, _ := attrs["href"].(string)
						text = "[" + text + "](" + href + ")"
					}
				}
			}
		}
		return text
	case "hardBreak":
		return "\n"
	case "mention":
		attrs, ok := nodeMap["attrs"].(map[string]interface{})
		if !ok {
			return ""
		}
		display, _ := attrs["text"].(string)
		display = strings.TrimPrefix(display, "@")
		// Resolve to a Windshift username when we know who this is. The
		// MentionService will pick `@username` up via its regex; unresolved
		// mentions fall back to the display text so the comment still reads
		// naturally even when the user wasn't part of the import.
		if resolver != nil {
			if accountID, _ := attrs["id"].(string); accountID != "" {
				if uname := resolver(accountID); uname != "" {
					return "@" + uname
				}
			}
		}
		if display == "" {
			return ""
		}
		return "@" + display
	default:
		// For unknown types, try to extract content
		return convertADFContentWithResolver(nodeMap, resolver)
	}
}

func convertADFContentWithResolver(nodeMap map[string]interface{}, resolver MentionResolver) string {
	content, ok := nodeMap["content"].([]interface{})
	if !ok {
		// Check for direct text
		if text, ok := nodeMap["text"].(string); ok {
			return text
		}
		return ""
	}

	var result strings.Builder
	for _, child := range content {
		result.WriteString(convertADFNodeWithResolver(child, resolver))
	}
	return result.String()
}

func convertADFListWithResolver(nodeMap map[string]interface{}, prefix string, resolver MentionResolver) string {
	items, ok := nodeMap["content"].([]interface{})
	if !ok {
		return ""
	}

	var result strings.Builder
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result.WriteString(prefix + strings.TrimSpace(convertADFContentWithResolver(itemMap, resolver)) + "\n")
	}
	return result.String() + "\n"
}

func convertADFOrderedListWithResolver(nodeMap map[string]interface{}, resolver MentionResolver) string {
	items, ok := nodeMap["content"].([]interface{})
	if !ok {
		return ""
	}

	var result strings.Builder
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		fmt.Fprintf(&result, "%d. %s\n", i+1, strings.TrimSpace(convertADFContentWithResolver(itemMap, resolver)))
	}
	return result.String() + "\n"
}
