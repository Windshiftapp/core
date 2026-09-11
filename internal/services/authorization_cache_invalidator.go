package services

import (
	"errors"
	"fmt"

	"windshift/internal/models"
)

// WorkspaceKeyInvalidator refreshes the process-local workspace key index.
type WorkspaceKeyInvalidator interface {
	Invalidate() error
}

// AuthorizationInvalidation describes the cache effects of a committed
// authorization-affecting mutation.
type AuthorizationInvalidation struct {
	UserIDs                 []int
	ResetPermissions        bool
	ActiveWorkspacesChanged bool
	WorkspaceKeysChanged    bool
}

// AuthorizationCacheInvalidator applies every process-local authorization
// cache effect through one post-commit boundary.
type AuthorizationCacheInvalidator struct {
	permissions   *PermissionService
	workspaceKeys WorkspaceKeyInvalidator
}

func NewAuthorizationCacheInvalidator(
	permissions *PermissionService,
	workspaceKeys WorkspaceKeyInvalidator,
) *AuthorizationCacheInvalidator {
	return &AuthorizationCacheInvalidator{
		permissions:   permissions,
		workspaceKeys: workspaceKeys,
	}
}

// Apply invalidates targeted snapshots after a commit. If targeted
// invalidation cannot be completed, it falls back to a full reset so a
// mutation cannot leave a revoked local snapshot usable.
func (i *AuthorizationCacheInvalidator) Apply(plan AuthorizationInvalidation) error {
	if i == nil {
		return nil
	}
	var errs []error
	if i.permissions != nil {
		if plan.ResetPermissions {
			if err := i.permissions.ResetPermissionCache(); err != nil {
				errs = append(errs, fmt.Errorf("reset permission cache: %w", err))
			}
		} else if len(plan.UserIDs) > 0 {
			if err := i.permissions.InvalidateMultipleUserCaches(uniqueAuthorizationUserIDs(plan.UserIDs)); err != nil {
				if resetErr := i.permissions.ResetPermissionCache(); resetErr != nil {
					errs = append(errs, fmt.Errorf("invalidate targeted permission caches: %v; reset permission cache: %w", err, resetErr))
				}
			}
		}
		if plan.ActiveWorkspacesChanged {
			i.permissions.InvalidateActiveWorkspaceCache()
		}
	}
	if plan.WorkspaceKeysChanged && i.workspaceKeys != nil {
		if err := i.workspaceKeys.Invalidate(); err != nil {
			errs = append(errs, fmt.Errorf("refresh workspace key cache: %w", err))
		}
	}
	return errors.Join(errs...)
}

// GroupPlan captures the users whose snapshots depend on a group before a
// membership rewrite, state change, or delete removes that information.
func (i *AuthorizationCacheInvalidator) GroupPlan(groupID int) (AuthorizationInvalidation, error) {
	if i == nil || i.permissions == nil {
		return AuthorizationInvalidation{}, nil
	}
	userIDs, err := i.permissions.getGroupMembers(groupID)
	if err != nil {
		return AuthorizationInvalidation{}, fmt.Errorf("capture group %d members: %w", groupID, err)
	}
	return AuthorizationInvalidation{UserIDs: userIDs}, nil
}

// GroupDeletePlan also detects deletion of the last explicit built-in role
// assignment, which changes implicit Everyone access for every user.
func (i *AuthorizationCacheInvalidator) GroupDeletePlan(groupID int) (AuthorizationInvalidation, error) {
	plan, err := i.GroupPlan(groupID)
	if err != nil || i == nil || i.permissions == nil {
		return plan, err
	}
	var changesEveryone bool
	err = i.permissions.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM group_workspace_roles target
			JOIN workspace_roles wr ON wr.id = target.role_id
			WHERE target.group_id = ?
			  AND wr.builtin_key IN (?, ?, ?)
			  AND NOT EXISTS (
				SELECT 1 FROM user_workspace_roles uwr
				WHERE uwr.workspace_id = target.workspace_id AND uwr.role_id = target.role_id
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM group_workspace_roles other
				WHERE other.workspace_id = target.workspace_id
				  AND other.role_id = target.role_id
				  AND other.group_id <> target.group_id
			  )
		)
	`, groupID, models.RoleBuiltinViewer, models.RoleBuiltinEditor, models.RoleBuiltinTester).Scan(&changesEveryone)
	if err != nil {
		return AuthorizationInvalidation{}, fmt.Errorf("capture group %d implicit access effect: %w", groupID, err)
	}
	plan.ResetPermissions = changesEveryone
	return plan, nil
}

func uniqueAuthorizationUserIDs(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
