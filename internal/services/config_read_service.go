package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
)

// ConfigReadService encapsulates business logic for reading configuration entities:
// item types, priorities, and custom fields.
type ConfigReadService struct {
	db database.Database
}

// NewConfigReadService creates a new ConfigReadService.
func NewConfigReadService(db database.Database) *ConfigReadService {
	return &ConfigReadService{db: db}
}

// ========================================
// Item Types
// ========================================

// ItemTypeResult represents an item type for API responses.
type ItemTypeResult struct {
	ID             int
	Name           string
	Description    string
	Icon           string
	Color          string
	HierarchyLevel int
	SortOrder      int
	IsDefault      bool
}

// ListItemTypes retrieves all item types ordered by hierarchy and sort order.
func (s *ConfigReadService) ListItemTypes() ([]ItemTypeResult, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, icon, color, hierarchy_level, sort_order, is_default
		FROM item_types
		ORDER BY hierarchy_level, sort_order, name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list item types: %w", err)
	}
	defer rows.Close()

	var types []ItemTypeResult
	for rows.Next() {
		var t ItemTypeResult
		var description, icon, color sql.NullString
		err := rows.Scan(&t.ID, &t.Name, &description, &icon, &color, &t.HierarchyLevel, &t.SortOrder, &t.IsDefault)
		if err != nil {
			continue
		}
		t.Description = description.String
		t.Icon = icon.String
		t.Color = color.String
		types = append(types, t)
	}

	if types == nil {
		types = []ItemTypeResult{}
	}

	return types, nil
}

// GetItemType retrieves an item type by ID.
func (s *ConfigReadService) GetItemType(id int) (*ItemTypeResult, error) {
	var t ItemTypeResult
	var description, icon, color sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, description, icon, color, hierarchy_level, sort_order, is_default
		FROM item_types WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &description, &icon, &color, &t.HierarchyLevel, &t.SortOrder, &t.IsDefault)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item type not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get item type: %w", err)
	}

	t.Description = description.String
	t.Icon = icon.String
	t.Color = color.String
	return &t, nil
}

// ========================================
// Priorities
// ========================================

// PriorityResult represents a priority for API responses.
type PriorityResult struct {
	ID          int
	Name        string
	Description string
	Icon        string
	Color       string
	SortOrder   int
	IsDefault   bool
}

// ListPriorities retrieves all priorities ordered by sort order and name.
func (s *ConfigReadService) ListPriorities() ([]PriorityResult, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, icon, color, sort_order, is_default
		FROM priorities
		ORDER BY sort_order, name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list priorities: %w", err)
	}
	defer rows.Close()

	var priorities []PriorityResult
	for rows.Next() {
		var p PriorityResult
		var description, icon, color sql.NullString
		err := rows.Scan(&p.ID, &p.Name, &description, &icon, &color, &p.SortOrder, &p.IsDefault)
		if err != nil {
			continue
		}
		p.Description = description.String
		p.Icon = icon.String
		p.Color = color.String
		priorities = append(priorities, p)
	}

	if priorities == nil {
		priorities = []PriorityResult{}
	}

	return priorities, nil
}

// GetPriority retrieves a priority by ID.
func (s *ConfigReadService) GetPriority(id int) (*PriorityResult, error) {
	var p PriorityResult
	var description, icon, color sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, description, icon, color, sort_order, is_default
		FROM priorities WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &description, &icon, &color, &p.SortOrder, &p.IsDefault)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("priority not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get priority: %w", err)
	}

	p.Description = description.String
	p.Icon = icon.String
	p.Color = color.String
	return &p, nil
}

// ========================================
// Custom Fields
// ========================================

// CustomFieldResult represents a custom field definition for API responses.
type CustomFieldResult struct {
	ID           int
	Name         string
	FieldType    string
	Description  string
	Options      string
	Required     bool
	DisplayOrder int
}

// ListCustomFields retrieves all custom field definitions ordered by display order and name.
func (s *ConfigReadService) ListCustomFields() ([]CustomFieldResult, error) {
	rows, err := s.db.Query(`
		SELECT id, name, field_type, description, options, required, display_order
		FROM custom_field_definitions
		ORDER BY display_order, name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom fields: %w", err)
	}
	defer rows.Close()

	var fields []CustomFieldResult
	for rows.Next() {
		var f CustomFieldResult
		var description, options sql.NullString
		err := rows.Scan(&f.ID, &f.Name, &f.FieldType, &description, &options, &f.Required, &f.DisplayOrder)
		if err != nil {
			continue
		}
		f.Description = description.String
		f.Options = options.String
		fields = append(fields, f)
	}

	if fields == nil {
		fields = []CustomFieldResult{}
	}

	return fields, nil
}

// GetCustomField retrieves a custom field definition by ID.
func (s *ConfigReadService) GetCustomField(id int) (*CustomFieldResult, error) {
	var f CustomFieldResult
	var description, options sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, field_type, description, options, required, display_order
		FROM custom_field_definitions WHERE id = ?
	`, id).Scan(&f.ID, &f.Name, &f.FieldType, &description, &options, &f.Required, &f.DisplayOrder)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("custom field not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get custom field: %w", err)
	}

	f.Description = description.String
	f.Options = options.String
	return &f, nil
}
