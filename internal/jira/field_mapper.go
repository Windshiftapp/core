package jira

import (
	"fmt"
	"regexp"
	"strings"
)

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

// NormalizeStatusName normalizes a status name for matching
func NormalizeStatusName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Remove common separators
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	// Remove extra spaces
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	return name
}

// SuggestStatusMapping suggests a Windshift status ID based on name matching
func SuggestStatusMapping(jiraStatusName string, windshiftStatuses []StatusCandidate) *int {
	normalizedJira := NormalizeStatusName(jiraStatusName)

	// First pass: exact match (normalized)
	for _, ws := range windshiftStatuses {
		if NormalizeStatusName(ws.Name) == normalizedJira {
			return &ws.ID
		}
	}

	// Second pass: common synonyms
	synonyms := map[string][]string{
		"todo":        {"to do", "open", "new", "backlog", "not started"},
		"in progress": {"in development", "doing", "in review", "active", "working"},
		"done":        {"closed", "resolved", "complete", "completed", "finished"},
		"blocked":     {"on hold", "waiting", "impediment"},
	}

	for canonical, alts := range synonyms {
		isJiraMatch := normalizedJira == canonical
		for _, alt := range alts {
			if normalizedJira == alt {
				isJiraMatch = true
				break
			}
		}

		if isJiraMatch {
			// Look for Windshift status matching this canonical form or alts
			for _, ws := range windshiftStatuses {
				normalizedWS := NormalizeStatusName(ws.Name)
				if normalizedWS == canonical {
					return &ws.ID
				}
				for _, alt := range alts {
					if normalizedWS == alt {
						return &ws.ID
					}
				}
			}
		}
	}

	return nil
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

// SuggestIssueTypeMapping suggests a Windshift item type based on name matching
func SuggestIssueTypeMapping(jiraIssueTypeName string, windshiftItemTypes []IssueTypeCandidate) *int {
	normalizedJira := NormalizeStatusName(jiraIssueTypeName)

	// First pass: exact match (normalized)
	for _, wt := range windshiftItemTypes {
		if NormalizeStatusName(wt.Name) == normalizedJira {
			return &wt.ID
		}
	}

	// Common issue type synonyms
	synonyms := map[string][]string{
		"epic":        {"initiative", "theme"},
		"story":       {"user story", "feature"},
		"task":        {"work item", "action"},
		"bug":         {"defect", "issue", "error"},
		"sub-task":    {"subtask", "sub task", "child"},
		"improvement": {"enhancement", "request"},
	}

	for canonical, alts := range synonyms {
		isJiraMatch := normalizedJira == canonical
		for _, alt := range alts {
			if normalizedJira == alt {
				isJiraMatch = true
				break
			}
		}

		if isJiraMatch {
			for _, wt := range windshiftItemTypes {
				normalizedWT := NormalizeStatusName(wt.Name)
				if normalizedWT == canonical {
					return &wt.ID
				}
				for _, alt := range alts {
					if normalizedWT == alt {
						return &wt.ID
					}
				}
			}
		}
	}

	return nil
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

// ConvertADFToMarkdown converts Atlassian Document Format to Markdown
// This is a simplified converter for common cases
func ConvertADFToMarkdown(adf interface{}) string {
	if adf == nil {
		return ""
	}

	// If it's already a string, return it
	if str, ok := adf.(string); ok {
		return str
	}

	// Try to convert from ADF structure
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
		result.WriteString(convertADFNode(node))
	}
	return result.String()
}

// convertADFNode converts a single ADF node to Markdown
func convertADFNode(node interface{}) string {
	nodeMap, ok := node.(map[string]interface{})
	if !ok {
		return ""
	}

	nodeType, _ := nodeMap["type"].(string)

	switch nodeType {
	case "paragraph":
		return convertADFContent(nodeMap) + "\n\n"
	case "heading":
		level, _ := nodeMap["attrs"].(map[string]interface{})["level"].(float64)
		prefix := strings.Repeat("#", int(level)) + " "
		return prefix + convertADFContent(nodeMap) + "\n\n"
	case "bulletList":
		return convertADFList(nodeMap, "- ")
	case "orderedList":
		return convertADFOrderedList(nodeMap)
	case "codeBlock":
		lang := ""
		if attrs, ok := nodeMap["attrs"].(map[string]interface{}); ok {
			lang, _ = attrs["language"].(string)
		}
		return "```" + lang + "\n" + convertADFContent(nodeMap) + "\n```\n\n"
	case "blockquote":
		lines := strings.Split(convertADFContent(nodeMap), "\n")
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
		if attrs, ok := nodeMap["attrs"].(map[string]interface{}); ok {
			text, _ := attrs["text"].(string)
			return "@" + text
		}
		return ""
	default:
		// For unknown types, try to extract content
		return convertADFContent(nodeMap)
	}
}

func convertADFContent(nodeMap map[string]interface{}) string {
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
		result.WriteString(convertADFNode(child))
	}
	return result.String()
}

func convertADFList(nodeMap map[string]interface{}, prefix string) string {
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
		result.WriteString(prefix + strings.TrimSpace(convertADFContent(itemMap)) + "\n")
	}
	return result.String() + "\n"
}

func convertADFOrderedList(nodeMap map[string]interface{}) string {
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
		fmt.Fprintf(&result, "%d. %s\n", i+1, strings.TrimSpace(convertADFContent(itemMap)))
	}
	return result.String() + "\n"
}
