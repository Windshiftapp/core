package jiraimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"windshift/internal/assetevents"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

func (s *Service) EnsureAssetSet(name, description string, creatorUserID int) (setID int, created bool, err error) {
	setID, err = s.assets.FindSetIDByName(name)
	if err == nil {
		return setID, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, false, err
	}
	setID, err = s.assets.CreateImportedSet(name, description, creatorUserID)
	return setID, err == nil, err
}

func (s *Service) EnsureAssetDefaultStatus(setID int) (int, error) {
	return s.assets.EnsureImportedDefaultStatus(setID)
}

func (s *Service) EnsureAssetType(setID int, name, description string, displayOrder int) (typeID int, created bool, err error) {
	typeID, err = s.assets.FindAssetTypeIDByName(setID, name)
	if err == nil {
		return typeID, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, false, err
	}
	now := time.Now()
	typeID, err = s.assets.CreateAssetType(&models.AssetType{
		SetID:        setID,
		Name:         name,
		Description:  description,
		Icon:         "Box",
		Color:        "#3b82f6",
		DisplayOrder: displayOrder,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	return typeID, err == nil, err
}

func (s *Service) EnsureAssetAttributeField(
	name, fieldType, description, options string,
	required bool,
	displayOrder int,
) (fieldID int, created bool, err error) {
	if models.IsBooleanCustomFieldType(fieldType) {
		err = s.db.QueryRow(`
			SELECT id FROM custom_field_definitions
			WHERE LOWER(name) = LOWER(?) AND field_type IN (?, ?)
			ORDER BY CASE WHEN field_type = ? THEN 0 ELSE 1 END, id
			LIMIT 1
		`, name, models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox, models.CustomFieldTypeBoolean).Scan(&fieldID)
	} else {
		err = s.db.QueryRow(`
			SELECT id FROM custom_field_definitions
			WHERE LOWER(name) = LOWER(?) AND field_type = ?
		`, name, fieldType).Scan(&fieldID)
	}
	if err == nil {
		return fieldID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	now := time.Now()
	id, err := s.customFields.Create(&models.CustomFieldDefinition{
		Name:         name,
		FieldType:    fieldType,
		Description:  description,
		Required:     required,
		Options:      options,
		DisplayOrder: displayOrder,
	}, now)
	return int(id), err == nil, err
}

func (s *Service) EnsureAssetCategory(setID, parentID int, name, description string) (categoryID int, created bool, err error) {
	categories, err := s.assets.FindAssetCategoriesForSet(setID)
	if err != nil {
		return 0, false, err
	}
	for _, category := range categories {
		categoryParentID := 0
		if category.ParentID != nil {
			categoryParentID = *category.ParentID
		}
		if category.Name == name && categoryParentID == parentID {
			return category.ID, false, nil
		}
	}
	var parent *int
	if parentID > 0 {
		parent = &parentID
	}
	categoryID, _, err = s.assets.CreateAssetCategory(repository.CreateAssetCategoryInput{
		SetID: setID, Name: name, Description: description, ParentID: parent,
	})
	return categoryID, err == nil, err
}

func (s *Service) EnsureAssetStatus(setID int, name, description string, category int) (statusID int, created bool, err error) {
	statuses, err := s.assets.FindAssetStatusesForSet(setID)
	if err != nil {
		return 0, false, err
	}
	for _, status := range statuses {
		if strings.EqualFold(status.Name, name) {
			return status.ID, false, nil
		}
	}
	color := "#22c55e"
	switch category {
	case 0:
		color = "#ef4444"
	case 2:
		color = "#f59e0b"
	}
	now := time.Now()
	statusID, err = s.assets.CreateAssetStatusTransactional(&models.AssetStatus{
		SetID: setID, Name: name, Color: color, Description: description,
		DisplayOrder: category, CreatedAt: now, UpdatedAt: now,
	})
	return statusID, err == nil, err
}

func (s *Service) LinkAssetTypeField(typeID, fieldID int, required bool, displayOrder int) error {
	return s.assets.UpsertAssetTypeField(typeID, fieldID, required, displayOrder)
}

type AssetReference struct {
	ID       int
	SetID    int
	Title    string
	AssetTag string
}

func (s *Service) AssetReference(jobID, jiraID, jiraKey string) (AssetReference, bool) {
	var assetID int
	var ok bool
	if jiraID != "" {
		assetID, ok = s.MappedEntity(jobID, "asset", jiraID)
	}
	if !ok && jiraKey != "" {
		assetID, ok = s.MappedEntityByKey(jobID, "asset", jiraKey)
	}
	if !ok {
		return AssetReference{}, false
	}
	asset, err := s.assets.GetAssetByID(assetID)
	if err != nil {
		return AssetReference{}, false
	}
	return AssetReference{
		ID:       asset.ID,
		SetID:    asset.SetID,
		Title:    asset.Title,
		AssetTag: asset.AssetTag,
	}, true
}

func (s *Service) InsertAsset(input repository.JiraImportAssetRowInput) (int, error) {
	var assetID int
	err := database.WithTx(s.db, func(tx database.Tx) error {
		var err error
		assetID, err = s.assets.InsertJiraImportedAssetInTx(tx, input)
		if err != nil {
			return err
		}
		values := map[string]any{
			"title": input.Title, "description": input.Description, "asset_tag": input.AssetTag,
			"asset_type_id": input.AssetTypeID, "status_id": input.StatusID,
		}
		var customValues map[string]any
		if json.Unmarshal([]byte(input.CustomFieldValuesJSON), &customValues) == nil {
			for key, value := range customValues {
				values[key] = value
			}
		}
		metadata := assetevents.Import(input.ImportJobID)
		snapshot := assetevents.AssetSnapshot{
			ID: assetID, SetID: input.SetID, AssetTypeID: input.AssetTypeID,
			StatusID: input.StatusID, Title: input.Title, Description: input.Description, AssetTag: input.AssetTag,
		}
		_, err = assetevents.NewRecorder(s.db).Created(context.Background(), tx, snapshot, values, metadata)
		return err
	})
	return assetID, err
}

func (s *Service) AssetCustomFieldValues(assetID int) (map[string]any, error) {
	var rawJSON string
	if err := s.db.QueryRow(`
		SELECT COALESCE(custom_field_values, '{}') FROM assets WHERE id = ?
	`, assetID).Scan(&rawJSON); err != nil {
		return nil, err
	}
	values := make(map[string]any)
	_ = json.Unmarshal([]byte(rawJSON), &values)
	return values, nil
}

func (s *Service) UpdateAssetCustomFieldValues(assetID int, values map[string]any) error {
	encoded, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = s.db.ExecWrite(`
		UPDATE assets SET custom_field_values = ?, updated_at = updated_at WHERE id = ?
	`, string(encoded), assetID)
	return err
}
