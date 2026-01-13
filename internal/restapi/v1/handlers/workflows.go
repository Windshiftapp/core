package handlers

import (
	"database/sql"
	"net/http"
	"strconv"


	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
)

// WorkflowHandler handles public API requests for workflows
type WorkflowHandler struct {
	db database.Database
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(db database.Database) *WorkflowHandler {
	return &WorkflowHandler{db: db}
}

// WorkflowResponse is the public API representation of a Workflow
type WorkflowResponse struct {
	ID          int                    `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	IsDefault   bool                   `json:"is_default"`
	Transitions []dto.TransitionResponse `json:"transitions,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// List handles GET /rest/api/v1/workflows
func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows
		ORDER BY name
	`)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}
	defer rows.Close()

	var workflows []WorkflowResponse
	for rows.Next() {
		var wf WorkflowResponse
		var description sql.NullString
		rows.Scan(&wf.ID, &wf.Name, &description, &wf.IsDefault, &wf.CreatedAt, &wf.UpdatedAt)
		if description.Valid {
			wf.Description = description.String
		}
		workflows = append(workflows, wf)
	}

	restapi.RespondOK(w, workflows)
}

// Get handles GET /rest/api/v1/workflows/{id}
func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workflow ID"))
		return
	}

	var wf WorkflowResponse
	var description sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows WHERE id = ?
	`, id).Scan(&wf.ID, &wf.Name, &description, &wf.IsDefault, &wf.CreatedAt, &wf.UpdatedAt)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	if description.Valid {
		wf.Description = description.String
	}

	// Check for expand=transitions
	expand := restapi.ParseExpand(r)
	if expand.WorkflowTransitions {
		transitions, _ := h.getWorkflowTransitions(id)
		wf.Transitions = transitions
	}

	restapi.RespondOK(w, wf)
}

// GetTransitions handles GET /rest/api/v1/workflows/{id}/transitions
func (h *WorkflowHandler) GetTransitions(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid workflow ID"))
		return
	}

	// Check workflow exists
	var exists int
	err = h.db.QueryRow("SELECT 1 FROM workflows WHERE id = ?", id).Scan(&exists)
	if err == sql.ErrNoRows {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	transitions, err := h.getWorkflowTransitions(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	restapi.RespondOK(w, transitions)
}

func (h *WorkflowHandler) getWorkflowTransitions(workflowID int) ([]dto.TransitionResponse, error) {
	rows, err := h.db.Query(`
		SELECT wt.id, wt.from_status_id, wt.to_status_id,
		       fs.name as from_status_name, ts.name as to_status_name,
		       fsc.name as from_category_name, fsc.color as from_category_color,
		       tsc.name as to_category_name, tsc.color as to_category_color
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		LEFT JOIN status_categories fsc ON fs.category_id = fsc.id
		JOIN status_categories tsc ON ts.category_id = tsc.id
		WHERE wt.workflow_id = ?
		ORDER BY wt.display_order
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []dto.TransitionResponse
	for rows.Next() {
		var t dto.TransitionResponse
		var fromStatusID sql.NullInt64
		var fromStatusName, fromCategoryName, fromCategoryColor sql.NullString
		var toCategoryName, toCategoryColor string

		rows.Scan(&t.ID, &fromStatusID, &t.ToStatusID,
			&fromStatusName, &t.ToStatus.Name,
			&fromCategoryName, &fromCategoryColor,
			&toCategoryName, &toCategoryColor)

		if fromStatusID.Valid {
			id := int(fromStatusID.Int64)
			t.FromStatusID = &id
			t.FromStatus = &dto.StatusSummary{
				ID:            id,
				Name:          fromStatusName.String,
				CategoryName:  fromCategoryName.String,
				CategoryColor: fromCategoryColor.String,
			}
		}

		t.ToStatus = &dto.StatusSummary{
			ID:            t.ToStatusID,
			CategoryName:  toCategoryName,
			CategoryColor: toCategoryColor,
		}

		transitions = append(transitions, t)
	}

	return transitions, nil
}
