package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// TestFolderRepository provides data access for test folders.
type TestFolderRepository struct {
	db database.Database
}

// NewTestFolderRepository creates a new test folder repository.
func NewTestFolderRepository(db database.Database) *TestFolderRepository {
	return &TestFolderRepository{db: db}
}

// FindAllWithCounts returns all folders for a workspace including test case counts.
func (r *TestFolderRepository) FindAllWithCounts(workspaceID int) ([]models.TestFolder, error) {
	rows, err := r.db.Query(`
		SELECT tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at,
		       COUNT(tc.id) as test_case_count
		FROM test_folders tf
		LEFT JOIN test_cases tc ON tf.id = tc.folder_id
		WHERE tf.workspace_id = ?
		GROUP BY tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at
		ORDER BY tf.sort_order, tf.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	folders := make([]models.TestFolder, 0)
	for rows.Next() {
		var folder models.TestFolder
		if err := rows.Scan(
			&folder.ID, &folder.WorkspaceID, &folder.ParentID, &folder.Name, &folder.Description, &folder.SortOrder,
			&folder.CreatedAt, &folder.UpdatedAt, &folder.TestCaseCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan test folder: %w", err)
		}
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate test folders: %w", err)
	}
	return folders, nil
}

// FindByIDWithCount returns a single folder with its test case count.
func (r *TestFolderRepository) FindByIDWithCount(id, workspaceID int) (*models.TestFolder, error) {
	var folder models.TestFolder
	err := r.db.QueryRow(`
		SELECT tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at,
		       COUNT(tc.id) as test_case_count
		FROM test_folders tf
		LEFT JOIN test_cases tc ON tf.id = tc.folder_id
		WHERE tf.id = ? AND tf.workspace_id = ?
		GROUP BY tf.id, tf.workspace_id, tf.parent_id, tf.name, tf.description, tf.sort_order, tf.created_at, tf.updated_at
	`, id, workspaceID).Scan(
		&folder.ID, &folder.WorkspaceID, &folder.ParentID, &folder.Name, &folder.Description, &folder.SortOrder,
		&folder.CreatedAt, &folder.UpdatedAt, &folder.TestCaseCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find test folder: %w", err)
	}
	return &folder, nil
}

// GetParentID returns the parent_id of a folder (used for parent depth validation).
// Returns ErrNotFound if the folder does not exist.
func (r *TestFolderRepository) GetParentID(id, workspaceID int) (sql.NullInt64, error) {
	var parentID sql.NullInt64
	err := r.db.QueryRow(
		"SELECT parent_id FROM test_folders WHERE id = ? AND workspace_id = ?",
		id, workspaceID,
	).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, ErrNotFound
	}
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("failed to fetch parent id: %w", err)
	}
	return parentID, nil
}

// CountChildren returns the number of direct subfolders of a folder.
func (r *TestFolderRepository) CountChildren(folderID, workspaceID int) (int, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM test_folders WHERE parent_id = ? AND workspace_id = ?",
		folderID, workspaceID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count child folders: %w", err)
	}
	return count, nil
}

// MaxSortOrder returns the current maximum sort_order for folders in a workspace, or 0.
func (r *TestFolderRepository) MaxSortOrder(workspaceID int) (int, error) {
	var maxSortOrder sql.NullInt64
	err := r.db.QueryRow(
		"SELECT MAX(sort_order) FROM test_folders WHERE workspace_id = ?",
		workspaceID,
	).Scan(&maxSortOrder)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to query max sort order: %w", err)
	}
	return int(maxSortOrder.Int64), nil
}

// FindParentAndSortOrder returns the current parent_id and sort_order for a folder.
// Returns ErrNotFound if the folder does not exist.
func (r *TestFolderRepository) FindParentAndSortOrder(id, workspaceID int) (parentID sql.NullInt64, sortOrder int, err error) {
	err = r.db.QueryRow(
		"SELECT parent_id, sort_order FROM test_folders WHERE id = ? AND workspace_id = ?",
		id, workspaceID,
	).Scan(&parentID, &sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, 0, ErrNotFound
	}
	if err != nil {
		return sql.NullInt64{}, 0, fmt.Errorf("failed to read folder parent/sort order: %w", err)
	}
	return parentID, sortOrder, nil
}

// Create inserts a new folder and returns its id.
func (r *TestFolderRepository) Create(folder *models.TestFolder) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO test_folders (workspace_id, name, parent_id, description, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		folder.WorkspaceID,
		folder.Name,
		nullableInt(folder.ParentID),
		folder.Description,
		folder.SortOrder,
		folder.CreatedAt,
		folder.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create test folder: %w", err)
	}
	return int(id), nil
}

// Update applies new values to a folder. Returns ErrNotFound when no row is affected.
func (r *TestFolderRepository) Update(id, workspaceID int, folder *models.TestFolder) error {
	result, err := r.db.ExecWrite(`
		UPDATE test_folders
		SET name = ?, description = ?, parent_id = ?, sort_order = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ?
	`,
		folder.Name,
		folder.Description,
		nullableInt(folder.ParentID),
		folder.SortOrder,
		folder.UpdatedAt,
		id,
		workspaceID,
	)
	if err != nil {
		return fmt.Errorf("failed to update test folder: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWithCascade detaches child content and deletes a folder in a single transaction.
// Test cases are moved to no folder; subfolders are promoted to the root level.
// Returns ErrNotFound if the folder does not exist.
func (r *TestFolderRepository) DeleteWithCascade(id, workspaceID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		"UPDATE test_cases SET folder_id = NULL WHERE folder_id = ? AND workspace_id = ?",
		id, workspaceID,
	); err != nil {
		return fmt.Errorf("failed to detach test cases: %w", err)
	}

	if _, err := tx.Exec(
		"UPDATE test_folders SET parent_id = NULL WHERE parent_id = ? AND workspace_id = ?",
		id, workspaceID,
	); err != nil {
		return fmt.Errorf("failed to promote subfolders: %w", err)
	}

	result, err := tx.Exec(
		"DELETE FROM test_folders WHERE id = ? AND workspace_id = ?",
		id, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete test folder: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}

// Reorder sets sort_order for each folder in the supplied id list, in one transaction.
// The resulting sort_order is (index+1)*1000, leaving gaps for future insertions.
func (r *TestFolderRepository) Reorder(workspaceID int, folderIDs []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for i, folderID := range folderIDs {
		sortOrder := (i + 1) * 1000
		if _, err := tx.Exec(
			"UPDATE test_folders SET sort_order = ?, updated_at = ? WHERE id = ? AND workspace_id = ?",
			sortOrder, now, folderID, workspaceID,
		); err != nil {
			return fmt.Errorf("failed to reorder test folder: %w", err)
		}
	}

	return tx.Commit()
}

// nullableInt converts an *int into a sql.NullInt64, preserving null semantics.
func nullableInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}
