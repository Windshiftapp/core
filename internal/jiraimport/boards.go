package jiraimport

import (
	"errors"
	"strings"

	"windshift/internal/itemorder"
	"windshift/internal/models"
	"windshift/internal/repository"
)

func (s *Service) EnsureCollection(
	jobID, jiraID, jiraKey, name, description, ql string,
	defaultSort itemorder.Sort,
	workspaceID, createdByUserID int,
	metadata map[string]any,
) (int, bool) {
	if id, ok := s.MappedEntity(jobID, "collection", jiraID); ok {
		return id, true
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Jira Collection " + jiraKey
	}
	collection, err := s.collections.FindByWorkspaceAndName(workspaceID, name)
	action := "reuse_existing"
	if errors.Is(err, repository.ErrNotFound) {
		var creator *int
		if createdByUserID > 0 {
			creator = &createdByUserID
		}
		collection = &models.Collection{
			Name: name, Description: description, QLQuery: ql, DefaultSort: defaultSort, WorkspaceID: &workspaceID,
		}
		err = s.collections.CreateForImport(collection, creator)
		action = "create"
	} else if err == nil && (strings.TrimSpace(collection.Description) != strings.TrimSpace(description) ||
		strings.TrimSpace(collection.QLQuery) != strings.TrimSpace(ql) ||
		!collectionSortEqual(collection.DefaultSort, defaultSort)) {
		collection.Description = description
		collection.QLQuery = ql
		collection.DefaultSort = defaultSort
		err = s.collections.Update(collection.ID, collection)
		action = "update_existing"
	}
	if err != nil {
		return 0, false
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["action"] = action
	metadata["workspace_id"] = workspaceID
	if s.RecordMapping(jobID, "collection", jiraID, jiraKey, collection.ID, metadata) != nil {
		return 0, false
	}
	return collection.ID, true
}

// collectionSortEqual reports whether two saved sort vectors are identical, so
// a re-import only rewrites the collection when the source order changed.
func collectionSortEqual(left, right itemorder.Sort) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *Service) StatusCategoryIDs(statusIDs []int) (map[int]int, error) {
	return s.statuses.CategoryIDs(statusIDs)
}

func (s *Service) EnsureBoardConfiguration(
	jobID, jiraID, jiraName string,
	collectionID int,
	request *models.BoardConfigurationRequest,
	metadata map[string]any,
) (int, bool) {
	if id, ok := s.MappedEntity(jobID, "board_configuration", jiraID); ok {
		return id, true
	}
	config, err := s.boards.GetByCollectionID(collectionID)
	action := "reuse_existing"
	configID := 0
	if errors.Is(err, repository.ErrNotFound) {
		configID, err = s.boards.Create(&collectionID, nil, request)
		action = "create"
	} else if err == nil {
		configID = config.ID
		err = s.boards.Update(configID, request)
		action = "update_existing"
	}
	if err != nil {
		return 0, false
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["action"] = action
	metadata["collection_id"] = collectionID
	metadata["column_count"] = len(request.Columns)
	if s.RecordMapping(jobID, "board_configuration", jiraID, jiraName, configID, metadata) != nil {
		return 0, false
	}
	return configID, true
}
