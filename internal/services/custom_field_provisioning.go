package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// CustomFieldProvisioningHooks injects the CFVCleanupScheduler job enqueuers,
// which live in a package that imports services (so calling them directly from
// here would create an import cycle).
type CustomFieldProvisioningHooks struct {
	EnqueueOptionRemoval     func(db database.Database, fieldID int, fieldType string, optionIDs []int) error
	EnqueueFieldCleanup      func(db database.Database, fieldID int) error
	EnqueueIndexBuild        func(db database.Database, fieldID int, fieldType, targetTable, indexName string) error
	CancelPendingIndexBuilds func(db database.Database, fieldID int) error
}

// CustomFieldProvisioningService owns the custom-field definition mutation
// flows shared by the admin browser surface and public API v2 (WI-1306):
// validation, normalization, sanitization, mirror linking fields, index
// management, value scrubbing, and audit.
type CustomFieldProvisioningService struct {
	db             database.Database
	repo           *repository.CustomFieldRepository
	linkTypeRepo   *repository.LinkTypeRepository
	systemSettings *repository.SystemSettingRepository
	hooks          CustomFieldProvisioningHooks
}

func NewCustomFieldProvisioningService(db database.Database, hooks CustomFieldProvisioningHooks) *CustomFieldProvisioningService {
	return &CustomFieldProvisioningService{
		db:             db,
		repo:           repository.NewCustomFieldRepository(db),
		linkTypeRepo:   repository.NewLinkTypeRepository(db),
		systemSettings: repository.NewSystemSettingRepository(db),
		hooks:          hooks,
	}
}

// CustomFieldMutationResult carries the persisted definition plus the
// warnings and deferred-index state the HTTP surfaces render.
type CustomFieldMutationResult struct {
	Field            *models.CustomFieldDefinition
	Warnings         []string
	IndexingDeferred bool
	DeferredIndexes  *models.CustomFieldIndexInfo
}

// CustomFieldUpdateInput is the Update input: the new definition and the
// optional indexing state (nil leaves indexes unchanged).
type CustomFieldUpdateInput struct {
	Definition models.CustomFieldDefinition
	Indexed    *models.CustomFieldIndexInfo
}

const (
	maxIndexesSettingKey = "max_custom_field_indexes_per_table"
	defaultMaxIndexes    = 20
)

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

var errCustomFieldIndexLimit = errors.New("custom field index limit reached")

var validFieldTypes = map[string]bool{
	"text":                         true,
	"textarea":                     true,
	"select":                       true,
	"multiselect":                  true,
	"number":                       true,
	"milestone":                    true,
	"multi_milestone":              true,
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

// IsValidCustomFieldType reports whether the canonical custom-field type is
// one Windshift can store values for.
func IsValidCustomFieldType(t string) bool {
	return validFieldTypes[t]
}

// IsIndexableCustomFieldType reports whether the field type supports B-tree
// indexing on items/assets.
func IsIndexableCustomFieldType(t string) bool {
	return indexableFieldTypes[t]
}

// Create validates and persists a new custom-field definition, auto-creating
// the mirror field when the linking options request one.
func (s *CustomFieldProvisioningService) Create(actor AuditActor, cf *models.CustomFieldDefinition) (*CustomFieldMutationResult, error) {
	linkingOpts, warnings, err := s.validateAndNormalize(cf)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	id, err := s.repo.Create(cf, now)
	if err != nil {
		return nil, internalProvisioningError(err)
	}

	// Auto-create mirror field for linking fields if mirror_name is provided
	if cf.FieldType == "linking" && linkingOpts != nil && linkingOpts.MirrorName != "" {
		mirrorID, mirrorErr := s.createMirrorField(int(id), linkingOpts, now)
		if mirrorErr != nil {
			return nil, internalProvisioningError(mirrorErr)
		}
		var primaryOpts map[string]any
		if err := json.Unmarshal([]byte(cf.Options), &primaryOpts); err == nil {
			delete(primaryOpts, "mirror_name")
			delete(primaryOpts, "mirror_allowed_item_type_ids")
			primaryOpts["mirror_field_id"] = mirrorID
			if updatedJSON, err := json.Marshal(primaryOpts); err == nil {
				_ = s.repo.UpdateOptions(id, string(updatedJSON))
			}
		}
	}

	createdCF, err := s.repo.FindByID(int(id))
	if err != nil {
		return nil, internalProvisioningError(err)
	}

	s.audit(actor, logger.ActionCustomFieldCreate, &createdCF.ID, createdCF.Name, map[string]any{
		"field_type":    createdCF.FieldType,
		"required":      createdCF.Required,
		"display_order": createdCF.DisplayOrder,
	})

	return &CustomFieldMutationResult{Field: createdCF, Warnings: warnings}, nil
}

// Update validates and persists an edit of an existing definition. Update is
// refused on system-default fields, which are read-only like Delete.
func (s *CustomFieldProvisioningService) Update(actor AuditActor, id int, input CustomFieldUpdateInput) (*CustomFieldMutationResult, error) {
	oldCF, err := s.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, NewServiceError(404, "Custom field not found")
		}
		return nil, internalProvisioningError(err)
	}

	// System-default fields are system-owned and read-only, matching Delete.
	// Otherwise option edits could enqueue scrubbing of item/asset values on
	// system data.
	if oldCF.SystemDefault {
		return nil, NewServiceError(403, "System-default custom fields cannot be modified")
	}

	cf := input.Definition
	_, warnings, err := s.validateAndNormalize(&cf)
	if err != nil {
		return nil, err
	}
	if models.CanonicalCustomFieldType(oldCF.FieldType) != cf.FieldType {
		return nil, NewServiceError(400, "Custom field type cannot be changed after creation")
	}
	if input.Indexed != nil && !indexableFieldTypes[cf.FieldType] {
		return nil, NewServiceError(400, fmt.Sprintf("Field type '%s' cannot be indexed. Only number, date, and text fields support indexing.", cf.FieldType))
	}

	now := time.Now()
	if err := s.repo.Update(id, &cf, now); err != nil {
		return nil, internalProvisioningError(err)
	}

	// Clean up item/asset values when select/multiselect options are removed
	if cf.FieldType == "select" || cf.FieldType == "multiselect" {
		s.cleanupRemovedOptions(id, oldCF.Options, cf.Options, cf.FieldType)
	}

	deferredIndexes := &models.CustomFieldIndexInfo{}
	indexingDeferred := false

	// Handle indexing changes if provided
	if input.Indexed != nil {
		for _, table := range []struct {
			name   string
			wanted bool
		}{
			{"items", input.Indexed.Items},
			{"assets", input.Indexed.Assets},
		} {
			deferred, err := s.manageFieldIndex(id, oldCF.FieldType, table.name, table.wanted)
			if err != nil {
				if errors.Is(err, errCustomFieldIndexLimit) {
					return nil, NewServiceError(400, err.Error())
				}
				return nil, internalProvisioningError(err)
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

	updatedCF, err := s.repo.FindByID(id)
	if err != nil {
		return nil, internalProvisioningError(err)
	}

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
	if input.Indexed != nil {
		details["indexed"] = input.Indexed
	}
	s.audit(actor, logger.ActionCustomFieldUpdate, &updatedCF.ID, updatedCF.Name, details)

	return &CustomFieldMutationResult{
		Field:            updatedCF,
		Warnings:         warnings,
		IndexingDeferred: indexingDeferred,
		DeferredIndexes:  deferredIndexes,
	}, nil
}

// Delete removes a custom field (with its mirror, for linking fields) after
// verifying nothing references it. System-default fields are protected.
func (s *CustomFieldProvisioningService) Delete(actor AuditActor, id int) error {
	info, err := s.repo.FindDeleteInfo(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return NewServiceError(404, "Custom field not found")
		}
		return internalProvisioningError(err)
	}

	if info.SystemDefault {
		return NewServiceError(403, "System-default custom fields cannot be deleted")
	}

	deleteIDs := []int{id}
	var linkingOpts *customFieldLinkingOptions
	if info.FieldType == "linking" {
		linkingOpts, err = s.findLinkingDeleteOptions(id)
		if err != nil {
			return internalProvisioningError(err)
		}
		if linkingOpts != nil && linkingOpts.MirrorFieldID > 0 {
			if _, err := s.repo.FindDeleteInfo(linkingOpts.MirrorFieldID); err == nil {
				deleteIDs = append([]int{linkingOpts.MirrorFieldID}, deleteIDs...)
			} else if !errors.Is(err, repository.ErrNotFound) {
				return internalProvisioningError(err)
			}
		}
	}

	// Check every field in a linking cascade before deleting either definition.
	// The asynchronous scrub remains defense-in-depth for concurrent writes.
	for _, fieldID := range deleteIDs {
		inUse, err := s.repo.CountRowsUsingField(fieldID)
		if err != nil {
			return internalProvisioningError(err)
		}
		if inUse > 0 {
			return NewServiceError(409, fmt.Sprintf("Cannot delete custom field: it is used by %d record(s). Clear those values first.", inUse))
		}
	}

	indexesByField := make(map[int][]string, len(deleteIDs))
	for _, fieldID := range deleteIDs {
		indexNames, err := s.repo.ListIndexNamesForField(fieldID)
		if err != nil {
			return internalProvisioningError(err)
		}
		indexesByField[fieldID] = indexNames
	}

	if linkingOpts != nil && linkingOpts.MirrorOfFieldID > 0 {
		if err := s.clearPrimaryMirrorReference(linkingOpts.MirrorOfFieldID); err != nil {
			return internalProvisioningError(err)
		}
	}

	for _, fieldID := range deleteIDs {
		for _, indexName := range indexesByField[fieldID] {
			dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
			if err := s.repo.ExecDDL(dropSQL); err != nil {
				slog.Warn("failed to drop index during field deletion", slog.String("component", "custom_fields"), slog.String("index", indexName), slog.Any("error", err))
			}
		}

		if s.hooks.CancelPendingIndexBuilds != nil {
			if err := s.hooks.CancelPendingIndexBuilds(s.db, fieldID); err != nil {
				slog.Warn("custom_fields: failed to cancel pending index builds",
					slog.Int("field_id", fieldID), slog.Any("error", err))
			}

			if err := s.repo.Delete(fieldID); err != nil {
				return internalProvisioningError(err)
			}

		}
		if s.hooks.EnqueueFieldCleanup != nil {
			if err := s.hooks.EnqueueFieldCleanup(s.db, fieldID); err != nil {
				slog.Warn("custom_fields: failed to enqueue cfv cleanup job",
					slog.Int("field_id", fieldID), slog.Any("error", err))
			}
		}
	}

	s.audit(actor, logger.ActionCustomFieldDelete, &id, info.Name, map[string]any{
		"field_type": info.FieldType,
	})
	return nil
}

// UpdateSettings stores the per-table index budget. Values below current
// usage are refused.
func (s *CustomFieldProvisioningService) UpdateSettings(actor AuditActor, maxIndexesPerTable int) error {
	if maxIndexesPerTable < 1 || maxIndexesPerTable > 100 {
		return NewServiceError(400, "Maximum indexes per table must be between 1 and 100")
	}

	// Check that new limit is not below current usage for any table
	counts, err := s.repo.CountIndexesPerTable()
	if err != nil {
		return internalProvisioningError(err)
	}
	for table, count := range counts {
		if count > maxIndexesPerTable {
			return NewServiceError(400, fmt.Sprintf("Cannot set limit to %d: %s table already has %d indexes", maxIndexesPerTable, table, count))
		}
	}

	err = s.systemSettings.Upsert(
		maxIndexesSettingKey,
		strconv.Itoa(maxIndexesPerTable),
		"integer",
		"Maximum number of custom field indexes per table",
		"performance",
	)
	if err != nil {
		return internalProvisioningError(err)
	}

	s.audit(actor, logger.ActionCustomFieldUpdate, nil, "custom_field_settings", map[string]any{
		"max_indexes_per_table": maxIndexesPerTable,
	})
	return nil
}

// MaxIndexesPerTable returns the configured max (or the default on error).
func (s *CustomFieldProvisioningService) MaxIndexesPerTable() int {
	value, ok, err := s.systemSettings.GetValue(maxIndexesSettingKey)
	if err != nil || !ok {
		return defaultMaxIndexes
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return defaultMaxIndexes
	}
	return v
}

func (s *CustomFieldProvisioningService) audit(actor AuditActor, action string, resourceID *int, name string, details map[string]any) {
	_ = logger.LogAudit(s.db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   action,
		ResourceType: logger.ResourceCustomField,
		ResourceID:   resourceID,
		ResourceName: name,
		Details:      details,
		Success:      true,
	})
}

// validateAndNormalize runs the name + field-type + per-type option
// validation, select/multiselect normalization, and XSS sanitization shared by
// Create and Update. It returns the linking-field options parsed during
// validation (nil unless FieldType == "linking").
func (s *CustomFieldProvisioningService) validateAndNormalize(cf *models.CustomFieldDefinition) (*customFieldLinkingOptions, []string, error) {
	if strings.TrimSpace(cf.Name) == "" {
		return nil, nil, NewServiceError(400, "Field name is required")
	}
	cf.FieldType = models.CanonicalCustomFieldType(cf.FieldType)
	if !IsValidCustomFieldType(cf.FieldType) {
		return nil, nil, NewServiceError(400, "Invalid field type")
	}

	var linkingOpts *customFieldLinkingOptions
	if cf.FieldType == "linking" {
		var linkErr error
		linkingOpts, linkErr = s.validateLinkingOptions(cf.Options)
		if linkErr != nil {
			return nil, nil, NewServiceError(400, linkErr.Error())
		}
	}

	if cf.FieldType == "asset" {
		if err := validateAssetFieldOptions(cf.Options); err != nil {
			return nil, nil, NewServiceError(400, err.Error())
		}
	}

	if models.IsBooleanCustomFieldType(cf.FieldType) && strings.TrimSpace(cf.Options) != "" {
		return nil, nil, NewServiceError(400, "Checkbox fields do not support options")
	}

	if cf.FieldType == "select" || cf.FieldType == "multiselect" {
		normalized, vErr := normalizeSelectOptions(cf.Options)
		if vErr != nil {
			if vErr.validation {
				return nil, nil, NewServiceError(400, vErr.msg)
			}
			return nil, nil, internalProvisioningError(errors.New(vErr.msg))
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

	return linkingOpts, warnings, nil
}

// customFieldLinkingOptions holds parsed options for linking custom fields
type customFieldLinkingOptions struct {
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
func (s *CustomFieldProvisioningService) validateLinkingOptions(optionsJSON string) (*customFieldLinkingOptions, error) {
	if optionsJSON == "" {
		return nil, fmt.Errorf("linking fields require options with link_type_id")
	}
	var opts customFieldLinkingOptions
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
	basic, err := s.linkTypeRepo.FindBasicByID(opts.LinkTypeID)
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
					ltName, _ := s.linkTypeRepo.FindNameByID(opts.LinkTypeID)
					return nil, fmt.Errorf("link type '%s' only supports entity types: %s", ltName, strings.Join(ltAllowed, ", "))
				}
			}
		}
	}
	return &opts, nil
}

// createMirrorField creates a mirror linking field for the given primary field
func (s *CustomFieldProvisioningService) createMirrorField(primaryID int, opts *customFieldLinkingOptions, now time.Time) (int64, error) {
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

	mirrorID, err := s.repo.CreateMirror(opts.MirrorName, string(mirrorOptsJSON), now)
	if err != nil {
		return 0, fmt.Errorf("failed to create mirror field: %w", err)
	}
	return mirrorID, nil
}

func (s *CustomFieldProvisioningService) findLinkingDeleteOptions(fieldID int) (*customFieldLinkingOptions, error) {
	optionsJSON, err := s.repo.FindOptions(fieldID)
	if err != nil {
		return nil, err
	}
	if optionsJSON == "" {
		return nil, nil
	}

	var opts customFieldLinkingOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return nil, fmt.Errorf("decode linking field options: %w", err)
	}
	return &opts, nil
}

func (s *CustomFieldProvisioningService) clearPrimaryMirrorReference(primaryFieldID int) error {
	primaryOptsJSON, err := s.repo.FindOptions(primaryFieldID)
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
	return s.repo.UpdateOptions(int64(primaryFieldID), string(updatedJSON))
}

// cleanupRemovedOptions detects which option IDs an edit removed and enqueues an
// async job to scrub references to them from items, assets, and portal
// custom_field_values. Doing this inline would load every row carrying a value
// for the field into memory and block the admin request for as long as the
// workspace has items/assets; CFVCleanupScheduler drains the job in bounded
// keyset-paginated batches instead (WI-419).
func (s *CustomFieldProvisioningService) cleanupRemovedOptions(fieldID int, oldOptionsJSON, newOptionsJSON, fieldType string) {
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

	if s.hooks.EnqueueOptionRemoval != nil {
		if err := s.hooks.EnqueueOptionRemoval(s.db, fieldID, fieldType, removedIDs); err != nil {
			slog.Warn("custom_fields: failed to enqueue option-removal cleanup job",
				slog.Int("field_id", fieldID), slog.Any("error", err))
			// Best-effort: until the job drains, a removed option id renders as its
			// bare value (the renderer tolerates unknown ids), so don't fail the
			// request.
		}
	}
}

// manageFieldIndex creates, drops, or schedules a database index for a custom
// field on a target table. It returns true when index creation was deferred.
func (s *CustomFieldProvisioningService) manageFieldIndex(fieldID int, fieldType, targetTable string, enable bool) (bool, error) {
	if !indexableTargetTables[targetTable] {
		return false, fmt.Errorf("invalid target table: %s", targetTable)
	}

	indexName := fmt.Sprintf("idx_cf_%s_%d", targetTable, fieldID)

	currentlyEnabled, err := s.repo.IsIndexRecorded(fieldID, targetTable)
	if err != nil {
		return false, fmt.Errorf("failed to check index state: %w", err)
	}

	if enable == currentlyEnabled {
		return false, nil
	}

	if enable {
		currentCount, err := s.repo.CountIndexesForTable(targetTable)
		if err != nil {
			return false, fmt.Errorf("failed to count indexes: %w", err)
		}

		maxIndexes := s.MaxIndexesPerTable()

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
		if err := s.repo.RecordIndex(fieldID, targetTable, indexName); err != nil {
			return false, fmt.Errorf("failed to schedule index: %w", err)
		}
		if s.repo.DriverName() != "sqlite" {
			if err := s.hooks.EnqueueIndexBuild(s.db, fieldID, fieldType, targetTable, indexName); err != nil {
				// The record stands so the index still builds on a later edit or
				// retry; surface the failure rather than leaving it silent.
				return false, fmt.Errorf("failed to enqueue index build: %w", err)
			}
		}
		return true, nil
	} else {
		if err := s.repo.ExecDDL(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)); err != nil {
			return false, fmt.Errorf("failed to drop index: %w", err)
		}
		if err := s.repo.DeleteIndexRecord(fieldID, targetTable); err != nil {
			return false, fmt.Errorf("failed to remove index record: %w", err)
		}
	}

	return false, nil
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

func internalProvisioningError(err error) error {
	return fmt.Errorf("custom field provisioning: %w", err)
}
