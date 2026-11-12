package services

import (
	"encoding/json"
	"strconv"
	"strings"

	"windshift/internal/models"
)

const (
	historyCustomFieldPrefix  = "cf_"
	historyVirtualFieldPrefix = "virtual_"
)

// historyResolution carries the per-read permission scope and memoizes id-to-
// name lookups so a long feed does not issue one query per row.
type historyResolution struct {
	userID          int
	allowedProjects map[int]struct{}
	names           map[string]string
	customFields    map[int]*models.CustomFieldDefinition
	virtualFields   map[string]*historyVirtualField
}

// historyVirtualField is the request-type field behind a virtual_ history key.
type historyVirtualField struct {
	label   string
	kind    string
	options string
}

// resolveHistoryFieldNameUncached returns a display label for field keys that
// encode an id. Ordinary field names return "".
func (s *ItemApplicationService) resolveHistoryFieldNameUncached(field string, res *historyResolution) string {
	if id, ok := historyCustomFieldID(field); ok {
		return s.resolver.ResolveCustomFieldName(id)
	}
	if identifier, ok := strings.CutPrefix(field, historyVirtualFieldPrefix); ok {
		if v := s.virtualField(identifier, res); v != nil {
			return v.label
		}
	}
	return ""
}

// resolveCustomFieldValue resolves a cf_<id> value to its display text. It
// returns "" for values that need no lookup or cannot be resolved, leaving the
// raw value in place.
func (s *ItemApplicationService) resolveCustomFieldValue(field, value string, res *historyResolution) string {
	id, ok := historyCustomFieldID(field)
	if !ok {
		return ""
	}
	def := s.customFieldDefinition(id, res)
	if def == nil {
		return ""
	}
	switch def.FieldType {
	case "user", "multi_user":
		return historyResolveRefs(value, func(ref historyRef) string {
			return s.resolver.ResolveUserName(ref.id)
		})
	case "milestone", "multi_milestone":
		return historyResolveRefs(value, func(ref historyRef) string {
			return s.resolver.ResolveMilestoneName(ref.id)
		})
	case "iteration":
		return historyResolveRefs(value, func(ref historyRef) string {
			return s.resolver.ResolveIterationName(ref.id)
		})
	case "portalcustomer":
		return historyResolveRefs(value, func(ref historyRef) string {
			return s.resolver.ResolvePortalCustomerName(ref.id)
		})
	case "customerorganisation":
		return historyResolveRefs(value, func(ref historyRef) string {
			return s.resolver.ResolveCustomerOrganisationName(ref.id)
		})
	case "asset":
		return historyResolveRefs(value, func(ref historyRef) string {
			if !s.assetVisible(res.userID, ref.id) {
				return ""
			}
			return s.resolver.ResolveAssetName(ref.id)
		})
	case "select", "multiselect":
		return historySelectLabels(def.Options, value)
	default:
		return ""
	}
}

// resolveVirtualFieldValue resolves a virtual_<identifier> value. Only select
// virtual fields carry an id; their stored value is the configured option
// value, which resolves to the option label.
func (s *ItemApplicationService) resolveVirtualFieldValue(field, value string, res *historyResolution) string {
	identifier, ok := strings.CutPrefix(field, historyVirtualFieldPrefix)
	if !ok {
		return ""
	}
	v := s.virtualField(identifier, res)
	if v == nil || v.kind != "select" || v.options == "" {
		return ""
	}
	return historyVirtualSelectLabel(v.options, value)
}

func (s *ItemApplicationService) customFieldDefinition(id int, res *historyResolution) *models.CustomFieldDefinition {
	if res.customFields == nil {
		res.customFields = map[int]*models.CustomFieldDefinition{}
	}
	if def, ok := res.customFields[id]; ok {
		return def
	}
	var def models.CustomFieldDefinition
	err := s.db.QueryRow(
		`SELECT id, name, field_type, COALESCE(options, '') FROM custom_field_definitions WHERE id = ?`,
		id,
	).Scan(&def.ID, &def.Name, &def.FieldType, &def.Options)
	if err != nil {
		res.customFields[id] = nil
		return nil
	}
	res.customFields[id] = &def
	return &def
}

// virtualField resolves a request-type field by identifier. Identifiers are
// only unique per request type, so the first match wins; a field's label is
// stable enough for a history line either way.
func (s *ItemApplicationService) virtualField(identifier string, res *historyResolution) *historyVirtualField {
	if res != nil {
		if res.virtualFields == nil {
			res.virtualFields = map[string]*historyVirtualField{}
		}
		if v, ok := res.virtualFields[identifier]; ok {
			return v
		}
	}
	var v historyVirtualField
	err := s.db.QueryRow(`
		SELECT COALESCE(NULLIF(TRIM(display_name), ''), field_identifier),
		       COALESCE(virtual_field_type, ''),
		       COALESCE(virtual_field_options, '')
		FROM request_type_fields
		WHERE field_identifier = ? AND field_type = 'virtual'
		ORDER BY id
		LIMIT 1
	`, identifier).Scan(&v.label, &v.kind, &v.options)
	if err != nil {
		if res != nil {
			res.virtualFields[identifier] = nil
		}
		return nil
	}
	if res != nil {
		res.virtualFields[identifier] = &v
	}
	return &v
}

// historyCustomFieldID extracts the numeric id from a cf_<id> field key.
func historyCustomFieldID(field string) (int, bool) {
	suffix, ok := strings.CutPrefix(field, historyCustomFieldPrefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.Atoi(suffix)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// assetVisible reports whether the viewer can see an asset's set. A nil
// checker fails closed, leaving the raw reference in place.
func (s *ItemApplicationService) assetVisible(userID, assetID int) bool {
	if s.assetPerm == nil {
		return false
	}
	var setID int
	if err := s.db.QueryRow(`SELECT set_id FROM assets WHERE id = ?`, assetID).Scan(&setID); err != nil {
		return false
	}
	allowed, err := s.assetPerm.HasAssetSetPermission(userID, setID, AssetPermissionKeyView)
	if err != nil {
		return false
	}
	return allowed
}

// historyRef is one reference value from a custom field: an id, an already
// resolved display name, or both when the stored object carried a name.
type historyRef struct {
	id   int
	name string
}

// historyResolveRefs resolves every reference in a stored custom-field value,
// joining multi-value fields with ", ". References that no longer resolve are
// omitted; "" means nothing resolved and the caller should show the raw value.
func historyResolveRefs(raw string, resolve func(historyRef) string) string {
	refs := historyRefs(raw)
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := ref.name
		if name == "" && ref.id > 0 {
			name = resolve(ref)
		}
		if name != "" {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
}

// historyRefs decodes the stored value of a reference custom field. Values are
// stored as scalars, JSON arrays, or objects carrying a display name.
func historyRefs(raw string) []historyRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []historyRef{historyRefFromString(raw)}
	}
	return historyRefsFromValue(parsed)
}

func historyRefsFromValue(value any) []historyRef {
	switch typed := value.(type) {
	case float64:
		return []historyRef{{id: int(typed)}}
	case json.Number:
		if id, err := typed.Int64(); err == nil {
			return []historyRef{{id: int(id)}}
		}
	case string:
		return []historyRef{historyRefFromString(typed)}
	case []any:
		refs := make([]historyRef, 0, len(typed))
		for _, entry := range typed {
			refs = append(refs, historyRefsFromValue(entry)...)
		}
		return refs
	case map[string]any:
		ref := historyRef{}
		if id, ok := historyIDFromValue(typed["id"]); ok {
			ref.id = id
		}
		for _, key := range []string{"name", "title", "label", "asset_tag"} {
			if name, ok := typed[key].(string); ok && strings.TrimSpace(name) != "" {
				ref.name = strings.TrimSpace(name)
				break
			}
		}
		if ref.id == 0 && ref.name == "" {
			return nil
		}
		return []historyRef{ref}
	}
	return nil
}

func historyRefFromString(value string) historyRef {
	if id, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		return historyRef{id: id}
	}
	return historyRef{name: strings.TrimSpace(value)}
}

func historyIDFromValue(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case json.Number:
		if id, err := typed.Int64(); err == nil {
			return int(id), true
		}
	case string:
		if id, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return id, true
		}
	}
	return 0, false
}

// historySelectLabels maps stored select/multiselect option ids to their
// configured labels. Legacy string-array options store the label directly.
func historySelectLabels(optionsJSON, raw string) string {
	refs := historyRefs(raw)
	if len(refs) == 0 {
		return ""
	}
	opts, err := models.ParseSelectOptions(optionsJSON)
	if err != nil {
		return ""
	}
	labels := make(map[int]string, len(opts.Items))
	for _, item := range opts.Items {
		labels[item.ID] = item.Label
	}
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		label := ref.name
		if label == "" && ref.id > 0 {
			label = labels[ref.id]
		}
		if label != "" {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, ", ")
}

// historyVirtualSelectLabel maps a stored virtual select value to its label.
func historyVirtualSelectLabel(optionsJSON, raw string) string {
	var options []struct {
		Value any    `json:"value"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
		return ""
	}
	raw = strings.TrimSpace(raw)
	for _, option := range options {
		if strings.TrimSpace(historyStringValue(option.Value)) == raw {
			return option.Label
		}
	}
	return ""
}

func historyStringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}
