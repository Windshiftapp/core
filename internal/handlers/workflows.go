package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkflowHandler struct {
	repo            *repository.WorkflowRepository
	auditor         *logger.Auditor
	workflowService *services.WorkflowService
	translations    *objecttranslation.Service
}

func (h *WorkflowHandler) WithObjectTranslations(service *objecttranslation.Service) *WorkflowHandler {
	h.translations = service
	return h
}

// SetWorkflowService sets the workflow service for cache invalidation
func (h *WorkflowHandler) SetWorkflowService(ws *services.WorkflowService) {
	h.workflowService = ws
}

func NewWorkflowHandler(repo *repository.WorkflowRepository, auditor *logger.Auditor) *WorkflowHandler {
	return &WorkflowHandler{repo: repo, auditor: auditor}
}

func (h *WorkflowHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if r.URL.Query().Get("include_transitions") == "true" {
		transitions, err := h.repo.ListAllTransitions()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		workflowIndexes := make(map[int]int, len(workflows))
		for i := range workflows {
			workflows[i].Transitions = []models.WorkflowTransition{}
			workflowIndexes[workflows[i].ID] = i
		}
		for _, transition := range transitions {
			if index, ok := workflowIndexes[transition.WorkflowID]; ok {
				workflows[index].Transitions = append(workflows[index].Transitions, transition)
			}
		}
	}

	slog.Info("workflows listed", "count", len(workflows))
	if !localizeObjectResponse(w, r, h.translations, "workflow", workflows) {
		return
	}
	respondJSONOK(w, workflows)
}

func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	workflow, err := h.repo.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workflow")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load transitions for this workflow
	transitions, err := h.repo.ListTransitions(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	workflow.Transitions = transitions
	if !localizeObjectResponse(w, r, h.translations, "workflow", workflow) {
		return
	}

	respondJSONOK(w, workflow)
}

func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	workflow, ok := decodeJSON[models.Workflow](w, r)
	if !ok {
		return
	}
	// Workflow Name is the user-facing label on the workflow editor +
	// config-set picker; Description renders in the workflow directory.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &workflow.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &workflow.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	// Validate required fields
	if strings.TrimSpace(workflow.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Check if name already exists
	exists, err := h.repo.NameExists(workflow.Name)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Workflow with this name already exists")
		return
	}

	id, err := h.repo.Create(workflow.Name, workflow.Description, workflow.IsDefault)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	slog.Info("workflow created", "id", id, "name", workflow.Name)

	// Return the created workflow
	createdWorkflow, err := h.repo.Get(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load transitions (will be empty for new workflow)
	transitions, err := h.repo.ListTransitions(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	createdWorkflow.Transitions = transitions

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionWorkflowCreate, logger.ResourceWorkflow, &id, workflow.Name)
	}
	if !localizeObjectResponse(w, r, h.translations, "workflow", createdWorkflow) {
		return
	}

	respondJSONCreated(w, struct {
		models.Workflow
		Warnings []string `json:"warnings,omitempty"`
	}{*createdWorkflow, warnings})
}

func (h *WorkflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	workflow, ok := decodeJSON[models.Workflow](w, r)
	if !ok {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &workflow.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &workflow.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	// Validate required fields
	if strings.TrimSpace(workflow.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Check if name already exists (excluding current record)
	exists, err := h.repo.NameExistsExcluding(workflow.Name, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Workflow with this name already exists")
		return
	}

	if err := h.repo.Update(id, workflow.Name, workflow.Description, workflow.IsDefault); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated workflow
	updatedWorkflow, err := h.repo.Get(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load transitions
	transitions, err := h.repo.ListTransitions(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	updatedWorkflow.Transitions = transitions

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionWorkflowUpdate, logger.ResourceWorkflow, &id, workflow.Name)
	}

	// Invalidate initial status cache so new items get the correct initial status
	if h.workflowService != nil {
		h.workflowService.InvalidateInitialStatusCache()
	}
	if !localizeObjectResponse(w, r, h.translations, "workflow", updatedWorkflow) {
		return
	}

	respondJSONOK(w, struct {
		models.Workflow
		Warnings []string `json:"warnings,omitempty"`
	}{*updatedWorkflow, warnings})
}

func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Check if any configuration sets are using this workflow
	configCount, err := h.repo.ConfigurationSetCount(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if configCount > 0 {
		respondConflict(w, r, "Cannot delete workflow that is in use by configuration sets")
		return
	}

	cancelledApprovalIDs, err := h.repo.Delete(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		if len(cancelledApprovalIDs) > 0 {
			h.auditor.LogEvent(logger.AuditEvent{
				UserID:       currentUser.ID,
				Username:     currentUser.Username,
				IPAddress:    utils.GetClientIP(r),
				UserAgent:    r.UserAgent(),
				ActionType:   logger.ActionWorkflowDelete,
				ResourceType: logger.ResourceWorkflow,
				ResourceID:   &id,
				Details: map[string]any{
					"canceled_approval_request_ids": cancelledApprovalIDs,
					"cancellation_reason":           "workflow_deleted",
				},
				Success: true,
			})
		} else {
			h.auditor.Log(r, currentUser, logger.ActionWorkflowDelete, logger.ResourceWorkflow, &id, "")
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTransitions returns the transitions for a workflow.
func (h *WorkflowHandler) GetTransitions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	transitions, err := h.repo.ListTransitions(workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, transitions)
}

func (h *WorkflowHandler) UpdateTransitions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	transitions, ok := decodeJSON[[]models.WorkflowTransition](w, r)
	if !ok {
		return
	}

	cancelledApprovalIDs, err := h.repo.ReplaceTransitions(workflowID, transitions)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrTransitionToStatusRequired):
			respondValidationError(w, r, "To status ID is required for all transitions")
		case errors.Is(err, repository.ErrTransitionToStatusNotFound):
			respondValidationError(w, r, "To status not found")
		case errors.Is(err, repository.ErrTransitionFromStatusNotFound):
			respondValidationError(w, r, "From status not found")
		default:
			respondInternalError(w, r, err)
		}
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := map[string]any{"update_type": "transitions"}
		if len(cancelledApprovalIDs) > 0 {
			details["canceled_approval_request_ids"] = cancelledApprovalIDs
			details["cancellation_reason"] = "transition_removed"
		}
		h.auditor.LogEvent(logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWorkflowUpdate,
			ResourceType: logger.ResourceWorkflow,
			ResourceID:   &workflowID,
			Details:      details,
			Success:      true,
		})
	}

	// Invalidate initial status cache so new items get the correct initial status
	if h.workflowService != nil {
		h.workflowService.InvalidateInitialStatusCache()
	}

	updatedTransitions, err := h.repo.ListTransitions(workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updatedTransitions)
}

func (h *WorkflowHandler) GetAvailableTransitions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	statusID, ok := requireIDParam(w, r, "statusID")
	if !ok {
		return
	}

	transitions, err := h.repo.ListAvailableTransitions(workflowID, statusID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, transitions)
}
