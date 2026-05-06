package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type AssetReportHandler struct {
	repo        *repository.AssetReportRepository
	channelRepo *repository.ChannelRepository
	screenRepo  *repository.ScreenRepository
	auditor     *logger.Auditor
}

func NewAssetReportHandler(
	repo *repository.AssetReportRepository,
	channelRepo *repository.ChannelRepository,
	screenRepo *repository.ScreenRepository,
	auditor *logger.Auditor,
) *AssetReportHandler {
	return &AssetReportHandler{
		repo:        repo,
		channelRepo: channelRepo,
		screenRepo:  screenRepo,
		auditor:     auditor,
	}
}

// GetAllForChannel returns all asset reports for a specific channel
func (h *AssetReportHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
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

	ar, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, ar)
}

// Create creates a new asset report
func (h *AssetReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	ar, ok := decodeJSON[models.AssetReport](w, r)
	if !ok {
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

	channelExists, err := h.repo.ChannelExists(ar.ChannelID)
	if err != nil || !channelExists {
		respondBadRequest(w, r, "Channel not found")
		return
	}
	assetSetExists, err := h.repo.AssetSetExists(ar.AssetSetID)
	if err != nil || !assetSetExists {
		respondBadRequest(w, r, "Asset set not found")
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
	// Reports default to active on create — JSON's false zero-value collides
	// with "field omitted", so we always activate here. Callers that want to
	// land an inactive report can follow up with PUT (Update preserves the
	// requested is_active value verbatim).
	ar.IsActive = true
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
			"asset_report_create", "asset_report",
			&ar.ID, ar.Name,
			map[string]interface{}{
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
// channel_id and workspace_id are ignored — channel_id comes from the URL,
// and workspace_id is not mutable via this endpoint.
func (h *AssetReportHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	ar, ok := decodeJSON[models.AssetReport](w, r)
	if !ok {
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
		ar.RunMode = "direct"
	}
	if ar.RunMode != "direct" && ar.RunMode != "form" {
		respondValidationError(w, r, "Invalid run_mode")
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
		details := make(map[string]interface{})
		if old.Name != ar.Name {
			details["name_changed"] = map[string]interface{}{"old": old.Name, "new": ar.Name}
		}
		if old.AssetSetID != ar.AssetSetID {
			details["asset_set_changed"] = map[string]interface{}{"old": old.AssetSetID, "new": ar.AssetSetID}
		}
		if old.Icon != ar.Icon {
			details["icon_changed"] = map[string]interface{}{"old": old.Icon, "new": ar.Icon}
		}
		if old.Color != ar.Color {
			details["color_changed"] = map[string]interface{}{"old": old.Color, "new": ar.Color}
		}

		h.auditor.LogWithDetails(r, currentUser,
			"asset_report_update", "asset_report",
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

	// Best-effort portal-section cleanup before deleting the row itself —
	// strip this asset_report's id from every PortalSection.AssetReportIDs
	// list. The actual load/save dance lives in ChannelRepository.
	h.channelRepo.UpdatePortalSections(channelID, func(cfg *models.ChannelConfig) bool {
		modified := false
		for i := range cfg.PortalSections {
			ids := cfg.PortalSections[i].AssetReportIDs
			newIDs := make([]int, 0, len(ids))
			for _, v := range ids {
				if v == id {
					modified = true
					continue
				}
				newIDs = append(newIDs, v)
			}
			cfg.PortalSections[i].AssetReportIDs = newIDs
		}
		return modified
	})

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
			"asset_report_delete", "asset_report",
			&id, assetReportName,
			map[string]interface{}{
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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
			"asset_report_visibility_update", "asset_report",
			&ar.ID, ar.Name,
			map[string]interface{}{
				"visibility_group_ids": ar.VisibilityGroupIDs,
				"visibility_org_ids":   ar.VisibilityOrgIDs,
			},
		)
	}

	respondJSONOK(w, *ar)
}

// GetFields returns all fields for a form-mode asset report.
func (h *AssetReportHandler) GetFields(w http.ResponseWriter, r *http.Request) {
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

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
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if _, err := h.repo.GetNameForChannel(assetReportID, channelID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "asset_report")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	fields, ok := decodeJSON[[]models.AssetReportField](w, r)
	if !ok {
		return
	}

	if err := h.repo.ReplaceFields(assetReportID, fields); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			"asset_report_fields_update", "asset_report",
			&assetReportID, "",
			map[string]interface{}{"field_count": len(fields)},
		)
	}

	h.GetFields(w, r)
}

// GetAvailableFields returns fields available to bind on a form-mode asset report.
func (h *AssetReportHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
	assetReportID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	itemTypeID, workspaceID, err := h.repo.GetItemTypeAndWorkspace(assetReportID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "asset_report")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	fields := []AvailableField{
		{Identifier: "title", Name: "Title", Type: "default"},
		{Identifier: "description", Name: "Description", Type: "default"},
	}

	if workspaceID == nil || itemTypeID == nil {
		respondJSONOK(w, fields)
		return
	}

	createScreenID, err := h.screenRepo.GetCreateScreenID(*workspaceID, *itemTypeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if createScreenID == nil {
		respondJSONOK(w, fields)
		return
	}

	screenFields, err := h.screenRepo.ListFields(*createScreenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for _, sf := range screenFields {
		if sf.FieldType != "custom" {
			continue
		}
		fields = append(fields, AvailableField{
			Identifier: sf.FieldIdentifier,
			Name:       sf.FieldName,
			Type:       "custom",
			FieldType:  sf.CustomFieldType,
		})
	}

	respondJSONOK(w, fields)
}
