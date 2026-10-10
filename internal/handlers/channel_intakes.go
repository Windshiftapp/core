package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/email"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// intakeRequest is the create/update payload for a mailbox intake.
type intakeRequest struct {
	Folder                string `json:"folder"`
	PortalChannelID       *int   `json:"portal_channel_id"`
	WorkspaceID           int    `json:"workspace_id"`
	ItemTypeID            *int   `json:"item_type_id"`
	RateLimitPerHour      *int   `json:"rate_limit_per_hour"`
	ProcessingDisposition string `json:"processing_disposition"`
	Status                string `json:"status"`
}

// loadMailboxChannel resolves the path channel and confirms it is an inbound
// email channel. Returns nil when the channel is missing or the wrong type.
func (h *ChannelHandler) loadMailboxChannel(ctx context.Context, channelID int) (*models.Channel, error) {
	channel, err := h.service.GetByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.Type != "email" || channel.Direction != "inbound" {
		return nil, nil
	}
	return channel, nil
}

// portalServesWorkspace reports whether the portal's config lists the workspace
// as a target. An intake's routing workspace must be one the linked portal
// exposes, otherwise the customer could never see the ticket.
func (h *ChannelHandler) portalServesWorkspace(ctx context.Context, portalChannelID, workspaceID int) (bool, error) {
	raw, err := h.service.GetConfig(ctx, portalChannelID)
	if err != nil {
		return false, err
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return false, fmt.Errorf("parse portal config: %w", err)
	}
	for _, id := range cfg.PortalWorkspaceIDs {
		if id == workspaceID {
			return true, nil
		}
	}
	return false, nil
}

// authorizeIntakeTarget enforces the permission rule for an intake: the actor
// must manage the mailbox (channelMgmt middleware), the routing workspace, and,
// when linked, the portal. System admins bypass the target checks.
func (h *ChannelHandler) authorizeIntakeTarget(ctx context.Context, actorID, workspaceID int, portalChannelID *int) error {
	if workspaceID <= 0 {
		return fmt.Errorf("%w: workspace_id is required", errIntakeInvalid)
	}
	if portalChannelID != nil {
		portal, err := h.service.GetByID(ctx, *portalChannelID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		if portal == nil || portal.Type != "portal" || portal.Direction != "inbound" {
			return fmt.Errorf("%w: portal_channel_id must reference an inbound portal channel", errIntakeInvalid)
		}
	}

	admin, err := h.permissionService.IsSystemAdmin(actorID)
	if err != nil {
		return err
	}
	if admin {
		return nil
	}

	allowed, err := h.permissionService.HasWorkspacePermission(actorID, workspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		return err
	}
	if !allowed {
		return errIntakeTargetForbidden
	}
	if portalChannelID != nil {
		canManage, err := h.service.UserCanManage(ctx, actorID, *portalChannelID)
		if err != nil {
			return err
		}
		if !canManage {
			return errIntakeTargetForbidden
		}
	}
	return nil
}

var (
	errIntakeTargetForbidden = errors.New("permission to manage the intake target is required")
	errIntakeInvalid         = errors.New("invalid intake")
)

// validateIntakeRequest normalizes and validates the payload, then confirms the
// actor may bind the target.
func (h *ChannelHandler) validateIntakeRequest(ctx context.Context, actorID, mailboxID, intakeID int, req *intakeRequest) (*models.Intake, error) {
	folder := strings.TrimSpace(req.Folder)
	if folder == "" {
		return nil, fmt.Errorf("%w: folder is required", errIntakeInvalid)
	}
	if strings.ContainsAny(folder, "\r\n") {
		return nil, fmt.Errorf("%w: folder must not contain line breaks", errIntakeInvalid)
	}
	if req.WorkspaceID <= 0 {
		return nil, fmt.Errorf("%w: workspace_id is required", errIntakeInvalid)
	}
	if req.ItemTypeID == nil || *req.ItemTypeID <= 0 {
		return nil, fmt.Errorf("%w: item_type_id is required", errIntakeInvalid)
	}
	if !email.IsValidEmailDisposition(req.ProcessingDisposition) {
		return nil, fmt.Errorf("%w: processing_disposition must be leave, mark_read, or delete", errIntakeInvalid)
	}
	if req.RateLimitPerHour != nil && *req.RateLimitPerHour < 0 {
		return nil, fmt.Errorf("%w: rate_limit_per_hour must be 0 (unlimited) or a positive number", errIntakeInvalid)
	}
	status := req.Status
	if status == "" {
		status = models.IntakeStatusEnabled
	}
	if status != models.IntakeStatusEnabled && status != models.IntakeStatusDisabled {
		return nil, fmt.Errorf("%w: status must be enabled or disabled", errIntakeInvalid)
	}

	allowed, err := h.service.ItemTypeAllowedInWorkspace(req.WorkspaceID, *req.ItemTypeID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("%w: item type %d is not allowed in workspace %d", errIntakeInvalid, *req.ItemTypeID, req.WorkspaceID)
	}
	if req.PortalChannelID != nil {
		served, err := h.portalServesWorkspace(ctx, *req.PortalChannelID, req.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if !served {
			return nil, fmt.Errorf("%w: portal %d does not serve workspace %d", errIntakeInvalid, *req.PortalChannelID, req.WorkspaceID)
		}
	}

	if err := h.authorizeIntakeTarget(ctx, actorID, req.WorkspaceID, req.PortalChannelID); err != nil {
		return nil, err
	}

	taken, err := repository.NewIntakeRepository(h.channelRepo.DB()).MailboxFolderTaken(ctx, mailboxID, folder, intakeID)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, repository.ErrDuplicateEntry
	}

	return &models.Intake{
		ID:                    intakeID,
		MailboxID:             mailboxID,
		Folder:                folder,
		WorkspaceID:           req.WorkspaceID,
		PortalChannelID:       req.PortalChannelID,
		ItemTypeID:            req.ItemTypeID,
		RateLimitPerHour:      req.RateLimitPerHour,
		ProcessingDisposition: req.ProcessingDisposition,
		Status:                status,
	}, nil
}

// ListChannelIntakes returns the intakes reading from a mailbox channel.
func (h *ChannelHandler) ListChannelIntakes(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	channel, err := h.loadMailboxChannel(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}
	intakes, err := repository.NewIntakeRepository(h.channelRepo.DB()).ListByMailbox(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, intakes)
}

// CreateChannelIntake adds an intake to a mailbox channel.
func (h *ChannelHandler) CreateChannelIntake(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	channel, err := h.loadMailboxChannel(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}

	req, ok := decodeChannelJSON[intakeRequest](w, r)
	if !ok {
		return
	}
	intake, err := h.validateIntakeRequest(ctx, user.ID, channelID, 0, &req)
	if err != nil {
		h.respondIntakeError(w, r, err)
		return
	}
	id, err := repository.NewIntakeRepository(h.channelRepo.DB()).Create(ctx, intake)
	if err != nil {
		h.respondIntakeError(w, r, err)
		return
	}
	intake.ID = id
	h.auditor.LogWithDetails(r, user, "channel_intake_create", "channel_intake", &id, channel.Name, map[string]any{
		"mailbox_id":        channelID,
		"folder":            intake.Folder,
		"workspace_id":      intake.WorkspaceID,
		"portal_channel_id": intake.PortalChannelID,
	})
	respondJSONCreated(w, intake)
}

// UpdateChannelIntake edits an intake on a mailbox channel.
func (h *ChannelHandler) UpdateChannelIntake(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	intakeID, ok := requireIDParam(w, r, "intakeId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	channel, err := h.loadMailboxChannel(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}

	repo := repository.NewIntakeRepository(h.channelRepo.DB())
	existing, err := repo.GetByID(ctx, intakeID)
	if errors.Is(err, repository.ErrNotFound) || (existing != nil && existing.MailboxID != channelID) {
		respondNotFound(w, r, "intake")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	req, ok := decodeChannelJSON[intakeRequest](w, r)
	if !ok {
		return
	}
	intake, err := h.validateIntakeRequest(ctx, user.ID, channelID, intakeID, &req)
	if err != nil {
		h.respondIntakeError(w, r, err)
		return
	}
	if err := repo.Update(ctx, intake); err != nil {
		h.respondIntakeError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, "channel_intake_update", "channel_intake", &intakeID, channel.Name, map[string]any{
		"mailbox_id":        channelID,
		"folder":            intake.Folder,
		"workspace_id":      intake.WorkspaceID,
		"portal_channel_id": intake.PortalChannelID,
	})
	respondJSONOK(w, intake)
}

// DeleteChannelIntake removes an intake from a mailbox channel.
func (h *ChannelHandler) DeleteChannelIntake(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	intakeID, ok := requireIDParam(w, r, "intakeId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	channel, err := h.loadMailboxChannel(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}

	repo := repository.NewIntakeRepository(h.channelRepo.DB())
	existing, err := repo.GetByID(ctx, intakeID)
	if errors.Is(err, repository.ErrNotFound) || (existing != nil && existing.MailboxID != channelID) {
		respondNotFound(w, r, "intake")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	// Deleting an intake is a write to its targets, so it needs the same
	// workspace/portal permission create/update enforce.
	if err := h.authorizeIntakeTarget(ctx, user.ID, existing.WorkspaceID, existing.PortalChannelID); err != nil {
		h.respondIntakeError(w, r, err)
		return
	}
	if err := repo.Delete(ctx, intakeID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "intake")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	h.auditor.Log(r, user, "channel_intake_delete", "channel_intake", &intakeID, channel.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ChannelHandler) respondIntakeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrDuplicateEntry):
		respondConflict(w, r, "Another intake already reads this folder on the mailbox")
	case errors.Is(err, errIntakeTargetForbidden):
		respondForbidden(w, r)
	case errors.Is(err, errIntakeInvalid):
		respondValidationError(w, r, err.Error())
	default:
		respondInternalError(w, r, err)
	}
}
