package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/repository"
	"windshift/internal/services"
)

// TransitionGovernanceHandler exposes the per-transition governance lookup that
// powers the FE override-warning UI: which condition sets target this
// transition and which approval sets drive it.
//
// The shape is:
//
//	{
//	  "transition_id": 17,
//	  "from_status_id": 5,
//	  "to_status_id": 7,
//	  "from_status_name": "Review",
//	  "to_status_name": "Approved",
//	  "conditions": [
//	    { "condition_set_id": 3, "condition_set_name": "...", "condition_count": 5 }
//	  ],
//	  "approval_drivers": [
//	    { "approval_set_id": 8, "approval_set_name": "...",
//	      "approval_set_status_id": 12, "role": "approve_transition_id" }
//	  ]
//	}
//
// Both editors call this endpoint as the user picks a transition; the FE
// renders a warning when both lists are non-empty (or, for the condition-set
// editor, when approval_drivers is non-empty for a condition target).
type TransitionGovernanceHandler struct {
	transitionRepo     *repository.TransitionRepository
	approvalSetService *services.ApprovalSetService
}

func NewTransitionGovernanceHandler(transitionRepo *repository.TransitionRepository, approvalSetService *services.ApprovalSetService) *TransitionGovernanceHandler {
	return &TransitionGovernanceHandler{transitionRepo: transitionRepo, approvalSetService: approvalSetService}
}

type conditionTouch struct {
	ConditionSetID   int    `json:"condition_set_id"`
	ConditionSetName string `json:"condition_set_name"`
	ConditionCount   int    `json:"condition_count"`
}

type approvalDriver struct {
	ApprovalSetID       int    `json:"approval_set_id"`
	ApprovalSetName     string `json:"approval_set_name"`
	ApprovalSetStatusID int    `json:"approval_set_status_id"`
	Role                string `json:"role"` // 'approve_transition_id' | 'deny_transition_id'
}

type transitionGovernanceResponse struct {
	TransitionID    int              `json:"transition_id"`
	FromStatusID    *int             `json:"from_status_id"`
	ToStatusID      int              `json:"to_status_id"`
	FromStatusName  string           `json:"from_status_name,omitempty"`
	ToStatusName    string           `json:"to_status_name"`
	Conditions      []conditionTouch `json:"conditions"`
	ApprovalDrivers []approvalDriver `json:"approval_drivers"`
}

// Get handles GET /api/transitions/{id}/governance.
func (h *TransitionGovernanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	transition, err := h.transitionRepo.GetWithStatusNames(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Transition")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	resp := transitionGovernanceResponse{
		TransitionID:    transition.ID,
		FromStatusID:    transition.FromStatusID,
		ToStatusID:      transition.ToStatusID,
		FromStatusName:  transition.FromStatusName,
		ToStatusName:    transition.ToStatusName,
		Conditions:      []conditionTouch{},
		ApprovalDrivers: []approvalDriver{},
	}

	// Conditions targeting this transition (any mode).
	touches, err := h.transitionRepo.ListConditionSetTouches(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for _, t := range touches {
		resp.Conditions = append(resp.Conditions, conditionTouch{
			ConditionSetID:   t.ConditionSetID,
			ConditionSetName: t.ConditionSetName,
			ConditionCount:   t.ConditionCount,
		})
	}

	// Approval sets driving this transition (either approve or deny role).
	drivers, err := h.approvalSetService.FindDriversForTransition(r.Context(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for _, d := range drivers {
		resp.ApprovalDrivers = append(resp.ApprovalDrivers, approvalDriver{
			ApprovalSetID:       d.ApprovalSetID,
			ApprovalSetName:     d.ApprovalSetName,
			ApprovalSetStatusID: d.ApprovalSetStatusID,
			Role:                d.Role,
		})
	}

	respondJSONOK(w, resp)
}
