package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestCaseRepository provides data access methods for test cases
type TestCaseRepository struct {
	db database.Database
}

// NewTestCaseRepository creates a new test case repository
func NewTestCaseRepository(db database.Database) *TestCaseRepository {
	return &TestCaseRepository{db: db}
}

// TestCaseListParams contains parameters for listing test cases
type TestCaseListParams struct {
	WorkspaceID int
	FolderID    *int // nil = root level, pointer to int = specific folder
	All         bool // true = return all test cases in workspace
}

// FindAll returns test cases with optional folder filtering
func (r *TestCaseRepository) FindAll(params TestCaseListParams) ([]models.TestCase, error) {
	var query string
	var args []interface{}

	if params.All {
		query = `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ?
			ORDER BY tf.sort_order, tc.sort_order, tc.title
		`
		args = append(args, params.WorkspaceID)
	} else if params.FolderID == nil {
		query = `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ? AND tc.folder_id IS NULL
			ORDER BY tc.sort_order, tc.title
		`
		args = append(args, params.WorkspaceID)
	} else {
		query = `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ? AND tc.folder_id = ?
			ORDER BY tc.sort_order, tc.title
		`
		args = append(args, params.WorkspaceID, *params.FolderID)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query test cases: %w", err)
	}
	defer rows.Close()

	var testCases []models.TestCase
	for rows.Next() {
		var tc models.TestCase
		var folderName sql.NullString

		err := rows.Scan(
			&tc.ID, &tc.WorkspaceID, &tc.FolderID, &tc.Title, &tc.Preconditions,
			&tc.Priority, &tc.Status, &tc.EstimatedDuration,
			&tc.SortOrder, &tc.CreatedAt, &tc.UpdatedAt, &folderName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test case: %w", err)
		}

		if folderName.Valid {
			tc.FolderName = folderName.String
		}

		testCases = append(testCases, tc)
	}

	return testCases, nil
}

// FindByID retrieves a single test case by ID
func (r *TestCaseRepository) FindByID(id, workspaceID int) (*models.TestCase, error) {
	query := `
		SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
		       COALESCE(tc.preconditions, '') as preconditions,
		       COALESCE(tc.priority, 'medium') as priority,
		       COALESCE(tc.status, 'active') as status,
		       COALESCE(tc.estimated_duration, 0) as estimated_duration,
		       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
		FROM test_cases tc
		LEFT JOIN test_folders tf ON tc.folder_id = tf.id
		WHERE tc.id = ? AND tc.workspace_id = ?
	`

	var tc models.TestCase
	var folderName sql.NullString

	err := r.db.QueryRow(query, id, workspaceID).Scan(
		&tc.ID, &tc.WorkspaceID, &tc.FolderID, &tc.Title, &tc.Preconditions,
		&tc.Priority, &tc.Status, &tc.EstimatedDuration,
		&tc.SortOrder, &tc.CreatedAt, &tc.UpdatedAt, &folderName,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find test case: %w", err)
	}

	if folderName.Valid {
		tc.FolderName = folderName.String
	}

	return &tc, nil
}

// FindByIDWithSteps retrieves a test case with its steps
func (r *TestCaseRepository) FindByIDWithSteps(id, workspaceID int) (*models.TestCase, error) {
	tc, err := r.FindByID(id, workspaceID)
	if err != nil {
		return nil, err
	}

	steps, err := r.FindSteps(id)
	if err != nil {
		return nil, err
	}
	tc.TestSteps = steps

	return tc, nil
}

// GetMaxSortOrder returns the highest sort_order for test cases in a folder
func (r *TestCaseRepository) GetMaxSortOrder(workspaceID int, folderID *int) (int, error) {
	var maxSortOrder sql.NullInt64
	var err error

	if folderID != nil {
		err = r.db.QueryRow(
			"SELECT MAX(sort_order) FROM test_cases WHERE workspace_id = ? AND folder_id = ?",
			workspaceID, *folderID,
		).Scan(&maxSortOrder)
	} else {
		err = r.db.QueryRow(
			"SELECT MAX(sort_order) FROM test_cases WHERE workspace_id = ? AND folder_id IS NULL",
			workspaceID,
		).Scan(&maxSortOrder)
	}

	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get max sort order: %w", err)
	}

	return int(maxSortOrder.Int64), nil
}

// Create inserts a new test case and returns its ID
func (r *TestCaseRepository) Create(tx database.Tx, tc *models.TestCase) (int, error) {
	query := `
		INSERT INTO test_cases (workspace_id, folder_id, title, preconditions, priority, status, estimated_duration, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err := tx.QueryRow(query, tc.WorkspaceID, tc.FolderID, tc.Title, tc.Preconditions,
		tc.Priority, tc.Status, tc.EstimatedDuration,
		tc.SortOrder, tc.CreatedAt, tc.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create test case: %w", err)
	}

	return int(id), nil
}

// Update updates an existing test case
func (r *TestCaseRepository) Update(tx database.Tx, tc *models.TestCase) error {
	query := `
		UPDATE test_cases
		SET folder_id = ?, title = ?, preconditions = ?,
		    priority = ?, status = ?, estimated_duration = ?,
		    sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	result, err := tx.Exec(query, tc.FolderID, tc.Title, tc.Preconditions,
		tc.Priority, tc.Status, tc.EstimatedDuration,
		tc.SortOrder, tc.UpdatedAt, tc.ID, tc.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to update test case: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete removes a test case by ID
func (r *TestCaseRepository) Delete(tx database.Tx, id, workspaceID int) error {
	result, err := tx.Exec("DELETE FROM test_cases WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete test case: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Move moves a test case to a different folder
func (r *TestCaseRepository) Move(tx database.Tx, id, workspaceID int, folderID *int, sortOrder int) error {
	query := `
		UPDATE test_cases
		SET folder_id = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	result, err := tx.Exec(query, folderID, sortOrder, time.Now(), id, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to move test case: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Reorder updates the sort order of multiple test cases
func (r *TestCaseRepository) Reorder(tx database.Tx, workspaceID int, testCaseIDs []int) error {
	for i, tcID := range testCaseIDs {
		sortOrder := (i + 1) * 1000
		_, err := tx.Exec(
			"UPDATE test_cases SET sort_order = ?, updated_at = ? WHERE id = ? AND workspace_id = ?",
			sortOrder, time.Now(), tcID, workspaceID,
		)
		if err != nil {
			return fmt.Errorf("failed to reorder test case %d: %w", tcID, err)
		}
	}
	return nil
}

// Exists checks if a test case exists in a workspace
func (r *TestCaseRepository) Exists(id, workspaceID int) (bool, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM test_cases WHERE id = ? AND workspace_id = ?",
		id, workspaceID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check test case existence: %w", err)
	}
	return count > 0, nil
}

// Test Step methods

// FindSteps returns all steps for a test case
func (r *TestCaseRepository) FindSteps(testCaseID int) ([]models.TestStep, error) {
	query := `
		SELECT id, test_case_id, step_number, action, data, expected, created_at, updated_at
		FROM test_steps
		WHERE test_case_id = ?
		ORDER BY step_number
	`

	rows, err := r.db.Query(query, testCaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test steps: %w", err)
	}
	defer rows.Close()

	var steps []models.TestStep
	for rows.Next() {
		var step models.TestStep
		err := rows.Scan(
			&step.ID, &step.TestCaseID, &step.StepNumber,
			&step.Action, &step.Data, &step.Expected,
			&step.CreatedAt, &step.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test step: %w", err)
		}
		steps = append(steps, step)
	}

	return steps, nil
}

// GetMaxStepNumber returns the highest step_number for a test case
func (r *TestCaseRepository) GetMaxStepNumber(testCaseID int) (int, error) {
	var maxStepNumber sql.NullInt64
	err := r.db.QueryRow(
		"SELECT MAX(step_number) FROM test_steps WHERE test_case_id = ?",
		testCaseID,
	).Scan(&maxStepNumber)

	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get max step number: %w", err)
	}

	return int(maxStepNumber.Int64), nil
}

// CreateStep inserts a new test step
func (r *TestCaseRepository) CreateStep(tx database.Tx, step *models.TestStep) (int, error) {
	query := `
		INSERT INTO test_steps (test_case_id, step_number, action, data, expected, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err := tx.QueryRow(query, step.TestCaseID, step.StepNumber,
		step.Action, step.Data, step.Expected, step.CreatedAt, step.UpdatedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create test step: %w", err)
	}

	return int(id), nil
}

// UpdateStep updates an existing test step
func (r *TestCaseRepository) UpdateStep(tx database.Tx, step *models.TestStep) error {
	query := `
		UPDATE test_steps
		SET step_number = ?, action = ?, data = ?, expected = ?, updated_at = ?
		WHERE id = ? AND test_case_id = ?
	`

	result, err := tx.Exec(query, step.StepNumber, step.Action, step.Data,
		step.Expected, step.UpdatedAt, step.ID, step.TestCaseID)
	if err != nil {
		return fmt.Errorf("failed to update test step: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteStep removes a test step
func (r *TestCaseRepository) DeleteStep(tx database.Tx, stepID, testCaseID int) error {
	result, err := tx.Exec("DELETE FROM test_steps WHERE id = ? AND test_case_id = ?", stepID, testCaseID)
	if err != nil {
		return fmt.Errorf("failed to delete test step: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ReorderSteps updates the step order of multiple steps
func (r *TestCaseRepository) ReorderSteps(tx database.Tx, testCaseID int, stepIDs []int) error {
	for i, stepID := range stepIDs {
		stepNumber := i + 1
		_, err := tx.Exec(
			"UPDATE test_steps SET step_number = ?, updated_at = ? WHERE id = ? AND test_case_id = ?",
			stepNumber, time.Now(), stepID, testCaseID,
		)
		if err != nil {
			return fmt.Errorf("failed to reorder test step %d: %w", stepID, err)
		}
	}
	return nil
}

// Test Label methods

// FindAllLabels returns all labels for a workspace
func (r *TestCaseRepository) FindAllLabels(workspaceID int) ([]models.TestLabel, error) {
	query := `
		SELECT id, workspace_id, name, color, description, created_at, updated_at
		FROM test_labels
		WHERE workspace_id = ?
		ORDER BY name
	`

	rows, err := r.db.Query(query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test labels: %w", err)
	}
	defer rows.Close()

	var labels []models.TestLabel
	for rows.Next() {
		var label models.TestLabel
		err := rows.Scan(&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
			&label.CreatedAt, &label.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test label: %w", err)
		}
		labels = append(labels, label)
	}

	return labels, nil
}

// FindLabelsByTestCaseID returns all labels for a specific test case
func (r *TestCaseRepository) FindLabelsByTestCaseID(testCaseID int) ([]models.TestLabel, error) {
	query := `
		SELECT tl.id, tl.workspace_id, tl.name, tl.color, tl.description, tl.created_at, tl.updated_at
		FROM test_labels tl
		INNER JOIN test_case_labels tcl ON tl.id = tcl.label_id
		WHERE tcl.test_case_id = ?
		ORDER BY tl.name
	`

	rows, err := r.db.Query(query, testCaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test case labels: %w", err)
	}
	defer rows.Close()

	var labels []models.TestLabel
	for rows.Next() {
		var label models.TestLabel
		err := rows.Scan(&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
			&label.CreatedAt, &label.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test label: %w", err)
		}
		labels = append(labels, label)
	}

	return labels, nil
}

// CreateLabel creates a new test label
func (r *TestCaseRepository) CreateLabel(tx database.Tx, label *models.TestLabel) (int, error) {
	query := `
		INSERT INTO test_labels (workspace_id, name, color, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, created_at, updated_at
	`

	err := tx.QueryRow(query, label.WorkspaceID, label.Name, label.Color, label.Description,
		label.CreatedAt, label.UpdatedAt).Scan(&label.ID, &label.CreatedAt, &label.UpdatedAt)
	if err != nil {
		return 0, fmt.Errorf("failed to create test label: %w", err)
	}

	return label.ID, nil
}

// UpdateLabel updates an existing test label
func (r *TestCaseRepository) UpdateLabel(tx database.Tx, label *models.TestLabel) error {
	query := `
		UPDATE test_labels
		SET name = ?, color = ?, description = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`

	_, err := tx.Exec(query, label.Name, label.Color, label.Description,
		time.Now(), label.ID, label.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to update test label: %w", err)
	}

	return nil
}

// DeleteLabel removes a test label
func (r *TestCaseRepository) DeleteLabel(tx database.Tx, labelID, workspaceID int) error {
	_, err := tx.Exec("DELETE FROM test_labels WHERE id = ? AND workspace_id = ?", labelID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete test label: %w", err)
	}
	return nil
}

// LabelExists checks if a label exists in a workspace
func (r *TestCaseRepository) LabelExists(labelID, workspaceID int) (bool, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM test_labels WHERE id = ? AND workspace_id = ?",
		labelID, workspaceID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check label existence: %w", err)
	}
	return count > 0, nil
}

// GetLabel retrieves a single label by ID
func (r *TestCaseRepository) GetLabel(labelID, workspaceID int) (*models.TestLabel, error) {
	query := `
		SELECT id, workspace_id, name, color, description, created_at, updated_at
		FROM test_labels
		WHERE id = ? AND workspace_id = ?
	`

	var label models.TestLabel
	err := r.db.QueryRow(query, labelID, workspaceID).Scan(
		&label.ID, &label.WorkspaceID, &label.Name, &label.Color, &label.Description,
		&label.CreatedAt, &label.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get test label: %w", err)
	}

	return &label, nil
}

// AddLabelToTestCase adds a label to a test case
func (r *TestCaseRepository) AddLabelToTestCase(tx database.Tx, testCaseID, labelID int) error {
	_, err := tx.Exec(`
		INSERT INTO test_case_labels (test_case_id, label_id, created_at)
		VALUES (?, ?, ?)
	`, testCaseID, labelID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add label to test case: %w", err)
	}
	return nil
}

// RemoveLabelFromTestCase removes a label from a test case
func (r *TestCaseRepository) RemoveLabelFromTestCase(tx database.Tx, testCaseID, labelID int) error {
	_, err := tx.Exec(`
		DELETE FROM test_case_labels
		WHERE test_case_id = ? AND label_id = ?
	`, testCaseID, labelID)
	if err != nil {
		return fmt.Errorf("failed to remove label from test case: %w", err)
	}
	return nil
}

// TestCaseConnections contains related entities for a test case
type TestCaseConnections struct {
	TestSets     []TestSetSummary     `json:"test_sets"`
	RunTemplates []RunTemplateSummary `json:"run_templates"`
	Executions   []ExecutionSummary   `json:"executions"`
}

// TestSetSummary contains basic test set info
type TestSetSummary struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RunTemplateSummary contains basic run template info
type RunTemplateSummary struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SetID       int    `json:"set_id"`
	SetName     string `json:"set_name"`
}

// ExecutionSummary contains test run execution info
type ExecutionSummary struct {
	RunID        int        `json:"run_id"`
	RunName      string     `json:"run_name"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	TemplateID   *int       `json:"template_id,omitempty"`
	TemplateName string     `json:"template_name,omitempty"`
	SetID        int        `json:"set_id"`
	SetName      string     `json:"set_name"`
}

// GetConnections returns related sets, templates, and executions for a test case
func (r *TestCaseRepository) GetConnections(testCaseID, workspaceID int) (*TestCaseConnections, error) {
	connections := &TestCaseConnections{
		TestSets:     []TestSetSummary{},
		RunTemplates: []RunTemplateSummary{},
		Executions:   []ExecutionSummary{},
	}

	// Get test sets containing this test case
	setRows, err := r.db.Query(`
		SELECT ts.id, ts.name, COALESCE(ts.description, '')
		FROM test_sets ts
		JOIN set_test_cases stc ON stc.set_id = ts.id
		WHERE stc.test_case_id = ? AND ts.workspace_id = ?
		ORDER BY ts.name
	`, testCaseID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test sets: %w", err)
	}
	defer setRows.Close()

	for setRows.Next() {
		var summary TestSetSummary
		if err := setRows.Scan(&summary.ID, &summary.Name, &summary.Description); err != nil {
			return nil, fmt.Errorf("failed to scan test set: %w", err)
		}
		connections.TestSets = append(connections.TestSets, summary)
	}

	// Get run templates
	tmplRows, err := r.db.Query(`
		SELECT trt.id, trt.name, COALESCE(trt.description, ''), trt.set_id, COALESCE(ts.name, '')
		FROM test_run_templates trt
		JOIN set_test_cases stc ON stc.set_id = trt.set_id
		LEFT JOIN test_sets ts ON trt.set_id = ts.id
		WHERE stc.test_case_id = ? AND trt.workspace_id = ?
		ORDER BY trt.updated_at DESC
	`, testCaseID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query run templates: %w", err)
	}
	defer tmplRows.Close()

	for tmplRows.Next() {
		var summary RunTemplateSummary
		if err := tmplRows.Scan(&summary.ID, &summary.Name, &summary.Description, &summary.SetID, &summary.SetName); err != nil {
			return nil, fmt.Errorf("failed to scan run template: %w", err)
		}
		connections.RunTemplates = append(connections.RunTemplates, summary)
	}

	// Get executions
	runRows, err := r.db.Query(`
		SELECT tr.id, tr.name, tr.set_id, COALESCE(ts.name, ''), tr.template_id, COALESCE(trt.name, ''),
		       tr.started_at, tr.ended_at, trr.status
		FROM test_runs tr
		JOIN test_results trr ON trr.run_id = tr.id AND trr.test_case_id = ?
		LEFT JOIN test_sets ts ON tr.set_id = ts.id
		LEFT JOIN test_run_templates trt ON tr.template_id = trt.id
		WHERE tr.workspace_id = ?
		ORDER BY tr.started_at DESC
		LIMIT 20
	`, testCaseID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer runRows.Close()

	for runRows.Next() {
		var record struct {
			RunID        int
			RunName      string
			SetID        int
			SetName      string
			TemplateID   sql.NullInt64
			TemplateName string
			StartedAt    time.Time
			EndedAt      sql.NullTime
			Status       string
		}
		if err := runRows.Scan(&record.RunID, &record.RunName, &record.SetID, &record.SetName,
			&record.TemplateID, &record.TemplateName, &record.StartedAt, &record.EndedAt, &record.Status); err != nil {
			return nil, fmt.Errorf("failed to scan execution: %w", err)
		}
		execution := ExecutionSummary{
			RunID:     record.RunID,
			RunName:   record.RunName,
			Status:    record.Status,
			StartedAt: record.StartedAt,
			SetID:     record.SetID,
			SetName:   record.SetName,
		}
		if record.EndedAt.Valid {
			end := record.EndedAt.Time
			execution.EndedAt = &end
		}
		if record.TemplateID.Valid {
			tid := int(record.TemplateID.Int64)
			execution.TemplateID = &tid
			execution.TemplateName = record.TemplateName
		}
		connections.Executions = append(connections.Executions, execution)
	}

	return connections, nil
}
