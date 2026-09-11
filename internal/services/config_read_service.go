package services

import (
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

const customFieldIndexLimitSetting = "max_custom_field_indexes_per_table"

// ConfigReadService projects canonical configuration repositories for compact catalog consumers.
type ConfigReadService struct {
	itemTypes    *repository.ItemTypeRepository
	priorities   *repository.PriorityRepository
	customFields *repository.CustomFieldRepository
	settings     *repository.SystemSettingRepository
}

// NewConfigReadService creates a configuration catalog reader.
func NewConfigReadService(db database.Database) *ConfigReadService {
	return &ConfigReadService{
		itemTypes:    repository.NewItemTypeRepository(db),
		priorities:   repository.NewPriorityRepository(db),
		customFields: repository.NewCustomFieldRepository(db),
		settings:     repository.NewSystemSettingRepository(db),
	}
}

// ItemTypeResult is the compact item-type catalog projection.
type ItemTypeResult struct {
	ID             int    `json:"id"`
	BuiltinKey     string `json:"builtin_key,omitempty"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	HierarchyLevel int    `json:"hierarchy_level"`
	SortOrder      int    `json:"sort_order"`
	IsDefault      bool   `json:"is_default"`
}

// ListItemTypes returns all item types in catalog order.
func (s *ConfigReadService) ListItemTypes() ([]ItemTypeResult, error) {
	rows, err := s.itemTypes.List(nil)
	if err != nil {
		return nil, err
	}
	result := make([]ItemTypeResult, len(rows))
	for i, row := range rows {
		result[i] = ItemTypeResult{
			ID: row.ID, BuiltinKey: row.BuiltinKey, Name: row.Name, Description: row.Description,
			Icon: row.Icon, Color: row.Color, HierarchyLevel: row.HierarchyLevel,
			SortOrder: row.SortOrder, IsDefault: row.IsDefault,
		}
	}
	return result, nil
}

// GetItemType returns one compact item-type projection.
func (s *ConfigReadService) GetItemType(id int) (*ItemTypeResult, error) {
	row, err := s.itemTypes.GetByID(id)
	if err != nil {
		return nil, err
	}
	return &ItemTypeResult{
		ID: row.ID, BuiltinKey: row.BuiltinKey, Name: row.Name, Description: row.Description,
		Icon: row.Icon, Color: row.Color, HierarchyLevel: row.HierarchyLevel,
		SortOrder: row.SortOrder, IsDefault: row.IsDefault,
	}, nil
}

// PriorityResult is the compact priority catalog projection.
type PriorityResult struct {
	ID          int
	BuiltinKey  string
	Name        string
	Description string
	Icon        string
	Color       string
	SortOrder   int
	IsDefault   bool
}

// ListPriorities returns all priorities in catalog order.
func (s *ConfigReadService) ListPriorities() ([]PriorityResult, error) {
	rows, err := s.priorities.List(nil)
	if err != nil {
		return nil, err
	}
	result := make([]PriorityResult, len(rows))
	for i, row := range rows {
		result[i] = PriorityResult{
			ID: row.ID, BuiltinKey: row.BuiltinKey, Name: row.Name, Description: row.Description,
			Icon: row.Icon, Color: row.Color, SortOrder: row.SortOrder, IsDefault: row.IsDefault,
		}
	}
	return result, nil
}

// GetPriority returns one compact priority projection.
func (s *ConfigReadService) GetPriority(id int) (*PriorityResult, error) {
	row, err := s.priorities.GetByID(id)
	if err != nil {
		return nil, err
	}
	return &PriorityResult{
		ID: row.ID, BuiltinKey: row.BuiltinKey, Name: row.Name, Description: row.Description,
		Icon: row.Icon, Color: row.Color, SortOrder: row.SortOrder, IsDefault: row.IsDefault,
	}, nil
}

// CustomFieldResult is the compact custom-field catalog projection.
type CustomFieldResult struct {
	ID                             int
	Name                           string
	FieldType                      string
	Description                    string
	Options                        string
	Required                       bool
	DisplayOrder                   int
	SystemDefault                  bool
	AppliesToPortalCustomers       bool
	AppliesToCustomerOrganisations bool
	AssetTypeUsages                []CustomFieldAssetUsage
	Indexed                        models.CustomFieldIndexInfo
}

type CustomFieldAssetUsage struct {
	SetID         int    `json:"-"`
	AssetTypeName string `json:"asset_type_name"`
	SetName       string `json:"set_name"`
}

type CustomFieldIndexCount struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

type CustomFieldCatalogMeta struct {
	IndexCounts map[string]CustomFieldIndexCount `json:"index_counts"`
}

// ListCustomFields returns all custom-field definitions in catalog order.
func (s *ConfigReadService) ListCustomFields() ([]CustomFieldResult, error) {
	rows, err := s.customFields.List()
	if err != nil {
		return nil, err
	}
	result := make([]CustomFieldResult, len(rows))
	for i, row := range rows {
		result[i] = CustomFieldResult{
			ID: row.ID, Name: row.Name, FieldType: row.FieldType, Description: row.Description,
			Options: row.Options, Required: row.Required, DisplayOrder: row.DisplayOrder,
			SystemDefault: row.SystemDefault, AppliesToPortalCustomers: row.AppliesToPortalCustomers,
			AppliesToCustomerOrganisations: row.AppliesToCustomerOrganisations,
		}
	}
	return result, nil
}

func (s *ConfigReadService) ListCustomFieldsWithMeta() ([]CustomFieldResult, CustomFieldCatalogMeta, error) {
	items, err := s.ListCustomFields()
	if err != nil {
		return nil, CustomFieldCatalogMeta{}, err
	}
	byID := make(map[int]int, len(items))
	for i := range items {
		byID[items[i].ID] = i
		items[i].AssetTypeUsages = []CustomFieldAssetUsage{}
	}
	usages, err := s.customFields.ListAssetTypeUsages()
	if err != nil {
		return nil, CustomFieldCatalogMeta{}, err
	}
	for _, usage := range usages {
		if index, ok := byID[usage.CustomFieldID]; ok {
			items[index].AssetTypeUsages = append(items[index].AssetTypeUsages, CustomFieldAssetUsage{
				SetID: usage.SetID, AssetTypeName: usage.AssetTypeName, SetName: usage.SetName,
			})
		}
	}

	counts := map[string]int{"items": 0, "assets": 0}
	indexes, err := s.customFields.ListIndexes()
	if err != nil {
		return nil, CustomFieldCatalogMeta{}, err
	}
	for _, index := range indexes {
		itemIndex, ok := byID[index.CustomFieldID]
		if !ok {
			continue
		}
		switch index.TargetTable {
		case "items":
			items[itemIndex].Indexed.Items = true
			counts["items"]++
		case "assets":
			items[itemIndex].Indexed.Assets = true
			counts["assets"]++
		}
	}

	limit := 20
	if value, ok, err := s.settings.GetValue(customFieldIndexLimitSetting); err == nil && ok {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil {
			limit = parsed
		}
	}
	return items, CustomFieldCatalogMeta{IndexCounts: map[string]CustomFieldIndexCount{
		"items": {Current: counts["items"], Max: limit}, "assets": {Current: counts["assets"], Max: limit},
	}}, nil
}

// GetCustomField returns one compact custom-field projection.
func (s *ConfigReadService) GetCustomField(id int) (*CustomFieldResult, error) {
	row, err := s.customFields.FindByID(id)
	if err != nil {
		return nil, err
	}
	return &CustomFieldResult{
		ID: row.ID, Name: row.Name, FieldType: row.FieldType, Description: row.Description,
		Options: row.Options, Required: row.Required, DisplayOrder: row.DisplayOrder,
		SystemDefault: row.SystemDefault, AppliesToPortalCustomers: row.AppliesToPortalCustomers,
		AppliesToCustomerOrganisations: row.AppliesToCustomerOrganisations,
	}, nil
}
