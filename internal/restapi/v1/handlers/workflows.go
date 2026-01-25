package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// WorkflowHandler handles public API requests for workflows
type WorkflowHandler struct {
	db              database.Database
	workflowService *services.WorkflowService
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(db database.Database) *WorkflowHandler {
	return &WorkflowHandler{
		db:              db,
		workflowService: services.NewWorkflowService(db),
	}
}

// SetWorkflowService allows injecting a configured workflow service
func (h *WorkflowHandler) SetWorkflowService(ws *services.WorkflowService) {
	h.workflowService = ws
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

	results, err := h.workflowService.List()
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrInternalError)
		return
	}

	var workflows []WorkflowResponse
	for _, wf := range results {
		workflows = append(workflows, WorkflowResponse{
			ID:          wf.ID,
			Name:        wf.Name,
			Description: wf.Description,
			IsDefault:   wf.IsDefault,
			CreatedAt:   wf.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   wf.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	if workflows == nil {
		workflows = []WorkflowResponse{}
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

	wfResult, err := h.workflowService.GetByID(id)
	if err != nil {
		restapi.RespondError(w, r, restapi.ErrNotFound)
		return
	}

	wf := WorkflowResponse{
		ID:          wfResult.ID,
		Name:        wfResult.Name,
		Description: wfResult.Description,
		IsDefault:   wfResult.IsDefault,
		CreatedAt:   wfResult.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   wfResult.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
	exists, err := h.workflowService.Exists(id)
	if err != nil || !exists {
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
	results, err := h.workflowService.GetTransitions(workflowID)
	if err != nil {
		return nil, err
	}

	var transitions []dto.TransitionResponse
	for _, t := range results {
		tr := dto.TransitionResponse{
			ID:         t.ID,
			ToStatusID: t.ToStatusID,
			ToStatus: &dto.StatusSummary{
				ID:            t.ToStatusID,
				Name:          t.ToStatusName,
				CategoryName:  t.ToCategoryName,
				CategoryColor: t.ToCategoryColor,
			},
		}

		if t.FromStatusID != nil {
			tr.FromStatusID = t.FromStatusID
			tr.FromStatus = &dto.StatusSummary{
				ID:            *t.FromStatusID,
				Name:          t.FromStatusName,
				CategoryName:  t.FromCategoryName,
				CategoryColor: t.FromCategoryColor,
			}
		}

		transitions = append(transitions, tr)
	}

	if transitions == nil {
		transitions = []dto.TransitionResponse{}
	}

	return transitions, nil
}
