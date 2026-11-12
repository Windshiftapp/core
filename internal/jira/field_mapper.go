package jira

import (
	"fmt"
	"strconv"
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
	FieldTypeMultiUser   WindshiftFieldType = "multi_user"
	FieldTypeMilestone   WindshiftFieldType = "milestone"
	FieldTypeIteration   WindshiftFieldType = "iteration"
	FieldTypeAsset       WindshiftFieldType = "asset"
	FieldTypeBoolean     WindshiftFieldType = "boolean"
	// FieldTypeCheckbox is retained as a source-compatible name for callers
	// that present the canonical boolean field as a checkbox control.
	FieldTypeCheckbox WindshiftFieldType = FieldTypeBoolean
	FieldTypeUnmapped WindshiftFieldType = "unmapped"
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
	PreserveRaw        bool               `json:"preserve_raw,omitempty"`
}

// jiraFieldTypeMap maps Jira field type keys to Windshift field types
var jiraFieldTypeMap = map[string]WindshiftFieldType{
	// Standard Jira field types (from schema.type)
	"string":    FieldTypeText,
	"text":      FieldTypeTextarea,
	"number":    FieldTypeNumber,
	"date":      FieldTypeDate,
	"datetime":  FieldTypeDate,
	"boolean":   FieldTypeBoolean,
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
	"com.atlassian.jira.plugin.system.customfieldtypes:boolean":          FieldTypeBoolean,
	"com.atlassian.jira.plugin.system.customfieldtypes:datepicker":       FieldTypeDate,
	"com.atlassian.jira.plugin.system.customfieldtypes:datetime":         FieldTypeDate,
	"com.atlassian.jira.plugin.system.customfieldtypes:url":              FieldTypeText,
	"com.atlassian.jira.plugin.system.customfieldtypes:userpicker":       FieldTypeUser,
	"com.atlassian.jira.plugin.system.customfieldtypes:multiuserpicker":  FieldTypeMultiUser,
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
	"com.atlassian.servicedesk:vp-origin":                 FieldTypeUnmapped,
	"com.atlassian.servicedesk:sd-customer-organizations": FieldTypeMultiselect,

	// Assets/Insight fields
	"com.atlassian.jira.plugins.jira-servicedesk-cmdb-plugin:insight-object-field": FieldTypeAsset,
	"com.atlassian.jira.plugins.cmdb:cmdb-object-cftype":                           FieldTypeAsset,
}

// IsKnownFieldType reports whether a field has an explicit plugin-type mapping.
// Unknown plugin keys must still be offered by SuggestFieldMappings because
// Jira's schema type can often provide a safe native or raw-preservation
// fallback.
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
		if field.Schema != nil && field.Schema.Type == "array" && field.Schema.Items == "user" {
			suggestion.WindshiftFieldType = FieldTypeMultiUser
			suggestion.Notes = "Jira schema items=user; values map to Windshift users."
		}
		if windshiftType == FieldTypeUnmapped {
			suggestion.CanMap = false
			switch fieldTypeKey {
			case "com.atlassian.servicedesk:vp-origin":
				suggestion.Notes = "Imported as the item's first-class portal request type"
			case "com.pyxis.greenhopper.jira:gh-lexo-rank":
				suggestion.Notes = "Used to order issue creation; Windshift fractional indexes are generated in Jira Rank order"
			default:
				suggestion.Notes = "This field type cannot be directly mapped and will be skipped"
			}
		} else {
			addJiraChoiceMappingNote(&suggestion)
		}
		if jiraFieldIsDateTime(field) {
			suggestion.Notes = strings.TrimSpace(suggestion.Notes +
				" Jira datetime values are stored in the date field as a calendar date; the original RFC3339 timestamp is retained in the item's Jira custom-field metadata, but time-of-day editing in the date field is lossy.")
		}
		return suggestion
	}

	// Try to infer from schema type if custom key not found
	if field.Schema != nil {
		switch field.Schema.Type {
		case "string":
			suggestion.WindshiftFieldType = FieldTypeText
			suggestion.Notes = "Inferred from Jira schema type string."
		case "number":
			suggestion.WindshiftFieldType = FieldTypeNumber
			suggestion.Notes = "Inferred from Jira schema type number."
		case "date", "datetime":
			suggestion.WindshiftFieldType = FieldTypeDate
			suggestion.Notes = "Inferred from Jira schema; a datetime is stored as a calendar date and its original timestamp is retained in item metadata."
		case "user":
			suggestion.WindshiftFieldType = FieldTypeUser
			suggestion.Notes = "Inferred from Jira schema type user."
		case "array":
			// Array type depends on items
			switch field.Schema.Items {
			case "option", "option2", "option-with-child":
				suggestion.WindshiftFieldType = FieldTypeMultiselect
				suggestion.Notes = "Inferred from Jira schema array of option values."
			case "user":
				suggestion.WindshiftFieldType = FieldTypeMultiUser
				suggestion.Notes = "Inferred from Jira schema; values map to Windshift users."
			case "string":
				suggestion.WindshiftFieldType = FieldTypeMultiselect
				suggestion.Notes = "Inferred from Jira schema array items=string."
			default:
				suggestion.WindshiftFieldType = FieldTypeTextarea
				suggestion.PreserveRaw = true
				suggestion.Notes = "Complex Jira array has no native equivalent and will be preserved as JSON text."
			}
		case "option":
			suggestion.WindshiftFieldType = FieldTypeSelect
			suggestion.Notes = "Inferred from Jira schema type option."
		case "option2", "option-with-child":
			suggestion.WindshiftFieldType = FieldTypeSelect
			suggestion.Notes = "Jira option structure is flattened to its display path."
		default:
			suggestion.WindshiftFieldType = FieldTypeTextarea
			suggestion.PreserveRaw = true
			suggestion.Notes = "App-owned Jira value has no proven native shape and will be preserved as JSON text."
		}
		addJiraChoiceMappingNote(&suggestion)
		if jiraFieldIsDateTime(field) {
			suggestion.Notes = strings.TrimSpace(suggestion.Notes +
				" Jira datetime values are stored in the date field as a calendar date; the original RFC3339 timestamp is retained in the item's Jira custom-field metadata, but time-of-day editing in the date field is lossy.")
		}
		return suggestion
	}

	// Preserve rather than hide a field whose definition has no usable schema.
	suggestion.WindshiftFieldType = FieldTypeTextarea
	suggestion.PreserveRaw = true
	suggestion.Notes = "Jira did not expose a usable schema; populated values will be preserved as JSON text."
	return suggestion
}

// SuggestFieldMappings analyzes all custom fields and suggests mappings
func SuggestFieldMappings(fields []JiraCustomField) []FieldMappingSuggestion {
	suggestions := make([]FieldMappingSuggestion, 0, len(fields))
	for _, field := range fields {
		suggestions = append(suggestions, MapJiraFieldToWindshift(field))
	}
	return suggestions
}

func addJiraChoiceMappingNote(suggestion *FieldMappingSuggestion) {
	if suggestion == nil ||
		(suggestion.WindshiftFieldType != FieldTypeSelect && suggestion.WindshiftFieldType != FieldTypeMultiselect) {
		return
	}
	const note = " Populated option labels will be normalized to stable Windshift option IDs before issue import."
	suggestion.Notes = strings.TrimSpace(suggestion.Notes + note)
}

func jiraFieldIsDateTime(field JiraCustomField) bool {
	if field.Schema == nil {
		return false
	}
	return strings.EqualFold(field.Schema.Type, "datetime") ||
		strings.EqualFold(field.Schema.Custom, "com.atlassian.jira.plugin.system.customfieldtypes:datetime")
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

// PriorityMappingTarget returns the Windshift priority a Jira name maps to and
// whether that mapping is a known synonym. Unknown names are returned unchanged
// so importers can preserve and create them instead of collapsing to the
// default.
func PriorityMappingTarget(jiraPriorityName string) (target string, known bool) {
	name := strings.TrimSpace(jiraPriorityName)
	if mapped, ok := PriorityMapping[strings.ToLower(name)]; ok {
		return mapped, true
	}
	return name, false
}

// MentionResolver maps a Jira accountID to the Windshift username that should
// be rendered for an `@mention`. Returning "" falls back to the mention's
// display text.
type MentionResolver func(accountID string) string

// MediaResolution is the Markdown a MediaResolver returns for a single ADF
// media node. When Resolved is true the converter uses Markdown verbatim
// (already a proper image/link to the imported attachment); otherwise it falls
// back to the lossy [media] placeholder.
type MediaResolution struct {
	Resolved bool
	Markdown string
}

// MediaResolver maps an ADF media node's Jira attachment id (the `attrs.id`
// Jira surfaces on `media` nodes, which is the same id as the issue's
// attachment list) to a Windshift Markdown reference for that imported
// attachment. Returning Resolved=false (or "" markdown) leaves the default
// placeholder.
type MediaResolver func(jiraAttachmentID string) MediaResolution

// NewMediaResolver builds a MediaResolver from a Jira attachment id →
// Windshift MediaAttachment map. Images render inline as `![alt](/api/.../download)`;
// everything else renders as a `[name](/api/.../download)` link. An unknown or
// missing id yields an unresolved result so the caller keeps its placeholder.
// The reference path is the same relative path Windshift's own attachment
// upload endpoints document (`/api/attachments/{id}/download`).
//
// Data Center wiki bodies name files instead of carrying a media id, so the
// resolver also matches on filename. A filename only identifies a file when it
// is unique among the issue's attachments; duplicated names stay unresolved so
// the converter keeps its placeholder rather than linking an arbitrary file.
func NewMediaResolver(refs map[string]MediaAttachment) MediaResolver {
	byFilename := make(map[string]MediaAttachment, len(refs))
	ambiguous := make(map[string]struct{})
	for _, ref := range refs {
		name := strings.TrimSpace(ref.OriginalFilename)
		if name == "" {
			continue
		}
		if _, seen := byFilename[name]; seen {
			ambiguous[name] = struct{}{}
			continue
		}
		byFilename[name] = ref
	}

	return func(jiraAttachmentID string) MediaResolution {
		if refs == nil {
			return MediaResolution{}
		}
		ref, ok := refs[jiraAttachmentID]
		if !ok || ref.ID == 0 {
			name := strings.TrimSpace(jiraAttachmentID)
			if _, duplicated := ambiguous[name]; duplicated {
				return MediaResolution{}
			}
			ref, ok = byFilename[name]
			if !ok || ref.ID == 0 {
				return MediaResolution{}
			}
		}
		href := fmt.Sprintf("/api/attachments/%d/download", ref.ID)
		alt := ref.OriginalFilename
		if alt == "" {
			alt = "attachment"
		}
		if isImageMimeType(ref.MimeType) {
			return MediaResolution{
				Resolved: true,
				Markdown: "![" + alt + "](" + href + ")",
			}
		}
		return MediaResolution{
			Resolved: true,
			Markdown: "[" + alt + "](" + href + ")",
		}
	}
}

// isImageMimeType reports whether a MIME type represents a raster/vector
// image that an inline Markdown image reference can render directly.
func isImageMimeType(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "" {
		return false
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return false
	}
	return true
}

// ConvertADFToMarkdownWithUsers is the resolver-aware variant. The supplied
// MentionResolver is consulted for every `mention` node so the output uses
// Windshift's `@username` syntax — picked up later by MentionService and
// by the rendered comment view.
func ConvertADFToMarkdownWithUsers(adf any, resolver MentionResolver) string {
	return ConvertADFToMarkdown(adf, resolver, nil)
}

// ConvertJiraRichText renders a Jira rich-text value to Markdown, choosing the
// converter from the value's shape: ADF documents (Cloud) use the ADF
// converter, strings (Server/Data Center wiki markup) use the wiki converter.
func ConvertJiraRichText(value any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return ConvertWikiMarkupToMarkdown(str, mediaResolver)
	}
	return ConvertADFToMarkdown(value, mentionResolver, mediaResolver)
}

// ConvertADFToMarkdown converts an ADF document to Markdown, consulting the
// optional mention and media resolvers. mentionResolver renders `@mention`
// nodes as Windshift `@username`s; mediaResolver links `media` nodes to the
// imported attachments where possible (otherwise they fall back to a
// placeholder). Either may be nil.
func ConvertADFToMarkdown(adf any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	if adf == nil {
		return ""
	}
	if str, ok := adf.(string); ok {
		return str
	}
	adfMap, ok := adf.(map[string]any)
	if !ok {
		return ""
	}
	content, ok := adfMap["content"].([]any)
	if !ok {
		return ""
	}

	var result strings.Builder
	for _, node := range content {
		result.WriteString(convertADFNode(node, mentionResolver, mediaResolver))
	}
	return result.String()
}

// convertADFNode converts a single ADF node to Markdown,
// consulting the resolvers (when non-nil) for `mention` and `media` nodes.
func convertADFNode(node any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	nodeMap, ok := node.(map[string]any)
	if !ok {
		return ""
	}

	nodeType, _ := nodeMap["type"].(string)

	switch nodeType {
	case "paragraph":
		return convertADFContent(nodeMap, mentionResolver, mediaResolver) + "\n\n"
	case "heading":
		// Guard each step: a missing or non-map attrs would otherwise nil-deref,
		// and a non-float64 level would silently produce a heading with zero "#".
		attrs, _ := nodeMap["attrs"].(map[string]any)
		levelF, _ := attrs["level"].(float64)
		level := int(levelF)
		if level < 1 || level > 6 {
			level = 1
		}
		prefix := strings.Repeat("#", level) + " "
		return prefix + convertADFContent(nodeMap, mentionResolver, mediaResolver) + "\n\n"
	case "bulletList":
		return convertADFList(nodeMap, "- ", mentionResolver, mediaResolver)
	case "orderedList":
		return convertADFOrderedList(nodeMap, mentionResolver, mediaResolver)
	case "codeBlock":
		lang := ""
		if attrs, ok := nodeMap["attrs"].(map[string]any); ok {
			lang, _ = attrs["language"].(string)
		}
		return "```" + lang + "\n" + convertADFContent(nodeMap, mentionResolver, mediaResolver) + "\n```\n\n"
	case "blockquote":
		lines := strings.Split(convertADFContent(nodeMap, mentionResolver, mediaResolver), "\n")
		var quoted strings.Builder
		for _, line := range lines {
			quoted.WriteString("> " + line + "\n")
		}
		return quoted.String() + "\n"
	case "table":
		return convertADFTable(nodeMap, mentionResolver, mediaResolver)
	case "panel":
		return convertADFPanel(nodeMap, mentionResolver, mediaResolver)
	case "taskList":
		return convertADFTaskList(nodeMap, mentionResolver, mediaResolver)
	case "inlineCard", "blockCard":
		return convertADFCard(nodeMap)
	case "mediaSingle", "mediaGroup":
		return convertADFContent(nodeMap, mentionResolver, mediaResolver)
	case "media":
		return convertADFMedia(nodeMap, mediaResolver)
	case "expand", "nestedExpand":
		return convertADFExpand(nodeMap, mentionResolver, mediaResolver)
	case "status":
		return convertADFStatus(nodeMap)
	case "emoji":
		return convertADFEmoji(nodeMap)
	case "date":
		return convertADFDate(nodeMap)
	case "rule":
		return "---\n\n"
	case "text":
		text, _ := nodeMap["text"].(string)
		// Apply marks (bold, italic, etc.)
		if marks, ok := nodeMap["marks"].([]any); ok {
			for _, mark := range marks {
				markMap, _ := mark.(map[string]any)
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
					if attrs, ok := markMap["attrs"].(map[string]any); ok {
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
		attrs, ok := nodeMap["attrs"].(map[string]any)
		if !ok {
			return ""
		}
		display, _ := attrs["text"].(string)
		display = strings.TrimPrefix(display, "@")
		// Resolve to a Windshift username when we know who this is. The
		// MentionService will pick `@username` up via its regex; unresolved
		// mentions fall back to the display text so the comment still reads
		// naturally even when the user wasn't part of the import.
		if mentionResolver != nil {
			if accountID, _ := attrs["id"].(string); accountID != "" {
				if uname := mentionResolver(accountID); uname != "" {
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
		return convertADFContent(nodeMap, mentionResolver, mediaResolver)
	}
}

func convertADFTable(nodeMap map[string]any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	rowsRaw, ok := nodeMap["content"].([]any)
	if !ok || len(rowsRaw) == 0 {
		return ""
	}

	rows := make([][]string, 0, len(rowsRaw))
	for _, rowRaw := range rowsRaw {
		rowMap, ok := rowRaw.(map[string]any)
		if !ok {
			continue
		}
		cellsRaw, ok := rowMap["content"].([]any)
		if !ok {
			continue
		}
		row := make([]string, 0, len(cellsRaw))
		for _, cellRaw := range cellsRaw {
			cellMap, ok := cellRaw.(map[string]any)
			if !ok {
				continue
			}
			row = append(row, markdownTableCell(convertADFContent(cellMap, mentionResolver, mediaResolver)))
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return ""
	}

	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i := range rows {
		for len(rows[i]) < maxCols {
			rows[i] = append(rows[i], "")
		}
	}

	var out strings.Builder
	writeMarkdownTableRow(&out, rows[0])
	separator := make([]string, maxCols)
	for i := range separator {
		separator[i] = "---"
	}
	writeMarkdownTableRow(&out, separator)
	for _, row := range rows[1:] {
		writeMarkdownTableRow(&out, row)
	}
	out.WriteString("\n")
	return out.String()
}

func markdownTableCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, strings.ReplaceAll(line, "|", "\\|"))
		}
	}
	return strings.Join(clean, "<br>")
}

func writeMarkdownTableRow(out *strings.Builder, cells []string) {
	out.WriteString("| ")
	out.WriteString(strings.Join(cells, " | "))
	out.WriteString(" |\n")
}

func convertADFPanel(nodeMap map[string]any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	panelType := "info"
	if attrs, ok := nodeMap["attrs"].(map[string]any); ok {
		if raw, _ := attrs["panelType"].(string); raw != "" {
			panelType = raw
		}
	}
	content := strings.TrimSpace(convertADFContent(nodeMap, mentionResolver, mediaResolver))
	if content == "" {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "> [!%s]\n", strings.ToUpper(panelType))
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out.WriteString("> " + line + "\n")
	}
	out.WriteString("\n")
	return out.String()
}

func convertADFTaskList(nodeMap map[string]any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	items, ok := nodeMap["content"].([]any)
	if !ok {
		return ""
	}
	var out strings.Builder
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		state := "TODO"
		if attrs, ok := itemMap["attrs"].(map[string]any); ok {
			state, _ = attrs["state"].(string)
		}
		checkbox := "[ ]"
		if strings.EqualFold(state, "DONE") || strings.EqualFold(state, "checked") {
			checkbox = "[x]"
		}
		content := strings.TrimSpace(convertADFContent(itemMap, mentionResolver, mediaResolver))
		if content == "" {
			continue
		}
		fmt.Fprintf(&out, "- %s %s\n", checkbox, collapseMarkdownWhitespace(content))
	}
	out.WriteString("\n")
	return out.String()
}

func convertADFCard(nodeMap map[string]any) string {
	attrs, ok := nodeMap["attrs"].(map[string]any)
	if !ok {
		return ""
	}
	url, _ := attrs["url"].(string)
	if url == "" {
		return ""
	}
	return "[" + url + "](" + url + ")"
}

func convertADFMedia(nodeMap map[string]any, mediaResolver MediaResolver) string {
	attrs, ok := nodeMap["attrs"].(map[string]any)
	if !ok {
		return "[media]"
	}
	// `media` nodes reference an attachment by its Jira attachment id (the same
	// id the issue's attachment list carries). Resolve it to the imported
	// attachment's download link when the importer has one; otherwise keep the
	// lossy placeholder so the reference isn't silently dropped.
	id, _ := attrs["id"].(string)
	if mediaResolver != nil && id != "" {
		if res := mediaResolver(id); res.Resolved && res.Markdown != "" {
			return res.Markdown
		}
	}
	alt := firstADFAttrString(attrs, "alt", "id", "type")
	if alt == "" {
		return "[media]"
	}
	return "[media: " + alt + "]"
}

// IsADFDocument reports whether a decoded Jira field value is an Atlassian
// Document Format document rather than a plain string or option object.
func IsADFDocument(value any) bool {
	doc, ok := value.(map[string]any)
	if !ok {
		return false
	}
	nodeType, _ := doc["type"].(string)
	return nodeType == "doc"
}

// CollectADFMediaIDs returns every Jira attachment id referenced by `media`
// nodes in an ADF document. Callers use it to decide which imported
// attachments are linked from a body a portal customer can see.
func CollectADFMediaIDs(adf any) map[string]bool {
	ids := make(map[string]bool)
	collectADFMediaIDs(adf, ids)
	return ids
}

func collectADFMediaIDs(node any, ids map[string]bool) {
	switch typed := node.(type) {
	case []any:
		for _, child := range typed {
			collectADFMediaIDs(child, ids)
		}
	case map[string]any:
		if nodeType, _ := typed["type"].(string); nodeType == "media" {
			if attrs, ok := typed["attrs"].(map[string]any); ok {
				if id, _ := attrs["id"].(string); id != "" {
					ids[id] = true
				}
			}
		}
		if content, ok := typed["content"].([]any); ok {
			collectADFMediaIDs(content, ids)
		}
	}
}

func convertADFExpand(nodeMap map[string]any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	title := "Details"
	if attrs, ok := nodeMap["attrs"].(map[string]any); ok {
		if raw, _ := attrs["title"].(string); strings.TrimSpace(raw) != "" {
			title = strings.TrimSpace(raw)
		}
	}
	content := strings.TrimSpace(convertADFContent(nodeMap, mentionResolver, mediaResolver))
	if content == "" {
		return ""
	}
	return "<details>\n<summary>" + title + "</summary>\n\n" + content + "\n\n</details>\n\n"
}

func convertADFStatus(nodeMap map[string]any) string {
	attrs, ok := nodeMap["attrs"].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := attrs["text"].(string)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return "[" + strings.TrimSpace(text) + "]"
}

func convertADFEmoji(nodeMap map[string]any) string {
	attrs, ok := nodeMap["attrs"].(map[string]any)
	if !ok {
		return ""
	}
	return firstADFAttrString(attrs, "text", "shortName", "id")
}

func convertADFDate(nodeMap map[string]any) string {
	attrs, ok := nodeMap["attrs"].(map[string]any)
	if !ok {
		return ""
	}
	timestamp, _ := attrs["timestamp"].(string)
	if timestamp == "" {
		return ""
	}
	millis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return timestamp
	}
	return time.UnixMilli(millis).UTC().Format("2006-01-02")
}

func firstADFAttrString(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, _ := attrs[key].(string); strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func collapseMarkdownWhitespace(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	return strings.Join(clean, " ")
}

func convertADFContent(nodeMap map[string]any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	content, ok := nodeMap["content"].([]any)
	if !ok {
		// Check for direct text
		if text, ok := nodeMap["text"].(string); ok {
			return text
		}
		return ""
	}

	var result strings.Builder
	for _, child := range content {
		result.WriteString(convertADFNode(child, mentionResolver, mediaResolver))
	}
	return result.String()
}

func convertADFList(nodeMap map[string]any, prefix string, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	items, ok := nodeMap["content"].([]any)
	if !ok {
		return ""
	}

	var result strings.Builder
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result.WriteString(prefix + strings.TrimSpace(convertADFContent(itemMap, mentionResolver, mediaResolver)) + "\n")
	}
	return result.String() + "\n"
}

func convertADFOrderedList(nodeMap map[string]any, mentionResolver MentionResolver, mediaResolver MediaResolver) string {
	items, ok := nodeMap["content"].([]any)
	if !ok {
		return ""
	}

	var result strings.Builder
	for i, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fmt.Fprintf(&result, "%d. %s\n", i+1, strings.TrimSpace(convertADFContent(itemMap, mentionResolver, mediaResolver)))
	}
	return result.String() + "\n"
}
