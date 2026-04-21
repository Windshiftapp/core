package services

import (
	"database/sql"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// BuildItemContext builds a map of item fields for condition evaluation.
// It loads the item via the item repository to enrich the context with
// creator, assignee, title, and custom fields used by script/role/group
// conditions.
func BuildItemContext(db database.Database, itemID, workspaceID int, currentStatusID, itemTypeID sql.NullInt64) map[string]interface{} {
	ctx := map[string]interface{}{
		"id":           itemID,
		"workspace_id": workspaceID,
	}
	if currentStatusID.Valid {
		ctx["status_id"] = int(currentStatusID.Int64)
	}
	if itemTypeID.Valid {
		ctx["item_type_id"] = int(itemTypeID.Int64)
	}

	if item, err := repository.NewItemRepository(db).FindByID(itemID); err == nil {
		if item.CreatorID != nil {
			ctx["creator_id"] = *item.CreatorID
		}
		if item.AssigneeID != nil {
			ctx["assignee_id"] = *item.AssigneeID
		}
		if item.Title != "" {
			ctx["title"] = item.Title
		}
		if len(item.CustomFieldValues) > 0 {
			ctx["custom_fields"] = item.CustomFieldValues
		}
	}

	return ctx
}
