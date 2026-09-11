package services

import (
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

const defaultEnumColor = "#3b82f6"

type coloredEnumFields struct {
	id          *int
	name        *string
	color       *string
	description *string
	createdAt   *time.Time
	updatedAt   *time.Time
}

type coloredEnumSpec[T EnumEntity] struct {
	tableName             string
	entityName            string
	dependencyTable       string
	dependencyColumn      string
	dependencyMessage     string
	auditResourceType     string
	defaultColor          string
	requireColor          bool
	caseInsensitiveUnique bool
	newEntity             func() T
	fields                func(T) coloredEnumFields
}

func newColoredEnumConfig[T EnumEntity](spec coloredEnumSpec[T]) EnumConfig {
	config := EnumConfig{
		TableName:      spec.tableName,
		EntityName:     spec.entityName,
		SelectColumns:  "id, name, color, description, created_at, updated_at",
		DefaultOrderBy: "name ASC",
		ScanRow: func(rows *sql.Rows) (EnumEntity, error) {
			return scanColoredEnum(rows, spec)
		},
		ScanSingleRow: func(row *sql.Row) (EnumEntity, error) {
			return scanColoredEnum(row, spec)
		},
		Validate: func(entity any, _ bool) string {
			return validateColoredEnum(spec.fields(asColoredEnum[T](entity)), spec.requireColor)
		},
		CheckUnique: func(db database.Database, entity any, excludeID int) (bool, error) {
			return coloredEnumExists(db, spec, asColoredEnum[T](entity), excludeID)
		},
		CheckDependencies: func(db database.Database, id int) string {
			return checkColoredEnumDependencies(db, spec, id)
		},
		InsertArgs: func(entity any, now time.Time) (string, string, []any) {
			fields := spec.fields(asColoredEnum[T](entity))
			return "name, color, description, created_at, updated_at",
				"?, ?, ?, ?, ?",
				[]any{*fields.name, *fields.color, *fields.description, now, now}
		},
		UpdateArgs: func(entity any, now time.Time) (string, []any) {
			fields := spec.fields(asColoredEnum[T](entity))
			return "name = ?, color = ?, description = ?, updated_at = ?",
				[]any{*fields.name, *fields.color, *fields.description, now}
		},
		AuditActionCreate: spec.auditResourceType + ".create",
		AuditActionUpdate: spec.auditResourceType + ".update",
		AuditActionDelete: spec.auditResourceType + ".delete",
		AuditResourceType: spec.auditResourceType,
	}
	if spec.defaultColor != "" {
		config.ApplyDefaults = func(entity any) {
			color := spec.fields(asColoredEnum[T](entity)).color
			if strings.TrimSpace(*color) == "" {
				*color = spec.defaultColor
			}
		}
	}
	return config
}

func asColoredEnum[T EnumEntity](entity any) T {
	typed, ok := entity.(T)
	if !ok {
		panic("colored enum config received an incompatible entity")
	}
	return typed
}

func scanColoredEnum[T EnumEntity](scanner rowScanner, spec coloredEnumSpec[T]) (EnumEntity, error) {
	entity := spec.newEntity()
	fields := spec.fields(entity)
	var description sql.NullString
	err := scanner.Scan(fields.id, fields.name, fields.color, &description, fields.createdAt, fields.updatedAt)
	if description.Valid {
		*fields.description = description.String
	}
	return entity, err
}

func validateColoredEnum(fields coloredEnumFields, requireColor bool) string {
	if strings.TrimSpace(*fields.name) == "" {
		return "Name is required"
	}
	if requireColor && strings.TrimSpace(*fields.color) == "" {
		return "Color is required"
	}
	return ""
}

func coloredEnumExists[T EnumEntity](db database.Database, spec coloredEnumSpec[T], entity T, excludeID int) (bool, error) {
	nameExpression := "name = ?"
	if spec.caseInsensitiveUnique {
		nameExpression = "LOWER(name) = LOWER(?)"
	}
	query := "SELECT COUNT(*) FROM " + spec.tableName + " WHERE " + nameExpression //nolint:gosec // table names come from internal specs
	args := []any{*spec.fields(entity).name}
	if excludeID != 0 {
		query += " AND id != ?"
		args = append(args, excludeID)
	}
	var count int
	err := db.QueryRow(query, args...).Scan(&count)
	return count > 0, err
}

func checkColoredEnumDependencies[T EnumEntity](db database.Database, spec coloredEnumSpec[T], id int) string {
	query := "SELECT COUNT(*) FROM " + spec.dependencyTable + " WHERE " + spec.dependencyColumn + " = ?" //nolint:gosec // table and column names come from internal specs
	var count int
	if err := db.QueryRow(query, id).Scan(&count); err != nil {
		slog.Error("dependency check failed", slog.Any("error", err), slog.String("table", spec.dependencyTable), slog.Int("id", id))
		return "Unable to verify dependencies — please try again"
	}
	if count > 0 {
		return spec.dependencyMessage
	}
	return ""
}

func milestoneCategorySpec() coloredEnumSpec[*models.MilestoneCategory] {
	return coloredEnumSpec[*models.MilestoneCategory]{
		tableName:             "milestone_categories",
		entityName:            "Milestone category",
		dependencyTable:       "milestones",
		dependencyColumn:      "category_id",
		dependencyMessage:     "Cannot delete milestone category that is in use by milestones",
		auditResourceType:     "milestone_category",
		defaultColor:          defaultEnumColor,
		caseInsensitiveUnique: true,
		newEntity:             func() *models.MilestoneCategory { return &models.MilestoneCategory{} },
		fields: func(entity *models.MilestoneCategory) coloredEnumFields {
			return coloredEnumFields{&entity.ID, &entity.Name, &entity.Color, &entity.Description, &entity.CreatedAt, &entity.UpdatedAt}
		},
	}
}

func collectionCategorySpec() coloredEnumSpec[*models.CollectionCategory] {
	return coloredEnumSpec[*models.CollectionCategory]{
		tableName:             "collection_categories",
		entityName:            "Collection category",
		dependencyTable:       "collections",
		dependencyColumn:      "category_id",
		dependencyMessage:     "Cannot delete collection category that is in use by collections",
		auditResourceType:     "collection_category",
		defaultColor:          defaultEnumColor,
		caseInsensitiveUnique: true,
		newEntity:             func() *models.CollectionCategory { return &models.CollectionCategory{} },
		fields: func(entity *models.CollectionCategory) coloredEnumFields {
			return coloredEnumFields{&entity.ID, &entity.Name, &entity.Color, &entity.Description, &entity.CreatedAt, &entity.UpdatedAt}
		},
	}
}

func channelCategorySpec() coloredEnumSpec[*models.ChannelCategory] {
	return coloredEnumSpec[*models.ChannelCategory]{
		tableName:             "channel_categories",
		entityName:            "Channel category",
		dependencyTable:       "channels",
		dependencyColumn:      "category_id",
		dependencyMessage:     "Cannot delete channel category that is in use by channels",
		auditResourceType:     "channel_category",
		defaultColor:          defaultEnumColor,
		caseInsensitiveUnique: true,
		newEntity:             func() *models.ChannelCategory { return &models.ChannelCategory{} },
		fields: func(entity *models.ChannelCategory) coloredEnumFields {
			return coloredEnumFields{&entity.ID, &entity.Name, &entity.Color, &entity.Description, &entity.CreatedAt, &entity.UpdatedAt}
		},
	}
}

func iterationTypeSpec() coloredEnumSpec[*models.IterationType] {
	return coloredEnumSpec[*models.IterationType]{
		tableName:         "iteration_types",
		entityName:        "Iteration type",
		dependencyTable:   "iterations",
		dependencyColumn:  "type_id",
		dependencyMessage: "Cannot delete iteration type that is in use by iterations",
		auditResourceType: "iteration_type",
		requireColor:      true,
		newEntity:         func() *models.IterationType { return &models.IterationType{} },
		fields: func(entity *models.IterationType) coloredEnumFields {
			return coloredEnumFields{&entity.ID, &entity.Name, &entity.Color, &entity.Description, &entity.CreatedAt, &entity.UpdatedAt}
		},
	}
}
