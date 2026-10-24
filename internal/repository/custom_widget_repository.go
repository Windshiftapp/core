package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CustomWidgetRepository persists user-authored OpenUI dashboard widgets.
type CustomWidgetRepository struct {
	db database.Database
}

func NewCustomWidgetRepository(db database.Database) *CustomWidgetRepository {
	return &CustomWidgetRepository{db: db}
}

const customWidgetColumns = `id, workspace_id, owner_user_id, name, description, program,
	lang_version, library_version, queries, visibility, created_by, created_at, updated_at`

// Create inserts a widget and returns its ID.
func (r *CustomWidgetRepository) Create(widget *models.CustomWidget) (int, error) {
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		return tx.QueryRow(`
			INSERT INTO custom_widgets (
				workspace_id, owner_user_id, name, description, program,
				lang_version, library_version, queries, visibility, created_by
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
		`,
			widget.WorkspaceID, widget.OwnerUserID, widget.Name, widget.Description,
			widget.Program, widget.LangVersion, widget.LibraryVersion, string(widget.Queries),
			widget.Visibility, widget.CreatedBy,
		).Scan(&id)
	})
	if err != nil {
		return 0, fmt.Errorf("create custom widget: %w", err)
	}
	return id, nil
}

// Get returns one widget, or ErrNotFound when it does not exist.
func (r *CustomWidgetRepository) Get(id int) (*models.CustomWidget, error) {
	row := r.db.QueryRow(`SELECT `+customWidgetColumns+` FROM custom_widgets WHERE id = ?`, id)
	widget, err := scanCustomWidget(row)
	if err != nil {
		return nil, notFoundOrWrap(err, "get custom widget")
	}
	return widget, nil
}

// ListForUser returns the user's private widgets plus every widget in the
// given accessible workspaces.
func (r *CustomWidgetRepository) ListForUser(userID int, workspaceIDs []int) ([]models.CustomWidget, error) {
	query := `SELECT ` + customWidgetColumns + ` FROM custom_widgets WHERE (workspace_id IS NULL AND owner_user_id = ?)`
	args := []any{userID}
	if len(workspaceIDs) > 0 {
		query += ` OR workspace_id IN (` + placeholders(len(workspaceIDs)) + `)`
		for _, id := range workspaceIDs {
			args = append(args, id)
		}
	}
	query += ` ORDER BY updated_at DESC, id DESC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list custom widgets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	widgets := make([]models.CustomWidget, 0)
	for rows.Next() {
		widget, err := scanCustomWidget(rows)
		if err != nil {
			return nil, err
		}
		widgets = append(widgets, *widget)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list custom widgets: %w", err)
	}
	return widgets, nil
}

// Update writes the mutable fields of an existing widget.
func (r *CustomWidgetRepository) Update(widget *models.CustomWidget) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		result, err := tx.ExecWrite(`
			UPDATE custom_widgets SET
				name = ?, description = ?, program = ?, lang_version = ?,
				library_version = ?, queries = ?, visibility = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`,
			widget.Name, widget.Description, widget.Program, widget.LangVersion,
			widget.LibraryVersion, string(widget.Queries), widget.Visibility, widget.ID,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// Delete removes a widget. Dashboard layouts keep the stale ID and render an
// unavailable card, so no layout rewrite is needed here.
func (r *CustomWidgetRepository) Delete(id int) error {
	result, err := r.db.ExecWrite(`DELETE FROM custom_widgets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete custom widget: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete custom widget: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountOwned returns how many widgets the user has created.
func (r *CustomWidgetRepository) CountOwned(userID int) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM custom_widgets WHERE created_by = ?`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count custom widgets: %w", err)
	}
	return count, nil
}

type customWidgetScanner interface {
	Scan(dest ...any) error
}

func scanCustomWidget(scanner customWidgetScanner) (*models.CustomWidget, error) {
	var (
		widget      models.CustomWidget
		workspaceID sql.NullInt64
		ownerUserID sql.NullInt64
		queries     string
	)
	err := scanner.Scan(
		&widget.ID, &workspaceID, &ownerUserID, &widget.Name, &widget.Description,
		&widget.Program, &widget.LangVersion, &widget.LibraryVersion, &queries,
		&widget.Visibility, &widget.CreatedBy, &widget.CreatedAt, &widget.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if workspaceID.Valid {
		value := int(workspaceID.Int64)
		widget.WorkspaceID = &value
	}
	if ownerUserID.Valid {
		value := int(ownerUserID.Int64)
		widget.OwnerUserID = &value
	}
	if strings.TrimSpace(queries) == "" {
		queries = "{}"
	}
	widget.Queries = json.RawMessage(queries)
	return &widget, nil
}
