package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type RequestTypeHandler struct {
	repo           *repository.RequestTypeRepository
	channelRepo    *repository.ChannelRepository
	screenRepo     *repository.ScreenRepository
	auditor        *logger.Auditor
	channelService *services.ChannelService
}

func NewRequestTypeHandler(
	repo *repository.RequestTypeRepository,
	channelRepo *repository.ChannelRepository,
	screenRepo *repository.ScreenRepository,
	auditor *logger.Auditor,
	channelService *services.ChannelService,
) *RequestTypeHandler {
	return &RequestTypeHandler{
		repo:           repo,
		channelRepo:    channelRepo,
		channelService: channelService,
		screenRepo:     screenRepo,
		auditor:        auditor,
	}
}

// GetAllForChannel returns all request types for a specific channel
func (h *RequestTypeHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	requestTypes, err := h.repo.ListByChannel(channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, requestTypes)
}

// Get returns a specific request type by ID
func (h *RequestTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rt, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Gate by manager scope on the owning channel. See bughunt2.md Run 6
	// finding #4.
	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, rt.ChannelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "request_type")
		return
	}

	respondJSONOK(w, rt)
}

// Create creates a new request type
func (h *RequestTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	rt, ok := decodeJSON[models.RequestType](w, r)
	if !ok {
		return
	}

	rt.ChannelID = channelID

	if strings.TrimSpace(rt.Name) == "" {
		respondValidationError(w, r, "Request type name is required")
		return
	}
	if rt.ItemTypeID == 0 {
		respondValidationError(w, r, "Item type ID is required")
		return
	}

	channelExists, err := h.repo.ChannelExists(rt.ChannelID)
	if err != nil || !channelExists {
		respondValidationError(w, r, "Channel not found")
		return
	}
	itemTypeExists, err := h.repo.ItemTypeExists(rt.ItemTypeID)
	if err != nil || !itemTypeExists {
		respondValidationError(w, r, "Item type not found")
		return
	}

	if rt.Icon == "" {
		rt.Icon = "FileText"
	}
	if rt.Color == "" {
		rt.Color = "#3b82f6"
	}
	rt.TitleTemplate = strings.TrimSpace(rt.TitleTemplate)
	if rt.DisplayOrder == 0 {
		maxOrder, err := h.repo.MaxDisplayOrder(rt.ChannelID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		rt.DisplayOrder = maxOrder + 1
	}

	nameExists, err := h.repo.NameExistsInChannel(rt.ChannelID, rt.Name, 0)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if nameExists {
		respondConflict(w, r, "Request type with this name already exists for this channel")
		return
	}

	id, err := h.repo.Create(&rt)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Request type with this name already exists for this channel")
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
	rt = *created

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			"request_type_create", "request_type",
			&rt.ID, rt.Name,
			map[string]interface{}{
				"channel_id":     rt.ChannelID,
				"item_type_id":   rt.ItemTypeID,
				"icon":           rt.Icon,
				"color":          rt.Color,
				"title_template": rt.TitleTemplate,
			},
		)
	}

	respondJSONCreated(w, rt)
}

// Update updates an existing request type. The route is
// PUT /channels/{channel_id}/request-types/{id}; channelMgmt middleware gates
// access and the SQL UPDATE is constrained by channel_id so a request type
// belonging to another channel cannot be touched. Body-supplied channel_id
// and workspace_id are ignored — channel_id comes from the URL and
// workspace_id is not mutable via this endpoint.
func (h *RequestTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rt, ok := decodeJSON[models.RequestType](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(rt.Name) == "" {
		respondValidationError(w, r, "Request type name is required")
		return
	}
	if rt.ItemTypeID == 0 {
		respondValidationError(w, r, "Item type ID is required")
		return
	}

	itemTypeExists, err := h.repo.ItemTypeExists(rt.ItemTypeID)
	if err != nil || !itemTypeExists {
		respondValidationError(w, r, "Item type not found")
		return
	}

	rt.TitleTemplate = strings.TrimSpace(rt.TitleTemplate)

	nameExists, err := h.repo.NameExistsInChannel(channelID, rt.Name, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if nameExists {
		respondConflict(w, r, "Request type with this name already exists for this channel")
		return
	}

	if err := h.repo.Update(id, channelID, &rt); err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicateEntry):
			respondConflict(w, r, "Request type with this name already exists for this channel")
		case errors.Is(err, repository.ErrNotFound):
			respondNotFound(w, r, "request_type")
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
	rt = *updated

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})
		if old.Name != rt.Name {
			details["name_changed"] = map[string]interface{}{"old": old.Name, "new": rt.Name}
		}
		if old.ItemTypeID != rt.ItemTypeID {
			details["item_type_changed"] = map[string]interface{}{"old": old.ItemTypeID, "new": rt.ItemTypeID}
		}
		if old.Icon != rt.Icon {
			details["icon_changed"] = map[string]interface{}{"old": old.Icon, "new": rt.Icon}
		}
		if old.Color != rt.Color {
			details["color_changed"] = map[string]interface{}{"old": old.Color, "new": rt.Color}
		}
		if old.TitleTemplate != rt.TitleTemplate {
			details["title_template_changed"] = map[string]interface{}{"old": old.TitleTemplate, "new": rt.TitleTemplate}
		}

		h.auditor.LogWithDetails(r, currentUser,
			"request_type_update", "request_type",
			&rt.ID, rt.Name, details,
		)
	}

	respondJSONOK(w, rt)
}

// Delete deletes a request type. Route is DELETE /channels/{channel_id}/request-types/{id};
// channelMgmt middleware gates and the DELETE is constrained by channel_id so a
// request type belonging to another channel cannot be deleted via this URL.
func (h *RequestTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	requestTypeName, err := h.repo.GetNameForChannel(id, channelID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Best-effort portal-section cleanup before deleting the row itself —
	// strip this request_type's id from every PortalSection.RequestTypeIDs
	// list. The actual load/save dance lives in ChannelRepository.
	h.channelRepo.UpdatePortalSections(channelID, func(cfg *models.ChannelConfig) bool {
		modified := false
		for i := range cfg.PortalSections {
			ids := cfg.PortalSections[i].RequestTypeIDs
			newIDs := make([]int, 0, len(ids))
			for _, v := range ids {
				if v == id {
					modified = true
					continue
				}
				newIDs = append(newIDs, v)
			}
			cfg.PortalSections[i].RequestTypeIDs = newIDs
		}
		return modified
	})

	if err := h.repo.Delete(id, channelID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "request_type")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			"request_type_delete", "request_type",
			&id, requestTypeName,
			map[string]interface{}{
				"channel_id": channelID,
			},
		)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetFields returns all fields for a request type
func (h *RequestTypeHandler) GetFields(w http.ResponseWriter, r *http.Request) {
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rt, err := h.repo.GetByID(requestTypeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, rt.ChannelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "request_type")
		return
	}

	fields, err := h.repo.ListFields(requestTypeID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, fields)
}

// UpdateFields rewrites the field schema for a request type. Route is
// PUT /channels/{channel_id}/request-types/{id}/fields; gated by channelMgmt
// and constrained to request types that belong to the URL-supplied channel.
func (h *RequestTypeHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify request type exists in this channel before mutating any fields.
	if _, err := h.repo.GetNameForChannel(requestTypeID, channelID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "request_type")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	fields, ok := decodeJSON[[]models.RequestTypeField](w, r)
	if !ok {
		return
	}

	if err := h.repo.ReplaceFields(requestTypeID, fields); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			"request_type_fields_update", "request_type",
			&requestTypeID, "",
			map[string]interface{}{
				"field_count": len(fields),
			},
		)
	}

	// Return the updated fields
	h.GetFields(w, r)
}

// GetAvailableFields returns all fields available for a request type based on its item type and workspace.
// Resolves fields via: workspace → workspace_configuration_sets → configuration_set_item_types → create_screen → screen_fields.
// Falls back to default fields (title, description) when workspace_id is not set or no screen is found.
func (h *RequestTypeHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	itemTypeID, workspaceID, err := h.repo.GetItemTypeAndWorkspace(requestTypeID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Always include default fields
	fields := []AvailableField{
		{Identifier: "title", Name: "Title", Type: "default"},
		{Identifier: "description", Name: "Description", Type: "default"},
	}

	if workspaceID == nil {
		respondJSONOK(w, fields)
		return
	}

	createScreenID, err := h.screenRepo.GetCreateScreenID(*workspaceID, itemTypeID)
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

// UpdateVisibility updates only the visibility settings for a request type.
// Route is PUT /channels/{channel_id}/request-types/{id}/visibility — gated by
// channelMgmt and scoped by channel_id in the SQL.
func (h *RequestTypeHandler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
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
			respondNotFound(w, r, "request_type")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	rt, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			"request_type_visibility_update", "request_type",
			&rt.ID, rt.Name,
			map[string]interface{}{
				"visibility_group_ids": rt.VisibilityGroupIDs,
				"visibility_org_ids":   rt.VisibilityOrgIDs,
			},
		)
	}

	respondJSONOK(w, *rt)
}
