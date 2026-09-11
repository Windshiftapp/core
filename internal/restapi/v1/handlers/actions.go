package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/services/actioncatalog"
)

// ActionHandler exposes the bearer-token API surface for workspace-scoped
// automations: catalog discovery, action CRUD, and dry-run validation. It
// shares the same validator and repository as the legacy cookie-auth
// handler in internal/handlers/actions.go — the only differences are auth
// (token scopes) and error shape (structured ValidationErrors list).
type ActionHandler struct {
	BaseHandler
	repo        *repository.ActionRepository
	definitions *services.ActionDefinitionService
}

// NewActionHandler constructs a v1 action handler. actionService is
// optional — when nil the handler skips the cache-invalidation hook and
// relies on the background refresher to pick up changes (acceptable for
// low-volume admin tooling, slower than the cookie path).
func NewActionHandler(db database.Database, permissionService *services.PermissionService, actionService *services.ActionService) *ActionHandler {
	repo := repository.NewActionRepository(db)
	definitions := services.NewActionDefinitionService(repo, actionService)
	return &ActionHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		repo:        repo,
		definitions: definitions,
	}
}

func (h *ActionHandler) SetAssetService(service *services.AssetService) {
	h.definitions.SetAssetService(service)
}

// requireActionManage authenticates, parses the workspace ID from the
// {id} path parameter, and requires the caller hold action.manage on that
// workspace. action.manage is the existing permission gating the cookie-
// auth /api/workspaces/{id}/actions surface; keeping the same gate here
// keeps v1 from accidentally widening the audience.
func (h *ActionHandler) requireActionManage(w http.ResponseWriter, r *http.Request) (userID, workspaceID int, ok bool) {
	user, authOK := h.RequireAuth(w, r)
	if !authOK {
		return 0, 0, false
	}
	wsID, idOK := h.ParsePathID(w, r, "id", "workspace ID")
	if !idOK {
		return 0, 0, false
	}
	allowed, err := h.PermissionService.HasWorkspacePermission(user.ID, wsID, models.PermissionActionManage)
	if err != nil {
		h.RespondInternalError(w, r)
		return 0, 0, false
	}
	if !allowed {
		// 404 to avoid disclosing the workspace's existence to unauthorized
		// callers — same posture as the items / workflows v1 surface.
		h.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return 0, 0, false
	}
	return user.ID, wsID, true
}

// respondValidationErrors writes the catalog validator's structured error
// list as a 400 with code VALIDATION_FAILED. The first error's message is
// surfaced as the top-level human-readable text; the full list is on
// details.errors so clients can render per-field annotations.
func (h *ActionHandler) respondValidationErrors(w http.ResponseWriter, r *http.Request, errs actioncatalog.ValidationErrors) {
	apiErr := restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, errs[0].Message).
		WithDetails(map[string]any{"errors": errs})
	restapi.RespondError(w, r, apiErr)
}

// actionResponse wraps *models.Action with the sanitize warnings the
// handler stamps on create / update responses. The action fields flow
// through as the top-level JSON shape; Warnings (omitempty) lands
// alongside.
type actionResponse struct {
	*models.Action
	Warnings []string `json:"warnings,omitempty"`
}

// actionDefinitionRequest is the wire shape clients send to create/update/
// validate. It mirrors the legacy CreateActionRequest but stays distinct
// so the v1 contract can evolve independently if needed. ActorUserID is
// omitted on purpose — token-authored actions run under the calling user
// by default and impersonation should go through admin tooling, not v1.
type actionDefinitionRequest struct {
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	TriggerType   models.ActionTriggerType `json:"trigger_type"`
	TriggerConfig string                   `json:"trigger_config,omitempty"`
	Nodes         []models.ActionNode      `json:"nodes,omitempty"`
	Edges         []models.ActionEdge      `json:"edges,omitempty"`
}

func (req *actionDefinitionRequest) toDefinition() actioncatalog.ActionDefinition {
	return actioncatalog.ActionDefinition{
		Name:          req.Name,
		Description:   req.Description,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		Nodes:         req.Nodes,
		Edges:         req.Edges,
	}
}

// nodeTypeDTO and triggerTypeDTO are the wire shapes for the catalog
// payload. We don't reuse actioncatalog.NodeTypeMetadata directly because
// that struct intentionally embeds the resolver pointer behind an
// unexported field — the marshaller would still see it via the public
// schema field, but having a clean DTO at the boundary protects the v1
// contract from internal refactors of the metadata struct.
type nodeTypeDTO struct {
	Type         models.ActionNodeType `json:"type"`
	Label        string                `json:"label"`
	Description  string                `json:"description"`
	Category     string                `json:"category"`
	ConfigSchema json.RawMessage       `json:"config_schema" swaggertype:"object"`
	IsIterator   bool                  `json:"is_iterator"`
	Outputs      []string              `json:"outputs"`
}

type triggerTypeDTO struct {
	Type         models.ActionTriggerType `json:"type"`
	Label        string                   `json:"label"`
	Description  string                   `json:"description"`
	ConfigSchema json.RawMessage          `json:"config_schema" swaggertype:"object"`
}

type capabilityDTO struct {
	ID             int                   `json:"id"`
	Name           string                `json:"name"`
	CapabilityType models.CapabilityType `json:"capability_type"`
}

type catalogResponse struct {
	Scope        string           `json:"scope"`
	Triggers     []triggerTypeDTO `json:"triggers"`
	Nodes        []nodeTypeDTO    `json:"nodes"`
	Capabilities []capabilityDTO  `json:"capabilities"`
}

// GetCatalog handles GET /rest/api/v1/workspaces/{id}/action-catalog
//
// @Summary      Action catalog for a workspace
// @Description  Lists every available trigger and node type with its JSON-Schema-typed config, plus the action capabilities (LLM, HTTP, docker) reachable from this workspace. Agents call this to discover what they can build before submitting a new action.
// @Tags         actions
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {object}  handlers.catalogResponse
// @Failure      400  {object}  handlers.ErrorResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the actions:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workspace not found or no action.manage permission"
// @Router       /workspaces/{id}/action-catalog [get]
func (h *ActionHandler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}

	cat := actioncatalog.Default()
	resp := catalogResponse{Scope: "workspace"}

	for _, t := range cat.Triggers() {
		schemaJSON, _ := json.Marshal(t.ConfigSchema)
		resp.Triggers = append(resp.Triggers, triggerTypeDTO{
			Type:         t.Type,
			Label:        t.Label,
			Description:  t.Description,
			ConfigSchema: schemaJSON,
		})
	}
	for _, n := range cat.Nodes() {
		schemaJSON, _ := json.Marshal(n.ConfigSchema)
		resp.Nodes = append(resp.Nodes, nodeTypeDTO{
			Type:         n.Type,
			Label:        n.Label,
			Description:  n.Description,
			Category:     n.Category,
			ConfigSchema: schemaJSON,
			IsIterator:   n.IsIterator,
			Outputs:      n.Outputs,
		})
	}

	caps, err := h.repo.ListCapabilitiesForWorkspace(workspaceID, "")
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	for _, c := range caps {
		resp.Capabilities = append(resp.Capabilities, capabilityDTO{
			ID:             c.ID,
			Name:           c.Name,
			CapabilityType: c.CapabilityType,
		})
	}
	if resp.Capabilities == nil {
		resp.Capabilities = []capabilityDTO{}
	}

	h.RespondOK(w, resp)
}

// ListActions handles GET /rest/api/v1/workspaces/{id}/actions
//
// @Summary      List actions in a workspace
// @Tags         actions
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workspace ID"
// @Success      200  {array}   models.Action
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse
// @Failure      404  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/actions [get]
func (h *ActionHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}
	actions, err := h.repo.ListByWorkspace(workspaceID)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if actions == nil {
		actions = []*models.Action{}
	}
	h.RespondOK(w, actions)
}

// GetAction handles GET /rest/api/v1/workspaces/{id}/actions/{actionId}
//
// @Summary      Get a single action
// @Tags         actions
// @Produce      json
// @Security     BearerAuth
// @Param        id        path      int  true  "Workspace ID"
// @Param        actionId  path      int  true  "Action ID"
// @Success      200  {object}  models.Action
// @Failure      404  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/actions/{actionId} [get]
func (h *ActionHandler) GetAction(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}
	actionID, ok := h.ParsePathID(w, r, "actionId", "action ID")
	if !ok {
		return
	}
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.WorkspaceID != workspaceID) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	h.RespondOK(w, action)
}

// CreateAction handles POST /rest/api/v1/workspaces/{id}/actions
//
// @Summary      Create an action from a JSON definition
// @Description  Agents call this with a full nodes/edges graph. The body is validated against the catalog: every trigger and node config must satisfy its JSON Schema, edges must reference existing nodes, the graph must be acyclic, and iterator bodies must be self-contained. Capability IDs are checked for workspace scope.
// @Tags         actions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Workspace ID"
// @Param        body  body      handlers.actionDefinitionRequest  true  "Action definition"
// @Success      201   {object}  models.Action
// @Failure      400   {object}  handlers.ErrorResponse  "Validation failed — see details.errors"
// @Failure      401   {object}  handlers.ErrorResponse
// @Failure      403   {object}  handlers.ErrorResponse  "Token lacks actions:write"
// @Failure      404   {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/actions [post]
func (h *ActionHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}
	var req actionDefinitionRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	action := &models.Action{
		WorkspaceID:   workspaceID,
		Name:          req.Name,
		Description:   req.Description,
		IsEnabled:     true,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		CreatedBy:     &userID,
	}
	created, validationErrors, err := h.definitions.Create(action, req.Nodes, req.Edges)
	if len(validationErrors) > 0 {
		h.respondValidationErrors(w, r, validationErrors)
		return
	}
	if err != nil {
		if errors.Is(err, services.ErrActionDefinitionInvalid) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, err.Error()))
		} else {
			h.RespondInternalError(w, r)
		}
		return
	}
	if user := middleware.GetUser(r.Context()); user != nil {
		h.Auditor.LogWithDetails(r, user, logger.ActionAutomationCreate, logger.ResourceAutomation, &created.ID, created.Name, map[string]any{"workspace_id": workspaceID})
	}
	h.RespondCreated(w, actionResponse{Action: created, Warnings: warnings})
}

// UpdateAction handles PUT /rest/api/v1/workspaces/{id}/actions/{actionId}
//
// @Summary      Replace an action's definition
// @Description  Validates the new graph against the catalog before persisting. Pass the full action body (the legacy partial-update semantics aren't supported on v1 — agents should fetch, mutate, and PUT back).
// @Tags         actions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id        path      int                              true  "Workspace ID"
// @Param        actionId  path      int                              true  "Action ID"
// @Param        body      body      handlers.actionDefinitionRequest  true  "Updated definition"
// @Success      200       {object}  models.Action
// @Failure      400       {object}  handlers.ErrorResponse
// @Failure      404       {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/actions/{actionId} [put]
func (h *ActionHandler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}
	actionID, ok := h.ParsePathID(w, r, "actionId", "action ID")
	if !ok {
		return
	}
	existing, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && existing.WorkspaceID != workspaceID) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	var req actionDefinitionRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	existing.Name = req.Name
	existing.Description = req.Description
	existing.TriggerType = req.TriggerType
	existing.TriggerConfig = req.TriggerConfig

	updated, validationErrors, err := h.definitions.Save(existing, req.Nodes, req.Edges, true, true)
	if len(validationErrors) > 0 {
		h.respondValidationErrors(w, r, validationErrors)
		return
	}
	if err != nil {
		if errors.Is(err, services.ErrActionDefinitionInvalid) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, err.Error()))
		} else {
			h.RespondInternalError(w, r)
		}
		return
	}
	if user := middleware.GetUser(r.Context()); user != nil {
		h.Auditor.LogWithDetails(r, user, logger.ActionAutomationUpdate, logger.ResourceAutomation, &updated.ID, updated.Name, map[string]any{"workspace_id": workspaceID})
	}
	h.RespondOK(w, actionResponse{Action: updated, Warnings: warnings})
}

// DeleteAction handles DELETE /rest/api/v1/workspaces/{id}/actions/{actionId}.
//
// @Summary      Delete an action
// @Tags         actions
// @Security     BearerAuth
// @Param        id        path  int  true  "Workspace ID"
// @Param        actionId  path  int  true  "Action ID"
// @Success      204
// @Failure      404  {object}  handlers.ErrorResponse
// @Router       /workspaces/{id}/actions/{actionId} [delete]
func (h *ActionHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}
	actionID, ok := h.ParsePathID(w, r, "actionId", "action ID")
	if !ok {
		return
	}
	action, err := h.repo.GetByID(actionID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && action.WorkspaceID != workspaceID) {
		h.RespondNotFound(w, r)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if err := h.definitions.Delete(actionID, workspaceID); err != nil {
		h.RespondInternalError(w, r)
		return
	}
	if user := middleware.GetUser(r.Context()); user != nil {
		h.Auditor.LogWithDetails(r, user, logger.ActionAutomationDelete, logger.ResourceAutomation, &actionID, action.Name, map[string]any{"workspace_id": workspaceID})
	}
	h.RespondNoContent(w)
}

// validateActionResponse is the structured payload returned by the
// /validate endpoint. Errors is empty when the definition would persist
// successfully; clients can rely on len(errors)==0 as the success signal.
type validateActionResponse struct {
	Errors actioncatalog.ValidationErrors `json:"errors"`
}

// ValidateAction handles POST /rest/api/v1/workspaces/{id}/actions/validate
//
// @Summary      Dry-run validate an action definition
// @Description  Runs the same catalog validator the create/update endpoints use, without persisting. Useful for agents that want to iterate on a draft before committing.
// @Tags         actions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Workspace ID"
// @Param        body  body      handlers.actionDefinitionRequest  true  "Definition to validate"
// @Success      200   {object}  handlers.validateActionResponse  "Validation result — errors is empty when the definition would persist"
// @Router       /workspaces/{id}/actions/validate [post]
func (h *ActionHandler) ValidateAction(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireActionManage(w, r)
	if !ok {
		return
	}
	var req actionDefinitionRequest
	if !h.DecodeBodyOrRespond(w, r, &req) {
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText},
	)
	errs, validationErr := h.definitions.Validate(workspaceID, req.toDefinition())
	if validationErr != nil {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, validationErr.Error()))
		return
	}
	if errs == nil {
		errs = actioncatalog.ValidationErrors{}
	}
	h.RespondOK(w, validateActionResponse{Errors: errs})
}
