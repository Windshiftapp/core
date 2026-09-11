package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/constants"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// NewStatusCategoryConfig returns the configuration for StatusCategory CRUD
func NewStatusCategoryConfig() EnumConfig {
	return EnumConfig{
		TableName:      "status_categories",
		EntityName:     "Status category",
		SelectColumns:  "id, COALESCE(builtin_key, ''), name, color, description, is_default, is_completed, created_at, updated_at",
		DefaultOrderBy: "is_default DESC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.StatusCategory
			err := rows.Scan(&c.ID, &c.BuiltinKey, &c.Name, &c.Color, &c.Description,
				&c.IsDefault, &c.IsCompleted, &c.CreatedAt, &c.UpdatedAt)
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.StatusCategory
			err := row.Scan(&c.ID, &c.BuiltinKey, &c.Name, &c.Color, &c.Description,
				&c.IsDefault, &c.IsCompleted, &c.CreatedAt, &c.UpdatedAt)
			return &c, err
		},

		Validate: func(entity any, isUpdate bool) string {
			c := entity.(*models.StatusCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.StatusCategory
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			if strings.TrimSpace(c.Color) == "" {
				return "Color is required"
			}
			if !ValidateColor(c.Color) {
				return "Color must be a valid color name (e.g., blue, red) or hex color (e.g., #3b82f6)"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity any, excludeID int) (bool, error) {
			c := entity.(*models.StatusCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.StatusCategory
			var exists bool
			var err error
			if excludeID == 0 {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE name = ?)",
					c.Name).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE name = ? AND id != ?)",
					c.Name, excludeID).Scan(&exists)
			}
			return exists, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM statuses WHERE category_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "statuses"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete status category that is in use by statuses"
			}
			return ""
		},

		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			c := entity.(*models.StatusCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.StatusCategory
			return "name, color, description, is_default, is_completed, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?",
				[]any{c.Name, c.Color, c.Description, c.IsDefault, c.IsCompleted, now, now}
		},

		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			c := entity.(*models.StatusCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.StatusCategory
			return "name = ?, color = ?, description = ?, is_default = ?, is_completed = ?, updated_at = ?",
				[]any{c.Name, c.Color, c.Description, c.IsDefault, c.IsCompleted, now}
		},

		AuditActionCreate: "status_category.create",
		AuditActionUpdate: "status_category.update",
		AuditActionDelete: "status_category.delete",
		AuditResourceType: "status_category",
	}
}

// NewMilestoneCategoryConfig returns the configuration for MilestoneCategory CRUD
func NewMilestoneCategoryConfig() EnumConfig {
	return newColoredEnumConfig(milestoneCategorySpec())
}

// NewCollectionCategoryConfig returns the configuration for CollectionCategory CRUD
func NewCollectionCategoryConfig() EnumConfig {
	return newColoredEnumConfig(collectionCategorySpec())
}

// NewChannelCategoryConfig returns the configuration for ChannelCategory CRUD
func NewChannelCategoryConfig() EnumConfig {
	return newColoredEnumConfig(channelCategorySpec())
}

// NewIterationTypeConfig returns the configuration for IterationType CRUD
func NewIterationTypeConfig() EnumConfig {
	return newColoredEnumConfig(iterationTypeSpec())
}

// NewHierarchyLevelConfig returns the configuration for HierarchyLevel CRUD
func NewHierarchyLevelConfig() EnumConfig {
	return EnumConfig{
		TableName:      "hierarchy_levels",
		EntityName:     "Hierarchy level",
		SelectColumns:  "id, COALESCE(builtin_key, ''), level, name, description, created_at, updated_at",
		DefaultOrderBy: "level ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var h models.HierarchyLevel
			err := rows.Scan(&h.ID, &h.BuiltinKey, &h.Level, &h.Name, &h.Description, &h.CreatedAt, &h.UpdatedAt)
			return &h, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var h models.HierarchyLevel
			err := row.Scan(&h.ID, &h.BuiltinKey, &h.Level, &h.Name, &h.Description, &h.CreatedAt, &h.UpdatedAt)
			return &h, err
		},

		Validate: func(entity any, isUpdate bool) string {
			h := entity.(*models.HierarchyLevel) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(h.Name) == "" {
				return "Name is required"
			}
			if h.Level < 0 {
				return "Level must be 0 or greater"
			}
			return ""
		},

		// No CheckUnique - relies on DB UNIQUE constraint on `level` column
		// database.IsUniqueConstraintError will catch duplicates and return 409

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM item_types WHERE hierarchy_level = (SELECT level FROM hierarchy_levels WHERE id = ?)", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "item_types"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete hierarchy level that is in use by item types"
			}
			return ""
		},

		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			h := entity.(*models.HierarchyLevel) //nolint:errcheck // type assertion is safe here
			return "level, name, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]any{h.Level, h.Name, h.Description, now, now}
		},

		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			h := entity.(*models.HierarchyLevel) //nolint:errcheck // type assertion is safe here
			return "level = ?, name = ?, description = ?, updated_at = ?",
				[]any{h.Level, h.Name, h.Description, now}
		},

		AuditActionCreate: "hierarchy_level.create",
		AuditActionUpdate: "hierarchy_level.update",
		AuditActionDelete: "hierarchy_level.delete",
		AuditResourceType: "hierarchy_level",
	}
}

// NewContactRoleConfig returns the configuration for ContactRole CRUD
func NewContactRoleConfig() EnumConfig {
	return EnumConfig{
		TableName:      "contact_roles",
		EntityName:     "Contact role",
		SelectColumns:  "id, name, description, is_system, created_at",
		DefaultOrderBy: "is_system DESC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.ContactRole
			var createdAtStr string
			err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.IsSystem, &createdAtStr)
			if err == nil {
				c.CreatedAt = ParseTimestamp(createdAtStr)
			}
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.ContactRole
			var createdAtStr string
			err := row.Scan(&c.ID, &c.Name, &c.Description, &c.IsSystem, &createdAtStr)
			if err == nil {
				c.CreatedAt = ParseTimestamp(createdAtStr)
			}
			return &c, err
		},

		Validate: func(entity any, isUpdate bool) string {
			c := entity.(*models.ContactRole) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(c.Name) == "" {
				return "Contact role name is required"
			}
			return ""
		},

		// No CheckUnique - relies on DB UNIQUE constraint on `name` column
		// database.IsUniqueConstraintError will catch duplicates and return 409

		BeforeUpdate: func(db database.Database, id int, entity any) (bool, int, string) {
			return checkSystemEnumMutation(db, id,
				"SELECT is_system FROM contact_roles WHERE id = ?",
				"Contact role not found", "System contact roles cannot be modified")
		},

		BeforeDelete: func(db database.Database, id int) (bool, int, string) {
			return checkSystemEnumMutation(db, id,
				"SELECT is_system FROM contact_roles WHERE id = ?",
				"Contact role not found", "System contact roles cannot be deleted")
		},

		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			c := entity.(*models.ContactRole) //nolint:errcheck // type assertion is safe here
			// Force is_system to false for user-created roles
			return "name, description, is_system, created_at",
				"?, ?, false, ?",
				[]any{c.Name, c.Description, now}
		},

		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			c := entity.(*models.ContactRole) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?",
				[]any{c.Name, c.Description}
		},

		AuditActionCreate: "contact_role.create",
		AuditActionUpdate: "contact_role.update",
		AuditActionDelete: "contact_role.delete",
		AuditResourceType: "contact_role",
	}
}

// NewStatusConfig returns the configuration for Status CRUD
func NewStatusConfig() EnumConfig {
	return EnumConfig{
		TableName:      "statuses",
		EntityName:     "Status",
		SelectColumns:  statusSelectColumns,
		SelectQuery:    statusSelectQuery,
		GetByIDQuery:   statusByIDQuery,
		DefaultOrderBy: "s.is_default DESC, sc.name ASC, s.name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			return scanStatus(rows)
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			return scanStatus(row)
		},

		Validate: func(entity any, isUpdate bool) string {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(s.Name) == "" {
				return "Name is required"
			}
			if s.CategoryID <= 0 {
				return "Category ID is required"
			}
			return ""
		},

		ValidateFKs: func(db database.Database, entity any) string {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			var exists bool
			err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", s.CategoryID).Scan(&exists)
			if err != nil || !exists {
				return "Status category not found"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity any, excludeID int) (bool, error) {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			var exists bool
			var err error
			if excludeID == 0 {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ?)",
					s.Name).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE name = ? AND id != ?)",
					s.Name, excludeID).Scan(&exists)
			}
			return exists, err
		},

		BeforeDelete: func(db database.Database, id int) (bool, int, string) {
			// Protect system-critical statuses from deletion
			if id == constants.StatusIDOpen || id == constants.StatusIDDone {
				return false, 403, "Cannot delete Open or Done status - these are required by the system"
			}
			return true, 0, ""
		},

		CheckDependencies: statusDependencyError,

		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			return "name, description, category_id, is_default, created_at, updated_at",
				"?, ?, ?, ?, ?, ?",
				[]any{s.Name, s.Description, s.CategoryID, s.IsDefault, now, now}
		},

		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?, category_id = ?, is_default = ?, updated_at = ?",
				[]any{s.Name, s.Description, s.CategoryID, s.IsDefault, now}
		},
	}
}

// NewLinkTypeConfig returns the configuration for LinkType CRUD
func NewLinkTypeConfig() EnumConfig {
	return EnumConfig{
		TableName:      "link_types",
		EntityName:     "Link type",
		SelectColumns:  "id, COALESCE(builtin_key, ''), name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at",
		DefaultOrderBy: "is_system DESC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			return scanLinkType(rows)
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			return scanLinkType(row)
		},

		Validate: func(entity any, isUpdate bool) string {
			l := entity.(*models.LinkType) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(l.Name) == "" {
				return "Name is required"
			}
			if strings.TrimSpace(l.ForwardLabel) == "" {
				return "Forward label is required"
			}
			if strings.TrimSpace(l.ReverseLabel) == "" {
				return "Reverse label is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity any, excludeID int) (bool, error) {
			l := entity.(*models.LinkType) //nolint:errcheck // type assertion is safe here
			var exists bool
			var err error
			if excludeID == 0 {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM link_types WHERE name = ?)",
					l.Name).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM link_types WHERE name = ? AND id != ?)",
					l.Name, excludeID).Scan(&exists)
			}
			return exists, err
		},

		BeforeUpdate: func(db database.Database, id int, entity any) (bool, int, string) {
			return checkSystemEnumMutation(db, id,
				"SELECT is_system FROM link_types WHERE id = ?",
				"Link type not found", "System link types cannot be modified")
		},

		BeforeDelete: func(db database.Database, id int) (bool, int, string) {
			return checkSystemEnumMutation(db, id,
				"SELECT is_system FROM link_types WHERE id = ?",
				"Link type not found", "System link types cannot be deleted")
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM item_links WHERE link_type_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "item_links"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete link type that is in use"
			}
			return ""
		},

		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			l := entity.(*models.LinkType) //nolint:errcheck // type assertion is safe here
			// Force is_system to false for user-created link types
			return "name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at",
				"?, ?, ?, ?, ?, false, ?, ?, ?",
				[]any{l.Name, l.Description, l.ForwardLabel, l.ReverseLabel, l.Color, l.Active, now, now}
		},

		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			l := entity.(*models.LinkType) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?, forward_label = ?, reverse_label = ?, color = ?, active = ?, updated_at = ?",
				[]any{l.Name, l.Description, l.ForwardLabel, l.ReverseLabel, l.Color, l.Active, now}
		},
	}
}

// NewItemTypeConfig returns the configuration for ItemType CRUD
// Used primarily by the Jira import to create item types
func NewItemTypeConfig() EnumConfig {
	return EnumConfig{
		TableName:      "item_types",
		EntityName:     "Item type",
		SelectColumns:  "id, COALESCE(builtin_key, ''), name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at",
		DefaultOrderBy: "hierarchy_level ASC, sort_order ASC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			return scanItemType(rows)
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			return scanItemType(row)
		},

		ApplyDefaults: func(entity any) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			if it.Icon == "" {
				it.Icon = "Circle"
			}
			if it.Color == "" {
				it.Color = "#3B82F6"
			}
		},

		Validate: func(entity any, isUpdate bool) string {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(it.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity any, excludeID int) (bool, error) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			var exists bool
			var err error
			if excludeID == 0 {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE name = ?)",
					it.Name).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE name = ? AND id != ?)",
					it.Name, excludeID).Scan(&exists)
			}
			return exists, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			count, err := repository.NewItemRepository(db).CountByField("item_type_id", id)
			if err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "items"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return fmt.Sprintf("Cannot delete item type that is in use by %d work item(s)", count)
			}
			return ""
		},

		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			return "name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?, ?, ?",
				[]any{it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, now}
		},

		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?, is_default = ?, icon = ?, color = ?, hierarchy_level = ?, sort_order = ?, updated_at = ?",
				[]any{it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now}
		},
	}
}
