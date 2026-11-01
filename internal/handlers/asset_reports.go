package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// sanitizeAssetReport applies field policies and validates Config as JSON rather
// than corrupting it by scrubbing. Invalid Config writes a validation error.
func sanitizeAssetReport(w http.ResponseWriter, r *http.Request, ar *models.AssetReport) bool {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &ar.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &ar.Description, Policy: sanitize.RichText},
		sanitize.Pair{Target: &ar.CQLQuery, Policy: sanitize.QueryText},
		sanitize.Pair{Target: &ar.Icon, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &ar.Color, Policy: sanitize.ShortIdentifier},
	)
	for i := range ar.ColumnConfig {
		sanitize.Apply(&ar.ColumnConfig[i], sanitize.ShortIdentifier)
	}
	if ar.Config != nil {
		if err := sanitize.ValidateJSONPayload("config", *ar.Config); err != nil {
			respondValidationError(w, r, err.Error())
			return false
		}
	}
	return true
}

// sanitizeAssetReportFields applies per-row policies and validates JSON options
// rather than scrubbing them. Invalid options write a validation error.
func sanitizeAssetReportFields(w http.ResponseWriter, r *http.Request, fields []models.AssetReportField) bool {
	for i := range fields {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &fields[i].FieldIdentifier, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &fields[i].FieldType, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: fields[i].DisplayName, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: fields[i].Description, Policy: sanitize.RichText},
			sanitize.Pair{Target: fields[i].VirtualFieldType, Policy: sanitize.ShortIdentifier},
		)
		if fields[i].VirtualFieldOptions != nil {
			if err := sanitize.ValidateJSONPayload("virtual_field_options", *fields[i].VirtualFieldOptions); err != nil {
				respondValidationError(w, r, err.Error())
				return false
			}
		}
	}
	return true
}

func assetReportFieldSchemas(fields []models.AssetReportField) []publicFormFieldSchema {
	schemas := make([]publicFormFieldSchema, 0, len(fields))
	for _, field := range fields {
		schemas = append(schemas, publicFormFieldSchema{
			Identifier:          field.FieldIdentifier,
			FieldType:           field.FieldType,
			DisplayOrder:        field.DisplayOrder,
			StepNumber:          field.StepNumber,
			VirtualFieldType:    field.VirtualFieldType,
			VirtualFieldOptions: field.VirtualFieldOptions,
		})
	}
	return schemas
}

type AssetReportHandler struct {
	repo           *repository.AssetReportRepository
	channelRepo    *repository.ChannelRepository
	screenRepo     *repository.ScreenRepository
	auditor        *logger.Auditor
	channelService *services.ChannelService
	assetPerm      *services.AssetPermissionService
}

type createAssetReportRequest struct {
	models.AssetReport
	IsActive *bool `json:"is_active"`
}

func NewAssetReportHandler(
	repo *repository.AssetReportRepository,
	channelRepo *repository.ChannelRepository,
	screenRepo *repository.ScreenRepository,
	auditor *logger.Auditor,
	channelService *services.ChannelService,
	assetPerm *services.AssetPermissionService,
) *AssetReportHandler {
	return &AssetReportHandler{
		repo:           repo,
		channelRepo:    channelRepo,
		screenRepo:     screenRepo,
		auditor:        auditor,
		channelService: channelService,
		assetPerm:      assetPerm,
	}
}

// GetAllForChannel returns all asset reports for a specific channel
func (h *AssetReportHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Report definitions expose cql_query and visibility ACL config, so gate
	// the list by manager scope on the channel just like the single-report Get
	// (404, not 403, to avoid disclosing the channel exists).
	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "channel")
		return
	}

	reports, err := h.repo.ListByChannel(channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, reports)
}

// Get returns a specific asset report by ID
func (h *AssetReportHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	ar, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Gate by manager scope on the owning channel. See bughunt2.md Run 6
	// finding #4.
	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, ar.ChannelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "asset_report")
		return
	}

	respondJSONOK(w, ar)
}

// requireAssetSetView confirms the acting user holds at least asset.view on the
// target set. Asset sets are governed solely by per-set roles (they have no
// workspace), so managing a channel must not be sufficient to bind a report to
// — and then read, via the portal execute path — a set the user has no role on.
// Writes the appropriate error and returns false when the check fails.
func (h *AssetReportHandler) requireAssetSetView(w http.ResponseWriter, r *http.Request, userID, assetSetID int) bool {
	allowed, err := h.assetPerm.HasAssetSetPermission(userID, assetSetID, services.AssetPermissionKeyView)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !allowed {
		// Do not distinguish "no such set" from "no role on set".
		respondBadRequest(w, r, "Asset set not found")
		return false
	}
	return true
}

func (h *AssetReportHandler) requirePortalOwner(w http.ResponseWriter, r *http.Request, channelID int) bool {
	channel, err := h.channelRepo.FindByID(r.Context(), channelID)
	if errors.Is(err, repository.ErrNotFound) {
		respondValidationError(w, r, "Channel not found")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !channelSupportsAssetReports(channel) {
		respondValidationError(w, r, "Asset reports require an inbound portal channel")
		return false
	}
	return true
}

func (h *AssetReportHandler) availableFieldsForAssetReport(ar *models.AssetReport) ([]AvailableField, error) {
	if ar.ItemTypeID == nil {
		return availableCreateFields(h.screenRepo, ar.WorkspaceID, 0)
	}
	return availableCreateFields(h.screenRepo, ar.WorkspaceID, *ar.ItemTypeID)
}

// validateAssetReportFormBinding applies the same workspace and item-type
// authorization as request-type routing. A form-mode report must pin both: the
// binding resolves the form's custom fields and is fixed at creation.
func (h *AssetReportHandler) validateAssetReportFormBinding(w http.ResponseWriter, r *http.Request, userID int, ar *models.AssetReport) bool {
	switch ar.RunMode {
	case "direct":
		// Direct reports do not have a form binding. Clear caller-supplied stale
		// values so the available-fields endpoint cannot later expose them.
		ar.ItemTypeID = nil
		ar.WorkspaceID = nil
		return true
	case "form":
	default:
		respondValidationError(w, r, "Invalid run_mode")
		return false
	}

	if ar.ItemTypeID == nil || *ar.ItemTypeID <= 0 {
		respondValidationError(w, r, "Item type ID is required for form-mode asset reports")
		return false
	}
	if ar.WorkspaceID == nil || *ar.WorkspaceID <= 0 {
		respondValidationError(w, r, "Workspace ID is required for form-mode asset reports")
		return false
	}

	configJSON, err := h.channelRepo.GetConfig(r.Context(), ar.ChannelID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		respondInternalError(w, r, fmt.Errorf("parse channel %d config: %w", ar.ChannelID, err))
		return false
	}
	if !containsID(config.PortalWorkspaceIDs, *ar.WorkspaceID) {
		respondValidationError(w, r, "Workspace is not served by this portal channel")
		return false
	}
	canConnect, err := h.channelService.UserCanConnectWorkspace(userID, *ar.WorkspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !canConnect {
		respondForbidden(w, r)
		return false
	}
	allowed, err := h.channelService.ItemTypeAllowedInWorkspace(*ar.WorkspaceID, *ar.ItemTypeID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !allowed {
		respondValidationError(w, r, "Item type is not allowed in the selected workspace")
		return false
	}
	return true
}

// sameOptionalInt reports whether two optional binding IDs are equal.
func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// Create creates a new asset report
func (h *AssetReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	input, ok := decodeChannelJSON[createAssetReportRequest](w, r)
	if !ok {
		return
	}
	ar := input.AssetReport
	ar.IsActive = true
	if input.IsActive != nil {
		ar.IsActive = *input.IsActive
	}
	if !sanitizeAssetReport(w, r, &ar) {
		return
	}

	ar.ChannelID = channelID

	if strings.TrimSpace(ar.Name) == "" {
		respondValidationError(w, r, "Asset report name is required")
		return
	}
	if ar.AssetSetID == 0 {
		respondValidationError(w, r, "Asset set ID is required")
		return
	}
	if strings.TrimSpace(ar.CQLQuery) == "" {
		respondValidationError(w, r, "QL query is required")
		return
	}

	if !h.requirePortalOwner(w, r, ar.ChannelID) {
		return
	}
	assetSetExists, err := h.repo.AssetSetExists(ar.AssetSetID)
	if err != nil || !assetSetExists {
		respondBadRequest(w, r, "Asset set not found")
		return
	}
	if !h.requireAssetSetView(w, r, user.ID, ar.AssetSetID) {
		return
	}

	if ar.Icon == "" {
		ar.Icon = "Table2"
	}
	if ar.Color == "" {
		ar.Color = "#6b7280"
	}
	if ar.RunMode == "" {
		ar.RunMode = "direct"
	}
	if ar.RunMode != "direct" && ar.RunMode != "form" {
		respondValidationError(w, r, "Invalid run_mode")
		return
	}
	if !h.validateAssetReportFormBinding(w, r, user.ID, &ar) {
		return
	}
	if ar.DisplayOrder == 0 {
		maxOrder, mErr := h.repo.MaxDisplayOrder(ar.ChannelID)
		if mErr != nil {
			slog.Warn("failed to get max display order for asset reports", slog.Any("error", mErr))
		}
		ar.DisplayOrder = maxOrder + 1
	}

	nameExists, err := h.repo.NameExistsInChannel(ar.ChannelID, ar.Name, 0)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if nameExists {
		respondConflict(w, r, "Asset report with this name already exists for this channel")
		return
	}

	id, err := h.repo.Create(&ar)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Asset report with this name already exists for this channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	created, err := h.repo.GetByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	ar = *created

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionAssetReportCreate, logger.ResourceAssetReport,
			&ar.ID, ar.Name,
			map[string]any{
				"channel_id":   ar.ChannelID,
				"asset_set_id": ar.AssetSetID,
				"icon":         ar.Icon,
				"color":        ar.Color,
			},
		)
	}

	respondJSONCreated(w, ar)
}

// Update updates an existing asset report. Route is
// PUT /channels/{channel_id}/asset-reports/{id}; channelMgmt middleware gates
// access and the SQL UPDATE is constrained by channel_id. Body-supplied
// channel_id is ignored; workspace/item-type changes are authorized below and
// atomically clear fields from the previous create-screen binding.
func (h *AssetReportHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	old, err := h.repo.GetBasicForChannel(id, channelID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !h.requirePortalOwner(w, r, channelID) {
		return
	}

	ar, ok := decodeChannelJSON[models.AssetReport](w, r)
	if !ok {
		return
	}
	if !sanitizeAssetReport(w, r, &ar) {
		return
	}

	if strings.TrimSpace(ar.Name) == "" {
		respondValidationError(w, r, "Asset report name is required")
		return
	}
	if ar.AssetSetID == 0 {
		respondValidationError(w, r, "Asset set ID is required")
		return
	}
	if strings.TrimSpace(ar.CQLQuery) == "" {
		respondValidationError(w, r, "QL query is required")
		return
	}

	assetSetExists, err := h.repo.AssetSetExists(ar.AssetSetID)
	if err != nil || !assetSetExists {
		respondBadRequest(w, r, "Asset set not found")
		return
	}
	if !h.requireAssetSetView(w, r, user.ID, ar.AssetSetID) {
		return
	}

	nameExists, err := h.repo.NameExistsInChannel(channelID, ar.Name, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if nameExists {
		respondConflict(w, r, "Asset report with this name already exists for this channel")
		return
	}

	if ar.RunMode == "" {
		ar.RunMode = old.RunMode
	}
	if ar.RunMode != "direct" && ar.RunMode != "form" {
		respondValidationError(w, r, "Invalid run_mode")
		return
	}
	ar.ChannelID = channelID

	// A form-mode report's workspace and item type resolve its configured
	// fields and are fixed at creation. Omitted values preserve the binding.
	// A direct report may be upgraded to form once, after which it is fixed.
	if old.RunMode == "form" {
		if ar.WorkspaceID == nil {
			ar.WorkspaceID = old.WorkspaceID
		}
		if ar.ItemTypeID == nil {
			ar.ItemTypeID = old.ItemTypeID
		}
		if ar.RunMode != "form" {
			respondValidationError(w, r, "A form-mode asset report cannot be changed to direct mode")
			return
		}
		if !sameOptionalInt(old.WorkspaceID, ar.WorkspaceID) || !sameOptionalInt(old.ItemTypeID, ar.ItemTypeID) {
			respondValidationError(w, r, "Asset report workspace and item type cannot be changed after creation")
			return
		}
	}

	if !h.validateAssetReportFormBinding(w, r, user.ID, &ar) {
		return
	}

	if err := h.repo.Update(id, channelID, &ar); err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicateEntry):
			respondConflict(w, r, "Asset report with this name already exists for this channel")
		case errors.Is(err, repository.ErrNotFound):
			respondNotFound(w, r, "asset_report")
		default:
			respondInternalError(w, r, err)
		}
		return
	}

	updated, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	ar = *updated

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]any)
		if old.Name != ar.Name {
			details["name_changed"] = map[string]any{"old": old.Name, "new": ar.Name}
		}
		if old.AssetSetID != ar.AssetSetID {
			details["asset_set_changed"] = map[string]any{"old": old.AssetSetID, "new": ar.AssetSetID}
		}
		if old.Icon != ar.Icon {
			details["icon_changed"] = map[string]any{"old": old.Icon, "new": ar.Icon}
		}
		if old.Color != ar.Color {
			details["color_changed"] = map[string]any{"old": old.Color, "new": ar.Color}
		}

		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionAssetReportUpdate, logger.ResourceAssetReport,
			&ar.ID, ar.Name, details,
		)
	}

	respondJSONOK(w, ar)
}

// Delete deletes an asset report. Route is
// DELETE /channels/{channel_id}/asset-reports/{id}; channelMgmt middleware
// gates and the DELETE is constrained by channel_id.
func (h *AssetReportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	assetReportName, err := h.repo.GetNameForChannel(id, channelID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.Delete(id, channelID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "asset_report")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionAssetReportDelete, logger.ResourceAssetReport,
			&id, assetReportName,
			map[string]any{
				"channel_id": channelID,
			},
		)
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateVisibility updates only the visibility settings for an asset report.
// Route is PUT /channels/{channel_id}/asset-reports/{id}/visibility — gated by
// channelMgmt and scoped by channel_id in the SQL.
func (h *AssetReportHandler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var req visibilityInput
	if !decodeChannelRequest(w, r, &req, false) {
		return
	}

	if err := h.repo.UpdateVisibility(id, channelID, req.GroupIDs, req.OrgIDs); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "asset_report")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	ar, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionAssetReportVisibilityUpdate, logger.ResourceAssetReport,
			&ar.ID, ar.Name,
			map[string]any{
				"visibility_group_ids": ar.VisibilityGroupIDs,
				"visibility_org_ids":   ar.VisibilityOrgIDs,
			},
		)
	}

	respondJSONOK(w, *ar)
}

// requireManagedAssetReport authenticates the request, resolves the asset
// report from the "id" path param, and gates by manager scope on the owning
// channel — the same gate Get applies. Responds 404 both when the report is
// missing and when the user can't manage the channel (no existence leak).
// Returns the report and true on success; writes the response and returns
// false otherwise.
func (h *AssetReportHandler) requireManagedAssetReport(w http.ResponseWriter, r *http.Request) (*models.AssetReport, *models.User, bool) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, nil, false
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, nil, false
	}

	ar, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_report")
		return nil, nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, nil, false
	}

	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, ar.ChannelID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, nil, false
	}
	if !canManage {
		respondNotFound(w, r, "asset_report")
		return nil, nil, false
	}

	return ar, user, true
}

// GetFields returns all fields for a form-mode asset report.
func (h *AssetReportHandler) GetFields(w http.ResponseWriter, r *http.Request) {
	ar, user, ok := h.requireManagedAssetReport(w, r)
	if !ok {
		return
	}
	if ar.RunMode != "form" {
		respondJSONOK(w, []models.AssetReportField{})
		return
	}
	if !h.validateAssetReportFormBinding(w, r, user.ID, ar) {
		return
	}
	assetReportID := ar.ID

	fields, err := h.repo.ListFields(assetReportID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, fields)
}

// UpdateFields rewrites the field schema for an asset report. Route is
// PUT /channels/{channel_id}/asset-reports/{id}/fields; channelMgmt-gated and
// scoped to asset reports that belong to the URL-supplied channel.
func (h *AssetReportHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ar, err := h.repo.GetByID(assetReportID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && ar.ChannelID != channelID) {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if ar.RunMode != "form" {
		respondValidationError(w, r, "Fields can only be configured for form-mode asset reports")
		return
	}
	if !h.validateAssetReportFormBinding(w, r, user.ID, ar) {
		return
	}

	fields, ok := decodeChannelJSON[[]models.AssetReportField](w, r)
	if !ok {
		return
	}
	if !sanitizeAssetReportFields(w, r, fields) {
		return
	}
	available, err := h.availableFieldsForAssetReport(ar)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := validatePublicFormFieldSchema(assetReportFieldSchemas(fields), available); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	if err := h.repo.ReplaceFields(assetReportID, fields); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionAssetReportFieldsUpdate, logger.ResourceAssetReport,
			&assetReportID, "",
			map[string]any{"field_count": len(fields)},
		)
	}

	h.GetFields(w, r)
}

// GetAvailableFields returns fields available to bind on a form-mode asset report.
func (h *AssetReportHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
	ar, user, ok := h.requireManagedAssetReport(w, r)
	if !ok {
		return
	}
	if ar.RunMode != "form" {
		respondJSONOK(w, []AvailableField{})
		return
	}
	if !h.validateAssetReportFormBinding(w, r, user.ID, ar) {
		return
	}

	fields, err := h.availableFieldsForAssetReport(ar)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}
