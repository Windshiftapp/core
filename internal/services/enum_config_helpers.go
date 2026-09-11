package services

import (
	"database/sql"
	"fmt"
	"log/slog"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

const statusSelectColumns = `s.id, COALESCE(s.builtin_key, ''), s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
	       sc.name as category_name, COALESCE(sc.builtin_key, ''), sc.description as category_description,
	       sc.color as category_color, sc.is_default as category_is_default, sc.is_completed`

const statusSelectQuery = `
	SELECT ` + statusSelectColumns + `
	FROM statuses s
	JOIN status_categories sc ON s.category_id = sc.id
	ORDER BY s.is_default DESC, sc.name ASC, s.name ASC`

const statusByIDQuery = `
	SELECT ` + statusSelectColumns + `
	FROM statuses s
	JOIN status_categories sc ON s.category_id = sc.id
	WHERE s.id = ?`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStatus(scanner rowScanner) (EnumEntity, error) {
	var status models.Status
	err := scanner.Scan(&status.ID, &status.BuiltinKey, &status.Name, &status.Description, &status.CategoryID,
		&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
		&status.CategoryName, &status.CategoryBuiltinKey, &status.CategoryDescription,
		&status.CategoryColor, &status.CategoryIsDefault, &status.IsCompleted)
	return &status, err
}

func scanLinkType(scanner rowScanner) (EnumEntity, error) {
	var linkType models.LinkType
	err := scanner.Scan(&linkType.ID, &linkType.BuiltinKey, &linkType.Name, &linkType.Description,
		&linkType.ForwardLabel, &linkType.ReverseLabel, &linkType.Color, &linkType.IsSystem,
		&linkType.Active, &linkType.CreatedAt, &linkType.UpdatedAt)
	return &linkType, err
}

func scanItemType(scanner rowScanner) (EnumEntity, error) {
	var itemType models.ItemType
	var description sql.NullString
	err := scanner.Scan(&itemType.ID, &itemType.BuiltinKey, &itemType.Name, &description, &itemType.IsDefault,
		&itemType.Icon, &itemType.Color, &itemType.HierarchyLevel, &itemType.SortOrder,
		&itemType.CreatedAt, &itemType.UpdatedAt)
	if description.Valid {
		itemType.Description = description.String
	}
	return &itemType, err
}

func checkSystemEnumMutation(
	db database.Database,
	id int,
	query string,
	notFoundMessage string,
	protectedMessage string,
) (allowed bool, status int, message string) {
	var isSystem bool
	if err := db.QueryRow(query, id).Scan(&isSystem); err != nil {
		return false, 404, notFoundMessage
	}
	if isSystem {
		return false, 403, protectedMessage
	}
	return true, 0, ""
}

func statusDependencyError(db database.Database, id int) string {
	var transitionCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?", id, id).Scan(&transitionCount); err != nil {
		slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "workflow_transitions"), slog.Int("id", id))
		return "Unable to verify dependencies — please try again"
	}
	if transitionCount > 0 {
		return "Cannot delete status that is in use by workflow transitions"
	}

	itemCount, err := repository.NewItemRepository(db).CountByField("status_id", id)
	if err != nil {
		slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "items"), slog.Int("id", id))
		return "Unable to verify dependencies — please try again"
	}
	if itemCount > 0 {
		return fmt.Sprintf("Cannot delete status that is in use by %d work item(s)", itemCount)
	}
	return ""
}
