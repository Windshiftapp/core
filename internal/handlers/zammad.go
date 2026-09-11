package handlers

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"windshift/internal/integrations/zammad"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type ZammadHandler struct {
	itemRepo          *repository.ItemRepository
	service           *services.ZammadService
	permissionService *services.PermissionService
	auditor           *logger.Auditor
	syncAllTrigger    func() bool
}

func NewZammadHandler(itemRepo *repository.ItemRepository, service *services.ZammadService, permissionService *services.PermissionService, auditor *logger.Auditor) *ZammadHandler {
	return &ZammadHandler{itemRepo: itemRepo, service: service, permissionService: permissionService, auditor: auditor}
}

func (h *ZammadHandler) SetSyncAllTrigger(trigger func() bool) { h.syncAllTrigger = trigger }

func (h *ZammadHandler) ListConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := h.service.ListConnections()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, connections)
}

func (h *ZammadHandler) GetConnection(w http.ResponseWriter, r *http.Request) {
	connection, err := h.service.GetConnection(r.PathValue("id"))
	if !h.respondServiceError(w, r, err) {
		return
	}
	respondJSONOK(w, connection)
}

func (h *ZammadHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.CreateZammadConnectionRequest](w, r)
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	connection, err := h.service.CreateConnection(req, user.ID)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, user, logger.ActionZammadConnectionCreate, logger.ResourceZammadConnection, connection.ProviderID, connection.Name, map[string]any{"workspace_ids": connection.WorkspaceIDs, "all_workspaces": connection.AppliesToAllWorkspaces})
	respondJSONCreated(w, connection)
}

func (h *ZammadHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.UpdateZammadConnectionRequest](w, r)
	if !ok {
		return
	}
	connection, err := h.service.UpdateConnection(r.PathValue("id"), req)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, currentUser(r), logger.ActionZammadConnectionUpdate, logger.ResourceZammadConnection, connection.ProviderID, connection.Name, map[string]any{"token_rotated": req.APIToken != nil && *req.APIToken != ""})
	respondJSONOK(w, connection)
}

func (h *ZammadHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	connection, err := h.service.GetConnection(id)
	if !h.respondServiceError(w, r, err) {
		return
	}
	if !h.respondServiceError(w, r, h.service.DeleteConnection(id)) {
		return
	}
	h.audit(r, currentUser(r), logger.ActionZammadConnectionDelete, logger.ResourceZammadConnection, id, connection.Name, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ZammadHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	metadata, err := h.service.TestConnection(r.Context(), r.PathValue("id"))
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, currentUser(r), logger.ActionZammadConnectionTest, logger.ResourceZammadConnection, r.PathValue("id"), "", map[string]any{"groups": len(metadata.Groups), "states": len(metadata.States)})
	respondJSONOK(w, map[string]any{"ok": true, "metadata": metadata})
}

func (h *ZammadHandler) RefreshAllTickets(w http.ResponseWriter, r *http.Request) {
	if h.syncAllTrigger == nil {
		respondServiceUnavailable(w, r, "Zammad synchronization is not available")
		return
	}
	started := h.syncAllTrigger()
	h.audit(r, currentUser(r), logger.ActionZammadTicketRefreshAll, logger.ResourceZammadTicket, "all", "", map[string]any{
		"started": started,
	})
	respondJSON(w, http.StatusAccepted, map[string]bool{"started": started})
}

func (h *ZammadHandler) RetryUncertainTicketCreation(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	link, err := h.service.RetryUncertainTicketCreation(r.Context(), r.PathValue("linkId"), user.ID)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, user, logger.ActionZammadTicketCreate, logger.ResourceZammadTicket, link.ID, link.TicketNumber, map[string]any{
		"item_id": link.ItemID, "connection_id": link.ProviderID, "ticket_id": link.TicketID, "admin_uncertain_override": true,
	})
	respondJSONOK(w, link.Response())
}

// DetachTicketLinkLocally is an administrator-only recovery action. The route
// middleware enforces system-admin access; the handler records the deliberately
// orphan-prone operation so it remains distinguishable from a normal unlink.
func (h *ZammadHandler) DetachTicketLinkLocally(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	link, err := h.service.DetachTicketLinkLocally(r.PathValue("linkId"))
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, user, logger.ActionZammadTicketDetachLocal, logger.ResourceZammadTicket, link.ID, link.TicketNumber, map[string]any{
		"item_id": link.ItemID, "connection_id": link.ProviderID, "ticket_id": link.TicketID,
		"remote_correlation_may_remain": true,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *ZammadHandler) ListWorkspaceConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok || !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	connections, err := h.service.ListConnectionsForWorkspace(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	responses := make([]models.ZammadWorkspaceConnection, 0, len(connections))
	for _, connection := range connections {
		ready := connection.AuthMethod == models.ZammadAuthMethodAPIToken && connection.HasAPIToken
		if connection.AuthMethod == models.ZammadAuthMethodOAuth {
			ready = connection.OAuthConnected && !connection.ReauthorizationRequired
		}
		ready = ready && zammadConnectionHasUsableDefaultGroup(connection)
		responses = append(responses, models.ZammadWorkspaceConnection{
			ProviderID: connection.ProviderID, Name: connection.Name, AuthMethod: connection.AuthMethod,
			Ready: ready, OAuthConnected: connection.OAuthConnected, ReauthorizationRequired: connection.ReauthorizationRequired,
			DefaultGroupID: connection.DefaultGroupID, DefaultGroupName: connection.DefaultGroupName,
			AllowedGroups: connection.AllowedGroups,
		})
	}
	respondJSONOK(w, responses)
}

// zammadConnectionHasUsableDefaultGroup checks only locally persisted
// configuration. A name-based default is usable without an allowlist, but
// cannot be proven to belong to a non-empty allowlist until remote metadata
// resolves it to an ID. Do not present that ambiguous configuration as ready.
func zammadConnectionHasUsableDefaultGroup(connection *models.ZammadConnection) bool {
	if connection.DefaultGroupID <= 0 {
		return strings.TrimSpace(connection.DefaultGroupName) != "" && len(connection.AllowedGroups) == 0
	}
	return len(connection.AllowedGroups) == 0 || slices.ContainsFunc(connection.AllowedGroups, func(group models.ZammadGroupRef) bool {
		return group.ID == connection.DefaultGroupID && strings.TrimSpace(group.Name) != ""
	})
}

func (h *ZammadHandler) GetWorkspaceMetadata(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok || !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	metadata, err := h.service.MetadataForWorkspace(r.Context(), r.PathValue("id"), workspaceID)
	if !h.respondServiceError(w, r, err) {
		return
	}
	respondJSONOK(w, metadata)
}

func (h *ZammadHandler) GetWorkspaceOwners(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok || !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemEdit, h.permissionService) {
		return
	}
	groupID, err := strconv.Atoi(r.URL.Query().Get("group_id"))
	if err != nil || groupID <= 0 {
		respondValidationError(w, r, "group_id must be positive")
		return
	}
	owners, err := h.service.OwnersForWorkspace(r.Context(), r.PathValue("id"), workspaceID, groupID)
	if !h.respondServiceError(w, r, err) {
		return
	}
	respondJSONOK(w, owners)
}

func (h *ZammadHandler) GetItemLinks(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, itemID, models.PermissionItemView) {
		return
	}
	links, err := h.service.TicketLinksForItem(itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	responses := make([]models.ZammadTicketLinkResponse, 0, len(links))
	for _, link := range links {
		responses = append(responses, link.Response())
	}
	respondJSONOK(w, responses)
}

// ResolveTicketLink turns the opaque correlation key stored in Zammad into a
// current Windshift item destination. Item permissions are checked before the
// destination is disclosed so the link cannot be used to enumerate items.
func (h *ZammadHandler) ResolveTicketLink(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	itemID, workspaceID, err := h.service.ResolveTicketLink(r.PathValue("correlationKey"))
	if err != nil {
		respondNotFound(w, r, "Item")
		return
	}
	allowed, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		respondNotFound(w, r, "Item")
		return
	}
	respondJSONOK(w, map[string]int{"workspace_id": workspaceID, "item_id": itemID})
}

func (h *ZammadHandler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[models.CreateZammadTicketRequest](w, r)
	if !ok {
		return
	}
	link, err := h.service.CreateTicket(r.Context(), itemID, user.ID, req)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, user, logger.ActionZammadTicketCreate, logger.ResourceZammadTicket, link.ID, link.TicketNumber, map[string]any{"item_id": link.ItemID, "connection_id": link.ProviderID, "ticket_id": link.TicketID, "sync_state": link.SyncState})
	respondJSONOK(w, link.Response())
}

func (h *ZammadHandler) LinkExistingTicket(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[models.LinkZammadTicketRequest](w, r)
	if !ok {
		return
	}
	link, err := h.service.LinkExistingTicket(r.Context(), itemID, user.ID, req)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, user, logger.ActionZammadTicketLinkExisting, logger.ResourceZammadTicket, link.ID, link.TicketNumber, map[string]any{
		"item_id": link.ItemID, "connection_id": link.ProviderID, "ticket_id": link.TicketID,
	})
	respondJSONCreated(w, link.Response())
}

func (h *ZammadHandler) UpdateTicketLink(w http.ResponseWriter, r *http.Request) {
	link, err := h.service.GetTicketLink(r.PathValue("linkId"))
	if !h.respondServiceError(w, r, err) {
		return
	}
	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, link.ItemID, models.PermissionItemEdit) {
		return
	}
	req, ok := decodeJSON[models.UpdateZammadTicketLinkRequest](w, r)
	if !ok {
		return
	}
	updated, err := h.service.UpdateTicketLink(r.Context(), link.ID, req)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, currentUser(r), logger.ActionZammadTicketUpdate, logger.ResourceZammadTicket, updated.ID, updated.TicketNumber, map[string]any{
		"item_id": updated.ItemID, "connection_id": updated.ProviderID, "ticket_id": updated.TicketID,
		"state_id": updated.LastStatusID, "group_id": updated.GroupID, "owner_id": updated.OwnerID,
	})
	respondJSONOK(w, updated.Response())
}

func (h *ZammadHandler) DeleteTicketLink(w http.ResponseWriter, r *http.Request) {
	link, err := h.service.GetTicketLink(r.PathValue("linkId"))
	if !h.respondServiceError(w, r, err) {
		return
	}
	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, link.ItemID, models.PermissionItemEdit) {
		return
	}
	removed, err := h.service.UnlinkTicket(r.Context(), link.ID)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, currentUser(r), logger.ActionZammadTicketUnlink, logger.ResourceZammadTicket, removed.ID, removed.TicketNumber, map[string]any{
		"item_id": removed.ItemID, "connection_id": removed.ProviderID, "ticket_id": removed.TicketID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *ZammadHandler) RefreshTicket(w http.ResponseWriter, r *http.Request) {
	link, err := h.service.GetTicketLink(r.PathValue("linkId"))
	if !h.respondServiceError(w, r, err) {
		return
	}
	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, link.ItemID, models.PermissionItemEdit) {
		return
	}
	link, err = h.service.SyncTicketLink(r.Context(), link.ID)
	if !h.respondServiceError(w, r, err) {
		return
	}
	h.audit(r, currentUser(r), logger.ActionZammadTicketRefresh, logger.ResourceZammadTicket, link.ID, link.TicketNumber, map[string]any{"item_id": link.ItemID, "connection_id": link.ProviderID, "ticket_id": link.TicketID, "status_id": link.LastStatusID})
	respondJSONOK(w, link.Response())
}

func (h *ZammadHandler) audit(r *http.Request, user *models.User, action, resourceType, externalID, name string, details map[string]any) {
	if h.auditor == nil || user == nil {
		return
	}
	if details == nil {
		details = map[string]any{}
	}
	details["external_id"] = externalID
	h.auditor.LogWithDetails(r, user, action, resourceType, nil, name, details)
}

func currentUser(r *http.Request) *models.User {
	return utils.GetCurrentUser(r)
}

func (h *ZammadHandler) respondServiceError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrCredentialScopeMismatch):
		respondNotFound(w, r, "zammad_connection")
	case errors.Is(err, repository.ErrDuplicateEntry):
		respondConflict(w, r, "A Zammad connection or ticket link with these identifiers already exists")
	case errors.Is(err, repository.ErrConcurrentUpdate):
		respondConflict(w, r, "The Zammad connection changed while it was being updated. Reload and retry the action.")
	case errors.Is(err, services.ErrZammadLinkReservationConflict):
		respondConflict(w, r, "The Zammad connection or item changed while the ticket link was being prepared. Retry the action.")
	case errors.Is(err, services.ErrZammadOAuthSuperseded):
		w.Header().Set("Retry-After", "1")
		respondConflict(w, r, "The Zammad authorization changed while the request was running. Reload and retry the action.")
	case errors.Is(err, services.ErrZammadTicketGroupPolicyChanged):
		respondConflict(w, r, "The Zammad connection no longer allows the ticket's group. Review the connection group policy before retrying.")
	case errors.Is(err, services.ErrZammadConnectionBusy):
		w.Header().Set("Retry-After", "1")
		respondConflict(w, r, "A Zammad ticket operation is in progress. Retry the connection change shortly.")
	default:
		var apiErr *zammad.APIError
		var upstreamErr *zammad.UpstreamError
		var validationErr *services.ZammadValidationError
		var transitionErr *services.TransitionRejection
		if errors.Is(err, services.ErrZammadReauthorizationRequired) {
			respondError(w, r, restapi.NewAPIError(http.StatusConflict, "ZAMMAD_REAUTHORIZATION_REQUIRED", "Zammad authorization must be renewed by a system administrator"))
			return false
		}
		if errors.Is(err, services.ErrZammadOAuthRefreshInProgress) {
			w.Header().Set("Retry-After", "1")
			respondServiceUnavailable(w, r, "Zammad authorization is being refreshed. Retry shortly.")
			return false
		}
		switch {
		case errors.As(err, &validationErr):
			respondValidationError(w, r, validationErr.Error())
		case errors.As(err, &apiErr) || errors.As(err, &upstreamErr):
			respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "ZAMMAD_UPSTREAM_ERROR", "Zammad could not complete the request"))
		case errors.As(err, &transitionErr):
			respondBadRequest(w, r, "The configured Windshift completion transition is not currently allowed")
		default:
			respondInternalError(w, r, err)
		}
	}
	return false
}
