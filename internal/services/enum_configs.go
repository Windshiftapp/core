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
)

// NewStatusCategoryConfig returns the configuration for StatusCategory CRUD
func NewStatusCategoryConfig() EnumConfig {
	return EnumConfig{
		TableName:      "status_categories",
		EntityName:     "Status category",
		SelectColumns:  "id, name, color, description, is_default, is_completed, created_at, updated_at",
		DefaultOrderBy: "is_default DESC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.StatusCategory
			err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.Description,
				&c.IsDefault, &c.IsCompleted, &c.CreatedAt, &c.UpdatedAt)
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.StatusCategory
			err := row.Scan(&c.ID, &c.Name, &c.Color, &c.Description,
				&c.IsDefault, &c.IsCompleted, &c.CreatedAt, &c.UpdatedAt)
			return &c, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
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

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
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

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.StatusCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.StatusCategory
			return "name, color, description, is_default, is_completed, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Color, c.Description, c.IsDefault, c.IsCompleted, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.StatusCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.StatusCategory
			return "name = ?, color = ?, description = ?, is_default = ?, is_completed = ?, updated_at = ?",
				[]interface{}{c.Name, c.Color, c.Description, c.IsDefault, c.IsCompleted, now}
		},

		AuditActionCreate: "status_category.create",
		AuditActionUpdate: "status_category.update",
		AuditActionDelete: "status_category.delete",
		AuditResourceType: "status_category",
	}
}

// NewMilestoneCategoryConfig returns the configuration for MilestoneCategory CRUD
func NewMilestoneCategoryConfig() EnumConfig {
	return EnumConfig{
		TableName:      "milestone_categories",
		EntityName:     "Milestone category",
		SelectColumns:  "id, name, color, description, created_at, updated_at",
		DefaultOrderBy: "name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var m models.MilestoneCategory
			var description sql.NullString
			err := rows.Scan(&m.ID, &m.Name, &m.Color, &description, &m.CreatedAt, &m.UpdatedAt)
			if description.Valid {
				m.Description = description.String
			}
			return &m, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var m models.MilestoneCategory
			var description sql.NullString
			err := row.Scan(&m.ID, &m.Name, &m.Color, &description, &m.CreatedAt, &m.UpdatedAt)
			if description.Valid {
				m.Description = description.String
			}
			return &m, err
		},

		ApplyDefaults: func(entity interface{}) {
			m := entity.(*models.MilestoneCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.MilestoneCategory
			// Default color to blue if not provided
			if strings.TrimSpace(m.Color) == "" {
				m.Color = "#3b82f6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			m := entity.(*models.MilestoneCategory) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(m.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			m := entity.(*models.MilestoneCategory) //nolint:errcheck // type assertion is safe here
			var count int
			var err error
			// Case-insensitive uniqueness check
			if excludeID == 0 {
				err = db.QueryRow("SELECT COUNT(*) FROM milestone_categories WHERE LOWER(name) = LOWER(?)",
					m.Name).Scan(&count)
			} else {
				err = db.QueryRow("SELECT COUNT(*) FROM milestone_categories WHERE LOWER(name) = LOWER(?) AND id != ?",
					m.Name, excludeID).Scan(&count)
			}
			return count > 0, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM milestones WHERE category_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "milestones"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete milestone category that is in use by milestones"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			m := entity.(*models.MilestoneCategory) //nolint:errcheck // type assertion is safe here
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{m.Name, m.Color, m.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			m := entity.(*models.MilestoneCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.MilestoneCategory
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{m.Name, m.Color, m.Description, now}
		},

		AuditActionCreate: "milestone_category.create",
		AuditActionUpdate: "milestone_category.update",
		AuditActionDelete: "milestone_category.delete",
		AuditResourceType: "milestone_category",
	}
}

// NewCollectionCategoryConfig returns the configuration for CollectionCategory CRUD
func NewCollectionCategoryConfig() EnumConfig {
	return EnumConfig{
		TableName:      "collection_categories",
		EntityName:     "Collection category",
		SelectColumns:  "id, name, color, description, created_at, updated_at",
		DefaultOrderBy: "name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.CollectionCategory
			var description sql.NullString
			err := rows.Scan(&c.ID, &c.Name, &c.Color, &description, &c.CreatedAt, &c.UpdatedAt)
			if description.Valid {
				c.Description = description.String
			}
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.CollectionCategory
			var description sql.NullString
			err := row.Scan(&c.ID, &c.Name, &c.Color, &description, &c.CreatedAt, &c.UpdatedAt)
			if description.Valid {
				c.Description = description.String
			}
			return &c, err
		},

		ApplyDefaults: func(entity interface{}) {
			c := entity.(*models.CollectionCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.CollectionCategory
			// Default color to blue if not provided
			if strings.TrimSpace(c.Color) == "" {
				c.Color = "#3b82f6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.CollectionCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.CollectionCategory
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			c := entity.(*models.CollectionCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.CollectionCategory
			var count int
			var err error
			// Case-insensitive uniqueness check
			if excludeID == 0 {
				err = db.QueryRow("SELECT COUNT(*) FROM collection_categories WHERE LOWER(name) = LOWER(?)",
					c.Name).Scan(&count)
			} else {
				err = db.QueryRow("SELECT COUNT(*) FROM collection_categories WHERE LOWER(name) = LOWER(?) AND id != ?",
					c.Name, excludeID).Scan(&count)
			}
			return count > 0, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM collections WHERE category_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "collections"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete collection category that is in use by collections"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.CollectionCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.CollectionCategory
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Color, c.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.CollectionCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.CollectionCategory
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{c.Name, c.Color, c.Description, now}
		},

		AuditActionCreate: "collection_category.create",
		AuditActionUpdate: "collection_category.update",
		AuditActionDelete: "collection_category.delete",
		AuditResourceType: "collection_category",
	}
}

// NewChannelCategoryConfig returns the configuration for ChannelCategory CRUD
func NewChannelCategoryConfig() EnumConfig {
	return EnumConfig{
		TableName:      "channel_categories",
		EntityName:     "Channel category",
		SelectColumns:  "id, name, color, description, created_at, updated_at",
		DefaultOrderBy: "name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.ChannelCategory
			var description sql.NullString
			err := rows.Scan(&c.ID, &c.Name, &c.Color, &description, &c.CreatedAt, &c.UpdatedAt)
			if description.Valid {
				c.Description = description.String
			}
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.ChannelCategory
			var description sql.NullString
			err := row.Scan(&c.ID, &c.Name, &c.Color, &description, &c.CreatedAt, &c.UpdatedAt)
			if description.Valid {
				c.Description = description.String
			}
			return &c, err
		},

		ApplyDefaults: func(entity interface{}) {
			c := entity.(*models.ChannelCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.ChannelCategory
			// Default color to blue if not provided
			if strings.TrimSpace(c.Color) == "" {
				c.Color = "#3b82f6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.ChannelCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.ChannelCategory
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			c := entity.(*models.ChannelCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.ChannelCategory
			var count int
			var err error
			// Case-insensitive uniqueness check
			if excludeID == 0 {
				err = db.QueryRow("SELECT COUNT(*) FROM channel_categories WHERE LOWER(name) = LOWER(?)",
					c.Name).Scan(&count)
			} else {
				err = db.QueryRow("SELECT COUNT(*) FROM channel_categories WHERE LOWER(name) = LOWER(?) AND id != ?",
					c.Name, excludeID).Scan(&count)
			}
			return count > 0, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM channels WHERE category_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "channels"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete channel category that is in use by channels"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.ChannelCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.ChannelCategory
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Color, c.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.ChannelCategory) //nolint:errcheck // type assertion is safe here - entity is always *models.ChannelCategory
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{c.Name, c.Color, c.Description, now}
		},

		AuditActionCreate: "channel_category.create",
		AuditActionUpdate: "channel_category.update",
		AuditActionDelete: "channel_category.delete",
		AuditResourceType: "channel_category",
	}
}

// NewIterationTypeConfig returns the configuration for IterationType CRUD
func NewIterationTypeConfig() EnumConfig {
	return EnumConfig{
		TableName:      "iteration_types",
		EntityName:     "Iteration type",
		SelectColumns:  "id, name, color, description, created_at, updated_at",
		DefaultOrderBy: "name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var i models.IterationType
			var description sql.NullString
			err := rows.Scan(&i.ID, &i.Name, &i.Color, &description, &i.CreatedAt, &i.UpdatedAt)
			if description.Valid {
				i.Description = description.String
			}
			return &i, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var i models.IterationType
			var description sql.NullString
			err := row.Scan(&i.ID, &i.Name, &i.Color, &description, &i.CreatedAt, &i.UpdatedAt)
			if description.Valid {
				i.Description = description.String
			}
			return &i, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			i := entity.(*models.IterationType) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(i.Name) == "" {
				return "Name is required"
			}
			if strings.TrimSpace(i.Color) == "" {
				return "Color is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			i := entity.(*models.IterationType) //nolint:errcheck // type assertion is safe here
			var count int
			var err error
			if excludeID == 0 {
				err = db.QueryRow("SELECT COUNT(*) FROM iteration_types WHERE name = ?",
					i.Name).Scan(&count)
			} else {
				err = db.QueryRow("SELECT COUNT(*) FROM iteration_types WHERE name = ? AND id != ?",
					i.Name, excludeID).Scan(&count)
			}
			return count > 0, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM iterations WHERE type_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "iterations"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return "Cannot delete iteration type that is in use by iterations"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			i := entity.(*models.IterationType) //nolint:errcheck // type assertion is safe here
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{i.Name, i.Color, i.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			i := entity.(*models.IterationType) //nolint:errcheck // type assertion is safe here
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{i.Name, i.Color, i.Description, now}
		},

		AuditActionCreate: "iteration_type.create",
		AuditActionUpdate: "iteration_type.update",
		AuditActionDelete: "iteration_type.delete",
		AuditResourceType: "iteration_type",
	}
}

// NewHierarchyLevelConfig returns the configuration for HierarchyLevel CRUD
func NewHierarchyLevelConfig() EnumConfig {
	return EnumConfig{
		TableName:      "hierarchy_levels",
		EntityName:     "Hierarchy level",
		SelectColumns:  "id, level, name, description, created_at, updated_at",
		DefaultOrderBy: "level ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var h models.HierarchyLevel
			err := rows.Scan(&h.ID, &h.Level, &h.Name, &h.Description, &h.CreatedAt, &h.UpdatedAt)
			return &h, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var h models.HierarchyLevel
			err := row.Scan(&h.ID, &h.Level, &h.Name, &h.Description, &h.CreatedAt, &h.UpdatedAt)
			return &h, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
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

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			h := entity.(*models.HierarchyLevel) //nolint:errcheck // type assertion is safe here
			return "level, name, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{h.Level, h.Name, h.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			h := entity.(*models.HierarchyLevel) //nolint:errcheck // type assertion is safe here
			return "level = ?, name = ?, description = ?, updated_at = ?",
				[]interface{}{h.Level, h.Name, h.Description, now}
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
				c.CreatedAt, _ = ParseTimestamp(createdAtStr)
			}
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.ContactRole
			var createdAtStr string
			err := row.Scan(&c.ID, &c.Name, &c.Description, &c.IsSystem, &createdAtStr)
			if err == nil {
				c.CreatedAt, _ = ParseTimestamp(createdAtStr)
			}
			return &c, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.ContactRole) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(c.Name) == "" {
				return "Contact role name is required"
			}
			return ""
		},

		// No CheckUnique - relies on DB UNIQUE constraint on `name` column
		// database.IsUniqueConstraintError will catch duplicates and return 409

		BeforeUpdate: func(db database.Database, id int, entity interface{}) (bool, int, string) {
			var isSystem bool
			err := db.QueryRow("SELECT is_system FROM contact_roles WHERE id = ?", id).Scan(&isSystem)
			if err != nil {
				return false, 404, "Contact role not found"
			}
			if isSystem {
				return false, 403, "System contact roles cannot be modified"
			}
			return true, 0, ""
		},

		BeforeDelete: func(db database.Database, id int) (bool, int, string) {
			var isSystem bool
			err := db.QueryRow("SELECT is_system FROM contact_roles WHERE id = ?", id).Scan(&isSystem)
			if err != nil {
				return false, 404, "Contact role not found"
			}
			if isSystem {
				return false, 403, "System contact roles cannot be deleted"
			}
			return true, 0, ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.ContactRole) //nolint:errcheck // type assertion is safe here
			// Force is_system to false for user-created roles
			return "name, description, is_system, created_at",
				"?, ?, false, ?",
				[]interface{}{c.Name, c.Description, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.ContactRole) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?",
				[]interface{}{c.Name, c.Description}
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
		TableName:  "statuses",
		EntityName: "Status",
		SelectColumns: `s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color`,
		SelectQuery: `
			SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
			       sc.name as category_name, sc.color as category_color
			FROM statuses s
			JOIN status_categories sc ON s.category_id = sc.id
			ORDER BY s.is_default DESC, sc.name ASC, s.name ASC`,
		GetByIDQuery: `
			SELECT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
			       sc.name as category_name, sc.color as category_color
			FROM statuses s
			JOIN status_categories sc ON s.category_id = sc.id
			WHERE s.id = ?`,
		DefaultOrderBy: "s.is_default DESC, sc.name ASC, s.name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var s models.Status
			err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.CategoryID,
				&s.IsDefault, &s.CreatedAt, &s.UpdatedAt,
				&s.CategoryName, &s.CategoryColor)
			return &s, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var s models.Status
			err := row.Scan(&s.ID, &s.Name, &s.Description, &s.CategoryID,
				&s.IsDefault, &s.CreatedAt, &s.UpdatedAt,
				&s.CategoryName, &s.CategoryColor)
			return &s, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(s.Name) == "" {
				return "Name is required"
			}
			if s.CategoryID <= 0 {
				return "Category ID is required"
			}
			return ""
		},

		ValidateFKs: func(db database.Database, entity interface{}) string {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			var exists bool
			err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", s.CategoryID).Scan(&exists)
			if err != nil || !exists {
				return "Status category not found"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
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

		CheckDependencies: func(db database.Database, id int) string {
			// Check workflow transitions
			var transitionCount int
			if err := db.QueryRow("SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?", id, id).Scan(&transitionCount); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "workflow_transitions"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if transitionCount > 0 {
				return "Cannot delete status that is in use by workflow transitions"
			}

			// Check items
			var itemCount int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE status_id = ?", id).Scan(&itemCount); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "items"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if itemCount > 0 {
				return fmt.Sprintf("Cannot delete status that is in use by %d work item(s)", itemCount)
			}

			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			return "name, description, category_id, is_default, created_at, updated_at",
				"?, ?, ?, ?, ?, ?",
				[]interface{}{s.Name, s.Description, s.CategoryID, s.IsDefault, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			s := entity.(*models.Status) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?, category_id = ?, is_default = ?, updated_at = ?",
				[]interface{}{s.Name, s.Description, s.CategoryID, s.IsDefault, now}
		},
	}
}

// NewLinkTypeConfig returns the configuration for LinkType CRUD
func NewLinkTypeConfig() EnumConfig {
	return EnumConfig{
		TableName:      "link_types",
		EntityName:     "Link type",
		SelectColumns:  "id, name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at",
		DefaultOrderBy: "is_system DESC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var l models.LinkType
			err := rows.Scan(&l.ID, &l.Name, &l.Description, &l.ForwardLabel, &l.ReverseLabel,
				&l.Color, &l.IsSystem, &l.Active, &l.CreatedAt, &l.UpdatedAt)
			return &l, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var l models.LinkType
			err := row.Scan(&l.ID, &l.Name, &l.Description, &l.ForwardLabel, &l.ReverseLabel,
				&l.Color, &l.IsSystem, &l.Active, &l.CreatedAt, &l.UpdatedAt)
			return &l, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
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

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
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

		BeforeUpdate: func(db database.Database, id int, entity interface{}) (bool, int, string) {
			var isSystem bool
			err := db.QueryRow("SELECT is_system FROM link_types WHERE id = ?", id).Scan(&isSystem)
			if err != nil {
				return false, 404, "Link type not found"
			}
			if isSystem {
				return false, 403, "System link types cannot be modified"
			}
			return true, 0, ""
		},

		BeforeDelete: func(db database.Database, id int) (bool, int, string) {
			var isSystem bool
			err := db.QueryRow("SELECT is_system FROM link_types WHERE id = ?", id).Scan(&isSystem)
			if err != nil {
				return false, 404, "Link type not found"
			}
			if isSystem {
				return false, 403, "System link types cannot be deleted"
			}
			return true, 0, ""
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

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			l := entity.(*models.LinkType) //nolint:errcheck // type assertion is safe here
			// Force is_system to false for user-created link types
			return "name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at",
				"?, ?, ?, ?, ?, false, ?, ?, ?",
				[]interface{}{l.Name, l.Description, l.ForwardLabel, l.ReverseLabel, l.Color, l.Active, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			l := entity.(*models.LinkType) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?, forward_label = ?, reverse_label = ?, color = ?, active = ?, updated_at = ?",
				[]interface{}{l.Name, l.Description, l.ForwardLabel, l.ReverseLabel, l.Color, l.Active, now}
		},
	}
}

// NewItemTypeConfig returns the configuration for ItemType CRUD
// Used primarily by the Jira import to create item types
func NewItemTypeConfig() EnumConfig {
	return EnumConfig{
		TableName:      "item_types",
		EntityName:     "Item type",
		SelectColumns:  "id, name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at",
		DefaultOrderBy: "hierarchy_level ASC, sort_order ASC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var it models.ItemType
			var description sql.NullString
			err := rows.Scan(&it.ID, &it.Name, &description, &it.IsDefault,
				&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder,
				&it.CreatedAt, &it.UpdatedAt)
			if description.Valid {
				it.Description = description.String
			}
			return &it, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var it models.ItemType
			var description sql.NullString
			err := row.Scan(&it.ID, &it.Name, &description, &it.IsDefault,
				&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder,
				&it.CreatedAt, &it.UpdatedAt)
			if description.Valid {
				it.Description = description.String
			}
			return &it, err
		},

		ApplyDefaults: func(entity interface{}) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			if it.Icon == "" {
				it.Icon = "Circle"
			}
			if it.Color == "" {
				it.Color = "#3B82F6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			if strings.TrimSpace(it.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
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
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE item_type_id = ?", id).Scan(&count); err != nil {
				slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", "items"), slog.Int("id", id))
				return "Unable to verify dependencies — please try again"
			}
			if count > 0 {
				return fmt.Sprintf("Cannot delete item type that is in use by %d work item(s)", count)
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			return "name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?, ?, ?",
				[]interface{}{it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			it := entity.(*models.ItemType) //nolint:errcheck // type assertion is safe here
			return "name = ?, description = ?, is_default = ?, icon = ?, color = ?, hierarchy_level = ?, sort_order = ?, updated_at = ?",
				[]interface{}{it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now}
		},
	}
}
