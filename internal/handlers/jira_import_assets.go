package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

const jiraAssetsPageSize = 100

func (h *JiraImportHandler) importJiraAssets(ctx context.Context, jobID string, client jira.Client, createdByUserID int) {
	schemas, err := client.ListObjectSchemas(ctx)
	if err != nil {
		if errors.Is(err, jira.ErrAssetsNotAvailable) {
			slog.Debug("Jira Assets API not available; skipping Assets import", slog.String("component", "jira"))
			// Record a durable finding so the result does not imply native asset
			// resolution when no inventory could be read.
			_ = h.recordMapping(jobID, "fidelity_finding", "assets-capability",
				"Jira Assets/Insight object reads are not available for this connection; asset custom fields keep their original reference identifiers and no native asset inventory was imported.",
				0, map[string]any{
					"code":           "jira_assets_unavailable",
					"severity":       "warning",
					"disposition":    "unavailable",
					"source_count":   0,
					"resolved_count": 0,
				})
			return
		}
		slog.Warn("Failed to list Jira Assets schemas", slog.String("component", "jira"), slog.Any("error", err))
		return
	}

	setNames := jiraAssetSetNames(schemas)
	var pendingReferences []jiraPendingAssetReference
	for _, schema := range schemas {
		setID, ok := h.ensureJiraAssetSet(jobID, schema, setNames[string(schema.ID)], createdByUserID)
		if !ok {
			continue
		}
		typeMap := h.ensureJiraAssetTypes(ctx, jobID, client, setID, schema)
		for objectTypeID, importedType := range typeMap {
			pendingReferences = append(pendingReferences, h.importJiraAssetObjectsForType(
				ctx,
				jobID,
				client,
				setID,
				importedType.AssetTypeID,
				importedType.CategoryID,
				string(schema.ID),
				objectTypeID,
				importedType.Attributes,
				importedType.AttributeFieldIDs,
			)...)
		}
	}
	h.resolveJiraAssetReferences(jobID, pendingReferences)
}

func jiraAssetSetNames(schemas []jira.AssetObjectSchema) map[string]string {
	baseNameCounts := make(map[string]int, len(schemas))
	for _, schema := range schemas {
		baseNameCounts[jiraAssetSchemaBaseName(schema)]++
	}

	names := make(map[string]string, len(schemas))
	usedNames := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		baseName := jiraAssetSchemaBaseName(schema)
		if baseNameCounts[baseName] > 1 {
			identity := strings.TrimSpace(schema.ObjectSchemaKey)
			if identity == "" {
				identity = string(schema.ID)
			}
			baseName += " (" + identity + ")"
		}
		name := "Jira Assets: " + baseName
		if _, exists := usedNames[name]; exists {
			name += " [" + string(schema.ID) + "]"
		}
		usedNames[name] = struct{}{}
		names[string(schema.ID)] = name
	}
	return names
}

func jiraAssetSchemaBaseName(schema jira.AssetObjectSchema) string {
	if name := strings.TrimSpace(schema.Name); name != "" {
		return name
	}
	if key := strings.TrimSpace(schema.ObjectSchemaKey); key != "" {
		return key
	}
	return "Jira Assets " + string(schema.ID)
}

func (h *JiraImportHandler) ensureJiraAssetSet(jobID string, schema jira.AssetObjectSchema, name string, createdByUserID int) (int, bool) {
	if strings.TrimSpace(name) == "" {
		name = "Jira Assets: " + jiraAssetSchemaBaseName(schema)
	}

	setID, created, err := h.imports.EnsureAssetSet(
		name,
		strings.TrimSpace(schema.Description),
		createdByUserID,
	)
	if err != nil {
		slog.Warn("Failed to ensure Jira asset set", slog.String("component", "jira"), slog.String("schemaID", string(schema.ID)), slog.Any("error", err))
		return 0, false
	}

	if err := h.recordMapping(jobID, "asset_set", string(schema.ID), schema.ObjectSchemaKey, setID, map[string]any{
		"schema_name":  schema.Name,
		"object_count": schema.ObjectCount,
		"action":       map[bool]string{true: "create", false: "reuse_existing"}[created],
	}); err != nil {
		return 0, false
	}
	return setID, true
}

func (h *JiraImportHandler) ensureJiraAssetDefaultStatus(setID int) int {
	statusID, err := h.imports.EnsureAssetDefaultStatus(setID)
	if err != nil {
		slog.Warn("Failed to create default asset status", slog.String("component", "jira"), slog.Int("setID", setID), slog.Any("error", err))
		return 0
	}
	return statusID
}

type jiraAssetTypeImport struct {
	AssetTypeID       int
	CategoryID        int
	Attributes        map[string]jira.AssetObjectAttribute
	AttributeFieldIDs map[string]int
}

type jiraPendingAssetReference struct {
	AssetID     int
	SetID       int
	FieldID     int
	AttributeID string
	Multiple    bool
	Values      []jira.AssetAttributeValue
}

type jiraIssueAssetReference struct {
	AssetID  int
	SetID    int
	Title    string
	AssetTag string
}

func (h *JiraImportHandler) importedJiraAssetSetID(jobID, schemaID string) (int, bool) {
	return h.imports.MappedEntity(jobID, "asset_set", strings.TrimSpace(schemaID))
}

func (h *JiraImportHandler) resolveJiraIssueAssetReferences(jobID string, value any) []jiraIssueAssetReference {
	candidates := jiraIssueAssetCandidates(value)
	if len(candidates) == 0 {
		return nil
	}
	refs := make([]jiraIssueAssetReference, 0, len(candidates))
	seen := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		ref, ok := h.resolveJiraIssueAssetReference(jobID, candidate)
		if !ok {
			continue
		}
		if _, exists := seen[ref.AssetID]; exists {
			continue
		}
		seen[ref.AssetID] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

type jiraIssueAssetCandidate struct {
	ID    string
	Key   string
	Label string
}

func jiraIssueAssetCandidates(value any) []jiraIssueAssetCandidate {
	var candidates []jiraIssueAssetCandidate
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, entry := range x {
				walk(entry)
			}
		case map[string]any:
			candidate := jiraIssueAssetCandidate{
				// Cloud issue payloads include both id="<workspace UUID>:<id>"
				// and objectId="<id>". Asset import mappings use the latter.
				ID:    firstStringKey(x, "objectId", "objectID", "id"),
				Key:   firstStringKey(x, "objectKey", "key", "globalId"),
				Label: firstStringKey(x, "label", "name", "displayValue", "value"),
			}
			if candidate.ID != "" || candidate.Key != "" {
				candidates = append(candidates, candidate)
				return
			}
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(value)
	return candidates
}

func (h *JiraImportHandler) resolveJiraIssueAssetReference(jobID string, candidate jiraIssueAssetCandidate) (jiraIssueAssetReference, bool) {
	if candidate.ID == "" && candidate.Key == "" {
		return jiraIssueAssetReference{}, false
	}

	imported, ok := h.imports.AssetReference(jobID, candidate.ID, candidate.Key)
	if !ok {
		return jiraIssueAssetReference{}, false
	}
	ref := jiraIssueAssetReference{
		AssetID:  imported.ID,
		SetID:    imported.SetID,
		Title:    imported.Title,
		AssetTag: imported.AssetTag,
	}
	if strings.TrimSpace(ref.Title) == "" {
		ref.Title = candidate.Label
	}
	return ref, ref.AssetID > 0
}

func (h *JiraImportHandler) ensureJiraAssetTypes(ctx context.Context, jobID string, client jira.Client, setID int, schema jira.AssetObjectSchema) map[string]jiraAssetTypeImport {
	result := make(map[string]jiraAssetTypeImport)
	objectTypes, err := client.ListObjectTypes(ctx, string(schema.ID))
	if err != nil {
		slog.Warn("Failed to list Jira asset object types", slog.String("component", "jira"), slog.String("schemaID", string(schema.ID)), slog.Any("error", err))
		return result
	}

	categoryIDs := h.ensureJiraAssetTypeCategories(jobID, setID, objectTypes)
	for _, objectType := range objectTypes {
		if objectType.AbstractObjectType {
			continue
		}
		assetTypeID, ok := h.ensureJiraAssetType(jobID, setID, objectType)
		if !ok {
			continue
		}
		attrFieldIDs := make(map[string]int)
		attrsByID := make(map[string]jira.AssetObjectAttribute)

		attrs, err := client.GetObjectTypeAttributes(ctx, string(objectType.ID))
		if err != nil {
			slog.Warn("Failed to load Jira asset object type attributes", slog.String("component", "jira"), slog.String("objectTypeID", string(objectType.ID)), slog.Any("error", err))
			continue
		}
		for _, attr := range attrs {
			attrsByID[string(attr.ID)] = attr
			fieldID, ok := h.ensureJiraAssetAttributeField(jobID, setID, objectType, attr)
			if ok {
				h.linkJiraAssetTypeField(assetTypeID, fieldID, attr)
				attrFieldIDs[string(attr.ID)] = fieldID
			}
		}
		result[string(objectType.ID)] = jiraAssetTypeImport{
			AssetTypeID:       assetTypeID,
			CategoryID:        categoryIDs[string(objectType.ID)],
			Attributes:        attrsByID,
			AttributeFieldIDs: attrFieldIDs,
		}
	}
	return result
}

func (h *JiraImportHandler) ensureJiraAssetTypeCategories(
	jobID string,
	setID int,
	objectTypes []jira.AssetObjectType,
) map[string]int {
	result := make(map[string]int, len(objectTypes))
	pending := append([]jira.AssetObjectType{}, objectTypes...)
	for len(pending) > 0 {
		next := make([]jira.AssetObjectType, 0, len(pending))
		progress := false
		for _, objectType := range pending {
			parentID := 0
			if string(objectType.ParentObjectTypeID) != "" {
				var parentReady bool
				parentID, parentReady = result[string(objectType.ParentObjectTypeID)]
				if !parentReady {
					next = append(next, objectType)
					continue
				}
			}
			categoryID, ok := h.ensureJiraAssetTypeCategory(jobID, setID, parentID, objectType)
			if ok {
				result[string(objectType.ID)] = categoryID
			}
			progress = true
		}
		if progress {
			pending = next
			continue
		}
		// A missing or cyclic Jira parent must not drop the remaining object
		// types. Preserve them as roots and retain the unresolved parent ID in
		// mapping metadata for the fidelity report.
		for _, objectType := range next {
			categoryID, ok := h.ensureJiraAssetTypeCategory(jobID, setID, 0, objectType)
			if ok {
				result[string(objectType.ID)] = categoryID
			}
		}
		break
	}
	return result
}

func (h *JiraImportHandler) ensureJiraAssetTypeCategory(
	jobID string,
	setID, parentID int,
	objectType jira.AssetObjectType,
) (int, bool) {
	name := strings.TrimSpace(objectType.Name)
	if name == "" {
		name = "Jira Object Type " + string(objectType.ID)
	}
	categoryID, created, err := h.imports.EnsureAssetCategory(
		setID, parentID, name, strings.TrimSpace(objectType.Description),
	)
	if err != nil {
		slog.Warn("Failed to ensure Jira asset type category",
			slog.String("component", "jira"),
			slog.String("objectTypeID", string(objectType.ID)),
			slog.Any("error", err))
		return 0, false
	}
	if err := h.recordMapping(jobID, "asset_category", string(objectType.ID), name, categoryID, map[string]any{
		"asset_set_id":               setID,
		"jira_parent_object_type_id": string(objectType.ParentObjectTypeID),
		"abstract":                   objectType.AbstractObjectType,
		"action":                     map[bool]string{true: "create", false: "reuse_existing"}[created],
	}); err != nil {
		return 0, false
	}
	return categoryID, true
}

func (h *JiraImportHandler) ensureJiraAssetType(jobID string, setID int, objectType jira.AssetObjectType) (int, bool) {
	name := strings.TrimSpace(objectType.Name)
	if name == "" {
		name = "Jira Object Type " + string(objectType.ID)
	}
	typeID, created, err := h.imports.EnsureAssetType(
		setID,
		name,
		strings.TrimSpace(objectType.Description),
		objectType.Position,
	)
	if err != nil {
		slog.Warn("Failed to ensure Jira asset type", slog.String("component", "jira"), slog.String("objectTypeID", string(objectType.ID)), slog.Any("error", err))
		return 0, false
	}
	action := "reuse_existing"
	if created {
		action = "create"
	}
	if err := h.recordMapping(jobID, "asset_type", string(objectType.ID), name, typeID, map[string]any{"asset_set_id": setID, "action": action}); err != nil {
		return 0, false
	}
	return typeID, true
}

func (h *JiraImportHandler) ensureJiraAssetAttributeField(jobID string, setID int, objectType jira.AssetObjectType, attr jira.AssetObjectAttribute) (int, bool) {
	name := strings.TrimSpace(attr.Name)
	if name == "" || attr.Hidden {
		return 0, false
	}
	fieldName := fmt.Sprintf("Jira Assets %s: %s", strings.TrimSpace(objectType.Name), name)
	fieldType := jiraAssetAttributeFieldType(attr)
	options := ""
	if fieldType == "asset" {
		optionsBytes, _ := json.Marshal(map[string]any{
			"asset_set_id": setID,
			"multi":        attr.MaximumCardinality != 1,
		})
		options = string(optionsBytes)
	}

	fieldID, created, err := h.imports.EnsureAssetAttributeField(
		fieldName,
		fieldType,
		strings.TrimSpace(attr.Description),
		options,
		attr.MinimumCardinality > 0,
		attr.Position,
	)
	if err != nil {
		slog.Warn("Failed to ensure Jira asset attribute field", slog.String("component", "jira"), slog.String("attributeID", string(attr.ID)), slog.Any("error", err))
		return 0, false
	}
	if err := h.recordMapping(jobID, "custom_field", string(attr.ID), fieldName, fieldID, map[string]any{
		"asset_attribute": true,
		"asset_set_id":    setID,
		"object_type_id":  string(objectType.ID),
		"jira_type":       attr.Type,
		"action":          map[bool]string{true: "create", false: "reuse_existing"}[created],
	}); err != nil {
		return 0, false
	}
	return fieldID, true
}

func (h *JiraImportHandler) linkJiraAssetTypeField(assetTypeID, fieldID int, attr jira.AssetObjectAttribute) {
	err := h.imports.LinkAssetTypeField(assetTypeID, fieldID, attr.MinimumCardinality > 0, attr.Position)
	if err != nil {
		slog.Warn("Failed to link Jira asset type field", slog.String("component", "jira"), slog.Int("assetTypeID", assetTypeID), slog.Int("fieldID", fieldID), slog.Any("error", err))
	}
}

func (h *JiraImportHandler) importJiraAssetObjectsForType(
	ctx context.Context,
	jobID string,
	client jira.Client,
	setID, assetTypeID, categoryID int,
	schemaID, objectTypeID string,
	attributes map[string]jira.AssetObjectAttribute,
	attributeFields map[string]int,
) []jiraPendingAssetReference {
	var pending []jiraPendingAssetReference
	for page := 1; ; page++ {
		result, err := client.SearchObjects(ctx, jira.ObjectSearchOptions{
			ObjectSchemaID:    schemaID,
			ObjectTypeID:      objectTypeID,
			Page:              page,
			PageSize:          jiraAssetsPageSize,
			IncludeAttributes: true,
		})
		if err != nil {
			slog.Warn("Failed to search Jira asset objects", slog.String("component", "jira"), slog.String("schemaID", schemaID), slog.String("objectTypeID", objectTypeID), slog.Int("page", page), slog.Any("error", err))
			return pending
		}
		if result == nil || len(result.ObjectEntries) == 0 {
			return pending
		}
		for _, object := range result.ObjectEntries {
			pending = append(pending, h.importJiraAssetObject(
				ctx,
				jobID,
				client,
				setID,
				assetTypeID,
				categoryID,
				attributes,
				attributeFields,
				object,
			)...)
		}
		if result.IsLast || len(result.ObjectEntries) < jiraAssetsPageSize {
			return pending
		}
	}
}

func (h *JiraImportHandler) importJiraAssetObject(
	ctx context.Context,
	jobID string,
	client jira.Client,
	setID, assetTypeID, categoryID int,
	attributes map[string]jira.AssetObjectAttribute,
	attributeFields map[string]int,
	object jira.AssetObject,
) []jiraPendingAssetReference {
	if string(object.ID) == "" {
		return nil
	}
	if existingID := h.existingImportedJiraAsset(jobID, string(object.ID)); existingID > 0 {
		if err := h.recordMapping(jobID, "asset", string(object.ID), object.ObjectKey, existingID, map[string]any{"action": "reuse_existing_mapping"}); err != nil {
			return nil
		}
		return nil
	}

	customValues := make(map[string]any)
	statusID := h.ensureJiraAssetDefaultStatus(setID)
	userMap := h.ensureJiraAssetAttributeUsers(ctx, jobID, client, object)
	var pendingAttributes []jiraPendingAssetReference
	for _, attr := range object.Attributes {
		fieldID := attributeFields[string(attr.ObjectTypeAttributeID)]
		if fieldID == 0 {
			continue
		}
		definition := attributes[string(attr.ObjectTypeAttributeID)]
		for _, raw := range attr.ObjectAttributeValues {
			if raw.Status != nil {
				if mappedStatusID := h.ensureJiraAssetStatus(jobID, setID, *raw.Status); mappedStatusID > 0 {
					statusID = mappedStatusID
				}
				break
			}
		}
		switch jiraAssetAttributeFieldType(definition) {
		case "asset":
			if rawValue, ok := jiraAssetAttributeValue(attr); ok {
				customValues["_jira_asset_attribute_"+string(attr.ObjectTypeAttributeID)] = rawValue
			}
			pendingAttributes = append(pendingAttributes, jiraPendingAssetReference{
				SetID:       setID,
				FieldID:     fieldID,
				AttributeID: string(attr.ObjectTypeAttributeID),
				Multiple:    definition.MaximumCardinality != 1,
				Values:      attr.ObjectAttributeValues,
			})
			continue
		case "user":
			if value, ok := jiraAssetUserAttributeValue(attr, userMap); ok {
				customValues[strconv.Itoa(fieldID)] = value
			} else if rawValue, rawOK := jiraAssetAttributeValue(attr); rawOK {
				customValues["_jira_asset_attribute_"+string(attr.ObjectTypeAttributeID)] = rawValue
			}
			continue
		case models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox:
			if value, ok := jiraAssetBooleanAttributeValue(attr); ok {
				customValues[strconv.Itoa(fieldID)] = value
			} else if rawValue, rawOK := jiraAssetAttributeValue(attr); rawOK {
				customValues["_jira_asset_attribute_"+string(attr.ObjectTypeAttributeID)] = rawValue
			}
			continue
		}
		if value, ok := jiraAssetAttributeValue(attr); ok {
			customValues[strconv.Itoa(fieldID)] = value
		}
	}

	// External Jira attribute display values are untrusted — every string
	// gets at least the rich-text strip + length cap, then the asset CF
	// text pass (the same one CreateAsset/UpdateAsset apply, WI-319) lays
	// the rendering-matched policy over text/textarea-typed fields. Both
	// passes are idempotent no-ops on plain text.
	for key, v := range customValues {
		if s, ok := v.(string); ok {
			customValues[key] = sanitize.RichText.Sanitize(s)
		}
	}
	if err := services.NewAssetService(h.db, repository.NewAssetRepository(h.db)).SanitizeCustomFieldTextValues(assetTypeID, customValues); err != nil {
		slog.Warn("Failed to sanitize Jira asset custom field values", slog.String("component", "jira"), slog.String("objectID", string(object.ID)), slog.Any("error", err))
		return nil
	}

	customJSON := "{}"
	if len(customValues) > 0 {
		if b, err := json.Marshal(customValues); err == nil {
			customJSON = string(b)
		}
	}

	// object.Label / ObjectKey are external display strings — apply the
	// same per-column policies sanitizeAssetText runs on the normal asset
	// create path (PlainTextField title, RichText description,
	// ShortIdentifier asset_tag) before they reach the INSERT.
	title := sanitize.PlainTextField.Sanitize(object.Label)
	if title == "" {
		title = sanitize.PlainTextField.Sanitize(object.ObjectKey)
	}
	if title == "" {
		title = sanitize.PlainTextField.Sanitize("Jira Asset " + string(object.ID))
	}
	assetTag := sanitize.ShortIdentifier.Sanitize(object.ObjectKey)
	description := sanitize.RichText.Sanitize(fmt.Sprintf("Imported from Jira Assets object %s", object.ObjectKey))

	createdAt := nullableAssetTime(time.Time(object.Created))
	updatedAt := nullableAssetTime(time.Time(object.Updated))
	var status *int
	if statusID > 0 {
		status = &statusID
	}
	assetID, err := h.imports.InsertAsset(repository.JiraImportAssetRowInput{
		SetID:                 setID,
		AssetTypeID:           assetTypeID,
		StatusID:              status,
		Title:                 title,
		Description:           description,
		AssetTag:              assetTag,
		CustomFieldValuesJSON: customJSON,
		ImportJobID:           jobID,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
	})
	if err != nil {
		slog.Warn("Failed to import Jira asset object", slog.String("component", "jira"), slog.String("objectID", string(object.ID)), slog.String("objectKey", object.ObjectKey), slog.Any("error", err))
		return nil
	}

	if err := h.recordMapping(jobID, "asset", string(object.ID), object.ObjectKey, assetID, map[string]any{
		"asset_set_id":  setID,
		"asset_type_id": assetTypeID,
		"category_id":   categoryID,
		"label":         object.Label,
	}); err != nil {
		return nil
	}
	for idx := range pendingAttributes {
		pendingAttributes[idx].AssetID = assetID
	}
	return pendingAttributes
}

func (h *JiraImportHandler) ensureJiraAssetAttributeUsers(
	ctx context.Context,
	jobID string,
	client jira.Client,
	object jira.AssetObject,
) map[string]int {
	var users []JiraUserSummary
	seen := make(map[string]bool)
	for _, attr := range object.Attributes {
		for _, raw := range attr.ObjectAttributeValues {
			addJiraUserSummaryFromUser(raw.User, nil, &users, seen)
		}
	}
	if len(users) == 0 {
		return nil
	}
	userMap, _, err := h.ensureUsers(ctx, jobID, users, client)
	if err != nil {
		slog.Warn("Failed to ensure Jira Assets user attributes",
			slog.String("component", "jira"),
			slog.String("objectID", string(object.ID)),
			slog.Any("error", err))
		return nil
	}
	return userMap
}

func jiraAssetUserAttributeValue(attr jira.AssetObjectAttributeValue, userMap map[string]int) (any, bool) {
	for _, raw := range attr.ObjectAttributeValues {
		if raw.User == nil {
			continue
		}
		if userID := userMap[raw.User.GetIdentifier()]; userID > 0 {
			return userID, true
		}
	}
	return nil, false
}

func (h *JiraImportHandler) ensureJiraAssetStatus(jobID string, setID int, status jira.AssetStatus) int {
	name := strings.TrimSpace(status.Name)
	if name == "" {
		return 0
	}
	statusID, created, err := h.imports.EnsureAssetStatus(
		setID, name, strings.TrimSpace(status.Description), status.Category,
	)
	if err != nil {
		slog.Warn("Failed to ensure Jira asset status",
			slog.String("component", "jira"),
			slog.String("jiraStatusID", string(status.ID)),
			slog.Any("error", err))
		return 0
	}
	jiraStatusID := string(status.ID)
	if jiraStatusID == "" {
		jiraStatusID = name
	}
	if err := h.recordMapping(jobID, "asset_status", jiraStatusID, name, statusID, map[string]any{
		"asset_set_id": setID,
		"category":     status.Category,
		"action":       map[bool]string{true: "create", false: "reuse_existing"}[created],
	}); err != nil {
		return 0
	}
	return statusID
}

func (h *JiraImportHandler) resolveJiraAssetReferences(
	jobID string,
	pending []jiraPendingAssetReference,
) {
	for _, reference := range pending {
		refs := make([]jiraIssueAssetReference, 0, len(reference.Values))
		seen := make(map[int]struct{}, len(reference.Values))
		for _, raw := range reference.Values {
			candidate := jiraAssetAttributeReferenceCandidate(raw)
			resolved, ok := h.resolveJiraIssueAssetReference(jobID, candidate)
			if !ok || resolved.SetID != reference.SetID {
				continue
			}
			if _, exists := seen[resolved.AssetID]; exists {
				continue
			}
			seen[resolved.AssetID] = struct{}{}
			refs = append(refs, resolved)
		}
		if len(refs) < len(reference.Values) {
			if err := h.recordMapping(
				jobID,
				"fidelity_finding",
				fmt.Sprintf("asset:%d:attribute:%s", reference.AssetID, reference.AttributeID),
				"Unresolved Jira Assets references",
				reference.AssetID,
				map[string]any{
					"code":           "jira_asset_reference_unresolved",
					"severity":       "warning",
					"disposition":    "preserved_raw",
					"source_count":   len(reference.Values),
					"resolved_count": len(refs),
					"asset_id":       reference.AssetID,
					"attribute_id":   reference.AttributeID,
					"was_created":    false,
				},
			); err != nil {
				return
			}
		}
		if len(refs) == 0 {
			continue
		}
		values, err := h.imports.AssetCustomFieldValues(reference.AssetID)
		if err != nil {
			continue
		}
		fieldKey := strconv.Itoa(reference.FieldID)
		if reference.Multiple {
			mapped := make([]map[string]any, 0, len(refs))
			for _, ref := range refs {
				mapped = append(mapped, assetCustomFieldValue(ref))
			}
			values[fieldKey] = mapped
		} else {
			values[fieldKey] = assetCustomFieldValue(refs[0])
		}
		if err := h.imports.UpdateAssetCustomFieldValues(reference.AssetID, values); err != nil {
			slog.Warn("Failed to resolve Jira Assets object reference",
				slog.String("component", "jira"),
				slog.Int("assetID", reference.AssetID),
				slog.String("attributeID", reference.AttributeID),
				slog.Any("error", err))
		}
	}
}

func jiraAssetAttributeReferenceCandidate(raw jira.AssetAttributeValue) jiraIssueAssetCandidate {
	candidate := jiraIssueAssetCandidate{
		Key:   strings.TrimSpace(raw.SearchValue),
		Label: strings.TrimSpace(raw.DisplayValue),
	}
	switch value := raw.Value.(type) {
	case map[string]any:
		candidate.ID = firstStringKey(value, "objectId", "objectID", "id")
		if key := firstStringKey(value, "objectKey", "key", "globalId"); key != "" {
			candidate.Key = key
		}
		if label := firstStringKey(value, "label", "name", "displayValue", "value"); label != "" {
			candidate.Label = label
		}
	case string:
		candidate.ID = strings.TrimSpace(value)
	case float64:
		candidate.ID = strconv.FormatInt(int64(value), 10)
	case int:
		candidate.ID = strconv.Itoa(value)
	case int64:
		candidate.ID = strconv.FormatInt(value, 10)
	}
	return candidate
}

func (h *JiraImportHandler) existingImportedJiraAsset(jobID, objectID string) int {
	if id, ok := h.imports.MappedEntity(jobID, "asset", objectID); ok {
		return id
	}
	return 0
}

func jiraAssetAttributeFieldType(attr jira.AssetObjectAttribute) string {
	switch attr.Type {
	case 1:
		return "asset"
	case 2:
		return "user"
	case 0:
		// Continue below for Jira's built-in scalar types.
	default:
		return "textarea"
	}
	switch attr.DefaultTypeID {
	case 1, 3:
		return "number"
	case 2:
		return models.CustomFieldTypeBoolean
	case 4, 5:
		return "date"
	default:
		return "textarea"
	}
}

func jiraAssetBooleanAttributeValue(attr jira.AssetObjectAttributeValue) (value, ok bool) {
	for _, raw := range attr.ObjectAttributeValues {
		for _, candidate := range []any{raw.Value, raw.DisplayValue, raw.SearchValue} {
			if value, ok := jiraCheckboxValue(candidate); ok {
				return value, true
			}
		}
	}
	return false, false
}

func jiraAssetAttributeValue(attr jira.AssetObjectAttributeValue) (any, bool) {
	if len(attr.ObjectAttributeValues) == 0 {
		return nil, false
	}
	values := make([]string, 0, len(attr.ObjectAttributeValues))
	for _, raw := range attr.ObjectAttributeValues {
		if raw.DisplayValue != "" {
			values = append(values, raw.DisplayValue)
			continue
		}
		if raw.SearchValue != "" {
			values = append(values, raw.SearchValue)
			continue
		}
		if raw.User != nil && raw.User.DisplayName != "" {
			values = append(values, raw.User.DisplayName)
			continue
		}
		if raw.Status != nil && raw.Status.Name != "" {
			values = append(values, raw.Status.Name)
			continue
		}
		if raw.Value != nil {
			values = append(values, fmt.Sprint(raw.Value))
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	return strings.Join(values, "\n"), true
}

func nullableAssetTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
