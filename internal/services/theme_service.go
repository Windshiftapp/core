package services

import (
	"errors"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// ErrDefaultThemeDelete is returned when callers try to delete the built-in default theme.
var ErrDefaultThemeDelete = errors.New("cannot delete default theme")

// ThemeService owns application-theme use cases.
type ThemeService struct {
	repo *repository.ThemeRepository
}

// NewThemeService creates a ThemeService.
func NewThemeService(repo *repository.ThemeRepository) *ThemeService {
	return &ThemeService{repo: repo}
}

// List returns all themes.
func (s *ThemeService) List() ([]models.Theme, error) {
	return s.repo.List()
}

// GetActive returns the active theme.
func (s *ThemeService) GetActive() (models.Theme, error) {
	return s.repo.GetActive()
}

// Create creates a theme and returns the fully loaded row.
func (s *ThemeService) Create(req models.ThemeCreateRequest) (models.Theme, error) {
	id, err := s.repo.Create(req, time.Now())
	if err != nil {
		return models.Theme{}, err
	}
	return s.repo.GetByID(id)
}

// Update updates a theme and returns the fully loaded row.
func (s *ThemeService) Update(id int, req models.ThemeUpdateRequest) (models.Theme, error) {
	if req.IsActive {
		if err := s.repo.DeactivateAllExcept(id); err != nil {
			return models.Theme{}, err
		}
	}
	if err := s.repo.Update(id, req, time.Now()); err != nil {
		return models.Theme{}, err
	}
	return s.repo.GetByID(id)
}

// Delete removes a non-default theme.
func (s *ThemeService) Delete(id int) error {
	theme, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if theme.IsDefault {
		return ErrDefaultThemeDelete
	}
	return s.repo.Delete(id)
}

// Activate marks a theme active and all others inactive.
func (s *ThemeService) Activate(id int) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	if err := s.repo.DeactivateAll(); err != nil {
		return err
	}
	return s.repo.Activate(id, time.Now())
}
