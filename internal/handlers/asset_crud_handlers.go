package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// validateResourceBelongsToSet checks that a resource (by table name) with resourceID belongs to setID.
// Returns true if valid; writes an error response and returns false otherwise.
func (h *AssetHandler) validateResourceBelongsToSet(w http.ResponseWriter, r *http.Request, table string, resourceID, setID int, resourceName string) bool {
	resSetID, err := h.repo.GetResourceSetID(table, resourceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondValidationError(w, r, resourceName+" not found")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if resSetID != setID {
		respondValidationError(w, r, resourceName+" does not belong to this set")
		return false
	}
	return true
}

// serializeCustomFields normalizes user-type fields and marshals custom field values to JSON.
// Returns (serialized *string, ok bool). Writes error response on failure.
func (h *AssetHandler) serializeCustomFields(w http.ResponseWriter, r *http.Request, customFieldValues map[string]interface{}, assetTypeID int) (*string, bool) {
	if customFieldValues == nil {
		return nil, true
	}
	if err := h.normalizeUserFieldValues(customFieldValues, assetTypeID); err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to process custom field values: %w", err))
		return nil, false
	}
	b, err := json.Marshal(customFieldValues)
	if err != nil {
		respondValidationError(w, r, "Invalid custom field values")
		return nil, false
	}
	s := string(b)
	return &s, true
}

// assetRowToModel converts a repository.AssetRow into the models.Asset returned by the API.
// Parses custom_field_values JSON; on failure, sets Warnings and leaves the map empty.
func assetRowToModel(row repository.AssetRow) models.Asset {
	asset := models.Asset{
		ID:              row.ID,
		SetID:           row.SetID,
		AssetTypeID:     row.AssetTypeID,
		CategoryID:      utils.NullInt64ToPtr(row.CategoryID),
		StatusID:        utils.NullInt64ToPtr(row.StatusID),
		Title:           row.Title,
		Description:     row.Description.String,
		AssetTag:        row.AssetTag.String,
		FracIndex:       utils.NullStringToPtr(row.FracIndex),
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		SetName:         row.SetName.String,
		AssetTypeName:   row.AssetTypeName.String,
		AssetTypeIcon:   row.AssetTypeIcon.String,
		AssetTypeColor:  row.AssetTypeColor.String,
		CategoryName:    row.CategoryName.String,
		CategoryPath:    row.CategoryPath.String,
		StatusName:      row.StatusName.String,
		StatusColor:     row.StatusColor.String,
		CreatorName:     row.CreatorName.String,
		CreatorEmail:    row.CreatorEmail.String,
		LinkedItemCount: row.LinkedItemCount,
	}

	if row.CustomFieldValues.Valid && row.CustomFieldValues.String != "" {
		if err := json.Unmarshal([]byte(row.CustomFieldValues.String), &asset.CustomFieldValues); err != nil {
			slog.Error("failed to unmarshal asset custom_field_values",
				slog.Int("asset_id", asset.ID),
				slog.String("raw", row.CustomFieldValues.String),
				slog.Any("error", err))
			asset.CustomFieldValues = make(map[string]interface{})
			asset.Warnings = append(asset.Warnings, "custom field values could not be parsed")
		}
	}
	return asset
}

// GetAssets returns all assets in a set with pagination and subcategory support
func (h *AssetHandler) GetAssets(w http.ResponseWriter, r *http.Request) {
	user, setID, ok := h.requireSetViewAccess(w, r)
	if !ok {
		return
	}

	limit, offset := parseOffsetPagination(r, 25, 10000)

	filter := repository.AssetListFilter{
		SetID:                setID,
		AssetTypeID:          r.URL.Query().Get("type_id"),
		CategoryID:           r.URL.Query().Get("category_id"),
		IncludeSubcategories: r.URL.Query().Get("include_subcategories") != "false",
		StatusID:             r.URL.Query().Get("status_id"),
		Search:               r.URL.Query().Get("search"),
		Limit:                limit,
		Offset:               offset,
	}

	if cqlQuery := r.URL.Query().Get("ql"); cqlQuery != "" {
		setMap, err := h.repo.GetCQLSetMap()
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load set mapping: %w", err))
			return
		}
		workspaceMap, err := repository.NewWorkspaceRepository(h.db).ListNameKeyToIDMap()
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load workspace mapping: %w", err))
			return
		}
		customFieldMap, err := h.repo.GetCQLCustomFieldMap(setID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to load custom field mapping: %w", err))
			return
		}

		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap, customFieldMap, h.db.GetDriverName())
		resolvedQuery := cql.SubstituteFunctions(cqlQuery, cql.UserContext(user.ID))
		cqlSQL, cqlArgs, err := evaluator.EvaluateToSQL(resolvedQuery)
		if err != nil {
			respondValidationError(w, r, "CQL query error: "+err.Error())
			return
		}
		filter.CQLSQL = cqlSQL
		filter.CQLArgs = cqlArgs

		slog.Debug("asset query CQL",
			slog.String("cql", cqlQuery),
			slog.String("sql", cqlSQL),
			slog.Any("args", cqlArgs))
	}

	total, err := h.repo.CountAssets(filter)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rows, err := h.repo.ListAssets(filter)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	assets := make([]models.Asset, 0, len(rows))
	for _, row := range rows {
		asset := assetRowToModel(row)
		if err := h.enrichUserCustomFields(&asset); err != nil {
			continue
		}
		assets = append(assets, asset)
	}

	respondJSONOK(w, map[string]interface{}{
		"assets": assets,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// loadFullAsset fetches a single asset with all joined/enriched fields, matching
// the shape returned by GetAsset. Shared by GET and PUT so clients see a
// consistent payload after create/update.
func (h *AssetHandler) loadFullAsset(assetID int) (models.Asset, error) {
	row, err := h.repo.FindAssetFullByID(assetID)
	if err != nil {
		return models.Asset{}, err
	}
	asset := assetRowToModel(*row)
	if err := h.enrichUserCustomFields(&asset); err != nil {
		slog.Debug("failed to enrich user custom fields", slog.Any("error", err))
	}
	if err := h.enrichAssetRefCustomFields(&asset); err != nil {
		slog.Debug("failed to enrich asset-ref custom fields", slog.Any("error", err))
	}
	return asset, nil
}

func (h *AssetHandler) GetAsset(w http.ResponseWriter, r *http.Request) {
	_, assetID, ok := h.requireAssetViewAccess(w, r)
	if !ok {
		return
	}

	asset, err := h.loadFullAsset(assetID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, asset)
}

// CreateAssetRequest represents the request body for creating an asset
type CreateAssetRequest struct {
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
}

// CreateAsset creates a new asset
func (h *AssetHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetEditAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[CreateAssetRequest](w, r)
	if !ok {
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		respondValidationError(w, r, "Title is required")
		return
	}
	if req.AssetTypeID == 0 {
		respondValidationError(w, r, "Asset type is required")
		return
	}

	if !h.validateResourceBelongsToSet(w, r, "asset_types", req.AssetTypeID, setID, "Asset type") {
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeDescription(req.Description)

	if req.CategoryID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_categories", *req.CategoryID, setID, "Category") {
			return
		}
	}

	// Handle status_id - get default if not provided
	var statusID *int
	if req.StatusID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_statuses", *req.StatusID, setID, "Status") {
			return
		}
		statusID = req.StatusID
	} else {
		statusID, _ = h.repo.GetDefaultStatus(setID)
	}

	customFieldValuesJSON, ok := h.serializeCustomFields(w, r, req.CustomFieldValues, req.AssetTypeID)
	if !ok {
		return
	}

	now := time.Now()
	assetID, err := h.repo.CreateAsset(repository.CreateAssetInput{
		SetID:                 setID,
		AssetTypeID:           req.AssetTypeID,
		CategoryID:            req.CategoryID,
		StatusID:              statusID,
		Title:                 req.Title,
		Description:           req.Description,
		AssetTag:              req.AssetTag,
		CustomFieldValuesJSON: customFieldValuesJSON,
		CreatedBy:             currentUser.ID,
		CreatedAt:             now,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetCreate, logger.ResourceAsset, &assetID, req.Title)

	// Emit asset action event for automation
	if h.assetActionService != nil {
		h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:   models.AssetTriggerAssetCreated,
			SetID:       setID,
			AssetID:     assetID,
			ActorUserID: currentUser.ID,
			NewValues: map[string]interface{}{
				"title":         req.Title,
				"asset_type_id": req.AssetTypeID,
				"status_id":     statusID,
			},
		})
	}

	respondJSONCreated(w, models.Asset{
		ID:                assetID,
		SetID:             setID,
		AssetTypeID:       req.AssetTypeID,
		CategoryID:        req.CategoryID,
		StatusID:          statusID,
		Title:             req.Title,
		Description:       req.Description,
		AssetTag:          req.AssetTag,
		CustomFieldValues: req.CustomFieldValues,
		CreatedBy:         &currentUser.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
}

// UpdateAssetRequest represents the request body for updating an asset
type UpdateAssetRequest struct {
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
}

// UpdateAsset updates an existing asset
func (h *AssetHandler) UpdateAsset(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	assetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	snap, err := h.repo.GetAssetUpdateSnapshot(assetID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	canEdit, err := h.canEditSet(currentUser.ID, snap.SetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canEdit {
		respondNotFound(w, r, "asset")
		return
	}

	req, ok := decodeJSON[UpdateAssetRequest](w, r)
	if !ok {
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		respondValidationError(w, r, "Title is required")
		return
	}
	if req.AssetTypeID <= 0 {
		respondValidationError(w, r, "asset_type_id is required")
		return
	}

	// Sanitize user input to prevent XSS
	req.Title = utils.StripHTMLTags(req.Title)
	req.Description = utils.SanitizeDescription(req.Description)

	if !h.validateResourceBelongsToSet(w, r, "asset_types", req.AssetTypeID, snap.SetID, "Asset type") {
		return
	}
	if req.CategoryID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_categories", *req.CategoryID, snap.SetID, "Category") {
			return
		}
	}
	if req.StatusID != nil {
		if !h.validateResourceBelongsToSet(w, r, "asset_statuses", *req.StatusID, snap.SetID, "Status") {
			return
		}
	}

	customFieldValuesJSON, ok := h.serializeCustomFields(w, r, req.CustomFieldValues, req.AssetTypeID)
	if !ok {
		return
	}

	err = h.repo.UpdateAsset(assetID, repository.UpdateAssetInput{
		AssetTypeID:           req.AssetTypeID,
		CategoryID:            req.CategoryID,
		StatusID:              req.StatusID,
		Title:                 req.Title,
		Description:           req.Description,
		AssetTag:              req.AssetTag,
		CustomFieldValuesJSON: customFieldValuesJSON,
	})
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetUpdate, logger.ResourceAsset, &assetID, req.Title)

	// Emit asset action events for automation
	if h.assetActionService != nil {
		oldSID := 0
		if snap.StatusID.Valid {
			oldSID = int(snap.StatusID.Int64)
		}
		newSID := 0
		if req.StatusID != nil {
			newSID = *req.StatusID
		}
		statusChanged := oldSID != newSID

		if statusChanged {
			h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
				EventType:   models.AssetTriggerAssetStatusChanged,
				SetID:       snap.SetID,
				AssetID:     assetID,
				ActorUserID: currentUser.ID,
				OldValues:   map[string]interface{}{"status_id": oldSID},
				NewValues:   map[string]interface{}{"status_id": newSID},
			})
		}

		h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:   models.AssetTriggerAssetUpdated,
			SetID:       snap.SetID,
			AssetID:     assetID,
			ActorUserID: currentUser.ID,
			NewValues: map[string]interface{}{
				"title":         req.Title,
				"asset_type_id": req.AssetTypeID,
				"status_id":     req.StatusID,
			},
		})
	}

	asset, err := h.loadFullAsset(assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, asset)
}

// DeleteAsset deletes an asset
func (h *AssetHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	currentUser, assetID, ok := h.requireAssetEditAccess(w, r)
	if !ok {
		return
	}

	setID, title, err := h.repo.GetAssetSetAndTitle(assetID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.DeleteAssetWithLinks(assetID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "asset")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAssetDelete, logger.ResourceAsset, &assetID, title)

	if h.assetActionService != nil {
		h.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:   models.AssetTriggerAssetDeleted,
			SetID:       setID,
			AssetID:     assetID,
			ActorUserID: currentUser.ID,
			OldValues: map[string]interface{}{
				"title": title,
			},
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
