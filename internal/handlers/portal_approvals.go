package handlers

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"windshift/internal/models"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

var errPortalApprovalActorUnauthorized = errors.New("portal approval actor is not authenticated")

// Portal approval access is limited to active approver pools.

// beginPortalApprovalAction resolves the portal and the linked approval
// actor shared by every portal approval endpoint. It writes the
// 401/403/503/404 responses itself; callers defer the returned cancel.
func (h *PortalHandler) beginPortalApprovalAction(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, models.Channel, portalApprovalActor, bool) {
	if h.approvalService == nil {
		respondServiceUnavailable(w, r, "approvals not configured")
		return nil, func() {}, models.Channel{}, portalApprovalActor{}, false
	}

	ctx, cancel, channel, _, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return nil, cancel, models.Channel{}, portalApprovalActor{}, false
	}

	// The actor lookup reads the request context, so rebind the bounded
	// context onto the request before resolving linked identities.
	actor, ok := h.requirePortalApprovalActor(w, r.WithContext(ctx))
	if !ok {
		cancel()
		return nil, cancel, models.Channel{}, portalApprovalActor{}, false
	}
	return ctx, cancel, channel, actor, true
}

// GetMyApprovals lists an actor's pending portal approvals.
func (h *PortalHandler) GetMyApprovals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, actor, ok := h.beginPortalApprovalAction(w, r)
	if !ok {
		return
	}
	defer cancel()

	status := r.URL.Query().Get("status")
	requests, err := h.getApprovalsForPortalActor(ctx, actor, status, channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if requests == nil {
		requests = []*models.ApprovalRequest{}
	}
	respondJSONOK(w, requests)
}

// resolvePortalApprovalRequest loads an approval request scoped to the
// portal's channel and visible to the actor. Writes a 404 when the request
// does not exist in this channel, the actor cannot view it, or its workspace
// was removed from the portal's served scope.
func (h *PortalHandler) resolvePortalApprovalRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, channelID int, actor portalApprovalActor) (*models.ApprovalRequest, int, bool) {
	requestID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, 0, false
	}
	req, err := h.approvalService.GetRequestInChannel(ctx, requestID, channelID)
	if err != nil || !portalActorCanViewRequest(actor, req) {
		respondNotFound(w, r, "Approval request")
		return nil, 0, false
	}
	served, err := h.portalService.PortalServesWorkspace(ctx, channelID, req.WorkspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, false
	}
	if !served {
		respondNotFound(w, r, "Approval request")
		return nil, 0, false
	}
	return req, requestID, true
}

// GetApproval returns an approval visible to the portal actor.
func (h *PortalHandler) GetApproval(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, actor, ok := h.beginPortalApprovalAction(w, r)
	if !ok {
		return
	}
	defer cancel()

	req, _, ok := h.resolvePortalApprovalRequest(ctx, w, r, channel.ID, actor)
	if !ok {
		return
	}
	respondJSONOK(w, req)
}

// portalDecideRequest is the JSON payload for the portal-side decide.
type portalDecideRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

// DecideAsPortalCustomer records a portal actor's decision.
func (h *PortalHandler) DecideAsPortalCustomer(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, actor, ok := h.beginPortalApprovalAction(w, r)
	if !ok {
		return
	}
	defer cancel()

	scopedRequest, requestID, ok := h.resolvePortalApprovalRequest(ctx, w, r, channel.ID, actor)
	if !ok {
		return
	}

	var body portalDecideRequest
	if !decodeChannelRequest(w, r, &body, false) {
		return
	}
	// Portal comments share the internal approval-timeline policy.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &body.Comment, Policy: sanitize.RichText, Label: "Comment"},
	)
	switch body.Decision {
	case models.ApprovalDecisionApprove, models.ApprovalDecisionReject, models.ApprovalDecisionComment:
	default:
		respondValidationError(w, r, "decision must be 'approve', 'reject', or 'comment'")
		return
	}

	var (
		decision *models.ApprovalDecision
		req      *models.ApprovalRequest
		err      error
	)
	decideOptions := services.DecideOptions{ChannelID: &channel.ID}
	switch {
	case actor.userID != nil && portalActorCanActAsUser(actor, scopedRequest):
		decision, req, err = h.approvalService.Decide(ctx, requestID, *actor.userID, body.Decision, body.Comment, decideOptions)
	case actor.customerID != nil:
		decision, req, err = h.approvalService.DecideAsCustomer(ctx, requestID, *actor.customerID, body.Decision, body.Comment, decideOptions)
	case actor.userID != nil:
		decision, req, err = h.approvalService.Decide(ctx, requestID, *actor.userID, body.Decision, body.Comment, decideOptions)
	}
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	resp := map[string]any{
		"decision": decision,
		"request":  req,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	respondJSONOK(w, resp)
}

type portalApprovalActor struct {
	userID     *int
	customerID *int
}

// requirePortalApprovalActor retains linked user and customer identities so
// approvals addressed to either active-pool entry remain visible.
func (h *PortalHandler) requirePortalApprovalActor(w http.ResponseWriter, r *http.Request) (portalApprovalActor, bool) {
	actor, err := h.portalApprovalActorFromRequest(r)
	if errors.Is(err, errPortalApprovalActorUnauthorized) {
		respondUnauthorized(w, r)
		return portalApprovalActor{}, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return portalApprovalActor{}, false
	}
	return actor, true
}

func (h *PortalHandler) portalApprovalActorFromRequest(r *http.Request) (portalApprovalActor, error) {
	internalUserID, customerID := h.getAuthFromContext(r)
	actor := portalApprovalActor{userID: internalUserID, customerID: customerID}
	if internalUserID != nil && customerID == nil {
		cid, err := h.portalService.GetCustomerIDForUser(r.Context(), *internalUserID)
		if err == nil && cid > 0 {
			actor.customerID = &cid
		} else if err != nil && !errors.Is(err, services.ErrPortalCustomerNotFound) {
			return portalApprovalActor{}, err
		}
	}
	if actor.userID == nil && actor.customerID == nil {
		return portalApprovalActor{}, errPortalApprovalActorUnauthorized
	}
	return actor, nil
}

func (h *PortalHandler) getApprovalsForPortalActor(ctx context.Context, actor portalApprovalActor, status string, channelID int) ([]*models.ApprovalRequest, error) {
	// Resolve the portal's served-workspace scope once and drop requests whose
	// workspace was removed. The approval list is an alternate read path to the
	// same items, so it must honor the same restriction as request access.
	served, err := h.portalService.PortalServedWorkspaceIDs(ctx, channelID)
	if err != nil {
		return nil, err
	}
	byID := map[int]*models.ApprovalRequest{}
	add := func(requests []*models.ApprovalRequest) {
		for _, req := range requests {
			if portalServedWorkspace(served, req.WorkspaceID) {
				byID[req.ID] = req
			}
		}
	}
	if actor.customerID != nil {
		requests, err := h.approvalService.GetForPortalCustomerInChannel(ctx, *actor.customerID, status, channelID)
		if err != nil {
			return nil, err
		}
		add(requests)
	}
	if actor.userID != nil {
		requests, err := h.approvalService.GetForUserInChannel(ctx, *actor.userID, status, channelID)
		if err != nil {
			return nil, err
		}
		add(requests)
	}
	out := make([]*models.ApprovalRequest, 0, len(byID))
	for _, req := range byID {
		out = append(out, req)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// portalServedWorkspace applies the portal's served-workspace scope. An empty
// list means the restriction is disabled (legacy config).
func portalServedWorkspace(served []int, workspaceID int) bool {
	if len(served) == 0 {
		return true
	}
	for _, id := range served {
		if id == workspaceID {
			return true
		}
	}
	return false
}

// portalActorCanViewRequest checks the approver pool. While a request is
// pending only an active approver on a started pending step may read it; after
// it resolves, prior approvers retain read access to the history.
func portalActorCanViewRequest(actor portalApprovalActor, req *models.ApprovalRequest) bool {
	if req.Status == models.ApprovalRequestStatusPending {
		for _, si := range req.StepInstances {
			if si.Status != models.ApprovalStepStatusPending || si.StartedAt == nil {
				continue
			}
			for _, app := range si.Approvers {
				if app.IsActive && portalActorMatchesApprover(actor, app) {
					return true
				}
			}
		}
		return false
	}
	for _, si := range req.StepInstances {
		for _, app := range si.Approvers {
			if portalActorMatchesApprover(actor, app) {
				return true
			}
		}
	}
	return false
}

func portalActorCanActAsUser(actor portalApprovalActor, req *models.ApprovalRequest) bool {
	if actor.userID == nil {
		return false
	}
	for _, si := range req.StepInstances {
		if si.Status != models.ApprovalStepStatusPending || si.StartedAt == nil {
			continue
		}
		for _, app := range si.Approvers {
			if app.IsActive && app.UserID != nil && *app.UserID == *actor.userID {
				return true
			}
		}
	}
	return false
}

func portalActorMatchesApprover(actor portalApprovalActor, app models.ApprovalStepApprover) bool {
	if actor.customerID != nil && app.PortalCustomerID != nil && *app.PortalCustomerID == *actor.customerID {
		return true
	}
	return actor.userID != nil && app.UserID != nil && *app.UserID == *actor.userID
}
