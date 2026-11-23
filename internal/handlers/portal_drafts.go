package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// draftIdentityFromContext converts the (internalUserID, portalCustomerID)
// returned by getAuthFromContext into a DraftIdentity. The middleware
// guarantees exactly one is non-nil for portalAuth routes; if neither is, the
// caller writes a 401 and bails.
func (h *PortalHandler) draftIdentityFromContext(r *http.Request) (repository.DraftIdentity, bool) {
	userID, customerID := h.getAuthFromContext(r)
	if userID == nil && customerID == nil {
		return repository.DraftIdentity{}, false
	}
	return repository.DraftIdentity{
		PortalCustomerID: customerID,
		UserID:           userID,
	}, true
}

// resolveDraftPortal resolves the portal slug and the draft identity shared by
// all draft endpoints. It writes the 404/401 responses itself; callers defer
// the returned cancel.
func (h *PortalHandler) resolveDraftPortal(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, models.Channel, repository.DraftIdentity, bool) {
	ctx, cancel, channel, _, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return nil, cancel, models.Channel{}, repository.DraftIdentity{}, false
	}
	identity, ok := h.draftIdentityFromContext(r)
	if !ok {
		cancel()
		respondUnauthorized(w, r)
		return nil, cancel, models.Channel{}, repository.DraftIdentity{}, false
	}
	return ctx, cancel, channel, identity, true
}

// resolveDraftRequestType validates that a request type exists, lives in the
// given portal channel, and is visible to the caller. Same shape as the
// equivalent check in SubmitToPortal — uses 404 instead of 403 to avoid
// leaking existence. Without the visibility check, a customer could create,
// read, or delete drafts against hidden request types by guessing the ID,
// which would also confirm the request type exists.
func (h *PortalHandler) resolveDraftRequestType(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	channelID, requestTypeID int,
) bool {
	_, ok := h.resolveVisibleRequestType(ctx, w, r, channelID, requestTypeID)
	return ok
}

// SaveDraft upserts the caller's in-progress form state for one request type.
//
//	POST /portal/{slug}/drafts
//	body: { request_type_id, title, description, custom_fields, current_step }
//
// Returns the saved draft (id + updated_at + echoed payload). Re-saving
// overwrites the existing row — there is at most one draft per
// (identity, request_type).
func (h *PortalHandler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, identity, ok := h.resolveDraftPortal(w, r)
	if !ok {
		return
	}
	defer cancel()

	var body struct {
		RequestTypeID *int           `json:"request_type_id"`
		Title         string         `json:"title"`
		Description   string         `json:"description"`
		CustomFields  map[string]any `json:"custom_fields"`
		CurrentStep   int            `json:"current_step"`
	}
	if !decodeChannelRequest(w, r, &body, false) {
		return
	}
	// Drafts are submitted by portal customers and stored; both fields
	// surface in the portal draft preview and on the eventual ticket
	// once submitted. CustomFields is an opaque JSON blob — sanitized
	// when the draft is promoted to an item, not here.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &body.Title, Policy: sanitize.PlainTextField, Label: "Title"},
		sanitize.Pair{Target: &body.Description, Policy: sanitize.RichText, Label: "Description"},
	)
	if body.RequestTypeID == nil {
		respondValidationError(w, r, "request_type_id is required")
		return
	}
	if ok := h.resolveDraftRequestType(ctx, w, r, channel.ID, *body.RequestTypeID); !ok {
		return
	}
	if body.CurrentStep < 1 {
		body.CurrentStep = 1
	}

	draft, err := h.draftRepo.Upsert(ctx, channel.ID, *body.RequestTypeID, identity, repository.PortalRequestDraftPayload{
		Title:             body.Title,
		Description:       body.Description,
		CustomFieldValues: body.CustomFields,
		CurrentStep:       body.CurrentStep,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	resp := draftResponse(draft)
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	respondJSONOK(w, resp)
}

// GetMyDrafts returns every draft the caller has open in this portal,
// newest-first. One row per request type.
//
//	GET /portal/{slug}/drafts
func (h *PortalHandler) GetMyDrafts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, identity, ok := h.resolveDraftPortal(w, r)
	if !ok {
		return
	}
	defer cancel()

	summaries, err := h.draftRepo.ListByIdentityForChannel(ctx, channel.ID, identity)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	userGroupIDs, customerOrgID := h.callerVisibility(ctx, r, channel.ID)
	visible := make([]repository.PortalRequestDraftSummary, 0, len(summaries))
	for _, summary := range summaries {
		requestType, loadErr := h.getRequestTypeWithVisibility(ctx, summary.RequestTypeID)
		if loadErr != nil || requestType.ChannelID != channel.ID {
			continue
		}
		if requestType.IsVisibleTo(userGroupIDs, customerOrgID) {
			visible = append(visible, summary)
		}
	}

	respondJSONOK(w, visible)
}

// GetDraftByRequestType returns the caller's draft for a single request type
// in this portal. 404 if no draft exists (used by the form modal on open to
// detect "no draft to resume" without ceremony).
//
//	GET /portal/{slug}/drafts/{requestTypeId}
func (h *PortalHandler) GetDraftByRequestType(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, identity, ok := h.resolveDraftPortal(w, r)
	if !ok {
		return
	}
	defer cancel()

	requestTypeID, ok := requireIDParam(w, r, "requestTypeId")
	if !ok {
		return
	}

	if ok := h.resolveDraftRequestType(ctx, w, r, channel.ID, requestTypeID); !ok {
		return
	}

	draft, err := h.draftRepo.GetByIdentity(ctx, requestTypeID, identity)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if draft == nil {
		respondNotFound(w, r, "draft")
		return
	}

	respondJSONOK(w, draftResponse(draft))
}

// DeleteDraft removes the caller's draft for a request type. 404 if no draft
// exists. Called by "Start fresh" in the modal and by SubmitToPortal once the
// item has been created.
//
//	DELETE /portal/{slug}/drafts/{requestTypeId}
func (h *PortalHandler) DeleteDraft(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, identity, ok := h.resolveDraftPortal(w, r)
	if !ok {
		return
	}
	defer cancel()

	requestTypeID, ok := requireIDParam(w, r, "requestTypeId")
	if !ok {
		return
	}

	// Deletion is deliberately independent of the request type's current
	// active/visibility state. The draft itself is owner- and channel-scoped,
	// so a customer can remove stale data after access to its form is revoked.
	if err := h.draftRepo.DeleteByIdentity(ctx, channel.ID, requestTypeID, identity); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "draft")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteDraftAfterSubmit is the fire-and-forget counterpart used by
// SubmitToPortal once the item is successfully created. Errors are logged via
// the underlying repository return; the submission already succeeded so we do
// not surface failures to the client.
func (h *PortalHandler) deleteDraftAfterSubmit(ctx context.Context, channelID, requestTypeID int, identity repository.DraftIdentity) {
	if h.draftRepo == nil {
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = h.draftRepo.DeleteByIdentity(dctx, channelID, requestTypeID, identity)
}

func draftResponse(d *repository.PortalRequestDraft) map[string]any {
	resp := map[string]any{
		"id":                  d.ID,
		"channel_id":          d.ChannelID,
		"request_type_id":     d.RequestTypeID,
		"title":               d.Title,
		"description":         d.Description,
		"custom_field_values": d.CustomFieldValues,
		"current_step":        d.CurrentStep,
		"created_at":          d.CreatedAt,
		"updated_at":          d.UpdatedAt,
	}
	if d.PortalCustomerID != nil {
		resp["portal_customer_id"] = *d.PortalCustomerID
	}
	if d.UserID != nil {
		resp["user_id"] = *d.UserID
	}
	return resp
}
