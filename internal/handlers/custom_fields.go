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
	"windshift/internal/sanitize"
	"windshift/internal/scheduler"
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

var errCustomFieldIndexLimit = errors.New("custom field index limit reached")

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
	fields, err := h.repo.List()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	usages, err := h.repo.ListAssetTypeUsages()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	usageByField := make(map[int][]assetTypeUsage)
	for _, usage := range usages {
		usageByField[usage.CustomFieldID] = append(usageByField[usage.CustomFieldID], assetTypeUsage{
			AssetTypeName: usage.AssetTypeName, SetName: usage.SetName,
		})
	}
	indexes, err := h.repo.ListIndexes()
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	indexByField := make(map[int]*models.CustomFieldIndexInfo)
	counts := map[string]int{"items": 0, "assets": 0}
	for _, index := range indexes {
		if indexByField[index.CustomFieldID] == nil {
			indexByField[index.CustomFieldID] = &models.CustomFieldIndexInfo{}
		}
		switch index.TargetTable {
		case "items":
			indexByField[index.CustomFieldID].Items = true
			counts["items"]++
		case "assets":
			indexByField[index.CustomFieldID].Assets = true
			counts["assets"]++
		}
	}
	result := make([]customFieldWithUsage, len(fields))
	for i, field := range fields {
		fieldUsages := usageByField[field.ID]
		if fieldUsages == nil {
			fieldUsages = []assetTypeUsage{}
		}
		result[i] = customFieldWithUsage{CustomFieldDefinition: field, AssetTypeUsages: fieldUsages}
		if indexed, ok := indexByField[field.ID]; ok {
			result[i].Indexed = indexed
		} else if indexableFieldTypes[field.FieldType] {
			result[i].Indexed = &models.CustomFieldIndexInfo{}
		}
	}
	limit := h.maxIndexesPerTable()
	respondJSONOK(w, customFieldsResponse{Data: result, IndexCounts: map[string]indexCountInfo{
		"items": {Current: counts["items"], Max: limit}, "assets": {Current: counts["assets"], Max: limit},
	}})
}

func (h *CustomFieldHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	field, err := h.repo.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "custom_field")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, field)
}

// validateAndNormalizeCustomField runs the name + field-type + per-type option
// validation, select/multiselect normalization, and XSS sanitization shared by
// Create and Update. It returns the linking-field options parsed during
// validation (nil unless FieldType == "linking") and false if an HTTP error was
// already written to w.
func (h *CustomFieldHandler) validateAndNormalizeCustomField(w http.ResponseWriter, r *http.Request, cf *models.CustomFieldDefinition) (*linkingFieldOptions, []string, bool) {
	if strings.TrimSpace(cf.Name) == "" {
		respondValidationError(w, r, "Field name is required")
		return nil, nil, false
	}
	cf.FieldType = models.CanonicalCustomFieldType(cf.FieldType)
	if !isValidFieldType(cf.FieldType) {
		respondValidationError(w, r, "Invalid field type")
		return nil, nil, false
	}

	var linkingOpts *linkingFieldOptions
	if cf.FieldType == "linking" {
		var linkErr error
		linkingOpts, linkErr = h.validateLinkingOptions(cf.Options)
		if linkErr != nil {
			respondValidationError(w, r, linkErr.Error())
			return nil, nil, false
		}
	}

	if cf.FieldType == "asset" {
		if err := validateAssetFieldOptions(cf.Options); err != nil {
			respondValidationError(w, r, err.Error())
			return nil, nil, false
		}
	}

	if models.IsBooleanCustomFieldType(cf.FieldType) && strings.TrimSpace(cf.Options) != "" {
		respondValidationError(w, r, "Checkbox fields do not support options")
		return nil, nil, false
	}

	if cf.FieldType == "select" || cf.FieldType == "multiselect" {
		normalized, vErr := normalizeSelectOptions(cf.Options)
		if vErr != nil {
			if vErr.validation {
				respondValidationError(w, r, vErr.msg)
			} else {
				respondInternalError(w, r, errors.New(vErr.msg))
			}
			return nil, nil, false
		}
		cf.Options = normalized
	}

	// Sanitize user input to prevent XSS. Name is identifier-shaped
	// (referenced by the screen builder + workspace config sets);
	// Description is admin-facing help text rendered in the field
	// directory.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &cf.Name, Policy: sanitize.ShortIdentifier, Label: "Name"},
		sanitize.Pair{Target: &cf.Description, Policy: sanitize.Comment, Label: "Description"},
	)

	return linkingOpts, warnings, true
}

func (h *CustomFieldHandler) Create(w http.ResponseWriter, r *http.Request) {
	cf, ok := decodeJSON[models.CustomFieldDefinition](w, r)
	if !ok {
		return
	}

	linkingOpts, warnings, ok := h.validateAndNormalizeCustomField(w, r, &cf)
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
		var primaryOpts map[string]any
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
			Details: map[string]any{
				"field_type":    createdCF.FieldType,
				"required":      createdCF.Required,
				"display_order": createdCF.DisplayOrder,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, struct {
		*models.CustomFieldDefinition
		Warnings []string `json:"warnings,omitempty"`
	}{createdCF, warnings})
}

// updateRequest extends the custom field definition with optional indexing control
type updateRequest struct {
	models.CustomFieldDefinition
	Indexed *models.CustomFieldIndexInfo `json:"indexed,omitempty"`
}

// updateResponse returns the updated custom field plus any index builds that
// were deferred. SQLite cannot build indexes concurrently, so enabling an index
// records the desired state and the physical index is created on next restart.
type updateResponse struct {
	models.CustomFieldDefinition
	IndexingDeferred *models.CustomFieldIndexInfo `json:"indexing_deferred,omitempty"`
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

	_, warnings, ok := h.validateAndNormalizeCustomField(w, r, &cf)
	if !ok {
		return
	}
	if models.CanonicalCustomFieldType(oldCF.FieldType) != cf.FieldType {
		respondValidationError(w, r, "Custom field type cannot be changed after creation")
		return
	}
	if req.Indexed != nil && !indexableFieldTypes[cf.FieldType] {
		respondValidationError(w, r, fmt.Sprintf("Field type '%s' cannot be indexed. Only number, date, and text fields support indexing.", cf.FieldType))
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

	deferredIndexes := &models.CustomFieldIndexInfo{}
	indexingDeferred := false

	// Handle indexing changes if provided
	if req.Indexed != nil {
		for _, table := range []struct {
			name   string
			wanted bool
		}{
			{"items", req.Indexed.Items},
			{"assets", req.Indexed.Assets},
		} {
			deferred, err := h.manageFieldIndex(id, oldCF.FieldType, table.name, table.wanted)
			if err != nil {
				if errors.Is(err, errCustomFieldIndexLimit) {
					respondBadRequest(w, r, err.Error())
					return
				}
				respondInternalError(w, r, err)
				return
			}
			if deferred {
				indexingDeferred = true
				switch table.name {
				case "items":
					deferredIndexes.Items = true
				case "assets":
					deferredIndexes.Assets = true
				}
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
		details := make(map[string]any)

		if oldCF.Name != updatedCF.Name {
			details["name_changed"] = map[string]any{"old": oldCF.Name, "new": updatedCF.Name}
		}
		if oldCF.FieldType != updatedCF.FieldType {
			details["field_type_changed"] = map[string]any{"old": oldCF.FieldType, "new": updatedCF.FieldType}
		}
		if oldCF.Required != updatedCF.Required {
			details["required_changed"] = map[string]any{"old": oldCF.Required, "new": updatedCF.Required}
		}
		if oldCF.DisplayOrder != updatedCF.DisplayOrder {
			details["display_order_changed"] = map[string]any{"old": oldCF.DisplayOrder, "new": updatedCF.DisplayOrder}
		}
		if oldCF.Options != updatedCF.Options {
			details["options_changed"] = map[string]any{"old": oldCF.Options, "new": updatedCF.Options}
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

	if indexingDeferred {
		respondJSONOK(w, struct {
			updateResponse
			Warnings []string `json:"warnings,omitempty"`
		}{updateResponse{
			CustomFieldDefinition: *updatedCF,
			IndexingDeferred:      deferredIndexes,
		}, warnings})
		return
	}

	respondJSONOK(w, struct {
		*models.CustomFieldDefinition
		Warnings []string `json:"warnings,omitempty"`
	}{updatedCF, warnings})
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

	deleteIDs := []int{id}
	var linkingOpts *linkingFieldOptions
	if info.FieldType == "linking" {
		linkingOpts, err = h.findLinkingDeleteOptions(id)
		if err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		if linkingOpts != nil && linkingOpts.MirrorFieldID > 0 {
			if _, err := h.repo.FindDeleteInfo(linkingOpts.MirrorFieldID); err == nil {
				deleteIDs = append([]int{linkingOpts.MirrorFieldID}, deleteIDs...)
			} else if !errors.Is(err, repository.ErrNotFound) {
				h.logAndRespondDatabaseError(w, r, err)
				return
			}
		}
	}

	// Check every field in a linking cascade before deleting either definition.
	// The asynchronous scrub remains defense-in-depth for concurrent writes.
	for _, fieldID := range deleteIDs {
		inUse, err := h.repo.CountRowsUsingField(fieldID)
		if err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		if inUse > 0 {
			respondConflict(w, r, fmt.Sprintf("Cannot delete custom field: it is used by %d record(s). Clear those values first.", inUse))
			return
		}
	}

	indexesByField := make(map[int][]string, len(deleteIDs))
	for _, fieldID := range deleteIDs {
		indexNames, err := h.repo.ListIndexNamesForField(fieldID)
		if err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		indexesByField[fieldID] = indexNames
	}

	if linkingOpts != nil && linkingOpts.MirrorOfFieldID > 0 {
		if err := h.clearPrimaryMirrorReference(linkingOpts.MirrorOfFieldID); err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
	}

	for _, fieldID := range deleteIDs {
		for _, indexName := range indexesByField[fieldID] {
			dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
			if err := h.repo.ExecDDL(dropSQL); err != nil {
				slog.Warn("failed to drop index during field deletion", slog.String("component", "custom_fields"), slog.String("index", indexName), slog.Any("error", err))
			}
		}

		if err := scheduler.CancelPendingIndexBuilds(h.db, fieldID); err != nil {
			slog.Warn("custom_fields: failed to cancel pending index builds",
				slog.Int("field_id", fieldID), slog.Any("error", err))
		}

		if err := h.repo.Delete(fieldID); err != nil {
			respondInternalError(w, r, err)
			return
		}

		if err := scheduler.EnqueueFieldCleanup(h.db, fieldID); err != nil {
			slog.Warn("custom_fields: failed to enqueue cfv cleanup job",
				slog.Int("field_id", fieldID), slog.Any("error", err))
		}
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
			Details: map[string]any{
				"field_type": info.FieldType,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// manageFieldIndex creates, drops, or schedules a database index for a custom
// field on a target table. It returns true when index creation was deferred.
func (h *CustomFieldHandler) manageFieldIndex(fieldID int, fieldType, targetTable string, enable bool) (bool, error) {
	if !indexableTargetTables[targetTable] {
		return false, fmt.Errorf("invalid target table: %s", targetTable)
	}

	indexName := fmt.Sprintf("idx_cf_%s_%d", targetTable, fieldID)

	currentlyEnabled, err := h.repo.IsIndexRecorded(fieldID, targetTable)
	if err != nil {
		return false, fmt.Errorf("failed to check index state: %w", err)
	}

	if enable == currentlyEnabled {
		return false, nil
	}

	if enable {
		currentCount, err := h.repo.CountIndexesForTable(targetTable)
		if err != nil {
			return false, fmt.Errorf("failed to count indexes: %w", err)
		}

		maxIndexes := h.maxIndexesPerTable()

		if currentCount >= maxIndexes {
			return false, fmt.Errorf("%w: %d of %d indexes used on %s", errCustomFieldIndexLimit, currentCount, maxIndexes, targetTable)
		}

		// Building the physical index is deferred off the request thread on both
		// drivers — on large item/asset tables a synchronous CREATE INDEX blocks
		// writes and ties up the admin request. SQLite cannot build concurrently,
		// so its recorded indexes are materialized at the next restart before the
		// server takes traffic. Postgres builds CONCURRENTLY via the
		// CFVCleanupScheduler. Either way we record the desired index now (so the
		// index-limit check and the UI reflect intent) and report it as deferred.
		if err := h.repo.RecordIndex(fieldID, targetTable, indexName); err != nil {
			return false, fmt.Errorf("failed to schedule index: %w", err)
		}
		if h.repo.DriverName() != "sqlite" {
			if err := scheduler.EnqueueIndexBuild(h.db, fieldID, fieldType, targetTable, indexName); err != nil {
				// The record stands so the index still builds on a later edit or
				// retry; surface the failure rather than leaving it silent.
				return false, fmt.Errorf("failed to enqueue index build: %w", err)
			}
		}
		return true, nil
	} else {
		if err := h.repo.ExecDDL(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)); err != nil {
			return false, fmt.Errorf("failed to drop index: %w", err)
		}
		if err := h.repo.DeleteIndexRecord(fieldID, targetTable); err != nil {
			return false, fmt.Errorf("failed to remove index record: %w", err)
		}
	}

	return false, nil
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
			Details: map[string]any{
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
	mirrorOpts := map[string]any{
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

func (h *CustomFieldHandler) findLinkingDeleteOptions(fieldID int) (*linkingFieldOptions, error) {
	optionsJSON, err := h.repo.FindOptions(fieldID)
	if err != nil {
		return nil, err
	}
	if optionsJSON == "" {
		return nil, nil
	}

	var opts linkingFieldOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return nil, fmt.Errorf("decode linking field options: %w", err)
	}
	return &opts, nil
}

func (h *CustomFieldHandler) clearPrimaryMirrorReference(primaryFieldID int) error {
	primaryOptsJSON, err := h.repo.FindOptions(primaryFieldID)
	if err != nil || primaryOptsJSON == "" {
		return err
	}
	var primaryOpts map[string]any
	if err := json.Unmarshal([]byte(primaryOptsJSON), &primaryOpts); err != nil {
		return fmt.Errorf("decode primary linking field options: %w", err)
	}
	delete(primaryOpts, "mirror_field_id")
	updatedJSON, err := json.Marshal(primaryOpts)
	if err != nil {
		return err
	}
	return h.repo.UpdateOptions(int64(primaryFieldID), string(updatedJSON))
}

// cleanupRemovedOptions detects which option IDs an edit removed and enqueues an
// async job to scrub references to them from items, assets, and portal
// custom_field_values. Doing this inline would load every row carrying a value
// for the field into memory and block the admin request for as long as the
// workspace has items/assets; CFVCleanupScheduler drains the job in bounded
// keyset-paginated batches instead (WI-419).
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

	var removedIDs []int
	for _, item := range oldOpts.Items {
		if !newIDs[item.ID] {
			removedIDs = append(removedIDs, item.ID)
		}
	}

	if len(removedIDs) == 0 {
		return
	}

	if err := scheduler.EnqueueOptionRemoval(h.db, fieldID, fieldType, removedIDs); err != nil {
		slog.Warn("custom_fields: failed to enqueue option-removal cleanup job",
			slog.Int("field_id", fieldID), slog.Any("error", err))
		// Best-effort: until the job drains, a removed option id renders as its
		// bare value (the renderer tolerates unknown ids), so don't fail the
		// request.
	}
}

// --- small helpers extracted to keep Create/Update flows readable ----------

var validFieldTypes = map[string]bool{
	"text":                         true,
	"textarea":                     true,
	"select":                       true,
	"multiselect":                  true,
	"number":                       true,
	"milestone":                    true,
	"date":                         true,
	"user":                         true,
	"multi_user":                   true,
	"iteration":                    true,
	"asset":                        true,
	"portalcustomer":               true,
	"customerorganisation":         true, //nolint:misspell // matches database column
	"linking":                      true,
	models.CustomFieldTypeBoolean:  true,
	models.CustomFieldTypeCheckbox: true,
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
	// Reject duplicate labels (case-sensitive). The schema doesn't prevent
	// this and two options with the same label are indistinguishable in
	// the UI — surface the conflict at config time instead.
	seen := make(map[string]bool, len(opts.Items))
	for _, item := range opts.Items {
		if seen[item.Label] {
			return "", &selectValidationError{
				validation: true,
				msg:        fmt.Sprintf("Duplicate option label: %q", item.Label),
			}
		}
		seen[item.Label] = true
	}
	normalized, serErr := models.SerializeSelectOptions(opts)
	if serErr != nil {
		return "", &selectValidationError{validation: false, msg: serErr.Error()}
	}
	return normalized, nil
}
