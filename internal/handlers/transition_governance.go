package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"windshift/internal/database"
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
	db database.Database
}

func NewTransitionGovernanceHandler(db database.Database) *TransitionGovernanceHandler {
	return &TransitionGovernanceHandler{db: db}
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

	var fromID sql.NullInt64
	var toID int
	var fromName sql.NullString
	var toName string
	err := h.db.QueryRow(`
		SELECT wt.from_status_id, wt.to_status_id, fs.name AS from_name, ts.name AS to_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON fs.id = wt.from_status_id
		JOIN statuses ts ON ts.id = wt.to_status_id
		WHERE wt.id = ?
	`, id).Scan(&fromID, &toID, &fromName, &toName)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "Transition")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	resp := transitionGovernanceResponse{
		TransitionID:    id,
		ToStatusID:      toID,
		ToStatusName:    toName,
		Conditions:      []conditionTouch{},
		ApprovalDrivers: []approvalDriver{},
	}
	if fromID.Valid {
		f := int(fromID.Int64)
		resp.FromStatusID = &f
	}
	if fromName.Valid {
		resp.FromStatusName = fromName.String
	}

	// Conditions targeting this transition (any mode).
	condRows, err := h.db.Query(`
		SELECT cs.id, cs.name, COUNT(c.id) as condition_count
		FROM condition_set_transitions cst
		JOIN condition_sets cs ON cs.id = cst.condition_set_id
		LEFT JOIN conditions c ON c.condition_set_transition_id = cst.id
		WHERE cst.transition_id = ?
		GROUP BY cs.id, cs.name
		ORDER BY cs.name
	`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for condRows.Next() {
		var ct conditionTouch
		if err := condRows.Scan(&ct.ConditionSetID, &ct.ConditionSetName, &ct.ConditionCount); err != nil {
			_ = condRows.Close()
			respondInternalError(w, r, err)
			return
		}
		resp.Conditions = append(resp.Conditions, ct)
	}
	_ = condRows.Close()

	// Approval sets driving this transition (either approve or deny role).
	apprRows, err := h.db.Query(`
		SELECT aset.id, aset.name, ass.id, 'approve_transition_id' AS role
		FROM approval_set_statuses ass
		JOIN approval_sets aset ON aset.id = ass.approval_set_id
		WHERE ass.approve_transition_id = ?
		UNION ALL
		SELECT aset.id, aset.name, ass.id, 'deny_transition_id' AS role
		FROM approval_set_statuses ass
		JOIN approval_sets aset ON aset.id = ass.approval_set_id
		WHERE ass.deny_transition_id = ?
		ORDER BY 2
	`, id, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for apprRows.Next() {
		var ad approvalDriver
		if err := apprRows.Scan(&ad.ApprovalSetID, &ad.ApprovalSetName, &ad.ApprovalSetStatusID, &ad.Role); err != nil {
			_ = apprRows.Close()
			respondInternalError(w, r, err)
			return
		}
		resp.ApprovalDrivers = append(resp.ApprovalDrivers, ad)
	}
	_ = apprRows.Close()

	respondJSONOK(w, resp)
}
