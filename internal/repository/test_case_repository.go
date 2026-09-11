package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

// FindWorkspacesWithMatchingCases returns distinct workspace IDs containing
// test cases matching the given search term, before caller-level permission
// filtering is applied.
func (r *TestCaseRepository) FindWorkspacesWithMatchingCases(query string) ([]int, error) {
	searchTerm := "%" + query + "%"
	rows, err := r.db.Query(`
		SELECT DISTINCT workspace_id
		FROM test_cases
		WHERE title LIKE ? OR preconditions LIKE ?
	`, searchTerm, searchTerm)
	if err != nil {
		return nil, fmt.Errorf("find workspaces with matching test cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var workspaceIDs []int
	for rows.Next() {
		var workspaceID int
		if err := rows.Scan(&workspaceID); err != nil {
			return nil, fmt.Errorf("scan test case workspace: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test case workspaces: %w", err)
	}
	return workspaceIDs, nil
}

// Search returns linkable test-case rows matching query within the given
// workspaces. Callers must supply workspace IDs the user can access; an empty
// set yields no rows.
func (r *TestCaseRepository) Search(query string, workspaceIDs []int, limit int) ([]models.LinkableItem, error) {
	if len(workspaceIDs) == 0 {
		return []models.LinkableItem{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
	wsArgs := make([]any, len(workspaceIDs))
	for i, id := range workspaceIDs {
		wsArgs[i] = id
	}

	sqlQuery := fmt.Sprintf(`
		SELECT id, title, COALESCE(preconditions, '') AS summary
		FROM test_cases
		WHERE (title LIKE ? OR preconditions LIKE ?)
		  AND workspace_id IN (%s)
		ORDER BY title
		LIMIT ?
	`, placeholders)

	searchTerm := "%" + query + "%"
	args := make([]any, 0, 3+len(wsArgs))
	args = append(args, searchTerm, searchTerm)
	args = append(args, wsArgs...)
	args = append(args, limit)
	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search test cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []models.LinkableItem
	for rows.Next() {
		var item models.LinkableItem
		var summary sql.NullString

		if err := rows.Scan(&item.ID, &item.Title, &summary); err != nil {
			return nil, fmt.Errorf("scan test case search result: %w", err)
		}

		item.Description = summary.String
		item.Type = "test_case"
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test case search results: %w", err)
	}

	return items, nil
}

// TestCaseListParams contains parameters for listing test cases
type TestCaseListParams struct {
	WorkspaceID int
	FolderID    *int // nil = root level, pointer to int = specific folder
	All         bool // true = return all test cases in workspace
	Limit       int
	Offset      int
	Search      string
	LabelID     *int
}

// FindAll returns test cases with optional folder filtering
func (r *TestCaseRepository) FindAll(params TestCaseListParams) ([]models.TestCase, error) {
	query := `
			SELECT tc.id, tc.workspace_id, tc.folder_id, tc.title,
			       COALESCE(tc.preconditions, '') as preconditions,
			       COALESCE(tc.priority, 'medium') as priority,
			       COALESCE(tc.status, 'active') as status,
			       COALESCE(tc.estimated_duration, 0) as estimated_duration,
			       tc.sort_order, tc.created_at, tc.updated_at, tf.name as folder_name
			FROM test_cases tc
			LEFT JOIN test_folders tf ON tc.folder_id = tf.id
			WHERE tc.workspace_id = ?`
	args := []any{params.WorkspaceID}
	query, args = appendTestCaseListFilters(query, args, params)

	if params.All {
		query += " ORDER BY tf.sort_order, tc.sort_order, tc.title, tc.id"
	} else {
		query += " ORDER BY tc.sort_order, tc.title, tc.id"
	}
	if params.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, params.Limit, params.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query test cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test cases: %w", err)
	}

	return testCases, nil
}

// Count returns the total for the same filters used by FindAll.
func (r *TestCaseRepository) Count(params TestCaseListParams) (int, error) {
	query := "SELECT COUNT(*) FROM test_cases tc WHERE tc.workspace_id = ?"
	args := []any{params.WorkspaceID}
	query, args = appendTestCaseListFilters(query, args, params)
	var count int
	if err := r.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count test cases: %w", err)
	}
	return count, nil
}

func appendTestCaseListFilters(query string, args []any, params TestCaseListParams) (filteredQuery string, filteredArgs []any) {
	if !params.All {
		if params.FolderID == nil {
			query += " AND tc.folder_id IS NULL"
		} else {
			query += " AND tc.folder_id = ?"
			args = append(args, *params.FolderID)
		}
	}
	if params.LabelID != nil {
		query += " AND EXISTS (SELECT 1 FROM test_case_labels tcl WHERE tcl.test_case_id = tc.id AND tcl.label_id = ?)"
		args = append(args, *params.LabelID)
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		query += ` AND (
			LOWER(tc.title) LIKE ? OR LOWER(COALESCE(tc.preconditions, '')) LIKE ? OR
			LOWER(COALESCE(tc.priority, '')) LIKE ? OR LOWER(COALESCE(tc.status, '')) LIKE ? OR
			EXISTS (SELECT 1 FROM test_case_labels search_tcl
				JOIN test_labels search_tl ON search_tl.id = search_tcl.label_id
				WHERE search_tcl.test_case_id = tc.id AND LOWER(search_tl.name) LIKE ?)
		)`
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	return query, args
}

// CountAll returns the number of test cases in a workspace without loading them.
func (r *TestCaseRepository) CountAll(workspaceID int) (int, error) {
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM test_cases WHERE workspace_id = ?", workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count test cases: %w", err)
	}
	return count, nil
}

// GetWorkspaceID resolves the owning workspace for a test case. Returns
// ErrNotFound when the case is missing.
func (r *TestCaseRepository) GetWorkspaceID(id int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow(`SELECT workspace_id FROM test_cases WHERE id = ?`, id).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve test case workspace: %w", err)
	}
	return workspaceID, nil
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
	if errors.Is(err, sql.ErrNoRows) {
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

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to get max sort order: %w", err)
	}

	return int(maxSortOrder.Int64), nil
}

// GetMaxSortOrderTx is the transaction-scoped form used by atomic import
// workflows.
func (r *TestCaseRepository) GetMaxSortOrderTx(tx database.Tx, workspaceID int, folderID *int) (int, error) {
	var maxSortOrder sql.NullInt64
	var err error
	if folderID != nil {
		err = tx.QueryRow(
			"SELECT MAX(sort_order) FROM test_cases WHERE workspace_id = ? AND folder_id = ?",
			workspaceID,
			*folderID,
		).Scan(&maxSortOrder)
	} else {
		err = tx.QueryRow(
			"SELECT MAX(sort_order) FROM test_cases WHERE workspace_id = ? AND folder_id IS NULL",
			workspaceID,
		).Scan(&maxSortOrder)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
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
	defer func() { _ = rows.Close() }()

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test steps: %w", err)
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

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
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

// scanTestLabels scans test label rows into a slice.
func scanTestLabels(rows *sql.Rows) ([]models.TestLabel, error) {
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

// FindAllLabels returns all labels for a workspace
func (r *TestCaseRepository) FindAllLabels(workspaceID int) ([]models.TestLabel, error) {
	rows, err := r.db.Query(`
		SELECT id, workspace_id, name, color, description, created_at, updated_at
		FROM test_labels
		WHERE workspace_id = ?
		ORDER BY name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanTestLabels(rows)
}

// FindLabelsByTestCaseID returns all labels for a specific test case
func (r *TestCaseRepository) FindLabelsByTestCaseID(testCaseID int) ([]models.TestLabel, error) {
	rows, err := r.db.Query(`
		SELECT tl.id, tl.workspace_id, tl.name, tl.color, tl.description, tl.created_at, tl.updated_at
		FROM test_labels tl
		INNER JOIN test_case_labels tcl ON tl.id = tcl.label_id
		WHERE tcl.test_case_id = ?
		ORDER BY tl.name
	`, testCaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test case labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanTestLabels(rows)
}

// FindLabelByNameTx returns a case-insensitive workspace label match within
// the caller's transaction.
func (r *TestCaseRepository) FindLabelByNameTx(tx database.Tx, workspaceID int, name string) (*models.TestLabel, error) {
	var label models.TestLabel
	err := tx.QueryRow(`
		SELECT id, workspace_id, name, color, description, created_at, updated_at
		FROM test_labels
		WHERE workspace_id = ? AND LOWER(name) = LOWER(?)
	`, workspaceID, name).Scan(
		&label.ID,
		&label.WorkspaceID,
		&label.Name,
		&label.Color,
		&label.Description,
		&label.CreatedAt,
		&label.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find test label by name: %w", err)
	}
	return &label, nil
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
	if errors.Is(err, sql.ErrNoRows) {
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
	defer func() { _ = setRows.Close() }()

	for setRows.Next() {
		var summary TestSetSummary
		if err = setRows.Scan(&summary.ID, &summary.Name, &summary.Description); err != nil {
			return nil, fmt.Errorf("failed to scan test set: %w", err)
		}
		connections.TestSets = append(connections.TestSets, summary)
	}
	if err := setRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test sets: %w", err)
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
	defer func() { _ = tmplRows.Close() }()

	for tmplRows.Next() {
		var summary RunTemplateSummary
		if err = tmplRows.Scan(&summary.ID, &summary.Name, &summary.Description, &summary.SetID, &summary.SetName); err != nil {
			return nil, fmt.Errorf("failed to scan run template: %w", err)
		}
		connections.RunTemplates = append(connections.RunTemplates, summary)
	}
	if err := tmplRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate run templates: %w", err)
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
	defer func() { _ = runRows.Close() }()

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
	if err := runRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate executions: %w", err)
	}

	return connections, nil
}
