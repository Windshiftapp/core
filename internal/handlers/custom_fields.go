package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type CustomFieldHandler struct {
	db             database.Database
	repo           *repository.CustomFieldRepository
	linkTypeRepo   *repository.LinkTypeRepository
	systemSettings *repository.SystemSettingRepository
}

type assetTypeUsage struct {
	AssetTypeName string `json:"asset_type_name"`
	SetName       string `json:"set_name"`
}

type customFieldWithUsage struct {
	models.CustomFieldDefinition
	AssetTypeUsages []assetTypeUsage             `json:"asset_type_usages"`
	Indexed         *models.CustomFieldIndexInfo `json:"indexed,omitempty"`
}

type indexCountInfo struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

type customFieldsResponse struct {
	Data        []customFieldWithUsage    `json:"data"`
	IndexCounts map[string]indexCountInfo `json:"index_counts"`
}

// indexable field types that benefit from B-tree indexes
var indexableFieldTypes = map[string]bool{
	"number": true,
	"date":   true,
	"text":   true,
}

// allowed target tables for indexing
var indexableTargetTables = map[string]bool{
	"items":  true,
	"assets": true,
}

const (
	maxIndexesSettingKey = "max_custom_field_indexes_per_table"
	defaultMaxIndexes    = 20
)

// logAndRespondDatabaseError logs database errors and responds with a generic message
func (h *CustomFieldHandler) logAndRespondDatabaseError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("database error in custom field handler", slog.String("component", "custom_fields"), slog.Any("error", err))
	respondInternalError(w, r, err)
}

func NewCustomFieldHandler(db database.Database) *CustomFieldHandler {
	return &CustomFieldHandler{
		db:             db,
		repo:           repository.NewCustomFieldRepository(db),
		linkTypeRepo:   repository.NewLinkTypeRepository(db),
		systemSettings: repository.NewSystemSettingRepository(db),
	}
}

func (h *CustomFieldHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	customFields, err := h.repo.List()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}

	// Load asset type usages for all custom fields
	usageRows, err := h.repo.ListAssetTypeUsages()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	assetTypeUsages := make(map[int][]assetTypeUsage)
	for _, row := range usageRows {
		assetTypeUsages[row.CustomFieldID] = append(assetTypeUsages[row.CustomFieldID], assetTypeUsage{
			AssetTypeName: row.AssetTypeName,
			SetName:       row.SetName,
		})
	}

	// Load index info for all custom fields
	indexRows, err := h.repo.ListIndexes()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	fieldIndexes := make(map[int]*models.CustomFieldIndexInfo)
	indexCounts := map[string]int{"items": 0, "assets": 0}
	for _, row := range indexRows {
		if fieldIndexes[row.CustomFieldID] == nil {
			fieldIndexes[row.CustomFieldID] = &models.CustomFieldIndexInfo{}
		}
		switch row.TargetTable {
		case "items":
			fieldIndexes[row.CustomFieldID].Items = true
			indexCounts["items"]++
		case "assets":
			fieldIndexes[row.CustomFieldID].Assets = true
			indexCounts["assets"]++
		}
	}

	maxIndexes := h.maxIndexesPerTable()

	// Wrap each field with its asset type usages and index info
	result := make([]customFieldWithUsage, len(customFields))
	for i, cf := range customFields {
		usages := assetTypeUsages[cf.ID]
		if usages == nil {
			usages = []assetTypeUsage{}
		}
		entry := customFieldWithUsage{
			CustomFieldDefinition: cf,
			AssetTypeUsages:       usages,
		}
		if idx, ok := fieldIndexes[cf.ID]; ok {
			entry.Indexed = idx
		} else if indexableFieldTypes[cf.FieldType] {
			entry.Indexed = &models.CustomFieldIndexInfo{}
		}
		result[i] = entry
	}

	respondJSONOK(w, customFieldsResponse{
		Data: result,
		IndexCounts: map[string]indexCountInfo{
			"items":  {Current: indexCounts["items"], Max: maxIndexes},
			"assets": {Current: indexCounts["assets"], Max: maxIndexes},
		},
	})
}

func (h *CustomFieldHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	cf, err := h.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "custom_field")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, cf)
}

// validateAndNormalizeCustomField runs the name + field-type + per-type option
// validation, select/multiselect normalization, and XSS sanitization shared by
// Create and Update. It returns the linking-field options parsed during
// validation (nil unless FieldType == "linking") and false if an HTTP error was
// already written to w.
func (h *CustomFieldHandler) validateAndNormalizeCustomField(w http.ResponseWriter, r *http.Request, cf *models.CustomFieldDefinition) (*linkingFieldOptions, bool) {
	if strings.TrimSpace(cf.Name) == "" {
		respondValidationError(w, r, "Field name is required")
		return nil, false
	}
	if !isValidFieldType(cf.FieldType) {
		respondValidationError(w, r, "Invalid field type")
		return nil, false
	}

	var linkingOpts *linkingFieldOptions
	if cf.FieldType == "linking" {
		var linkErr error
		linkingOpts, linkErr = h.validateLinkingOptions(cf.Options)
		if linkErr != nil {
			respondValidationError(w, r, linkErr.Error())
			return nil, false
		}
	}

	if cf.FieldType == "asset" {
		if err := validateAssetFieldOptions(cf.Options); err != nil {
			respondValidationError(w, r, err.Error())
			return nil, false
		}
	}

	if (cf.FieldType == "select" || cf.FieldType == "multiselect") && cf.Options != "" {
		normalized, vErr := normalizeSelectOptions(cf.Options)
		if vErr != nil {
			if vErr.validation {
				respondValidationError(w, r, vErr.msg)
			} else {
				respondInternalError(w, r, errors.New(vErr.msg))
			}
			return nil, false
		}
		cf.Options = normalized
	}

	// Sanitize user input to prevent XSS.
	cf.Name = utils.SanitizeName(cf.Name)
	cf.Description = utils.SanitizeCommentContent(cf.Description)

	return linkingOpts, true
}

func (h *CustomFieldHandler) Create(w http.ResponseWriter, r *http.Request) {
	cf, ok := decodeJSON[models.CustomFieldDefinition](w, r)
	if !ok {
		return
	}

	linkingOpts, ok := h.validateAndNormalizeCustomField(w, r, &cf)
	if !ok {
		return
	}

	now := time.Now()
	id, err := h.repo.Create(&cf, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Auto-create mirror field for linking fields if mirror_name is provided
	if cf.FieldType == "linking" && linkingOpts != nil && linkingOpts.MirrorName != "" {
		mirrorID, mirrorErr := h.createMirrorField(int(id), linkingOpts, now)
		if mirrorErr != nil {
			respondInternalError(w, r, mirrorErr)
			return
		}
		var primaryOpts map[string]interface{}
		if err := json.Unmarshal([]byte(cf.Options), &primaryOpts); err == nil {
			delete(primaryOpts, "mirror_name")
			delete(primaryOpts, "mirror_allowed_item_type_ids")
			primaryOpts["mirror_field_id"] = mirrorID
			if updatedJSON, err := json.Marshal(primaryOpts); err == nil {
				_ = h.repo.UpdateOptions(id, string(updatedJSON))
			}
		}
	}

	createdCF, err := h.repo.FindByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldCreate,
			ResourceType: logger.ResourceCustomField,
			ResourceID:   &createdCF.ID,
			ResourceName: createdCF.Name,
			Details: map[string]interface{}{
				"field_type":    createdCF.FieldType,
				"required":      createdCF.Required,
				"display_order": createdCF.DisplayOrder,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, createdCF)
}

// updateRequest extends the custom field definition with optional indexing control
type updateRequest struct {
	models.CustomFieldDefinition
	Indexed *models.CustomFieldIndexInfo `json:"indexed,omitempty"`
}

func (h *CustomFieldHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	oldCF, err := h.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "custom_field")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	req, ok := decodeJSON[updateRequest](w, r)
	if !ok {
		return
	}

	cf := req.CustomFieldDefinition

	if _, ok := h.validateAndNormalizeCustomField(w, r, &cf); !ok {
		return
	}

	now := time.Now()
	if err := h.repo.Update(id, &cf, now); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clean up item/asset values when select/multiselect options are removed
	if cf.FieldType == "select" || cf.FieldType == "multiselect" {
		h.cleanupRemovedOptions(id, oldCF.Options, cf.Options, cf.FieldType)
	}

	// Handle indexing changes if provided
	if req.Indexed != nil {
		if !indexableFieldTypes[oldCF.FieldType] {
			respondValidationError(w, r, fmt.Sprintf("Field type '%s' cannot be indexed. Only number, date, and text fields support indexing.", oldCF.FieldType))
			return
		}

		for _, table := range []struct {
			name   string
			wanted bool
		}{
			{"items", req.Indexed.Items},
			{"assets", req.Indexed.Assets},
		} {
			if err := h.manageFieldIndex(id, oldCF.FieldType, table.name, table.wanted); err != nil {
				if strings.Contains(err.Error(), "index limit") {
					respondBadRequest(w, r, err.Error())
					return
				}
				respondInternalError(w, r, err)
				return
			}
		}
	}

	updatedCF, err := h.repo.FindByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		if oldCF.Name != updatedCF.Name {
			details["name_changed"] = map[string]interface{}{"old": oldCF.Name, "new": updatedCF.Name}
		}
		if oldCF.FieldType != updatedCF.FieldType {
			details["field_type_changed"] = map[string]interface{}{"old": oldCF.FieldType, "new": updatedCF.FieldType}
		}
		if oldCF.Required != updatedCF.Required {
			details["required_changed"] = map[string]interface{}{"old": oldCF.Required, "new": updatedCF.Required}
		}
		if oldCF.DisplayOrder != updatedCF.DisplayOrder {
			details["display_order_changed"] = map[string]interface{}{"old": oldCF.DisplayOrder, "new": updatedCF.DisplayOrder}
		}
		if oldCF.Options != updatedCF.Options {
			details["options_changed"] = map[string]interface{}{"old": oldCF.Options, "new": updatedCF.Options}
		}
		if req.Indexed != nil {
			details["indexed"] = req.Indexed
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldUpdate,
			ResourceType: logger.ResourceCustomField,
			ResourceID:   &updatedCF.ID,
			ResourceName: updatedCF.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOK(w, updatedCF)
}

func (h *CustomFieldHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	info, err := h.repo.FindDeleteInfo(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "custom_field")
			return
		}
		h.logAndRespondDatabaseError(w, r, err)
		return
	}

	if info.SystemDefault {
		respondForbidden(w, r)
		return
	}

	// Handle linking field cascade: delete mirror or clear mirror_field_id from primary
	if info.FieldType == "linking" {
		h.handleLinkingFieldDelete(id)
	}

	// Drop any database indexes before deleting the field
	indexNames, err := h.repo.ListIndexNamesForField(id)
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}

	for _, indexName := range indexNames {
		dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
		if err := h.repo.ExecDDL(dropSQL); err != nil {
			slog.Warn("failed to drop index during field deletion", slog.String("component", "custom_fields"), slog.String("index", indexName), slog.Any("error", err))
		}
	}

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldDelete,
			ResourceType: logger.ResourceCustomField,
			ResourceID:   &id,
			ResourceName: info.Name,
			Details: map[string]interface{}{
				"field_type": info.FieldType,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// manageFieldIndex creates or drops a database index for a custom field on a target table.
func (h *CustomFieldHandler) manageFieldIndex(fieldID int, fieldType, targetTable string, enable bool) error {
	if !indexableTargetTables[targetTable] {
		return fmt.Errorf("invalid target table: %s", targetTable)
	}

	indexName := fmt.Sprintf("idx_cf_%s_%d", targetTable, fieldID)

	currentlyEnabled, err := h.repo.IsIndexRecorded(fieldID, targetTable)
	if err != nil {
		return fmt.Errorf("failed to check index state: %w", err)
	}

	if enable == currentlyEnabled {
		return nil
	}

	if enable {
		currentCount, err := h.repo.CountIndexesForTable(targetTable)
		if err != nil {
			return fmt.Errorf("failed to count indexes: %w", err)
		}

		maxIndexes := h.maxIndexesPerTable()

		if currentCount >= maxIndexes {
			return fmt.Errorf("index limit reached: %d of %d indexes used on %s", currentCount, maxIndexes, targetTable)
		}

		createSQL := h.buildCreateIndexSQL(fieldID, fieldType, targetTable, indexName)
		if err := h.repo.ExecDDL(createSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}

		if err := h.repo.RecordIndex(fieldID, targetTable, indexName); err != nil {
			// Attempt to drop the index we just created
			_ = h.repo.ExecDDL(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName))
			return fmt.Errorf("failed to record index: %w", err)
		}
	} else {
		if err := h.repo.ExecDDL(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)); err != nil {
			return fmt.Errorf("failed to drop index: %w", err)
		}
		if err := h.repo.DeleteIndexRecord(fieldID, targetTable); err != nil {
			return fmt.Errorf("failed to remove index record: %w", err)
		}
	}

	return nil
}

type customFieldSettings struct {
	MaxIndexesPerTable int `json:"max_indexes_per_table"`
}

func (h *CustomFieldHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	settings, ok := decodeJSON[customFieldSettings](w, r)
	if !ok {
		return
	}

	if settings.MaxIndexesPerTable < 1 || settings.MaxIndexesPerTable > 100 {
		respondValidationError(w, r, "Maximum indexes per table must be between 1 and 100")
		return
	}

	// Check that new limit is not below current usage for any table
	counts, err := h.repo.CountIndexesPerTable()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	for table, count := range counts {
		if count > settings.MaxIndexesPerTable {
			respondBadRequest(w, r, fmt.Sprintf("Cannot set limit to %d: %s table already has %d indexes", settings.MaxIndexesPerTable, table, count))
			return
		}
	}

	value := strconv.Itoa(settings.MaxIndexesPerTable)
	err = h.systemSettings.Upsert(
		maxIndexesSettingKey,
		value,
		"integer",
		"Maximum number of custom field indexes per table",
		"performance",
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldUpdate,
			ResourceType: logger.ResourceCustomField,
			ResourceName: "custom_field_settings",
			Details: map[string]interface{}{
				"max_indexes_per_table": settings.MaxIndexesPerTable,
			},
			Success: true,
		})
	}

	respondJSONOK(w, settings)
}

// maxIndexesPerTable returns the configured max (or the default on error).
func (h *CustomFieldHandler) maxIndexesPerTable() int {
	value, ok, err := h.systemSettings.GetValue(maxIndexesSettingKey)
	if err != nil || !ok {
		return defaultMaxIndexes
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return defaultMaxIndexes
	}
	return v
}

// buildCreateIndexSQL generates the CREATE INDEX SQL based on driver and field type.
func (h *CustomFieldHandler) buildCreateIndexSQL(fieldID int, fieldType, targetTable, indexName string) string {
	fieldIDStr := strconv.Itoa(fieldID)
	driver := h.repo.DriverName()

	if driver == "postgres" {
		switch fieldType {
		case "number":
			return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(%s->>'%s' AS NUMERIC))`,
				indexName, targetTable, "custom_field_values", fieldIDStr)
		case "text":
			return fmt.Sprintf(`CREATE INDEX %s ON %s((%s->>'%s'))`,
				indexName, targetTable, "custom_field_values", fieldIDStr)
		case "date":
			return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(%s->>'%s' AS TEXT))`,
				indexName, targetTable, "custom_field_values", fieldIDStr)
		}
	}

	// SQLite
	castType := "TEXT"
	if fieldType == "number" {
		castType = "NUMERIC"
	}
	return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(NULLIF(custom_field_values,'') ->> '$.\"%s\"' AS %s))`,
		indexName, targetTable, fieldIDStr, castType)
}

// linkingFieldOptions holds parsed options for linking custom fields
type linkingFieldOptions struct {
	LinkTypeID               int      `json:"link_type_id"`
	AllowedItemTypeIDs       []int    `json:"allowed_item_type_ids"`
	AllowedEntityTypes       []string `json:"allowed_entity_types"`
	Multi                    bool     `json:"multi"`
	MirrorName               string   `json:"mirror_name"`
	MirrorAllowedItemTypeIDs []int    `json:"mirror_allowed_item_type_ids"`
	MirrorOfFieldID          int      `json:"mirror_of_field_id"`
	MirrorFieldID            int      `json:"mirror_field_id"`
}

// validateLinkingOptions validates options for a linking field type
func (h *CustomFieldHandler) validateLinkingOptions(optionsJSON string) (*linkingFieldOptions, error) {
	if optionsJSON == "" {
		return nil, fmt.Errorf("linking fields require options with link_type_id")
	}
	var opts linkingFieldOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return nil, fmt.Errorf("invalid linking options format")
	}
	// Mirror fields store mirror_of_field_id instead of link_type_id at the top level
	if opts.MirrorOfFieldID > 0 {
		return &opts, nil
	}
	if opts.LinkTypeID == 0 {
		return nil, fmt.Errorf("linking fields require link_type_id in options")
	}
	// Validate link type exists and is active, and fetch its allowed_entity_types
	basic, err := h.linkTypeRepo.FindBasicByID(opts.LinkTypeID)
	if err != nil {
		return nil, fmt.Errorf("link type not found")
	}
	if !basic.Active {
		return nil, fmt.Errorf("link type is not active")
	}
	// Validate allowed entity types
	for _, et := range opts.AllowedEntityTypes {
		if et != "item" && et != "test_case" && et != "asset" {
			return nil, fmt.Errorf("invalid entity type: %s", et)
		}
	}
	// If the link type declares allowed_entity_types, validate the field's entity types are a subset
	if basic.AllowedEntityTypes != "" {
		var ltAllowed []string
		if err := json.Unmarshal([]byte(basic.AllowedEntityTypes), &ltAllowed); err == nil && len(ltAllowed) > 0 {
			allowedSet := make(map[string]bool, len(ltAllowed))
			for _, a := range ltAllowed {
				allowedSet[a] = true
			}
			for _, et := range opts.AllowedEntityTypes {
				if !allowedSet[et] {
					ltName, _ := h.linkTypeRepo.FindNameByID(opts.LinkTypeID)
					return nil, fmt.Errorf("link type '%s' only supports entity types: %s", ltName, strings.Join(ltAllowed, ", "))
				}
			}
		}
	}
	return &opts, nil
}

// createMirrorField creates a mirror linking field for the given primary field
func (h *CustomFieldHandler) createMirrorField(primaryID int, opts *linkingFieldOptions, now time.Time) (int64, error) {
	mirrorOpts := map[string]interface{}{
		"mirror_of_field_id": primaryID,
		"link_type_id":       opts.LinkTypeID,
		"multi":              opts.Multi,
	}
	if len(opts.MirrorAllowedItemTypeIDs) > 0 {
		mirrorOpts["allowed_item_type_ids"] = opts.MirrorAllowedItemTypeIDs
	}
	if len(opts.AllowedEntityTypes) > 0 {
		mirrorOpts["allowed_entity_types"] = opts.AllowedEntityTypes
	}

	mirrorOptsJSON, err := json.Marshal(mirrorOpts)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal mirror options: %w", err)
	}

	mirrorID, err := h.repo.CreateMirror(opts.MirrorName, string(mirrorOptsJSON), now)
	if err != nil {
		return 0, fmt.Errorf("failed to create mirror field: %w", err)
	}
	return mirrorID, nil
}

// handleLinkingFieldDelete handles cascade deletion of mirror fields when a linking field is deleted
func (h *CustomFieldHandler) handleLinkingFieldDelete(fieldID int) {
	optionsJSON, err := h.repo.FindOptions(fieldID)
	if err != nil || optionsJSON == "" {
		return
	}

	var opts linkingFieldOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return
	}

	if opts.MirrorFieldID > 0 {
		// This is a primary field - delete its mirror
		_ = h.repo.Delete(opts.MirrorFieldID)
	} else if opts.MirrorOfFieldID > 0 {
		// This is a mirror field - clear mirror_field_id from primary
		primaryOptsJSON, err := h.repo.FindOptions(opts.MirrorOfFieldID)
		if err != nil || primaryOptsJSON == "" {
			return
		}
		var primaryOpts map[string]interface{}
		if err := json.Unmarshal([]byte(primaryOptsJSON), &primaryOpts); err == nil {
			delete(primaryOpts, "mirror_field_id")
			if updatedJSON, err := json.Marshal(primaryOpts); err == nil {
				_ = h.repo.UpdateOptions(int64(opts.MirrorOfFieldID), string(updatedJSON))
			}
		}
	}
}

// cleanupRemovedOptions detects which option IDs were removed and cleans up references.
func (h *CustomFieldHandler) cleanupRemovedOptions(fieldID int, oldOptionsJSON, newOptionsJSON, fieldType string) {
	oldOpts, err := models.ParseSelectOptions(oldOptionsJSON)
	if err != nil {
		return
	}
	newOpts, err := models.ParseSelectOptions(newOptionsJSON)
	if err != nil {
		return
	}

	newIDs := make(map[int]bool, len(newOpts.Items))
	for _, item := range newOpts.Items {
		newIDs[item.ID] = true
	}

	removedIDs := make(map[int]bool)
	for _, item := range oldOpts.Items {
		if !newIDs[item.ID] {
			removedIDs[item.ID] = true
		}
	}

	if len(removedIDs) == 0 {
		return
	}

	h.cleanupDeletedOptionValues(fieldID, fieldType, removedIDs)
}

// cleanupDeletedOptionValues removes references to deleted option IDs from items, assets,
// and portal custom_field_values. Logs warnings on failure but does not propagate errors.
func (h *CustomFieldHandler) cleanupDeletedOptionValues(fieldID int, fieldType string, removedIDs map[int]bool) {
	fieldKey := strconv.Itoa(fieldID)

	for _, table := range []string{"items", "assets"} {
		h.cleanupTableOptionValues(table, fieldKey, fieldType, removedIDs)
	}

	h.cleanupPortalOptionValues(fieldID, fieldType, removedIDs)
}

// cleanupTableOptionValues cleans deleted option references from items or assets.
func (h *CustomFieldHandler) cleanupTableOptionValues(tableName, fieldKey, fieldType string, removedIDs map[int]bool) {
	rows, err := h.repo.ListRowsWithCustomFields(tableName)
	if err != nil {
		slog.Warn("cleanup: failed to query table", slog.String("table", tableName), slog.Any("error", err))
		return
	}

	type rowUpdate struct {
		id     int
		newVal string
	}
	var updates []rowUpdate

	for _, row := range rows {
		var cfv map[string]interface{}
		if err := json.Unmarshal([]byte(row.Value), &cfv); err != nil {
			continue
		}

		val, exists := cfv[fieldKey]
		if !exists {
			continue
		}

		changed := false
		if fieldType == "select" {
			if num, ok := val.(float64); ok && removedIDs[int(num)] {
				delete(cfv, fieldKey)
				changed = true
			}
		} else if fieldType == "multiselect" {
			if arr, ok := val.([]interface{}); ok {
				var filtered []interface{}
				for _, item := range arr {
					if num, ok := item.(float64); ok && removedIDs[int(num)] {
						changed = true
						continue
					}
					filtered = append(filtered, item)
				}
				if changed {
					if len(filtered) == 0 {
						delete(cfv, fieldKey)
					} else {
						cfv[fieldKey] = filtered
					}
				}
			}
		}

		if changed {
			b, err := json.Marshal(cfv)
			if err != nil {
				continue
			}
			updates = append(updates, rowUpdate{id: row.ID, newVal: string(b)})
		}
	}

	for _, u := range updates {
		if err := h.repo.UpdateRowCustomFields(tableName, u.id, u.newVal); err != nil {
			slog.Warn("cleanup: failed to update row", slog.String("table", tableName), slog.Int("id", u.id), slog.Any("error", err))
		}
	}

	if len(updates) > 0 {
		slog.Info("cleaned up deleted option references", slog.String("table", tableName), slog.Int("rows_updated", len(updates)))
	}
}

// cleanupPortalOptionValues cleans deleted option references from the portal custom_field_values table.
func (h *CustomFieldHandler) cleanupPortalOptionValues(fieldID int, fieldType string, removedIDs map[int]bool) {
	rows, err := h.repo.ListPortalCFVsForField(fieldID)
	if err != nil {
		return // Table may not exist
	}

	var deleteIDs []int
	type rowUpdate struct {
		id     int
		newVal string
	}
	var updates []rowUpdate

	for _, row := range rows {
		if fieldType == "select" {
			if numVal, err := strconv.Atoi(row.Value); err == nil && removedIDs[numVal] {
				deleteIDs = append(deleteIDs, row.ID)
			}
		} else if fieldType == "multiselect" {
			var ids []int
			if err := json.Unmarshal([]byte(row.Value), &ids); err != nil {
				continue
			}
			changed := false
			var filtered []int
			for _, optID := range ids {
				if removedIDs[optID] {
					changed = true
					continue
				}
				filtered = append(filtered, optID)
			}
			if changed {
				if len(filtered) == 0 {
					deleteIDs = append(deleteIDs, row.ID)
				} else {
					b, _ := json.Marshal(filtered)
					updates = append(updates, rowUpdate{id: row.ID, newVal: string(b)})
				}
			}
		}
	}

	for _, id := range deleteIDs {
		if err := h.repo.DeletePortalCFV(id); err != nil {
			slog.Warn("cleanup: failed to delete portal custom field value", slog.Int("id", id), slog.Any("error", err))
		}
	}

	for _, u := range updates {
		if err := h.repo.UpdatePortalCFV(u.id, u.newVal); err != nil {
			slog.Warn("cleanup: failed to update portal custom field value", slog.Int("id", u.id), slog.Any("error", err))
		}
	}
}

// --- small helpers extracted to keep Create/Update flows readable ----------

var validFieldTypes = map[string]bool{
	"text":                 true,
	"textarea":             true,
	"select":               true,
	"multiselect":          true,
	"number":               true,
	"milestone":            true,
	"date":                 true,
	"user":                 true,
	"iteration":            true,
	"asset":                true,
	"portalcustomer":       true,
	"customerorganisation": true, //nolint:misspell // matches database column
	"linking":              true,
}

func isValidFieldType(t string) bool {
	return validFieldTypes[t]
}

func validateAssetFieldOptions(optionsJSON string) error {
	if optionsJSON == "" {
		return fmt.Errorf("asset fields require asset_set_id in options")
	}
	var config struct {
		AssetSetID int    `json:"asset_set_id"`
		QLQuery    string `json:"ql_query"`
	}
	if err := json.Unmarshal([]byte(optionsJSON), &config); err != nil || config.AssetSetID == 0 {
		return fmt.Errorf("asset fields require asset_set_id in options")
	}
	return nil
}

// selectValidationError distinguishes validation-failed vs. serialization-
// failed outcomes for select/multiselect option normalization.
type selectValidationError struct {
	validation bool
	msg        string
}

func normalizeSelectOptions(optionsJSON string) (string, *selectValidationError) {
	opts, parseErr := models.ParseSelectOptions(optionsJSON)
	if parseErr != nil {
		return "", &selectValidationError{validation: true, msg: "Invalid options format"}
	}
	if len(opts.Items) == 0 {
		return "", &selectValidationError{validation: true, msg: "Select fields must have at least one option"}
	}
	normalized, serErr := models.SerializeSelectOptions(opts)
	if serErr != nil {
		return "", &selectValidationError{validation: false, msg: serErr.Error()}
	}
	return normalized, nil
}
