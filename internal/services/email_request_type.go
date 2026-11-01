package services

import (
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// Email request type naming. The primary name is used unless an admin already
// claimed it in the portal, in which case the fallback keeps provisioning
// working without renaming their row.
const (
	emailRequestTypeName         = "Email"
	emailRequestTypeFallbackName = "Email intake"
)

// EnsureEmailRequestType returns the id of the system Email request type that
// routes email intake into a portal, provisioning it on first use.
//
// The row is hidden from the public portal form (kind = 'email') and carries no
// fields, so required form fields can never block an email. Its item type stays
// editable by admins. The workspace is pinned to a workspace the portal serves:
// preferredWorkspaceID (0 for none) when it is served, otherwise the portal's
// only served workspace. preferredItemTypeID seeds the item type when it is
// allowed in that workspace, otherwise the workspace default is used.
func EnsureEmailRequestType(db database.Database, portalChannelID, preferredWorkspaceID int, preferredItemTypeID *int) (int, error) {
	repo := repository.NewRequestTypeRepository(db)
	existing, err := repo.GetEmailIntakeForChannel(portalChannelID)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, err
	}

	workspaceID, err := resolveEmailIntakeWorkspace(db, portalChannelID, preferredWorkspaceID)
	if err != nil {
		return 0, err
	}
	itemTypeID, err := resolveItemTypeForCreation(db, workspaceID, allowedEmailIntakeItemType(db, workspaceID, preferredItemTypeID))
	if err != nil {
		return 0, fmt.Errorf("resolve email intake item type: %w", err)
	}

	name := emailRequestTypeName
	exists, err := repo.NameExistsInChannel(portalChannelID, name, 0)
	if err != nil {
		return 0, err
	}
	if exists {
		name = emailRequestTypeFallbackName
	}

	id, err := repo.Create(&models.RequestType{
		ChannelID:   portalChannelID,
		Name:        name,
		Description: "System request type for tickets created from email intake.",
		ItemTypeID:  *itemTypeID,
		Icon:        "Mail",
		Color:       "#6b7280",
		IsActive:    true,
		WorkspaceID: &workspaceID,
		Kind:        models.RequestTypeKindEmail,
	})
	if err != nil {
		return 0, fmt.Errorf("create email request type: %w", err)
	}
	return int(id), nil
}

// resolveEmailIntakeWorkspace picks the workspace the Email request type pins.
// A preferred workspace is honored only when the portal actually serves it; a
// portal with several workspaces and no usable preference is ambiguous and
// rejected rather than resolved by configuration order.
func resolveEmailIntakeWorkspace(db database.Database, portalChannelID, preferred int) (int, error) {
	var configJSON string
	if err := db.QueryRow(
		`SELECT COALESCE(config, '{}') FROM channels WHERE id = ? AND type = 'portal'`,
		portalChannelID,
	).Scan(&configJSON); err != nil {
		return 0, fmt.Errorf("load portal config: %w", err)
	}
	var cfg models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return 0, fmt.Errorf("parse portal config: %w", err)
	}
	if preferred > 0 && containsInt(cfg.PortalWorkspaceIDs, preferred) {
		return preferred, nil
	}
	switch len(cfg.PortalWorkspaceIDs) {
	case 0:
		return 0, fmt.Errorf("portal %d serves no workspaces", portalChannelID)
	case 1:
		return cfg.PortalWorkspaceIDs[0], nil
	default:
		return 0, fmt.Errorf("portal %d serves multiple workspaces; an email intake workspace must be specified", portalChannelID)
	}
}

// allowedEmailIntakeItemType returns preferred when it is an allowed item type
// for the workspace, else nil so the workspace default is resolved instead.
func allowedEmailIntakeItemType(db database.Database, workspaceID int, preferred *int) *int {
	if preferred == nil || *preferred <= 0 {
		return nil
	}
	allowed, err := IsItemTypeAllowedInWorkspace(db, workspaceID, *preferred)
	if err != nil || !allowed {
		return nil
	}
	return preferred
}
