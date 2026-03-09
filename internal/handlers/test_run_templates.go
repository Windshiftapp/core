package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
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
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT
			trt.id, trt.workspace_id, trt.set_id, trt.name, trt.description, trt.created_at, trt.updated_at,
			ts.name as set_name
		FROM test_run_templates trt
		LEFT JOIN test_sets ts ON trt.set_id = ts.id
		WHERE trt.workspace_id = ?
		ORDER BY trt.id DESC
	`, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	templates := make([]models.TestRunTemplate, 0)
	for rows.Next() {
		var template models.TestRunTemplate
		var setName sql.NullString

		err := rows.Scan(
			&template.ID, &template.WorkspaceID, &template.SetID, &template.Name, &template.Description,
			&template.CreatedAt, &template.UpdatedAt, &setName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if setName.Valid {
			template.SetName = setName.String
		}

		templates = append(templates, template)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(templates)
}

// Get returns a single test run template by ID
func (h *TestRunTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	var template models.TestRunTemplate
	var setName sql.NullString

	err = db.QueryRow(`
		SELECT
			trt.id, trt.workspace_id, trt.set_id, trt.name, trt.description, trt.created_at, trt.updated_at,
			ts.name as set_name
		FROM test_run_templates trt
		LEFT JOIN test_sets ts ON trt.set_id = ts.id
		WHERE trt.id = ? AND trt.workspace_id = ?
	`, id, workspaceID).Scan(
		&template.ID, &template.WorkspaceID, &template.SetID, &template.Name, &template.Description,
		&template.CreatedAt, &template.UpdatedAt, &setName,
	)

	if setName.Valid {
		template.SetName = setName.String
	}

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "test_run_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(template)
}

// Create creates a new test run template
func (h *TestRunTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	var template models.TestRunTemplate
	if err = json.NewDecoder(r.Body).Decode(&template); err != nil {
		respondBadRequest(w, r, err.Error())
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

	// Verify test set belongs to workspace if provided
	if template.SetID > 0 {
		var count int
		err = readDB.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", template.SetID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			respondNotFound(w, r, "test_set")
			return
		}
	}

	now := time.Now()
	var id int64
	err = writeDB.QueryRow(`
		INSERT INTO test_run_templates (workspace_id, set_id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, template.SetID, template.Name, template.Description, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	template.ID = int(id)
	template.WorkspaceID = workspaceID
	template.CreatedAt = now
	template.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(template)
}

// Update updates an existing test run template
func (h *TestRunTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var template models.TestRunTemplate
	if err = json.NewDecoder(r.Body).Decode(&template); err != nil {
		respondBadRequest(w, r, err.Error())
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

	// Verify test set belongs to workspace if provided
	if template.SetID > 0 {
		var count int
		err = readDB.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", template.SetID, workspaceID).Scan(&count)
		if err != nil || count == 0 {
			respondNotFound(w, r, "test_set")
			return
		}
	}

	now := time.Now()
	_, err = writeDB.Exec(`
		UPDATE test_run_templates
		SET set_id = ?, name = ?, description = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, template.SetID, template.Name, template.Description, now, id, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	template.ID = id
	template.WorkspaceID = workspaceID
	template.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(template)
}

// Delete deletes a test run template
func (h *TestRunTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err = writeDB.Exec("DELETE FROM test_run_templates WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetExecutions returns all test runs created from a template
func (h *TestRunTemplateHandler) GetExecutions(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	templateID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify template belongs to workspace
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_run_templates WHERE id = ? AND workspace_id = ?", templateID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_run_template")
		return
	}

	rows, err := db.Query(`
		SELECT id, workspace_id, template_id, set_id, name, started_at, ended_at, created_at
		FROM test_runs
		WHERE template_id = ? AND workspace_id = ?
		ORDER BY id DESC
	`, templateID, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	runs := make([]models.TestRun, 0)
	for rows.Next() {
		var run models.TestRun
		var templateID sql.NullInt64
		err := rows.Scan(&run.ID, &run.WorkspaceID, &templateID, &run.SetID, &run.Name, &run.StartedAt, &run.EndedAt, &run.CreatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if templateID.Valid {
			run.TemplateID = int(templateID.Int64)
		}
		runs = append(runs, run)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

// Execute creates a new test run from a template
func (h *TestRunTemplateHandler) Execute(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	templateID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
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

	// Get the template to retrieve set_id and verify workspace ownership
	var template models.TestRunTemplate
	err = readDB.QueryRow(`
		SELECT id, workspace_id, set_id, name, description
		FROM test_run_templates
		WHERE id = ? AND workspace_id = ?
	`, templateID, workspaceID).Scan(&template.ID, &template.WorkspaceID, &template.SetID, &template.Name, &template.Description)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "test_run_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get count of existing runs for this template to generate sequential name
	var runCount int
	err = readDB.QueryRow(`
		SELECT COUNT(*) FROM test_runs WHERE template_id = ? AND workspace_id = ?
	`, templateID, workspaceID).Scan(&runCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create new test run with template_id
	now := time.Now()
	runName := template.Name + " - Run " + strconv.Itoa(runCount+1)

	var runID int64
	err = writeDB.QueryRow(`
		INSERT INTO test_runs (workspace_id, template_id, set_id, name, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, templateID, template.SetID, runName, now, now).Scan(&runID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get test cases for this test set (workspace-scoped)
	rows, err := readDB.Query(`
		SELECT tc.id
		FROM test_cases tc
		JOIN set_test_cases stc ON tc.id = stc.test_case_id
		WHERE stc.set_id = ? AND tc.workspace_id = ?
		ORDER BY tc.id
	`, template.SetID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// Create test results for each test case
	for rows.Next() {
		var testCaseID int
		if err = rows.Scan(&testCaseID); err != nil {
			respondInternalError(w, r, err)
			return
		}

		_, err = writeDB.Exec(`
			INSERT INTO test_results (run_id, test_case_id, status, created_at, updated_at)
			VALUES (?, ?, 'pending', ?, ?)
		`, runID, testCaseID, time.Now(), time.Now())
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Return the created run
	var run models.TestRun
	var templateIDNullable sql.NullInt64
	err = readDB.QueryRow(`
		SELECT id, workspace_id, template_id, set_id, name, started_at, ended_at, created_at
		FROM test_runs
		WHERE id = ?
	`, runID).Scan(&run.ID, &run.WorkspaceID, &templateIDNullable, &run.SetID, &run.Name, &run.StartedAt, &run.EndedAt, &run.CreatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if templateIDNullable.Valid {
		run.TemplateID = int(templateIDNullable.Int64)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(run)
}
