package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type TestRunTemplateHandler struct {
	*BaseHandler
}

func NewTestRunTemplateHandlerWithPool(db database.Database) *TestRunTemplateHandler {
	return &TestRunTemplateHandler{
		BaseHandler: NewBaseHandler(db),
	}
}

// GetAll returns all test run templates
func (h *TestRunTemplateHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	templates, err := repository.NewTestRunTemplateRepository(db).FindAll(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, templates)
}

// Get returns a single test run template by ID
func (h *TestRunTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	template, err := repository.NewTestRunTemplateRepository(db).FindByID(id, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_run_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, template)
}

// prepareTemplateWrite extracts the workspace ID, decodes the JSON body, acquires
// read and write DB handles, and verifies the test set belongs to the workspace.
func (h *TestRunTemplateHandler) prepareTemplateWrite(w http.ResponseWriter, r *http.Request) (
	workspaceID int, template models.TestRunTemplate, writeDB database.Database, ok bool,
) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	template, ok = decodeJSON[models.TestRunTemplate](w, r)
	if !ok {
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	writeDB, ok = h.requireWriteDB(w, r)
	if !ok {
		return
	}

	// Verify test set belongs to workspace if provided
	if template.SetID > 0 {
		if !verifyResourceInWorkspace(readDB, w, r, "test_sets", template.SetID, workspaceID, "test_set") {
			ok = false
			return
		}
	}

	ok = true
	return
}

// Create creates a new test run template
func (h *TestRunTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, template, writeDB, ok := h.prepareTemplateWrite(w, r)
	if !ok {
		return
	}

	id, createdAt, err := repository.NewTestRunTemplateRepository(writeDB).Create(workspaceID, &template)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	template.ID = id
	template.WorkspaceID = workspaceID
	template.CreatedAt = createdAt
	template.UpdatedAt = createdAt

	respondJSONCreated(w, template)
}

// Update updates an existing test run template
func (h *TestRunTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, template, writeDB, ok := h.prepareTemplateWrite(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	updatedAt, err := repository.NewTestRunTemplateRepository(writeDB).Update(id, workspaceID, &template)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	template.ID = id
	template.WorkspaceID = workspaceID
	template.UpdatedAt = updatedAt

	respondJSONOK(w, template)
}

// Delete deletes a test run template
func (h *TestRunTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	if err := repository.NewTestRunTemplateRepository(writeDB).Delete(id, workspaceID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetExecutions returns all test runs created from a template
func (h *TestRunTemplateHandler) GetExecutions(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	templateID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	if !verifyResourceInWorkspace(db, w, r, "test_run_templates", templateID, workspaceID, "test_run_template") {
		return
	}

	runs, err := repository.NewTestRunTemplateRepository(db).FindExecutions(templateID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, runs)
}

// Execute creates a new test run from a template
func (h *TestRunTemplateHandler) Execute(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	templateID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	readRepo := repository.NewTestRunTemplateRepository(readDB)

	template, err := readRepo.FindCore(templateID, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_run_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	runCount, err := readRepo.CountExecutions(templateID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	runName := template.Name + " - Run " + strconv.Itoa(runCount+1)

	run, err := repository.NewTestRunTemplateRepository(writeDB).Execute(workspaceID, templateID, template.SetID, runName)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, run)
}
