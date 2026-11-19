package jiraimport

import (
	"errors"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// MappingTargets lists the existing Windshift entities an operator can map
// Jira entities onto during an import. Lists are empty when no reusable
// entity exists yet; "create new" stays the default.
type MappingTargets struct {
	Workspaces   []WorkspaceTarget   `json:"workspaces"`
	ItemTypes    []ItemTypeTarget    `json:"item_types"`
	Statuses     []StatusTarget      `json:"statuses"`
	CustomFields []CustomFieldTarget `json:"custom_fields"`
}

type WorkspaceTarget struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type ItemTypeTarget struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

type StatusTarget struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	CategoryID   int    `json:"category_id"`
	CategoryName string `json:"category_name"`
	Color        string `json:"color"`
}

type CustomFieldTarget struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	FieldType string `json:"field_type"`
}

// ListMappingTargets returns the existing entities offered as mapping targets.
func (s *Service) ListMappingTargets() (MappingTargets, error) {
	targets := MappingTargets{
		Workspaces:   []WorkspaceTarget{},
		ItemTypes:    []ItemTypeTarget{},
		Statuses:     []StatusTarget{},
		CustomFields: []CustomFieldTarget{},
	}

	workspaces, err := s.workspaces.ListActiveBasics()
	if err != nil {
		return MappingTargets{}, fmt.Errorf("list workspace mapping targets: %w", err)
	}
	for _, workspace := range workspaces {
		targets.Workspaces = append(targets.Workspaces, WorkspaceTarget{
			ID: workspace.ID, Key: workspace.Key, Name: workspace.Name,
		})
	}

	itemTypes, err := s.itemTypes.List(nil)
	if err != nil {
		return MappingTargets{}, fmt.Errorf("list item type mapping targets: %w", err)
	}
	for _, itemType := range itemTypes {
		targets.ItemTypes = append(targets.ItemTypes, ItemTypeTarget{
			ID: itemType.ID, Name: itemType.Name, HierarchyLevel: itemType.HierarchyLevel,
		})
	}

	statuses, err := s.statuses.List()
	if err != nil {
		return MappingTargets{}, fmt.Errorf("list status mapping targets: %w", err)
	}
	for _, status := range statuses {
		targets.Statuses = append(targets.Statuses, StatusTarget{
			ID: status.ID, Name: status.Name,
			CategoryID: status.CategoryID, CategoryName: status.CategoryName,
			Color: status.CategoryColor,
		})
	}

	customFields, err := s.customFields.List()
	if err != nil {
		return MappingTargets{}, fmt.Errorf("list custom field mapping targets: %w", err)
	}
	for _, field := range customFields {
		targets.CustomFields = append(targets.CustomFields, CustomFieldTarget{
			ID: field.ID, Name: field.Name, FieldType: field.FieldType,
		})
	}

	return targets, nil
}

// StatusExists reports whether a status ID is a valid mapping target.
func (s *Service) StatusExists(id int) (bool, error) {
	status, err := s.statuses.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status != nil, nil
}

// ItemTypeExists reports whether an item type ID is a valid mapping target.
func (s *Service) ItemTypeExists(id int) (bool, error) {
	return s.itemTypes.Exists(id)
}

// CustomFieldExists reports whether a custom field ID is a valid mapping target.
func (s *Service) CustomFieldExists(id int) (bool, error) {
	field, err := s.customFields.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return field != nil, nil
}

// CustomFieldTypesCompatible reports whether a value extracted for sourceType
// can be stored in a target field of targetType. Text and textarea are
// interchangeable; everything else must match so the stored value keeps the
// shape the target field expects.
func CustomFieldTypesCompatible(targetType, sourceType string) bool {
	target := models.CanonicalCustomFieldType(targetType)
	source := models.CanonicalCustomFieldType(sourceType)
	// An unclassified type cannot be checked, so do not block it.
	if target == "" || source == "" {
		return true
	}
	if target == source {
		return true
	}
	textish := func(t string) bool { return t == "text" || t == "textarea" || t == "richtext" }
	return textish(target) && textish(source)
}

// CustomFieldTargetType returns the existing custom field's type, or "" when
// the field does not exist. Used to reject mappings whose value shape would
// not fit the target field.
func (s *Service) CustomFieldTargetType(id int) (string, error) {
	field, err := s.customFields.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if field == nil {
		return "", nil
	}
	return field.FieldType, nil
}

// WorkspaceImportTarget returns an active, non-personal, non-template
// workspace that a Jira project may import into. A zero value with a nil
// error means the workspace cannot be a reuse target.
func (s *Service) WorkspaceImportTarget(id int) (*WorkspaceTarget, error) {
	workspace, err := s.workspaces.FindByIDBasic(id)
	if err != nil {
		return nil, err
	}
	if workspace == nil || !workspace.Active || workspace.IsPersonal || workspace.IsTemplate {
		return nil, nil
	}
	return &WorkspaceTarget{ID: workspace.ID, Key: workspace.Key, Name: workspace.Name}, nil
}
