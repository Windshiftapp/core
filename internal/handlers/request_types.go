package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// sanitizeRequestType sanitizes request-type fields and returns warnings.
// The system intake Kind is server-owned: admin requests can never set it, so
// only EnsureEmailRequestType can mint a system Email request type.
func sanitizeRequestType(rt *models.RequestType) []string {
	rt.Kind = ""
	return sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &rt.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &rt.Description, Policy: sanitize.RichText, Label: "Description"},
		sanitize.Pair{Target: &rt.Icon, Policy: sanitize.ShortIdentifier, Label: "Icon"},
		sanitize.Pair{Target: &rt.Color, Policy: sanitize.ShortIdentifier, Label: "Color"},
		sanitize.Pair{Target: &rt.TitleTemplate, Policy: sanitize.PlainTextField, Label: "Title template"},
	)
}

// sanitizeRequestTypeFields sanitizes portal field overrides without
// materializing optional string-pointer values.
func sanitizeRequestTypeFields(fields []models.RequestTypeField) []string {
	var warnings []string
	for i := range fields {
		w := sanitize.ApplyAllWithWarnings(
			sanitize.Pair{Target: &fields[i].FieldIdentifier, Policy: sanitize.ShortIdentifier, Label: "Field identifier"},
			sanitize.Pair{Target: &fields[i].FieldType, Policy: sanitize.ShortIdentifier, Label: "Field type"},
			sanitize.Pair{Target: fields[i].DisplayName, Policy: sanitize.PlainTextField, Label: "Field display name"},
			sanitize.Pair{Target: fields[i].Description, Policy: sanitize.RichText, Label: "Field help text"},
			sanitize.Pair{Target: fields[i].VirtualFieldType, Policy: sanitize.ShortIdentifier, Label: "Virtual field type"},
		)
		warnings = append(warnings, w...)
	}
	return warnings
}

func requestTypeFieldSchemas(fields []models.RequestTypeField) []publicFormFieldSchema {
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

type RequestTypeHandler struct {
	repo           *repository.RequestTypeRepository
	channelRepo    *repository.ChannelRepository
	screenRepo     *repository.ScreenRepository
	itemTypeRepo   *repository.ItemTypeRepository
	auditor        *logger.Auditor
	channelService *services.ChannelService
}

var errChannelDoesNotSupportRequestTypes = errors.New("channel does not support request types")

func NewRequestTypeHandler(
	repo *repository.RequestTypeRepository,
	channelRepo *repository.ChannelRepository,
	screenRepo *repository.ScreenRepository,
	itemTypeRepo *repository.ItemTypeRepository,
	auditor *logger.Auditor,
	channelService *services.ChannelService,
) *RequestTypeHandler {
	return &RequestTypeHandler{
		repo:           repo,
		channelRepo:    channelRepo,
		channelService: channelService,
		screenRepo:     screenRepo,
		itemTypeRepo:   itemTypeRepo,
		auditor:        auditor,
	}
}

func (h *RequestTypeHandler) ListVisibleFields(ctx context.Context, userID, requestTypeID int) ([]models.RequestTypeField, error) {
	requestType, err := h.repo.GetByID(requestTypeID)
	if err != nil {
		return nil, err
	}
	allowed, err := h.channelService.UserCanManage(ctx, userID, requestType.ChannelID)
	if err != nil || !allowed {
		return []models.RequestTypeField{}, err
	}
	fields, err := h.repo.ListFields(requestTypeID)
	if fields == nil {
		fields = []models.RequestTypeField{}
	}
	return fields, err
}

func channelSupportsRequestTypes(channel *models.Channel) bool {
	return channel != nil && channel.Direction == "inbound" && (channel.Type == "portal" || channel.Type == "form")
}

func channelSupportsAssetReports(channel *models.Channel) bool {
	return channel != nil && channel.Direction == "inbound" && channel.Type == "portal"
}

// GetAllForChannel returns a channel's request types.
func (h *RequestTypeHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "channel")
		return
	}

	requestTypes, err := h.repo.ListByChannel(channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, requestTypes)
}

// Get returns a request type by ID.
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

	// Do not disclose request types outside the owning channel's manager scope.
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

// channelServedWorkspaceIDs uses only the owning channel type's workspace list.
// Mixing portal and form lists could validate a workspace the runtime ignores.
func (h *RequestTypeHandler) channelServedWorkspaceIDs(ctx context.Context, channelID int) ([]int, error) {
	channel, err := h.channelService.GetByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !channelSupportsRequestTypes(channel) {
		return nil, fmt.Errorf("%w: channel %d", errChannelDoesNotSupportRequestTypes, channelID)
	}
	cfgStr, err := h.channelRepo.GetConfig(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var cfg models.ChannelConfig
	if strings.TrimSpace(cfgStr) != "" {
		if err := json.Unmarshal([]byte(cfgStr), &cfg); err != nil {
			return nil, fmt.Errorf("parse channel %d config: %w", channelID, err)
		}
	}
	switch channel.Type {
	case "portal":
		return append([]int(nil), cfg.PortalWorkspaceIDs...), nil
	case "form":
		return append([]int(nil), cfg.FormWorkspaceIDs...), nil
	default:
		return nil, fmt.Errorf("%w: channel %d", errChannelDoesNotSupportRequestTypes, channelID)
	}
}

func (h *RequestTypeHandler) availableFieldsForRequestType(rt *models.RequestType) ([]AvailableField, error) {
	return availableCreateFields(h.screenRepo, rt.WorkspaceID, rt.ItemTypeID)
}

// validateRequestTypeRouting requires an inbound portal/form channel, a pinned
// workspace served by that channel, and an item type allowed there.
func (h *RequestTypeHandler) validateRequestTypeRouting(w http.ResponseWriter, r *http.Request, channelID int, rt *models.RequestType) bool {
	served, err := h.channelServedWorkspaceIDs(r.Context(), channelID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			respondValidationError(w, r, "Channel not found")
		case errors.Is(err, errChannelDoesNotSupportRequestTypes):
			respondValidationError(w, r, "Channel does not support request types")
		default:
			respondInternalError(w, r, err)
		}
		return false
	}
	if rt.WorkspaceID == nil {
		respondValidationError(w, r, "Workspace is required for this request type")
		return false
	}
	workspaceID := *rt.WorkspaceID
	if !containsID(served, workspaceID) {
		respondValidationError(w, r, "Workspace is not served by this channel")
		return false
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	canConnect, err := h.channelService.UserCanConnectWorkspace(user.ID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !canConnect {
		respondForbidden(w, r)
		return false
	}
	allowed, err := h.repo.ItemTypeAllowedInWorkspace(workspaceID, rt.ItemTypeID)
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

// validateRequestTypeBasics validates the fields shared by create and update.
func (h *RequestTypeHandler) validateRequestTypeBasics(w http.ResponseWriter, r *http.Request, rt *models.RequestType) bool {
	if strings.TrimSpace(rt.Name) == "" {
		respondValidationError(w, r, "Request type name is required")
		return false
	}
	if rt.ItemTypeID == 0 {
		respondValidationError(w, r, "Item type ID is required")
		return false
	}

	itemTypeExists, err := h.itemTypeRepo.Exists(rt.ItemTypeID)
	if err != nil || !itemTypeExists {
		respondValidationError(w, r, "Item type not found")
		return false
	}
	return true
}

// Create creates a request type.
func (h *RequestTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	rt, ok := decodeChannelJSON[models.RequestType](w, r)
	if !ok {
		return
	}
	warnings := sanitizeRequestType(&rt)

	rt.ChannelID = channelID

	if !h.validateRequestTypeBasics(w, r, &rt) {
		return
	}
	if !h.validateRequestTypeRouting(w, r, rt.ChannelID, &rt) {
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
			logger.ActionRequestTypeCreate, logger.ResourceRequestType,
			&rt.ID, rt.Name,
			map[string]any{
				"channel_id":     rt.ChannelID,
				"item_type_id":   rt.ItemTypeID,
				"icon":           rt.Icon,
				"color":          rt.Color,
				"title_template": rt.TitleTemplate,
			},
		)
	}

	respondJSONCreated(w, struct {
		models.RequestType
		Warnings []string `json:"warnings,omitempty"`
	}{rt, warnings})
}

// requestTypeUpdateRequest is the partial body accepted by Update. Pointer
// fields distinguish "omitted" from "set to zero", so a rename from the form
// builder cannot clear the title template, reset display_order, or deactivate
// the form.
type requestTypeUpdateRequest struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	ItemTypeID    *int    `json:"item_type_id"`
	Icon          *string `json:"icon"`
	Color         *string `json:"color"`
	DisplayOrder  *int    `json:"display_order"`
	IsActive      *bool   `json:"is_active"`
	TitleTemplate *string `json:"title_template"`
	WorkspaceID   *int    `json:"workspace_id"`
}

// Update changes a request type within its URL-scoped channel. Omitted fields
// keep their stored values. The workspace and item type are fixed at creation;
// any change is rejected. System Email intake rows may still change their item
// type.
func (h *RequestTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	existing, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing.ChannelID != channelID {
		respondNotFound(w, r, "request_type")
		return
	}

	payload, ok := decodeChannelJSON[requestTypeUpdateRequest](w, r)
	if !ok {
		return
	}

	// Overlay only the supplied fields onto the stored row. This is what keeps a
	// routine metadata edit from silently clearing unrelated persisted values.
	rt := *existing
	if payload.Name != nil {
		rt.Name = *payload.Name
	}
	if payload.Description != nil {
		rt.Description = *payload.Description
	}
	if payload.Icon != nil {
		rt.Icon = *payload.Icon
	}
	if payload.Color != nil {
		rt.Color = *payload.Color
	}
	if payload.DisplayOrder != nil {
		rt.DisplayOrder = *payload.DisplayOrder
	}
	if payload.IsActive != nil {
		rt.IsActive = *payload.IsActive
	}
	if payload.TitleTemplate != nil {
		rt.TitleTemplate = *payload.TitleTemplate
	}

	// workspace_id and item_type_id are fixed at creation: changing either
	// invalidates the fields configured against them.
	if payload.WorkspaceID != nil {
		if existing.WorkspaceID == nil || *payload.WorkspaceID != *existing.WorkspaceID {
			respondValidationError(w, r, "Request type workspace cannot be changed after creation")
			return
		}
	}
	if payload.ItemTypeID != nil {
		if *payload.ItemTypeID != existing.ItemTypeID && existing.Kind != models.RequestTypeKindEmail {
			respondValidationError(w, r, "Request type item type cannot be changed after creation")
			return
		}
	}

	warnings := sanitizeRequestType(&rt)

	if !h.validateRequestTypeBasics(w, r, &rt) {
		return
	}
	if !h.validateRequestTypeRouting(w, r, channelID, &rt) {
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
		details := make(map[string]any)
		if existing.Name != rt.Name {
			details["name_changed"] = map[string]any{"old": existing.Name, "new": rt.Name}
		}
		if existing.ItemTypeID != rt.ItemTypeID {
			details["item_type_changed"] = map[string]any{"old": existing.ItemTypeID, "new": rt.ItemTypeID}
		}
		if existing.Icon != rt.Icon {
			details["icon_changed"] = map[string]any{"old": existing.Icon, "new": rt.Icon}
		}
		if existing.Color != rt.Color {
			details["color_changed"] = map[string]any{"old": existing.Color, "new": rt.Color}
		}
		if existing.TitleTemplate != rt.TitleTemplate {
			details["title_template_changed"] = map[string]any{"old": existing.TitleTemplate, "new": rt.TitleTemplate}
		}

		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionRequestTypeUpdate, logger.ResourceRequestType,
			&rt.ID, rt.Name, details,
		)
	}

	respondJSONOK(w, struct {
		models.RequestType
		Warnings []string `json:"warnings,omitempty"`
	}{rt, warnings})
}

// Delete removes a request type from its URL-scoped channel.
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
			logger.ActionRequestTypeDelete, logger.ResourceRequestType,
			&id, requestTypeName,
			map[string]any{
				"channel_id": channelID,
			},
		)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetFields returns a request type's fields.
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

// UpdateFields replaces fields for a request type in its URL-scoped channel.
func (h *RequestTypeHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	rt, err := h.repo.GetByID(requestTypeID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && rt.ChannelID != channelID) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	fields, ok := decodeChannelJSON[[]models.RequestTypeField](w, r)
	if !ok {
		return
	}
	// The legacy GetFields response cannot surface sanitization warnings.
	_ = sanitizeRequestTypeFields(fields)
	available, err := h.availableFieldsForRequestType(rt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := validatePublicFormFieldSchema(requestTypeFieldSchemas(fields), available); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	if err := h.repo.ReplaceFields(requestTypeID, fields); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionRequestTypeFieldsUpdate, logger.ResourceRequestType,
			&requestTypeID, "",
			map[string]any{
				"field_count": len(fields),
			},
		)
	}

	h.GetFields(w, r)
}

// GetAvailableFields resolves the request type's create-screen fields,
// falling back to title and description when no screen applies.
func (h *RequestTypeHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
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

	fields, err := h.availableFieldsForRequestType(rt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}

// UpdateVisibility updates a URL-scoped request type's visibility.
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
	if !decodeChannelRequest(w, r, &req, false) {
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
			logger.ActionRequestTypeVisibilityUpdate, logger.ResourceRequestType,
			&rt.ID, rt.Name,
			map[string]any{
				"visibility_group_ids": rt.VisibilityGroupIDs,
				"visibility_org_ids":   rt.VisibilityOrgIDs,
			},
		)
	}

	respondJSONOK(w, *rt)
}
