package jiraimport

import (
	"errors"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// EnsureImportedPriority returns the catalog priority named `name`, creating it
// when absent. Imported priorities are appended after the existing catalog. A
// workspace configuration set that curates priorities explicitly gets the
// priority linked so imported items may use it; a set with no priority list
// already admits every catalog priority, so linking one there would narrow it.
func (s *Service) EnsureImportedPriority(workspaceID int, name, icon, color string) (*models.Priority, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, errors.New("imported priority requires a name")
	}

	priority, err := s.priorities.FindByName(name)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}
	created := false
	if priority == nil {
		sortOrder, err := s.priorities.NextSortOrder()
		if err != nil {
			return nil, false, err
		}
		priority = &models.Priority{Name: name, Icon: icon, Color: color, SortOrder: sortOrder}
		id, err := s.priorities.Create(priority)
		switch {
		case errors.Is(err, repository.ErrDuplicateEntry):
			// A concurrent import created the same priority first; reuse it.
			priority, err = s.priorities.FindByName(name)
			if err != nil {
				return nil, false, err
			}
		case err != nil:
			return nil, false, err
		default:
			priority.ID = id
			created = true
		}
	}

	configSetID, err := s.WorkspaceConfigurationSetID(workspaceID)
	if err != nil {
		return nil, false, err
	}
	if configSetID != nil {
		curated, err := s.priorities.ConfigurationSetHasPriorities(*configSetID)
		if err != nil {
			return nil, false, err
		}
		if curated {
			if err := s.priorities.EnsureConfigurationSetLink(priority.ID, *configSetID); err != nil {
				return nil, false, err
			}
		}
	}
	return priority, created, nil
}

// Styling applied to priorities created on demand from a Jira priority name.
const (
	ImportedPriorityIcon  = "Flag"
	ImportedPriorityColor = "#6b7280"
)
