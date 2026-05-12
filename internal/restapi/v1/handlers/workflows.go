package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/services"
)

// WorkflowHandler handles public API requests for workflows
type WorkflowHandler struct {
	BaseHandler
	workflowService *services.WorkflowService
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(db database.Database, permissionService *services.PermissionService) *WorkflowHandler {
	return &WorkflowHandler{
		BaseHandler:     NewBaseHandler(db, permissionService),
		workflowService: services.NewWorkflowService(db),
	}
}

// WorkflowResponse is the public API representation of a Workflow
type WorkflowResponse struct {
	ID          int                      `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	IsDefault   bool                     `json:"is_default"`
	Transitions []dto.TransitionResponse `json:"transitions,omitempty"`
	CreatedAt   string                   `json:"created_at"`
	UpdatedAt   string                   `json:"updated_at"`
}

// List handles GET /rest/api/v1/workflows
//
// @Summary      List workflows
// @Tags         workflows
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handlers.WorkflowResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workflows:read scope"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workflows [get]
func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	results, err := h.workflowService.List()
	if err != nil {
		h.RespondInternalError(w, r)
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

	h.RespondOK(w, workflows)
}

// Get handles GET /rest/api/v1/workflows/{id}
//
// @Summary      Get a workflow by ID
// @Description  Pass `?expand=transitions` to include the workflow's transitions in the response.
// @Tags         workflows
// @Produce      json
// @Security     BearerAuth
// @Param        id      path      int     true   "Workflow ID"
// @Param        expand  query     string  false  "Comma-separated list of fields to expand (supports 'transitions')"
// @Success      200     {object}  handlers.WorkflowResponse
// @Failure      400     {object}  handlers.ErrorResponse  "Invalid workflow ID"
// @Failure      401     {object}  handlers.ErrorResponse
// @Failure      403     {object}  handlers.ErrorResponse  "Token lacks the workflows:read scope"
// @Failure      404     {object}  handlers.ErrorResponse  "Workflow not found"
// @Failure      500     {object}  handlers.ErrorResponse
// @Router       /workflows/{id} [get]
func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "workflow ID")
	if !ok {
		return
	}

	wfResult, err := h.workflowService.GetByID(id)
	if err != nil {
		h.RespondNotFound(w, r)
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
		if transitions, err := h.getWorkflowTransitions(id); err == nil {
			wf.Transitions = transitions
		}
	}

	h.RespondOK(w, wf)
}

// GetTransitions handles GET /rest/api/v1/workflows/{id}/transitions
//
// @Summary      List a workflow's transitions
// @Tags         workflows
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Workflow ID"
// @Success      200  {array}   dto.TransitionResponse
// @Failure      400  {object}  handlers.ErrorResponse  "Invalid workflow ID"
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the workflows:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Workflow not found"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /workflows/{id}/transitions [get]
func (h *WorkflowHandler) GetTransitions(w http.ResponseWriter, r *http.Request) {
	_, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := h.ParsePathID(w, r, "id", "workflow ID")
	if !ok {
		return
	}

	// Check workflow exists
	exists, err := h.workflowService.Exists(id)
	if err != nil || !exists {
		h.RespondNotFound(w, r)
		return
	}

	transitions, err := h.getWorkflowTransitions(id)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	h.RespondOK(w, transitions)
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
