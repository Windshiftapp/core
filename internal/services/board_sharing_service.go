package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type PublicBoardAvailability interface{ PublicBoardsEnabled() bool }

func (s *CollectionApplicationService) WithSharingNotifications(notifications interface {
	CreateNotification(models.Notification) (models.Notification, error)
}) *CollectionApplicationService {
	s.sharingNotifications = notifications
	return s
}

func (s *CollectionApplicationService) OwnsBoardCollections(ctx context.Context, userID int) (bool, error) {
	var owns bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM collections WHERE created_by = ?)`, userID).Scan(&owns)
	return owns, err
}

type BoardSharing struct {
	CollectionID       int                         `json:"collection_id"`
	Name               string                      `json:"name"`
	OwnerID            *int                        `json:"owner_id"`
	OwnerName          string                      `json:"owner_name"`
	OwnerActive        *bool                       `json:"owner_active"`
	WorkspaceID        *int                        `json:"workspace_id"`
	WorkspaceName      string                      `json:"workspace_name"`
	Scope              string                      `json:"scope"`
	Enabled            bool                        `json:"enabled"`
	StandaloneEnabled  bool                        `json:"standalone_enabled"`
	Slug               *string                     `json:"slug"`
	MilestoneSwimlanes bool                        `json:"milestone_swimlanes"`
	Policy             models.CustomerAccessPolicy `json:"policy"`
	Revision           int                         `json:"revision"`
	RevokedAt          *string                     `json:"revoked_at"`
	RevokeReason       string                      `json:"revoke_reason"`
	ChangedAt          *string                     `json:"changed_at"`
	ChangedBy          *int                        `json:"changed_by"`
	ChangedByName      string                      `json:"changed_by_name"`
	Organizations      []BoardAudienceOrganisation `json:"organisations"`
	Locations          []BoardPublicationLocation  `json:"locations"`
	InstanceEnabled    bool                        `json:"instance_enabled"`
	CanAdminister      bool                        `json:"can_administer"`
}

type BoardSharingSave struct {
	Revision           int                         `json:"revision"`
	Enabled            bool                        `json:"enabled"`
	StandaloneEnabled  bool                        `json:"standalone_enabled"`
	Slug               *string                     `json:"slug"`
	MilestoneSwimlanes bool                        `json:"milestone_swimlanes"`
	Policy             models.CustomerAccessPolicy `json:"policy"`
}

// Audit canonical publication state, not derived location summaries or viewer capabilities.
func boardSharingAuditState(value *BoardSharing) map[string]any {
	return map[string]any{"collection_id": value.CollectionID, "scope": value.Scope, "workspace_id": value.WorkspaceID,
		"enabled": value.Enabled, "standalone_enabled": value.StandaloneEnabled, "slug": value.Slug, "milestone_swimlanes": value.MilestoneSwimlanes,
		"policy": value.Policy, "revision": value.Revision, "revoked_at": value.RevokedAt, "revoke_reason": value.RevokeReason,
		"changed_at": value.ChangedAt, "changed_by": value.ChangedBy}
}

type BoardSharingEvent struct {
	ID        int             `json:"id"`
	ActorID   *int            `json:"actor_id"`
	ActorName string          `json:"actor_name"`
	Action    string          `json:"action"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	Reason    string          `json:"reason"`
	CreatedAt string          `json:"created_at"`
}

type BoardPublicationFilter struct {
	SystemAdminOnly bool
	Query           string
	OwnerID         *int
	WorkspaceID     *int
	Global          bool
	State           string
	AccessMode      string
	OrganisationID  *int
	Limit, Offset   int
}

func (s *CollectionApplicationService) boardsEnabled() (bool, error) {
	if s.publicBoards != nil {
		return s.publicBoards.PublicBoardsEnabled(), nil
	}
	value, exists, err := repository.NewSystemSettingRepository(s.db).GetValue("public_boards_enabled")
	return !exists || strings.EqualFold(value, "true"), err
}

func (s *CollectionApplicationService) sharingCollection(userID, id int) (*models.Collection, bool, error) {
	collection, err := s.repository.GetModel(id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, false, ErrCollectionNotFound
	}
	if err != nil {
		return nil, false, err
	}
	admin, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return nil, false, err
	}
	if !admin && (collection.CreatedBy == nil || *collection.CreatedBy != userID) {
		return nil, false, ErrCollectionForbidden
	}
	return collection, admin, nil
}

func (s *CollectionApplicationService) GetBoardSharing(userID, id int) (*BoardSharing, error) {
	_, admin, err := s.sharingCollection(userID, id)
	if err != nil {
		return nil, err
	}
	enabled, err := s.boardsEnabled()
	if err != nil {
		return nil, err
	}
	rows, err := s.boardSharingRows([]int{id}, admin, enabled)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrCollectionNotFound
	}
	return &rows[0], nil
}

func (s *CollectionApplicationService) SaveBoardSharing(actor AuditActor, id int, input BoardSharingSave) (*BoardSharing, error) {
	return s.saveBoardSharing(actor, id, input, logger.ActionCollectionSharingUpdate, "")
}

func (s *CollectionApplicationService) RevokeBoardSharing(actor AuditActor, id, revision int, reason string, unlock bool) (*BoardSharing, error) {
	admin, err := s.permissions.IsSystemAdmin(actor.UserID)
	if err != nil {
		return nil, err
	}
	if !admin {
		return nil, ErrCollectionForbidden
	}
	reason = strings.TrimSpace(reason)
	if !unlock && (reason == "" || len(reason) > 2000) {
		return nil, collectionValidation("a revoke reason of 1-2000 characters is required")
	}
	current, err := s.GetBoardSharing(actor.UserID, id)
	if err != nil {
		return nil, err
	}
	if !unlock && current.RevokedAt != nil && current.RevokeReason == reason {
		return current, nil
	}
	input := BoardSharingSave{Revision: revision, Enabled: false, StandaloneEnabled: current.StandaloneEnabled, Slug: current.Slug, MilestoneSwimlanes: current.MilestoneSwimlanes, Policy: current.Policy}
	action := logger.ActionCollectionPublicRevoke
	if unlock {
		action = logger.ActionCollectionPublicAllowRepublish
	}
	return s.saveBoardSharing(actor, id, input, action, reason)
}

func (s *CollectionApplicationService) saveBoardSharing(actor AuditActor, id int, input BoardSharingSave, action, reason string) (*BoardSharing, error) {
	collection, _, err := s.sharingCollection(actor.UserID, id)
	if err != nil {
		return nil, err
	}
	if input.Revision != collection.SharingRevision {
		return nil, ErrCollectionConflict
	}
	if collection.IsPrivate {
		if input.Enabled {
			return nil, collectionValidation("make the collection shared before publishing its board")
		}
		input.Slug = nil
	}
	if input.Enabled {
		enabled, err := s.boardsEnabled()
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, collectionValidation("public boards are disabled on this instance")
		}
		if collection.PublicRevokedAt != nil {
			return nil, collectionValidation("an administrator must allow re-publishing this board")
		}
		if err := s.validatePublicPermission(actor.UserID, true); err != nil {
			return nil, err
		}
		if err := s.authorizePublicScope(actor.UserID, collection.QLQuery); err != nil {
			return nil, err
		}
	}
	if input.Slug != nil {
		value := strings.TrimSpace(*input.Slug)
		input.Slug = &value
	}
	if err := s.validateSlug(input.Slug, input.Enabled && input.StandaloneEnabled); err != nil {
		return nil, err
	}
	before, err := s.GetBoardSharing(actor.UserID, id)
	if err != nil {
		return nil, err
	}
	if before.Revision != input.Revision {
		return nil, ErrCollectionConflict
	}
	policy := input.Policy
	// Removing exposure must still work when a retained audience names a deleted organisation.
	// New or changed policies, and every enable, continue to validate against current organisations.
	if input.Enabled || policy.Mode != before.Policy.Mode || !slices.Equal(policy.OrganisationIDs, before.Policy.OrganisationIDs) {
		policy, err = repository.NewCustomerAccessRepository(s.db).ValidatePolicy(context.Background(), input.Policy)
		if err != nil {
			if errors.Is(err, repository.ErrCustomerAccessPolicyInvalid) {
				return nil, collectionValidation(err.Error())
			}
			return nil, err
		}
	}
	input.Policy = policy
	if s.beforePublicSharingWrite != nil {
		s.beforePublicSharingWrite()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	revokedAt, revokedBy, revokeReason := collection.PublicRevokedAt, collection.PublicRevokedBy, collection.PublicRevokeReason
	if action == logger.ActionCollectionPublicRevoke {
		revokedAt, revokedBy, revokeReason = &now, &actor.UserID, reason
	}
	if action == logger.ActionCollectionPublicAllowRepublish {
		revokedAt, revokedBy, revokeReason = nil, nil, ""
	}
	after := *before
	after.Enabled, after.StandaloneEnabled, after.Slug = input.Enabled, input.StandaloneEnabled, input.Slug
	after.MilestoneSwimlanes, after.Policy, after.Revision = input.MilestoneSwimlanes, input.Policy, input.Revision+1
	after.RevokedAt, after.RevokeReason, after.ChangedAt, after.ChangedBy = revokedAt, revokeReason, &now, &actor.UserID
	policyJSON, err := json.Marshal(policy.OrganisationIDs)
	if err != nil {
		return nil, err
	}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		result, err := tx.Exec(`UPDATE collections SET is_public = ?, board_enabled = ?, board_standalone_enabled = ?,
		 board_ever_published = board_ever_published OR ?, public_slug = ?, public_milestone_swimlanes = ?,
		 sharing_revision = sharing_revision + 1, public_revoked_at = ?, public_revoked_by = ?, public_revoke_reason = ?,
		 sharing_changed_at = ?, sharing_changed_by = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND sharing_revision = ? AND COALESCE(ql_query, '') = ? AND is_private = ? AND COALESCE(workspace_id, 0) = COALESCE(?, 0)`,
			input.Enabled && input.StandaloneEnabled, input.Enabled, input.StandaloneEnabled, input.Enabled, input.Slug, input.MilestoneSwimlanes,
			revokedAt, revokedBy, revokeReason, now, actor.UserID, id, input.Revision, collection.QLQuery, collection.IsPrivate, collection.WorkspaceID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrCollectionConflict
		}
		if _, err := tx.Exec(`INSERT INTO customer_resource_access (resource_kind, resource_id, mode, organisation_ids) VALUES ('board', ?, ?, ?)
		 ON CONFLICT(resource_kind, resource_id) DO UPDATE SET mode = excluded.mode, organisation_ids = excluded.organisation_ids`, id, policy.Mode, string(policyJSON)); err != nil {
			return err
		}
		return logger.LogAuditTx(context.Background(), tx, logger.AuditEvent{
			UserID: actor.UserID, Username: actor.Username, IPAddress: actor.IPAddress, UserAgent: actor.UserAgent,
			Source: actor.Source, ActionType: action, ResourceType: logger.ResourceCollection,
			ResourceID: &id, ResourceName: collection.Name, Success: true,
			Details: mergeAuditDetails(map[string]any{"before": boardSharingAuditState(before), "after": boardSharingAuditState(&after), "reason": reason}, actor),
		})
	})
	if database.IsUniqueConstraintError(err) {
		return nil, ErrCollectionConflict
	}
	if err != nil {
		return nil, err
	}
	if action == logger.ActionCollectionPublicRevoke && collection.CreatedBy != nil && s.sharingNotifications != nil {
		_, notifyErr := s.sharingNotifications.CreateNotification(models.Notification{
			UserID: *collection.CreatedBy, Title: "Board publication revoked", Type: "warning", AuthorizationScope: models.NotificationScopeSystem,
			Message:   fmt.Sprintf("%s revoked sharing for %s: %s", actor.Username, collection.Name, reason),
			ActionURL: fmt.Sprintf("/collections/%d", id), SourceType: "collection.public_revoke", SourceID: &id,
			DeliveryKey: fmt.Sprintf("board-revoke:%d:%d", id, after.Revision),
		})
		if notifyErr != nil {
			slog.Error("board revoke owner notification failed", "collection_id", id, "error", notifyErr)
		}
	}
	return s.GetBoardSharing(actor.UserID, id)
}

func (s *CollectionApplicationService) BoardSharingHistory(userID, id int) ([]BoardSharingEvent, error) {
	if _, _, err := s.sharingCollection(userID, id); err != nil {
		return nil, err
	}
	rows, _, err := repository.NewAuditLogRepository(s.db).List(repository.AuditLogFilters{
		ResourceType: logger.ResourceCollection, ResourceID: &id,
		ActionTypes: []string{"collection.sharing_update", "collection.public_revoke", "collection.public_allow_republish", "collection.placement_update", "collection.placement_remove"},
	}, 1, 100)
	if err != nil {
		return nil, err
	}
	events := []BoardSharingEvent{}
	for _, row := range rows {
		before, err := json.Marshal(row.Details["before"])
		if err != nil {
			return nil, err
		}
		after, err := json.Marshal(row.Details["after"])
		if err != nil {
			return nil, err
		}
		reason, _ := row.Details["reason"].(string)
		event := BoardSharingEvent{ID: row.ID, ActorID: row.UserID, ActorName: row.Username, Action: row.ActionType,
			Before: before, After: after, Reason: reason, CreatedAt: row.Timestamp.UTC().Format(time.RFC3339Nano)}
		events = append(events, event)
	}
	return events, nil
}

func (s *CollectionApplicationService) ListBoardPublications(userID int, filter BoardPublicationFilter) ([]BoardSharing, int, error) {
	admin, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return nil, 0, err
	}
	if filter.SystemAdminOnly && !admin {
		return nil, 0, ErrCollectionForbidden
	}
	where := ` WHERE c.board_ever_published = true`
	args := []any{}
	if !admin {
		where += ` AND c.created_by = ?`
		args = append(args, userID)
	}
	if filter.Query != "" {
		where += ` AND (LOWER(c.name) LIKE ? OR LOWER(c.public_slug) LIKE ?)`
		q := "%" + strings.ToLower(filter.Query) + "%"
		args = append(args, q, q)
	}
	if filter.OwnerID != nil {
		where += ` AND c.created_by = ?`
		args = append(args, *filter.OwnerID)
	}
	if filter.WorkspaceID != nil {
		where += ` AND c.workspace_id = ?`
		args = append(args, *filter.WorkspaceID)
	}
	if filter.Global {
		where += ` AND c.workspace_id IS NULL`
	}
	switch filter.State {
	case "enabled":
		where += ` AND c.board_enabled = true`
	case "disabled":
		where += ` AND c.board_enabled = false`
	case "revoked":
		where += ` AND c.public_revoked_at IS NOT NULL`
	case "", "all":
	default:
		return nil, 0, collectionValidation("unknown publication state")
	}
	if filter.AccessMode != "" {
		if !slices.Contains([]string{"public", "organisations"}, filter.AccessMode) {
			return nil, 0, collectionValidation("unknown access mode")
		}
		where += ` AND COALESCE(a.mode, 'public') = ?`
		args = append(args, filter.AccessMode)
	}
	if filter.OrganisationID != nil {
		// IDs are normalized JSON integers; strip spaces and use comma boundaries.
		where += ` AND a.mode = 'organisations' AND (',' || REPLACE(REPLACE(REPLACE(a.organisation_ids, ' ', ''), '[', ''), ']', '') || ',') LIKE ?`
		args = append(args, fmt.Sprintf("%%,%d,%%", *filter.OrganisationID))
	}
	from := ` FROM collections c LEFT JOIN customer_resource_access a ON a.resource_kind = 'board' AND a.resource_id = c.id`
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, max(1, min(filter.Limit, 1000)), max(0, filter.Offset))
	rows, err := s.db.Query(`SELECT c.id`+from+where+` ORDER BY CASE WHEN c.sharing_changed_at IS NULL THEN 1 ELSE 0 END, c.sharing_changed_at DESC, c.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, 0, err
	}
	enabled, err := s.boardsEnabled()
	if err != nil {
		return nil, 0, err
	}
	result, err := s.boardSharingRows(ids, admin, enabled)
	return result, total, err
}
