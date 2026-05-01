package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/services"
)

// Portal-side approval endpoints. Customers (and internal users reaching the
// portal via portal_customers.user_id) decide on approvals where they're in
// the active pool. Authentication is handled upstream by RequirePortalAuth;
// these handlers pull the active customer id via getAuthFromContext.
//
// The active-pool snapshot is the gate (just like /api/approvals/{id}/decide
// after slice 4 loosened item.view): if the customer isn't in the pool,
// ApprovalService returns "actor is not an active approver" → 4xx.

// GetMyApprovals lists pending approvals where the authenticated portal
// customer (or a customer-linked internal user) is in the active pool.
//
// GET /portal/{slug}/approvals/mine
func (h *PortalHandler) GetMyApprovals(w http.ResponseWriter, r *http.Request) {
	if h.approvalService == nil {
		respondServiceUnavailable(w, r, "approvals not configured")
		return
	}

	customerID, ok := h.requirePortalCustomerID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	requests, err := h.approvalService.GetForPortalCustomer(customerID, status)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if requests == nil {
		requests = []*models.ApprovalRequest{}
	}
	respondJSONOK(w, requests)
}

// GetApproval returns a single approval request for the authenticated portal
// customer. Visibility gate: the customer must be in the snapshot pool of any
// step on the request.
//
// GET /portal/{slug}/approvals/{id}
func (h *PortalHandler) GetApproval(w http.ResponseWriter, r *http.Request) {
	if h.approvalService == nil {
		respondServiceUnavailable(w, r, "approvals not configured")
		return
	}

	customerID, ok := h.requirePortalCustomerID(w, r)
	if !ok {
		return
	}
	requestID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, err := h.approvalService.GetRequest(requestID)
	if err != nil {
		respondNotFound(w, r, "Approval request")
		return
	}
	if !portalCustomerCanViewRequest(customerID, req) {
		respondNotFound(w, r, "Approval request")
		return
	}
	respondJSONOK(w, req)
}

// portalDecideRequest is the JSON payload for the portal-side decide.
type portalDecideRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

// DecideAsPortalCustomer records a decision from a portal customer.
//
// POST /portal/{slug}/approvals/{id}/decide
func (h *PortalHandler) DecideAsPortalCustomer(w http.ResponseWriter, r *http.Request) {
	if h.approvalService == nil {
		respondServiceUnavailable(w, r, "approvals not configured")
		return
	}

	customerID, ok := h.requirePortalCustomerID(w, r)
	if !ok {
		return
	}
	requestID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var body portalDecideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondValidationError(w, r, "Invalid request body")
		return
	}
	switch body.Decision {
	case models.ApprovalDecisionApprove, models.ApprovalDecisionReject, models.ApprovalDecisionComment:
	default:
		respondValidationError(w, r, "decision must be 'approve', 'reject', or 'comment'")
		return
	}

	decision, req, err := h.approvalService.DecideAsCustomer(r.Context(), requestID, customerID, body.Decision, body.Comment, services.DecideOptions{})
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	respondJSONOK(w, map[string]any{
		"decision": decision,
		"request":  req,
	})
}

// requirePortalCustomerID resolves the active portal customer for the request
// (via portal session OR internal session with a customer.user_id link). Returns
// 401 if neither is present. Internal-only callers (no customer link) get 401
// here — they should use /api/approvals/* instead.
func (h *PortalHandler) requirePortalCustomerID(w http.ResponseWriter, r *http.Request) (int, bool) {
	internalUserID, customerID := h.getAuthFromContext(r)
	if customerID != nil {
		return *customerID, true
	}
	// Fall back to looking up a customer row linked to the internal user.
	if internalUserID != nil {
		var cid int
		err := h.db.QueryRow(`SELECT id FROM portal_customers WHERE user_id = ? LIMIT 1`, *internalUserID).Scan(&cid)
		if err == nil && cid > 0 {
			return cid, true
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			respondInternalError(w, r, err)
			return 0, false
		}
	}
	respondUnauthorized(w, r)
	return 0, false
}

// portalCustomerCanViewRequest returns true if the customer is in any step's
// approver pool — same gate as the internal userCanViewRequest helper.
func portalCustomerCanViewRequest(customerID int, req *models.ApprovalRequest) bool {
	for _, si := range req.StepInstances {
		for _, app := range si.Approvers {
			if app.PortalCustomerID != nil && *app.PortalCustomerID == customerID {
				return true
			}
		}
	}
	return false
}
