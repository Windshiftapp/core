package services

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// NotificationAuthorizer resolves the current permissions behind persisted
// notification provenance. Delivery surfaces use a snapshot so workspace
// permissions are decoded once per recipient while asset ownership is resolved
// from the current asset row.
type NotificationAuthorizer struct {
	db                   database.Database
	workspacePermissions *PermissionService
	assetPermissions     AssetSetPermissionChecker
	pagePermissions      *PagePermissionService
}

func NewNotificationAuthorizer(db database.Database, workspacePermissions *PermissionService, assetPermissions AssetSetPermissionChecker) *NotificationAuthorizer {
	authorizer := &NotificationAuthorizer{
		db:                   db,
		workspacePermissions: workspacePermissions,
		assetPermissions:     assetPermissions,
	}
	if db != nil && workspacePermissions != nil {
		authorizer.pagePermissions = NewPagePermissionService(db, workspacePermissions)
	}
	return authorizer
}

// NotificationAuthorizationSnapshot contains the current authorization state
// for one recipient. It is safe to reuse across a single tray or email batch.
type NotificationAuthorizationSnapshot struct {
	authorizer   *NotificationAuthorizer
	userID       int
	active       bool
	workspaceIDs map[int]struct{}
}

func (a *NotificationAuthorizer) Snapshot(userID int) (*NotificationAuthorizationSnapshot, error) {
	snapshot := &NotificationAuthorizationSnapshot{
		authorizer:   a,
		userID:       userID,
		workspaceIDs: make(map[int]struct{}),
	}
	if a == nil || a.db == nil {
		return snapshot, nil
	}
	err := a.db.QueryRow("SELECT is_active FROM users WHERE id = ?", userID).Scan(&snapshot.active)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve notification recipient state: %w", err)
	}
	if !snapshot.active || a.workspacePermissions == nil {
		return snapshot, nil
	}
	workspaceIDs, err := a.workspacePermissions.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, fmt.Errorf("resolve notification workspace scope: %w", err)
	}
	for _, workspaceID := range workspaceIDs {
		snapshot.workspaceIDs[workspaceID] = struct{}{}
	}
	return snapshot, nil
}

// Visible reports whether the notification may be disclosed to the snapshot's
// recipient. Unknown or incomplete provenance is denied.
func (s *NotificationAuthorizationSnapshot) Visible(notification models.Notification) (bool, error) {
	if s == nil || s.authorizer == nil || !s.active || notification.UserID != s.userID {
		return false, nil
	}
	var baseVisible bool
	switch notification.AuthorizationScope {
	case models.NotificationScopeSystem:
		baseVisible = true
	case models.NotificationScopeWorkspace:
		if notification.WorkspaceID == nil {
			return false, nil
		}
		_, baseVisible = s.workspaceIDs[*notification.WorkspaceID]
	case models.NotificationScopeAsset:
		var err error
		baseVisible, err = s.assetVisible(notification)
		if err != nil {
			return false, err
		}
	default:
		return false, nil
	}
	if !baseVisible {
		return false, nil
	}
	requiresReference := notification.SourceType == models.EventItemLinked || notification.SourceType == models.EventItemUnlinked
	hasReference := notification.ReferencedEntityType != "" || notification.ReferencedEntityID != nil ||
		notification.ReferencedWorkspaceID != nil || notification.ReferencedWorkspacePermission != ""
	if !requiresReference && !hasReference {
		return true, nil
	}
	if notification.ReferencedEntityType == "" || notification.ReferencedEntityID == nil || notification.ReferencedWorkspacePermission == "" {
		return false, nil
	}
	return s.referencedEntityVisible(notification)
}

func (s *NotificationAuthorizationSnapshot) assetVisible(notification models.Notification) (bool, error) {
	a := s.authorizer
	if a.db == nil || a.assetPermissions == nil || notification.SourceType != "asset" || notification.SourceID == nil || *notification.SourceID <= 0 {
		return false, nil
	}
	var setID int
	err := a.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", *notification.SourceID).Scan(&setID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve asset notification %d provenance: %w", notification.ID, err)
	}
	allowed, err := a.assetPermissions.HasAssetSetPermission(s.userID, setID, AssetPermissionKeyView)
	if err != nil {
		return false, fmt.Errorf("authorize asset notification %d: %w", notification.ID, err)
	}
	return allowed, nil
}

func (s *NotificationAuthorizationSnapshot) referencedEntityVisible(notification models.Notification) (bool, error) {
	a := s.authorizer
	if a.db == nil || notification.ReferencedEntityID == nil || *notification.ReferencedEntityID <= 0 {
		return false, nil
	}
	entityID := *notification.ReferencedEntityID
	switch notification.ReferencedEntityType {
	case "item":
		return s.workspaceEntityVisible("items", entityID, notification, models.PermissionItemView)
	case "test_case":
		return s.workspaceEntityVisible("test_cases", entityID, notification, models.PermissionTestView)
	case "page":
		if notification.ReferencedWorkspaceID == nil || notification.ReferencedWorkspacePermission != models.PermissionPageView || a.pagePermissions == nil {
			return false, nil
		}
		allowed, err := a.pagePermissions.Can(s.userID, *notification.ReferencedWorkspaceID, entityID, PageOpView)
		if err != nil {
			return false, fmt.Errorf("authorize referenced page notification %d: %w", notification.ID, err)
		}
		return allowed, nil
	case "asset":
		if notification.ReferencedWorkspaceID != nil || notification.ReferencedWorkspacePermission != AssetPermissionKeyView || a.assetPermissions == nil {
			return false, nil
		}
		var setID int
		err := a.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", entityID).Scan(&setID)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("resolve referenced asset notification %d: %w", notification.ID, err)
		}
		allowed, err := a.assetPermissions.HasAssetSetPermission(s.userID, setID, AssetPermissionKeyView)
		if err != nil {
			return false, fmt.Errorf("authorize referenced asset notification %d: %w", notification.ID, err)
		}
		return allowed, nil
	default:
		return false, nil
	}
}

func (s *NotificationAuthorizationSnapshot) workspaceEntityVisible(table string, entityID int, notification models.Notification, expectedPermission string) (bool, error) {
	a := s.authorizer
	if notification.ReferencedWorkspaceID == nil || notification.ReferencedWorkspacePermission != expectedPermission || a.workspacePermissions == nil {
		return false, nil
	}
	var currentWorkspaceID int
	err := a.db.QueryRow("SELECT workspace_id FROM "+table+" WHERE id = ?", entityID).Scan(&currentWorkspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve referenced %s notification %d: %w", table, notification.ID, err)
	}
	if currentWorkspaceID != *notification.ReferencedWorkspaceID {
		return false, nil
	}
	allowed, err := a.workspacePermissions.HasWorkspacePermission(s.userID, currentWorkspaceID, expectedPermission)
	if err != nil {
		return false, fmt.Errorf("authorize referenced %s notification %d: %w", table, notification.ID, err)
	}
	return allowed, nil
}
