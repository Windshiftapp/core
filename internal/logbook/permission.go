package logbook

import (
	"windshift/internal/models"
)

// PermissionService handles bucket permission checks for the logbook system.
// It is fully standalone — admin status and group memberships are provided
// by the caller (from the LogbookUser context populated by header auth).
type PermissionService struct {
	repo *Repository
}

// NewPermissionService creates a new logbook permission service.
func NewPermissionService(repo *Repository) *PermissionService {
	return &PermissionService{
		repo: repo,
	}
}

// HasBucketPermission checks if a user has a specific permission on a bucket.
// System admins have full access to all buckets.
func (s *PermissionService) HasBucketPermission(userID int, isAdmin bool, groupIDs []int, bucketID, permission string) (bool, error) {
	if isAdmin {
		return true, nil
	}

	// Check bucket-level permissions (direct user + group)
	has, err := s.repo.HasBucketPermission(userID, groupIDs, bucketID, permission)
	if err != nil {
		return false, err
	}
	if has {
		return true, nil
	}

	// bucket.admin implies bucket.edit implies bucket.view
	switch permission {
	case models.LogbookPermissionBucketView:
		has, err = s.repo.HasBucketPermission(userID, groupIDs, bucketID, models.LogbookPermissionBucketEdit)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
		return s.repo.HasBucketPermission(userID, groupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	case models.LogbookPermissionBucketEdit:
		return s.repo.HasBucketPermission(userID, groupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	}

	return false, nil
}

// GetAccessibleBucketIDs returns IDs of all buckets the user can access with at least view permission.
// System admins get all buckets.
func (s *PermissionService) GetAccessibleBucketIDs(userID int, isAdmin bool, groupIDs []int) ([]string, error) {
	if isAdmin {
		return s.repo.GetAllBucketIDs()
	}

	// Get buckets with any permission level (view, edit, or admin)
	viewIDs, err := s.repo.GetAccessibleBucketIDs(userID, groupIDs, models.LogbookPermissionBucketView)
	if err != nil {
		return nil, err
	}
	editIDs, err := s.repo.GetAccessibleBucketIDs(userID, groupIDs, models.LogbookPermissionBucketEdit)
	if err != nil {
		return nil, err
	}
	adminIDs, err := s.repo.GetAccessibleBucketIDs(userID, groupIDs, models.LogbookPermissionBucketAdmin)
	if err != nil {
		return nil, err
	}

	// Deduplicate
	seen := make(map[string]bool)
	var result []string
	for _, ids := range [][]string{viewIDs, editIDs, adminIDs} {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	return result, nil
}
