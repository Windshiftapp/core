package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TestSetHandler struct {
	*BaseHandler
	permissionService *services.PermissionService
}

func NewTestSetHandlerWithPool(db database.Database, permissionService *services.PermissionService) *TestSetHandler {
	return &TestSetHandler{
		BaseHandler:       NewBaseHandler(db),
		permissionService: permissionService,
	}
}

func (h *TestSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	rows, err := db.Query(`
		SELECT
			ts.id, ts.workspace_id, ts.name, ts.description, ts.milestone_id, ts.created_at, ts.updated_at,
			m.name as milestone_name,
			COALESCE(tc_count.count, 0) as test_case_count,
			COALESCE(run_stats.total_runs, 0) as total_runs,
			COALESCE(run_stats.successful_runs, 0) as successful_runs,
			COALESCE(run_stats.failed_runs, 0) as failed_runs,
			run_stats.last_run_status,
			run_stats.last_run_date
		FROM test_sets ts
		LEFT JOIN milestones m ON ts.milestone_id = m.id
		LEFT JOIN (
			SELECT set_id, COUNT(*) as count
			FROM set_test_cases
			GROUP BY set_id
		) tc_count ON ts.id = tc_count.set_id
		LEFT JOIN (
			SELECT
				set_id,
				COUNT(*) as total_runs,
				SUM(CASE WHEN ended_at IS NOT NULL THEN 1 ELSE 0 END) as successful_runs,
				SUM(CASE WHEN ended_at IS NULL THEN 1 ELSE 0 END) as failed_runs,
				CASE
					WHEN MAX(ended_at) IS NOT NULL THEN 'completed'
					WHEN COUNT(*) > 0 THEN 'in_progress'
					ELSE NULL
				END as last_run_status,
				MAX(started_at) as last_run_date
			FROM test_runs
			WHERE workspace_id = ?
			GROUP BY set_id
		) run_stats ON ts.id = run_stats.set_id
		WHERE ts.workspace_id = ?
		ORDER BY ts.id DESC
	`, workspaceID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	sets := make([]models.TestSet, 0)
	for rows.Next() {
		var set models.TestSet
		var milestoneName sql.NullString
		var lastRunStatus sql.NullString
		var lastRunDateStr sql.NullString

		err := rows.Scan(
			&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.MilestoneID, &set.CreatedAt, &set.UpdatedAt,
			&milestoneName, &set.TestCaseCount, &set.TotalRuns, &set.SuccessfulRuns, &set.FailedRuns,
			&lastRunStatus, &lastRunDateStr,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if milestoneName.Valid {
			set.MilestoneName = milestoneName.String
		}
		if lastRunStatus.Valid {
			set.LastRunStatus = lastRunStatus.String
		}
		if lastRunDateStr.Valid {
			if parsedTime, err := time.Parse("2006-01-02 15:04:05.999999-07:00", lastRunDateStr.String); err == nil {
				set.LastRunDate = &parsedTime
			}
		}

		sets = append(sets, set)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sets)
}

func (h *TestSetHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	var set models.TestSet
	var milestoneName sql.NullString

	err = db.QueryRow(`
		SELECT ts.id, ts.workspace_id, ts.name, ts.description, ts.milestone_id, ts.created_at, ts.updated_at,
		       m.name as milestone_name
		FROM test_sets ts
		LEFT JOIN milestones m ON ts.milestone_id = m.id
		WHERE ts.id = ? AND ts.workspace_id = ?
	`, id, workspaceID).Scan(&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.MilestoneID, &set.CreatedAt, &set.UpdatedAt, &milestoneName)

	if milestoneName.Valid {
		set.MilestoneName = milestoneName.String
	}

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "test_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

func (h *TestSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	var set models.TestSet
	if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	var id int64
	err = db.QueryRow(`
		INSERT INTO test_sets (workspace_id, name, description, milestone_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, set.Name, set.Description, set.MilestoneID, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.ID = int(id)
	set.WorkspaceID = workspaceID
	set.CreatedAt = now
	set.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(set)
}

func (h *TestSetHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	var set models.TestSet
	if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	now := time.Now()
	_, err = db.Exec(`
		UPDATE test_sets
		SET name = ?, description = ?, milestone_id = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`, set.Name, set.Description, set.MilestoneID, now, id, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.ID = id
	set.WorkspaceID = workspaceID
	set.UpdatedAt = now

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

func (h *TestSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	db, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err = db.Exec("DELETE FROM test_sets WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TestSetHandler) GetTestCases(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify test set belongs to workspace
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", setID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_set")
		return
	}

	rows, err := db.Query(`
		SELECT tc.id, tc.workspace_id, tc.title, tc.preconditions, tc.created_at, tc.updated_at
		FROM test_cases tc
		JOIN set_test_cases stc ON tc.id = stc.test_case_id
		WHERE stc.set_id = ? AND tc.workspace_id = ?
		ORDER BY tc.id
	`, setID, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	testCases := make([]models.TestCase, 0)
	for rows.Next() {
		var tc models.TestCase
		err := rows.Scan(&tc.ID, &tc.WorkspaceID, &tc.Title, &tc.Preconditions, &tc.CreatedAt, &tc.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		testCases = append(testCases, tc)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testCases)
}

func (h *TestSetHandler) AddTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	var request struct {
		TestCaseID int `json:"test_case_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify test set belongs to workspace
	var count int
	err = readDB.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", setID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_set")
		return
	}

	// Verify test case belongs to same workspace
	err = readDB.QueryRow("SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?", request.TestCaseID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_case")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err = writeDB.Exec(`
		INSERT INTO set_test_cases (set_id, test_case_id)
		VALUES (?, ?)
	`, setID, request.TestCaseID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *TestSetHandler) RemoveTestCase(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	testCaseID, err := strconv.Atoi(r.PathValue("testCaseId"))
	if err != nil {
		respondInvalidID(w, r, "testCaseId")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestManage)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	readDB, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify test set belongs to workspace
	var count int
	err = readDB.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", setID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_set")
		return
	}

	writeDB, ok := h.requireWriteDB(w, r)
	if !ok {
		return
	}

	_, err = writeDB.Exec(`
		DELETE FROM set_test_cases
		WHERE set_id = ? AND test_case_id = ?
	`, setID, testCaseID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TestSetHandler) GetRuns(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionTestView)
	if err != nil || !hasPermission {
		respondForbidden(w, r)
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	// Verify test set belongs to workspace
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_sets WHERE id = ? AND workspace_id = ?", setID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, "test_set")
		return
	}

	rows, err := db.Query(`
		SELECT id, workspace_id, set_id, name, started_at, ended_at, created_at
		FROM test_runs
		WHERE set_id = ? AND workspace_id = ?
		ORDER BY id DESC
	`, setID, workspaceID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	// Initialize as empty array instead of nil so JSON encoding returns [] instead of null
	runs := make([]models.TestRun, 0)
	for rows.Next() {
		var run models.TestRun
		err := rows.Scan(&run.ID, &run.WorkspaceID, &run.SetID, &run.Name, &run.StartedAt, &run.EndedAt, &run.CreatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		runs = append(runs, run)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}
