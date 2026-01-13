package services

import (
	"database/sql"
	"fmt"
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
			c := entity.(*models.StatusCategory)
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
			c := entity.(*models.StatusCategory)
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
			db.QueryRow("SELECT COUNT(*) FROM statuses WHERE category_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete status category that is in use by statuses"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.StatusCategory)
			return "name, color, description, is_default, is_completed, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Color, c.Description, c.IsDefault, c.IsCompleted, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.StatusCategory)
			return "name = ?, color = ?, description = ?, is_default = ?, is_completed = ?, updated_at = ?",
				[]interface{}{c.Name, c.Color, c.Description, c.IsDefault, c.IsCompleted, now}
		},
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
			m := entity.(*models.MilestoneCategory)
			// Default color to blue if not provided
			if strings.TrimSpace(m.Color) == "" {
				m.Color = "#3b82f6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			m := entity.(*models.MilestoneCategory)
			if strings.TrimSpace(m.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			m := entity.(*models.MilestoneCategory)
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
			db.QueryRow("SELECT COUNT(*) FROM milestones WHERE category_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete milestone category that is in use by milestones"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			m := entity.(*models.MilestoneCategory)
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{m.Name, m.Color, m.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			m := entity.(*models.MilestoneCategory)
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{m.Name, m.Color, m.Description, now}
		},
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
			c := entity.(*models.CollectionCategory)
			// Default color to blue if not provided
			if strings.TrimSpace(c.Color) == "" {
				c.Color = "#3b82f6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.CollectionCategory)
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			c := entity.(*models.CollectionCategory)
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
			db.QueryRow("SELECT COUNT(*) FROM collections WHERE category_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete collection category that is in use by collections"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.CollectionCategory)
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Color, c.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.CollectionCategory)
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{c.Name, c.Color, c.Description, now}
		},
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
			c := entity.(*models.ChannelCategory)
			// Default color to blue if not provided
			if strings.TrimSpace(c.Color) == "" {
				c.Color = "#3b82f6"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.ChannelCategory)
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			c := entity.(*models.ChannelCategory)
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
			db.QueryRow("SELECT COUNT(*) FROM channels WHERE category_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete channel category that is in use by channels"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.ChannelCategory)
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Color, c.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.ChannelCategory)
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{c.Name, c.Color, c.Description, now}
		},
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
			i := entity.(*models.IterationType)
			if strings.TrimSpace(i.Name) == "" {
				return "Name is required"
			}
			if strings.TrimSpace(i.Color) == "" {
				return "Color is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			i := entity.(*models.IterationType)
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
			db.QueryRow("SELECT COUNT(*) FROM iterations WHERE iteration_type_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete iteration type that is in use by iterations"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			i := entity.(*models.IterationType)
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{i.Name, i.Color, i.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			i := entity.(*models.IterationType)
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]interface{}{i.Name, i.Color, i.Description, now}
		},
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
			h := entity.(*models.HierarchyLevel)
			if strings.TrimSpace(h.Name) == "" {
				return "Name is required"
			}
			if h.Level < 0 {
				return "Level must be 0 or greater"
			}
			return ""
		},

		// No CheckUnique - relies on DB UNIQUE constraint on `level` column
		// isUniqueConstraintError will catch duplicates and return 409

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			db.QueryRow("SELECT COUNT(*) FROM item_types WHERE hierarchy_level = (SELECT level FROM hierarchy_levels WHERE id = ?)", id).Scan(&count)
			if count > 0 {
				return "Cannot delete hierarchy level that is in use by item types"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			h := entity.(*models.HierarchyLevel)
			return "level, name, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]interface{}{h.Level, h.Name, h.Description, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			h := entity.(*models.HierarchyLevel)
			return "level = ?, name = ?, description = ?, updated_at = ?",
				[]interface{}{h.Level, h.Name, h.Description, now}
		},
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
			c := entity.(*models.ContactRole)
			if strings.TrimSpace(c.Name) == "" {
				return "Contact role name is required"
			}
			return ""
		},

		// No CheckUnique - relies on DB UNIQUE constraint on `name` column
		// isUniqueConstraintError will catch duplicates and return 409

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
			c := entity.(*models.ContactRole)
			// Force is_system to false for user-created roles
			return "name, description, is_system, created_at",
				"?, ?, false, ?",
				[]interface{}{c.Name, c.Description, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.ContactRole)
			return "name = ?, description = ?",
				[]interface{}{c.Name, c.Description}
		},
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
			s := entity.(*models.Status)
			if strings.TrimSpace(s.Name) == "" {
				return "Name is required"
			}
			if s.CategoryID <= 0 {
				return "Category ID is required"
			}
			return ""
		},

		ValidateFKs: func(db database.Database, entity interface{}) string {
			s := entity.(*models.Status)
			var exists bool
			err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM status_categories WHERE id = ?)", s.CategoryID).Scan(&exists)
			if err != nil || !exists {
				return "Status category not found"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			s := entity.(*models.Status)
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
			if id == constants.StatusIDOpen || id == constants.StatusIDClosed {
				return false, 403, "Cannot delete Open or Closed status - these are required by the system"
			}
			return true, 0, ""
		},

		CheckDependencies: func(db database.Database, id int) string {
			// Check workflow transitions
			var transitionCount int
			db.QueryRow("SELECT COUNT(*) FROM workflow_transitions WHERE from_status_id = ? OR to_status_id = ?", id, id).Scan(&transitionCount)
			if transitionCount > 0 {
				return "Cannot delete status that is in use by workflow transitions"
			}

			// Check items
			var itemCount int
			db.QueryRow("SELECT COUNT(*) FROM items WHERE status_id = ?", id).Scan(&itemCount)
			if itemCount > 0 {
				return fmt.Sprintf("Cannot delete status that is in use by %d work item(s)", itemCount)
			}

			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			s := entity.(*models.Status)
			return "name, description, category_id, is_default, created_at, updated_at",
				"?, ?, ?, ?, ?, ?",
				[]interface{}{s.Name, s.Description, s.CategoryID, s.IsDefault, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			s := entity.(*models.Status)
			return "name = ?, description = ?, category_id = ?, is_default = ?, updated_at = ?",
				[]interface{}{s.Name, s.Description, s.CategoryID, s.IsDefault, now}
		},
	}
}

// NewTimeCustomerConfig returns the configuration for CustomerOrganisation CRUD
func NewTimeCustomerConfig() EnumConfig {
	return EnumConfig{
		TableName:      "customer_organisations",
		EntityName:     "Customer",
		SelectColumns:  "id, name, email, description, active, custom_field_values, created_at, updated_at",
		DefaultOrderBy: "name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.CustomerOrganisation
			var customFieldValuesStr sql.NullString
			err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Description, &c.Active, &customFieldValuesStr, &c.CreatedAt, &c.UpdatedAt)
			if err == nil {
				c.CustomFieldValues = ParseCustomFieldValues(customFieldValuesStr)
			}
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.CustomerOrganisation
			var customFieldValuesStr sql.NullString
			err := row.Scan(&c.ID, &c.Name, &c.Email, &c.Description, &c.Active, &customFieldValuesStr, &c.CreatedAt, &c.UpdatedAt)
			if err == nil {
				c.CustomFieldValues = ParseCustomFieldValues(customFieldValuesStr)
			}
			return &c, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.CustomerOrganisation)
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.CustomerOrganisation)
			customFieldValuesJSON, _ := MarshalCustomFieldValues(c.CustomFieldValues)
			return "name, email, description, active, custom_field_values, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Email, c.Description, c.Active, customFieldValuesJSON, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.CustomerOrganisation)
			customFieldValuesJSON, _ := MarshalCustomFieldValues(c.CustomFieldValues)
			return "name = ?, email = ?, description = ?, active = ?, custom_field_values = ?, updated_at = ?",
				[]interface{}{c.Name, c.Email, c.Description, c.Active, customFieldValuesJSON, now}
		},
	}
}

// NewTimeProjectCategoryConfig returns the configuration for TimeProjectCategory CRUD
func NewTimeProjectCategoryConfig() EnumConfig {
	return EnumConfig{
		TableName:      "time_project_categories",
		EntityName:     "Project category",
		SelectColumns:  "id, name, description, color, display_order, created_at, updated_at",
		DefaultOrderBy: "display_order ASC, name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var c models.TimeProjectCategory
			var description, color sql.NullString
			err := rows.Scan(&c.ID, &c.Name, &description, &color, &c.DisplayOrder, &c.CreatedAt, &c.UpdatedAt)
			if description.Valid {
				c.Description = description.String
			}
			if color.Valid {
				c.Color = color.String
			}
			return &c, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var c models.TimeProjectCategory
			var description, color sql.NullString
			err := row.Scan(&c.ID, &c.Name, &description, &color, &c.DisplayOrder, &c.CreatedAt, &c.UpdatedAt)
			if description.Valid {
				c.Description = description.String
			}
			if color.Valid {
				c.Color = color.String
			}
			return &c, err
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			c := entity.(*models.TimeProjectCategory)
			if strings.TrimSpace(c.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			c := entity.(*models.TimeProjectCategory)
			var exists bool
			var err error
			if excludeID == 0 {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE name = ?)",
					c.Name).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE name = ? AND id != ?)",
					c.Name, excludeID).Scan(&exists)
			}
			return exists, err
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			db.QueryRow("SELECT COUNT(*) FROM time_projects WHERE category_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete project category that is in use by time projects"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			c := entity.(*models.TimeProjectCategory)
			return "name, description, color, display_order, created_at, updated_at",
				"?, ?, ?, ?, ?, ?",
				[]interface{}{c.Name, c.Description, c.Color, c.DisplayOrder, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			c := entity.(*models.TimeProjectCategory)
			return "name = ?, description = ?, color = ?, display_order = ?, updated_at = ?",
				[]interface{}{c.Name, c.Description, c.Color, c.DisplayOrder, now}
		},
	}
}

// NewTimeProjectConfig returns the configuration for TimeProject CRUD
func NewTimeProjectConfig() EnumConfig {
	return EnumConfig{
		TableName:  "time_projects",
		EntityName: "Time project",
		SelectColumns: `p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color, p.hourly_rate, p.active, p.created_at, p.updated_at,
		       co.name as customer_name, c.name as category_name, c.color as category_color`,
		SelectQuery: `
			SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color, p.hourly_rate, p.active, p.created_at, p.updated_at,
			       co.name as customer_name, c.name as category_name, c.color as category_color
			FROM time_projects p
			LEFT JOIN customer_organisations co ON p.customer_id = co.id
			LEFT JOIN time_project_categories c ON p.category_id = c.id
			ORDER BY p.active DESC, p.name ASC`,
		GetByIDQuery: `
			SELECT p.id, p.customer_id, p.category_id, p.name, p.description, p.status, p.color, p.hourly_rate, p.active, p.created_at, p.updated_at,
			       co.name as customer_name, c.name as category_name, c.color as category_color
			FROM time_projects p
			LEFT JOIN customer_organisations co ON p.customer_id = co.id
			LEFT JOIN time_project_categories c ON p.category_id = c.id
			WHERE p.id = ?`,
		DefaultOrderBy: "p.active DESC, p.name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var p models.TimeProject
			var customerID, categoryID sql.NullInt64
			var description, status, color, customerName, categoryName, categoryColor sql.NullString
			err := rows.Scan(&p.ID, &customerID, &categoryID, &p.Name, &description, &status, &color,
				&p.HourlyRate, &p.Active, &p.CreatedAt, &p.UpdatedAt,
				&customerName, &categoryName, &categoryColor)
			if customerID.Valid {
				id := int(customerID.Int64)
				p.CustomerID = &id
			}
			if categoryID.Valid {
				id := int(categoryID.Int64)
				p.CategoryID = &id
			}
			if description.Valid {
				p.Description = description.String
			}
			if status.Valid {
				p.Status = status.String
			}
			if color.Valid {
				p.Color = color.String
			}
			if customerName.Valid {
				p.CustomerName = customerName.String
			}
			if categoryName.Valid {
				p.CategoryName = categoryName.String
			}
			if categoryColor.Valid {
				p.CategoryColor = categoryColor.String
			}
			return &p, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var p models.TimeProject
			var customerID, categoryID sql.NullInt64
			var description, status, color, customerName, categoryName, categoryColor sql.NullString
			err := row.Scan(&p.ID, &customerID, &categoryID, &p.Name, &description, &status, &color,
				&p.HourlyRate, &p.Active, &p.CreatedAt, &p.UpdatedAt,
				&customerName, &categoryName, &categoryColor)
			if customerID.Valid {
				id := int(customerID.Int64)
				p.CustomerID = &id
			}
			if categoryID.Valid {
				id := int(categoryID.Int64)
				p.CategoryID = &id
			}
			if description.Valid {
				p.Description = description.String
			}
			if status.Valid {
				p.Status = status.String
			}
			if color.Valid {
				p.Color = color.String
			}
			if customerName.Valid {
				p.CustomerName = customerName.String
			}
			if categoryName.Valid {
				p.CategoryName = categoryName.String
			}
			if categoryColor.Valid {
				p.CategoryColor = categoryColor.String
			}
			return &p, err
		},

		ApplyDefaults: func(entity interface{}) {
			p := entity.(*models.TimeProject)
			if p.Status == "" {
				p.Status = "active"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			p := entity.(*models.TimeProject)
			if strings.TrimSpace(p.Name) == "" {
				return "Name is required"
			}
			return ""
		},

		ValidateFKs: func(db database.Database, entity interface{}) string {
			p := entity.(*models.TimeProject)
			// Validate customer if provided
			if p.CustomerID != nil && *p.CustomerID > 0 {
				var exists bool
				db.QueryRow("SELECT EXISTS(SELECT 1 FROM customer_organisations WHERE id = ?)", *p.CustomerID).Scan(&exists)
				if !exists {
					return "Customer not found"
				}
			}
			// Validate category if provided
			if p.CategoryID != nil && *p.CategoryID > 0 {
				var exists bool
				db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE id = ?)", *p.CategoryID).Scan(&exists)
				if !exists {
					return "Project category not found"
				}
			}
			return ""
		},

		CheckDependencies: func(db database.Database, id int) string {
			var count int
			db.QueryRow("SELECT COUNT(*) FROM time_worklogs WHERE project_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete time project that has worklogs"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			p := entity.(*models.TimeProject)
			return "customer_id, category_id, name, description, status, color, hourly_rate, active, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?, ?, ?, ?",
				[]interface{}{p.CustomerID, p.CategoryID, p.Name, p.Description, p.Status, p.Color, p.HourlyRate, p.Active, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			p := entity.(*models.TimeProject)
			return "customer_id = ?, category_id = ?, name = ?, description = ?, status = ?, color = ?, hourly_rate = ?, active = ?, updated_at = ?",
				[]interface{}{p.CustomerID, p.CategoryID, p.Name, p.Description, p.Status, p.Color, p.HourlyRate, p.Active, now}
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
			l := entity.(*models.LinkType)
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
			l := entity.(*models.LinkType)
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
			db.QueryRow("SELECT COUNT(*) FROM item_links WHERE link_type_id = ?", id).Scan(&count)
			if count > 0 {
				return "Cannot delete link type that is in use"
			}
			return ""
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			l := entity.(*models.LinkType)
			// Force is_system to false for user-created link types
			return "name, description, forward_label, reverse_label, color, is_system, active, created_at, updated_at",
				"?, ?, ?, ?, ?, false, ?, ?, ?",
				[]interface{}{l.Name, l.Description, l.ForwardLabel, l.ReverseLabel, l.Color, l.Active, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			l := entity.(*models.LinkType)
			return "name = ?, description = ?, forward_label = ?, reverse_label = ?, color = ?, active = ?, updated_at = ?",
				[]interface{}{l.Name, l.Description, l.ForwardLabel, l.ReverseLabel, l.Color, l.Active, now}
		},
	}
}

// NewRequestTypeConfig returns the configuration for RequestType CRUD
func NewRequestTypeConfig() EnumConfig {
	return EnumConfig{
		TableName:  "request_types",
		EntityName: "Request type",
		SelectColumns: `rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id, rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
		       c.name as channel_name, it.name as item_type_name`,
		SelectQuery: `
			SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id, rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
			       c.name as channel_name, it.name as item_type_name
			FROM request_types rt
			LEFT JOIN channels c ON rt.channel_id = c.id
			LEFT JOIN item_types it ON rt.item_type_id = it.id
			ORDER BY rt.channel_id ASC, rt.display_order ASC, rt.name ASC`,
		GetByIDQuery: `
			SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id, rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
			       c.name as channel_name, it.name as item_type_name
			FROM request_types rt
			LEFT JOIN channels c ON rt.channel_id = c.id
			LEFT JOIN item_types it ON rt.item_type_id = it.id
			WHERE rt.id = ?`,
		DefaultOrderBy: "rt.channel_id ASC, rt.display_order ASC, rt.name ASC",

		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			var rt models.RequestType
			var channelName, itemTypeName sql.NullString
			err := rows.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
				&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt,
				&channelName, &itemTypeName)
			if channelName.Valid {
				rt.ChannelName = channelName.String
			}
			if itemTypeName.Valid {
				rt.ItemTypeName = itemTypeName.String
			}
			return &rt, err
		},

		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			var rt models.RequestType
			var channelName, itemTypeName sql.NullString
			err := row.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
				&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt,
				&channelName, &itemTypeName)
			if channelName.Valid {
				rt.ChannelName = channelName.String
			}
			if itemTypeName.Valid {
				rt.ItemTypeName = itemTypeName.String
			}
			return &rt, err
		},

		ApplyDefaults: func(entity interface{}) {
			rt := entity.(*models.RequestType)
			if rt.Icon == "" {
				rt.Icon = "FileQuestion"
			}
			if rt.Color == "" {
				rt.Color = "#6366f1"
			}
		},

		Validate: func(entity interface{}, isUpdate bool) string {
			rt := entity.(*models.RequestType)
			if strings.TrimSpace(rt.Name) == "" {
				return "Name is required"
			}
			if rt.ChannelID <= 0 {
				return "Channel ID is required"
			}
			if rt.ItemTypeID <= 0 {
				return "Item type ID is required"
			}
			return ""
		},

		ValidateFKs: func(db database.Database, entity interface{}) string {
			rt := entity.(*models.RequestType)
			// Validate channel exists
			var channelExists bool
			db.QueryRow("SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", rt.ChannelID).Scan(&channelExists)
			if !channelExists {
				return "Channel not found"
			}
			// Validate item type exists
			var itemTypeExists bool
			db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", rt.ItemTypeID).Scan(&itemTypeExists)
			if !itemTypeExists {
				return "Item type not found"
			}
			return ""
		},

		CheckUnique: func(db database.Database, entity interface{}, excludeID int) (bool, error) {
			rt := entity.(*models.RequestType)
			var exists bool
			var err error
			// Name must be unique within the same channel
			if excludeID == 0 {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE channel_id = ? AND name = ?)",
					rt.ChannelID, rt.Name).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE channel_id = ? AND name = ? AND id != ?)",
					rt.ChannelID, rt.Name, excludeID).Scan(&exists)
			}
			return exists, err
		},

		InsertArgs: func(entity interface{}, now time.Time) (string, string, []interface{}) {
			rt := entity.(*models.RequestType)
			return "channel_id, name, description, item_type_id, icon, color, display_order, is_active, created_at, updated_at",
				"?, ?, ?, ?, ?, ?, ?, ?, ?, ?",
				[]interface{}{rt.ChannelID, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive, now, now}
		},

		UpdateArgs: func(entity interface{}, now time.Time) (string, []interface{}) {
			rt := entity.(*models.RequestType)
			return "channel_id = ?, name = ?, description = ?, item_type_id = ?, icon = ?, color = ?, display_order = ?, is_active = ?, updated_at = ?",
				[]interface{}{rt.ChannelID, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive, now}
		},
	}
}
