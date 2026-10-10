package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// PortalService encapsulates database logic for portal requests
type PortalService struct {
	db       database.Database
	items    *repository.ItemRepository
	comments *CommentService
}

// NewPortalService creates a new PortalService
func NewPortalService(db database.Database) *PortalService {
	return &PortalService{db: db, items: repository.NewItemRepository(db)}
}

// SetCommentService wires the application comment service so portal replies
// run the standard side-effect pipeline (notifications, mentions, webhooks).
func (s *PortalService) SetCommentService(cs *CommentService) {
	s.comments = cs
}

// GetCustomerIDForUser returns the portal customer linked to an internal user.
func (s *PortalService) GetCustomerIDForUser(ctx context.Context, userID int) (int, error) {
	var customerID int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM portal_customers WHERE user_id = ? LIMIT 1`, userID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrPortalCustomerNotFound
	}
	return customerID, err
}

// GetUserRequesterTemplateVars returns requester template values for an internal user.
func (s *PortalService) GetUserRequesterTemplateVars(ctx context.Context, userID int) (name, email string, err error) {
	var firstName, lastName string
	err = s.db.QueryRowContext(ctx, `SELECT first_name, last_name, email FROM users WHERE id = ?`, userID).Scan(&firstName, &lastName, &email)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(firstName + " " + lastName), email, nil
}

// GetCustomerRequesterTemplateVars returns requester template values for a portal customer.
func (s *PortalService) GetCustomerRequesterTemplateVars(ctx context.Context, customerID int) (name, email string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT name, email FROM portal_customers WHERE id = ?`, customerID).Scan(&name, &email)
	return name, email, err
}

// GetCustomFieldNamesByID returns custom field definition names keyed by id.
func (s *PortalService) GetCustomFieldNamesByID(ctx context.Context, ids []int) (map[int]string, error) {
	if len(ids) == 0 {
		return map[int]string{}, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM custom_field_definitions WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// CreatedPortalComment describes a comment created by a portal request participant.
type CreatedPortalComment struct {
	ID               int64
	ItemID           int
	Content          string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	AuthorName       string
	AuthorAvatar     string
	AuthorID         *int
	PortalCustomerID *int
}

// CreateRequestComment creates a comment as either an internal user or portal customer and enriches author details.
func (s *PortalService) CreateRequestComment(ctx context.Context, itemID int, content string, internalUserID, portalCustomerID *int) (*CreatedPortalComment, error) {
	now := time.Now()
	out := &CreatedPortalComment{ItemID: itemID, Content: content, CreatedAt: now, UpdatedAt: now}

	// Route through the wired CommentService — the single comment-write
	// chokepoint — so customer replies notify assignee, creator, and watchers
	// and dispatch comment webhooks. The email reply consumer skips
	// portal-customer authors, so customers never receive an echo of their own
	// reply. Use a bare service only when the caller did not wire one (tests).
	commentService := s.comments
	if commentService == nil {
		commentService = NewCommentService(s.db)
	}
	params := CreateCommentParams{
		ItemID:    itemID,
		Content:   content,
		CreatedAt: &now,
	}
	if internalUserID != nil {
		params.AuthorID = *internalUserID
	} else if portalCustomerID != nil {
		params.PortalCustomerID = portalCustomerID
	}
	res, err := commentService.Create(params)
	if err != nil {
		return nil, err
	}
	out.ID = res.CommentID

	if internalUserID != nil {
		out.AuthorID = internalUserID
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(first_name || ' ' || last_name, 'Unknown'), COALESCE(avatar_url, '') FROM users WHERE id = ?`, *internalUserID).Scan(&out.AuthorName, &out.AuthorAvatar); err != nil {
			out.AuthorName = "Unknown"
			out.AuthorAvatar = ""
		}
		return out, nil
	}
	out.PortalCustomerID = portalCustomerID
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(name, 'Unknown') FROM portal_customers WHERE id = ?`, *portalCustomerID).Scan(&out.AuthorName); err != nil {
		out.AuthorName = "Unknown"
	}
	out.AuthorAvatar = ""
	return out, nil
}

type PortalRequestSummary struct {
	ID                  int     `json:"id"`
	WorkspaceID         int     `json:"workspace_id"`
	WorkspaceItemNumber int     `json:"workspace_item_number"`
	WorkspaceName       string  `json:"workspace_name"`
	WorkspaceKey        string  `json:"workspace_key"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	DescriptionHTML     string  `json:"description_html,omitempty"`
	Status              string  `json:"status"`
	Priority            string  `json:"priority"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	ChannelID           *int    `json:"channel_id"`
	IntakeID            *int    `json:"intake_id,omitempty"`
	RequestTypeID       *int    `json:"request_type_id"`
	RequestTypeName     *string `json:"request_type_name"`
	RequestTypeIcon     *string `json:"request_type_icon"`
	RequestTypeColor    *string `json:"request_type_color"`
	CommentCount        int     `json:"comment_count"`
	StatusCategoryColor *string `json:"status_category_color"`
	StatusIsCompleted   bool    `json:"status_is_completed"`
	// MergedIntoItemID points at the canonical request when this one was
	// merged away; the portal redirects there (WI-1528).
	MergedIntoItemID *int `json:"merged_into_item_id,omitempty"`
	// Shared is true when the request is visible through organisation sharing
	// rather than because the viewer created or participates in it (WI-1139).
	Shared bool `json:"shared,omitempty"`
}

// PortalRequestDetail represents detailed portal request info including ownership
type PortalRequestDetail struct {
	PortalRequestSummary
	CreatorID               *int `json:"creator_id,omitempty"`
	CreatorPortalCustomerID *int `json:"creator_portal_customer_id,omitempty"`
}

// PortalComment represents a comment on a portal request
type PortalComment struct {
	ID               int    `json:"id"`
	ItemID           int    `json:"item_id"`
	AuthorID         *int   `json:"author_id,omitempty"`
	PortalCustomerID *int   `json:"portal_customer_id,omitempty"`
	Content          string `json:"content"`
	ContentHTML      string `json:"content_html,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	AuthorName       string `json:"author_name"`
	AuthorAvatar     string `json:"author_avatar"`
}

// portalRequestSummaryFromRow maps a repository portal request row to the
// portal API summary shape.
func portalRequestSummaryFromRow(row repository.PortalRequestRow) PortalRequestSummary {
	descriptionHTML, _ := markdown.Render(row.Description)
	return PortalRequestSummary{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		WorkspaceItemNumber: row.WorkspaceItemNumber,
		WorkspaceName:       row.WorkspaceName,
		WorkspaceKey:        row.WorkspaceKey,
		Title:               row.Title,
		Description:         row.Description,
		DescriptionHTML:     descriptionHTML,
		Status:              row.StatusName,
		Priority:            row.PriorityName,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		ChannelID:           row.ChannelID,
		IntakeID:            row.IntakeID,
		RequestTypeID:       row.RequestTypeID,
		RequestTypeName:     row.RequestTypeName,
		RequestTypeIcon:     row.RequestTypeIcon,
		RequestTypeColor:    row.RequestTypeColor,
		CommentCount:        row.CommentCount,
		StatusCategoryColor: row.StatusCategoryColor,
		StatusIsCompleted:   row.StatusIsCompleted,
		MergedIntoItemID:    row.MergedIntoItemID,
	}
}

func portalRequestSummariesFromRows(rows []repository.PortalRequestRow) []PortalRequestSummary {
	requests := make([]PortalRequestSummary, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, portalRequestSummaryFromRow(row))
	}
	return requests
}

// portalRequestVisibility resolves the WI-1547 visibility rules for one portal:
// the enabled intakes linked to it (so their email-originated tickets surface
// in "My Requests" and pass ownership checks), legacy email channels still
// linked through email_connected_portal_id without an intake, plus the
// workspaces the portal serves (defense-in-depth against an intake routed at an
// unrelated workspace). Intake-scoped linking keeps a sibling internal intake
// on the same mailbox out of the portal. Configs are parsed in Go — no
// JSON-in-SQL dialect concerns. A missing portal channel yields an empty
// visibility: nothing extra is exposed and the caller's own-channel query
// degrades to the portal itself.
func (s *PortalService) portalRequestVisibility(ctx context.Context, portalChannelID int) (repository.PortalRequestVisibility, error) {
	vis := repository.PortalRequestVisibility{PortalChannelID: portalChannelID}

	var portalConfig string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(config, '{}') FROM channels WHERE id = ?`, portalChannelID).Scan(&portalConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return vis, nil
	}
	if err != nil {
		return vis, fmt.Errorf("load portal config: %w", err)
	}
	var portalCfg models.ChannelConfig
	if json.Unmarshal([]byte(portalConfig), &portalCfg) == nil {
		vis.ServedWorkspaceIDs = portalCfg.PortalWorkspaceIDs
	}

	// Intakes linked to this portal own the visibility since WI-1644: an item is
	// visible when it came from one of those intakes, so a sibling internal
	// intake on the same mailbox stays private.
	intakes, err := repository.NewIntakeRepository(s.db).ListByPortal(ctx, portalChannelID)
	if err != nil {
		return vis, fmt.Errorf("list portal intakes: %w", err)
	}
	for _, intake := range intakes {
		if intake.Status != models.IntakeStatusEnabled {
			continue
		}
		vis.LinkedIntakeIDs = append(vis.LinkedIntakeIDs, intake.ID)
	}

	// Legacy fallback: enabled email channels that still carry
	// email_connected_portal_id but have no intake row.
	linked := map[int]struct{}{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, '{}') FROM channels
		WHERE type = 'email' AND direction = 'inbound' AND status = 'enabled'
	`)
	if err != nil {
		return vis, fmt.Errorf("list intake email channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var configJSON string
		if err := rows.Scan(&id, &configJSON); err != nil {
			continue
		}
		var cfg models.ChannelConfig
		if json.Unmarshal([]byte(configJSON), &cfg) != nil {
			continue
		}
		if cfg.EmailConnectedPortalID == nil || *cfg.EmailConnectedPortalID != portalChannelID {
			continue
		}
		if _, seen := linked[id]; seen {
			continue
		}
		linked[id] = struct{}{}
		vis.LinkedEmailChannelIDs = append(vis.LinkedEmailChannelIDs, id)
	}
	return vis, rows.Err()
}

// portalServesWorkspace reports whether the visibility's served-workspace
// restriction admits the item. An empty list means the portal has no
// configured workspaces (legacy): restriction disabled.
func portalServesWorkspace(vis repository.PortalRequestVisibility, workspaceID int) bool {
	if len(vis.ServedWorkspaceIDs) == 0 {
		return true
	}
	for _, id := range vis.ServedWorkspaceIDs {
		if id == workspaceID {
			return true
		}
	}
	return false
}

// loadPortalOrgShare resolves whether organisation sharing applies to the given
// portal customer and, if so, which mode. The organisation settings live in a
// JSON column, so they are parsed here in Go; the hot portal query only ever
// receives the resolved mode.
func (s *PortalService) loadPortalOrgShare(ctx context.Context, portalCustomerID int) (repository.PortalOrgShare, error) {
	orgID, settings, ok, err := s.loadPortalCustomerOrgSettings(ctx, portalCustomerID)
	if err != nil || !ok {
		return repository.PortalOrgShare{}, err
	}
	parsed := models.ParseOrgRequestSharingSettings(settings)
	if parsed.RequestSharing == models.OrgRequestSharingDisabled {
		return repository.PortalOrgShare{}, nil
	}
	if parsed.RequestSharingAudience == models.OrgRequestSharingAudienceRoles {
		holds, err := s.portalCustomerHoldsRole(ctx, portalCustomerID, parsed.RequestVisibleRoleIDs)
		if err != nil {
			return repository.PortalOrgShare{}, err
		}
		if !holds {
			return repository.PortalOrgShare{}, nil
		}
	}
	return repository.PortalOrgShare{OrganisationID: orgID, Mode: parsed.RequestSharing}, nil
}

// PortalCustomerOrgRequestSharing returns the request-sharing settings for the
// customer's organisation, or the defaults when the customer has none. The
// submit path uses it to decide whether the creator may opt into organisation
// sharing; the audience filter does not apply to the creator's own choice.
func (s *PortalService) PortalCustomerOrgRequestSharing(ctx context.Context, portalCustomerID int) (models.OrgRequestSharingSettings, error) {
	_, settings, ok, err := s.loadPortalCustomerOrgSettings(ctx, portalCustomerID)
	if err != nil || !ok {
		return models.DefaultOrgRequestSharingSettings(), err
	}
	return models.ParseOrgRequestSharingSettings(settings), nil
}

// loadPortalCustomerOrgSettings resolves the customer's organisation and its
// settings blob. ok is false when the customer has no organisation (or no
// longer exists), in which case organisation sharing does not apply.
func (s *PortalService) loadPortalCustomerOrgSettings(ctx context.Context, portalCustomerID int) (orgID int, settings map[string]any, ok bool, err error) {
	var id sql.NullInt64
	err = s.db.QueryRowContext(ctx,
		`SELECT customer_organisation_id FROM portal_customers WHERE id = ?`, portalCustomerID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, fmt.Errorf("load portal customer organisation: %w", err)
	}
	if !id.Valid {
		return 0, nil, false, nil
	}

	var settingsJSON string
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(settings, '{}') FROM customer_organisations WHERE id = ?`, id.Int64,
	).Scan(&settingsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, fmt.Errorf("load organisation settings: %w", err)
	}
	if settingsJSON != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			return 0, nil, false, fmt.Errorf("parse organisation settings: %w", err)
		}
	}
	return int(id.Int64), settings, true, nil
}

// portalCustomerHoldsRole reports whether the customer holds at least one of
// the allowed contact roles. An empty allowlist never matches, so a "roles"
// audience with no configured roles exposes nothing.
func (s *PortalService) portalCustomerHoldsRole(ctx context.Context, portalCustomerID int, allowed []int) (bool, error) {
	if len(allowed) == 0 {
		return false, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT contact_role_id FROM portal_customer_roles WHERE portal_customer_id = ?`, portalCustomerID)
	if err != nil {
		return false, fmt.Errorf("load portal customer roles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	allowedSet := make(map[int]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	for rows.Next() {
		var roleID int
		if err := rows.Scan(&roleID); err != nil {
			return false, fmt.Errorf("scan portal customer role: %w", err)
		}
		if _, ok := allowedSet[roleID]; ok {
			return true, nil
		}
	}
	return false, rows.Err()
}

// GetRequestsByCreatorID gets requests for internal user (by creator_id)
func (s *PortalService) GetRequestsByCreatorID(ctx context.Context, creatorID, channelID int) ([]PortalRequestSummary, error) {
	vis, err := s.portalRequestVisibility(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve portal visibility: %w", err)
	}
	rows, err := s.items.ListChannelRequestsByCreator(creatorID, vis)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch requests: %w", err)
	}
	return portalRequestSummariesFromRows(rows), nil
}

// GetRequestsByPortalCustomerID gets requests for portal customer (by creator_portal_customer_id)
func (s *PortalService) GetRequestsByPortalCustomerID(ctx context.Context, portalCustomerID, channelID int) ([]PortalRequestSummary, error) {
	vis, err := s.portalRequestVisibility(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve portal visibility: %w", err)
	}
	orgShare, err := s.loadPortalOrgShare(ctx, portalCustomerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.items.ListChannelRequestsByPortalCustomer(portalCustomerID, orgShare, vis)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch requests: %w", err)
	}
	summaries := portalRequestSummariesFromRows(rows)

	// Label organisation-shared rows so the portal can distinguish them from the
	// customer's own requests and requests they participate in. One query for
	// the viewer's participations keeps this off the per-row path.
	participantIDs, err := repository.NewItemParticipantRepository(s.db).ListItemIDsForCustomer(portalCustomerID)
	if err != nil {
		return nil, fmt.Errorf("load portal customer participations: %w", err)
	}
	participants := make(map[int]struct{}, len(participantIDs))
	for _, id := range participantIDs {
		participants[id] = struct{}{}
	}
	for i := range summaries {
		if i >= len(rows) {
			break
		}
		creatorID := rows[i].CreatorPortalCustomerID
		_, isParticipant := participants[rows[i].ID]
		summaries[i].Shared = (creatorID == nil || *creatorID != portalCustomerID) && !isParticipant
	}
	return summaries, nil
}

// GetRequestDetail gets request detail with ownership info. Returns nil
// (without error) when the item does not exist.
func (s *PortalService) GetRequestDetail(_ context.Context, itemID int) (*PortalRequestDetail, error) {
	row, err := s.items.GetPortalRequest(itemID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch request: %w", err)
	}
	return &PortalRequestDetail{
		PortalRequestSummary:    portalRequestSummaryFromRow(*row),
		CreatorID:               row.CreatorID,
		CreatorPortalCustomerID: row.CreatorPortalCustomerID,
	}, nil
}

// PortalServesRequestWorkspace reports whether the portal channel currently
// serves the item's workspace. VerifyRequestOwnership applies the same check
// for owners; approver-derived access must apply it too, otherwise removing a
// workspace from a portal leaves active approvers able to read its requests.
func (s *PortalService) PortalServesRequestWorkspace(ctx context.Context, itemID, portalChannelID int) (bool, error) {
	detail, err := s.GetRequestDetail(ctx, itemID)
	if err != nil {
		return false, err
	}
	if detail == nil {
		return false, nil
	}
	vis, err := s.portalRequestVisibility(ctx, portalChannelID)
	if err != nil {
		return false, err
	}
	return portalServesWorkspace(vis, detail.WorkspaceID), nil
}

// PortalServedWorkspaceIDs returns the workspace IDs the portal channel
// serves. An empty slice means the restriction is disabled (legacy config).
// Portal callers that check many items should resolve the scope once.
func (s *PortalService) PortalServedWorkspaceIDs(ctx context.Context, portalChannelID int) ([]int, error) {
	vis, err := s.portalRequestVisibility(ctx, portalChannelID)
	if err != nil {
		return nil, err
	}
	return vis.ServedWorkspaceIDs, nil
}

// PortalServesWorkspace reports whether the portal channel's served-workspace
// scope admits workspaceID. It is the workspace-level counterpart to
// PortalServesRequestWorkspace and lets approval reads apply the same
// restriction without re-resolving the item.
func (s *PortalService) PortalServesWorkspace(ctx context.Context, portalChannelID, workspaceID int) (bool, error) {
	vis, err := s.portalRequestVisibility(ctx, portalChannelID)
	if err != nil {
		return false, err
	}
	return portalServesWorkspace(vis, workspaceID), nil
}

// VerifyRequestOwnership verifies that a user owns a request
// Returns true if the user owns the request within the specified channel.
// Since WI-1547 the check also admits items created through enabled intake
// email channels linked to this portal (email_connected_portal_id), so a
// customer who opened a ticket by email can read and continue it in the
// connected portal. Since WI-1136 it also admits external request
// participants. Since WI-1139 it also admits organisation-shared requests
// (automatic mode, or requester_choice with the item flag set) when the
// viewer qualifies for the configured audience. Items outside the portal's
// served workspaces never pass, regardless of channel linkage or participation.
func (s *PortalService) VerifyRequestOwnership(ctx context.Context, itemID, channelID int, internalUserID, portalCustomerID *int) (bool, error) {
	detail, err := s.GetRequestDetail(ctx, itemID)
	if err != nil {
		return false, err
	}
	if detail == nil {
		return false, nil
	}

	// Verify channel: the portal's own channel, or a linked intake channel.
	if detail.ChannelID == nil {
		return false, nil
	}
	vis, err := s.portalRequestVisibility(ctx, channelID)
	if err != nil {
		return false, err
	}
	if *detail.ChannelID != channelID {
		linked := false
		for _, id := range vis.LinkedEmailChannelIDs {
			if id == *detail.ChannelID {
				linked = true
				break
			}
		}
		if !linked && detail.IntakeID != nil {
			for _, id := range vis.LinkedIntakeIDs {
				if id == *detail.IntakeID {
					linked = true
					break
				}
			}
		}
		if !linked {
			return false, nil
		}
	}

	// Defense-in-depth: the item must live in a workspace the portal serves.
	if !portalServesWorkspace(vis, detail.WorkspaceID) {
		return false, nil
	}

	// Check ownership based on auth type
	if internalUserID != nil && detail.CreatorID != nil && *detail.CreatorID == *internalUserID {
		return true, nil
	}
	if portalCustomerID != nil && detail.CreatorPortalCustomerID != nil && *detail.CreatorPortalCustomerID == *portalCustomerID {
		return true, nil
	}

	// External request participants (WI-1136) may read and reply to tickets
	// they were added to, even though they are not the creator.
	if portalCustomerID != nil {
		isParticipant, err := repository.NewItemParticipantRepository(s.db).IsParticipant(itemID, *portalCustomerID)
		if err != nil {
			return false, fmt.Errorf("check request participant: %w", err)
		}
		if isParticipant {
			return true, nil
		}
	}

	// Org-shared requests (WI-1139): the creator opted in (or the org shares
	// automatically) and the viewer qualifies for the audience.
	if portalCustomerID != nil {
		orgShare, err := s.loadPortalOrgShare(ctx, *portalCustomerID)
		if err != nil {
			return false, err
		}
		if orgShare.OrganisationID != 0 {
			creatorOrgID, shared, err := s.items.PortalRequestOrgShare(itemID)
			if err != nil {
				return false, fmt.Errorf("check request organisation share: %w", err)
			}
			if creatorOrgID != nil && *creatorOrgID == orgShare.OrganisationID {
				if orgShare.Mode == models.OrgRequestSharingAutomatic ||
					(orgShare.Mode == models.OrgRequestSharingRequesterChoice && shared) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// GetRequestComments gets comments for a request (public only)
func (s *PortalService) GetRequestComments(ctx context.Context, itemID int) ([]PortalComment, error) {
	query := `
		SELECT
			c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.created_at, c.updated_at,
			COALESCE(u.first_name || ' ' || u.last_name, pc.name, 'Unknown') AS author_name,
			COALESCE(u.avatar_url, '') AS author_avatar
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.item_id = ? AND (c.is_private = false OR c.is_private IS NULL)
		ORDER BY c.created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	defer rows.Close()

	var comments []PortalComment
	for rows.Next() {
		var comment PortalComment
		var authorID, portalCustomerID sql.NullInt64
		err := rows.Scan(
			&comment.ID, &comment.ItemID, &authorID, &portalCustomerID, &comment.Content,
			&comment.CreatedAt, &comment.UpdatedAt, &comment.AuthorName, &comment.AuthorAvatar,
		)
		if err != nil {
			continue
		}
		if authorID.Valid {
			id := int(authorID.Int64)
			comment.AuthorID = &id
		}
		if portalCustomerID.Valid {
			id := int(portalCustomerID.Int64)
			comment.PortalCustomerID = &id
		}
		comment.ContentHTML, err = markdown.Render(comment.Content)
		if err != nil {
			return nil, fmt.Errorf("render portal comment %d: %w", comment.ID, err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate comments: %w", err)
	}

	if comments == nil {
		comments = []PortalComment{}
	}

	return comments, nil
}

// RequestTypeField represents a field in a request type form
type RequestTypeField struct {
	ID                  int       `json:"id"`
	RequestTypeID       int       `json:"request_type_id"`
	FieldIdentifier     string    `json:"field_identifier"`
	FieldType           string    `json:"field_type"`
	DisplayOrder        int       `json:"display_order"`
	IsRequired          bool      `json:"is_required"`
	DisplayName         *string   `json:"display_name"`
	Description         *string   `json:"description"`
	StepNumber          int       `json:"step_number"`
	VirtualFieldType    *string   `json:"virtual_field_type"`
	VirtualFieldOptions *string   `json:"virtual_field_options"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	FieldName           string    `json:"field_name"`
	FieldLabel          string    `json:"field_label"`
}

// GetRequestTypeFields gets fields for a request type
func (s *PortalService) GetRequestTypeFields(ctx context.Context, requestTypeID int) ([]RequestTypeField, error) {
	allowedCustomFieldIDs, err := allowedRequestTypeCustomFieldIdentifiers(ctx, s.db, requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("resolve request type create-screen fields: %w", err)
	}
	query := `
		SELECT rtf.id, rtf.request_type_id, rtf.field_identifier, rtf.field_type,
		       rtf.display_order, rtf.is_required, rtf.display_name, rtf.description,
		       COALESCE(rtf.step_number, 1) as step_number,
		       rtf.virtual_field_type, rtf.virtual_field_options,
		       rtf.created_at, rtf.updated_at,
		       CASE
		           WHEN rtf.field_type = 'virtual' THEN rtf.field_identifier
		           ELSE COALESCE(cfd.name, rtf.field_identifier)
		       END as field_name,
		       CASE
		           WHEN rtf.display_name IS NOT NULL AND rtf.display_name != '' THEN rtf.display_name
		           WHEN rtf.field_type = 'virtual' THEN rtf.field_identifier
		           ELSE COALESCE(cfd.name, rtf.field_identifier)
		       END as field_label
		FROM request_type_fields rtf
		LEFT JOIN custom_field_definitions cfd ON rtf.field_type = 'custom'
		    AND rtf.field_identifier = CAST(cfd.id AS TEXT)
		WHERE rtf.request_type_id = ?
		ORDER BY rtf.step_number, rtf.display_order, rtf.id`

	rows, err := s.db.QueryContext(ctx, query, requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch request type fields: %w", err)
	}
	defer rows.Close()

	var fields []RequestTypeField
	for rows.Next() {
		var field RequestTypeField
		var displayName, description, virtualFieldType, virtualFieldOptions sql.NullString
		err := rows.Scan(
			&field.ID, &field.RequestTypeID, &field.FieldIdentifier, &field.FieldType,
			&field.DisplayOrder, &field.IsRequired, &displayName, &description,
			&field.StepNumber, &virtualFieldType, &virtualFieldOptions,
			&field.CreatedAt, &field.UpdatedAt,
			&field.FieldName, &field.FieldLabel,
		)
		if err != nil {
			continue
		}
		if field.FieldType == "custom" {
			if _, allowed := allowedCustomFieldIDs[field.FieldIdentifier]; !allowed {
				continue
			}
		}
		if displayName.Valid {
			field.DisplayName = &displayName.String
		}
		if description.Valid {
			field.Description = &description.String
		}
		if virtualFieldType.Valid {
			field.VirtualFieldType = &virtualFieldType.String
		}
		if virtualFieldOptions.Valid {
			field.VirtualFieldOptions = &virtualFieldOptions.String
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate request type fields: %w", err)
	}

	if fields == nil {
		fields = []RequestTypeField{}
	}

	return fields, nil
}

// ValidateRequestTypeBelongsToChannel verifies request type is in the channel
func (s *PortalService) ValidateRequestTypeBelongsToChannel(ctx context.Context, requestTypeID, channelID int) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM request_types WHERE id = ? AND channel_id = ? AND is_active = true AND kind != 'email')",
		requestTypeID, channelID).Scan(&exists)
	return exists, err
}

// GetCustomFieldsForChannel returns custom field definitions used by the portal
// request types and form-mode asset reports the caller can see.
//
// Visibility: when isAdmin is false, request types and asset reports are
// filtered by IsVisibleTo(userGroupIDs, customerOrgID) before their custom
// field references are collected. Without this filter the endpoint leaks
// field names, descriptions, and option vocabularies for request types /
// reports that are hidden from the caller via visibility_group_ids /
// visibility_org_ids — the same gate enforced by GetRequestTypes and
// GetAssetReports.
func (s *PortalService) GetCustomFieldsForChannel(ctx context.Context, channelID int, userGroupIDs []int, customerOrgID *int, isAdmin bool) ([]models.CustomFieldDefinition, error) {
	cfIDs, err := s.collectVisibleChannelCustomFieldIDs(ctx, channelID, userGroupIDs, customerOrgID, isAdmin)
	if err != nil {
		return nil, err
	}
	return s.getCustomFieldDefinitions(ctx, cfIDs)
}

// GetRequestTypeForm returns the filtered form fields together with exactly
// the custom-field definitions referenced by those fields. Deriving the IDs
// from the filtered result avoids resolving the create-screen allowance twice
// and excludes unrelated fields from the complete public form response.
func (s *PortalService) GetRequestTypeForm(ctx context.Context, requestTypeID int) ([]RequestTypeField, []models.CustomFieldDefinition, error) {
	fields, err := s.GetRequestTypeFields(ctx, requestTypeID)
	if err != nil {
		return nil, nil, err
	}
	cfIDs := make(map[int]struct{})
	for _, field := range fields {
		if field.FieldType != "custom" {
			continue
		}
		id, err := strconv.Atoi(field.FieldIdentifier)
		if err == nil {
			cfIDs[id] = struct{}{}
		}
	}
	definitions, err := s.getCustomFieldDefinitions(ctx, cfIDs)
	if err != nil {
		return nil, nil, err
	}
	return fields, definitions, nil
}

func (s *PortalService) getCustomFieldDefinitions(ctx context.Context, cfIDs map[int]struct{}) ([]models.CustomFieldDefinition, error) {
	if len(cfIDs) == 0 {
		return []models.CustomFieldDefinition{}, nil
	}
	placeholders := make([]string, 0, len(cfIDs))
	args := make([]any, 0, len(cfIDs))
	for id := range cfIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := fmt.Sprintf(`
		SELECT cfd.id, cfd.name, cfd.field_type, cfd.description,
		       cfd.required, cfd.options, cfd.display_order, cfd.system_default,
		       cfd.applies_to_portal_customers, cfd.applies_to_customer_organisations,
		       cfd.created_at, cfd.updated_at
		FROM custom_field_definitions cfd
		WHERE cfd.id IN (%s)
		ORDER BY cfd.display_order, cfd.name`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch custom-field definitions: %w", err)
	}
	defer rows.Close()

	var fields []models.CustomFieldDefinition
	for rows.Next() {
		var field models.CustomFieldDefinition
		var description, options sql.NullString
		err := rows.Scan(
			&field.ID, &field.Name, &field.FieldType, &description,
			&field.Required, &options, &field.DisplayOrder, &field.SystemDefault,
			&field.AppliesToPortalCustomers, &field.AppliesToCustomerOrganisations,
			&field.CreatedAt, &field.UpdatedAt,
		)
		if err != nil {
			continue
		}
		if description.Valid {
			field.Description = description.String
		}
		if options.Valid {
			field.Options = options.String
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate custom-field definitions: %w", err)
	}

	if fields == nil {
		fields = []models.CustomFieldDefinition{}
	}

	return fields, nil
}

// collectVisibleChannelCustomFieldIDs returns the set of custom-field IDs
// referenced by request types and form-mode asset reports in the channel
// after applying portal visibility rules. Admins skip the IsVisibleTo gate.
func (s *PortalService) collectVisibleChannelCustomFieldIDs(ctx context.Context, channelID int, userGroupIDs []int, customerOrgID *int, isAdmin bool) (map[int]struct{}, error) {
	cfIDs := make(map[int]struct{})
	var channelType, configJSON string
	if err := s.db.QueryRowContext(ctx, "SELECT type, config FROM channels WHERE id = ?", channelID).Scan(&channelType, &configJSON); err != nil {
		return nil, fmt.Errorf("load channel custom-field routing: %w", err)
	}
	var channelConfig models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &channelConfig); err != nil {
		return nil, fmt.Errorf("parse channel custom-field routing: %w", err)
	}
	var servedWorkspaceIDs []int
	switch channelType {
	case "portal":
		servedWorkspaceIDs = channelConfig.PortalWorkspaceIDs
	case "form":
		servedWorkspaceIDs = channelConfig.FormWorkspaceIDs
	}

	// Visible request types → their custom-typed configured fields.
	rtRows, err := s.db.QueryContext(ctx, `
		SELECT rt.id, rt.item_type_id, rt.workspace_id, rt.visibility_group_ids, rt.visibility_org_ids
		FROM request_types rt
		WHERE rt.channel_id = ? AND rt.is_active = true AND rt.kind != 'email'
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel request types: %w", err)
	}
	type visibleRoute struct {
		id          int
		itemTypeID  int
		workspaceID int
	}
	var visibleRTRoutes []visibleRoute
	func() {
		defer rtRows.Close()
		for rtRows.Next() {
			var id, itemTypeID int
			var workspaceID *int
			var groups, orgs sql.NullString
			if err := rtRows.Scan(&id, &itemTypeID, &workspaceID, &groups, &orgs); err != nil {
				continue
			}
			if !isAdmin && !portalRowVisible(groups, orgs, userGroupIDs, customerOrgID) {
				continue
			}
			// Request types pin their workspace at creation; an unpinned row is
			// not routable and contributes no fields.
			if workspaceID == nil || !containsInt(servedWorkspaceIDs, *workspaceID) {
				continue
			}
			visibleRTRoutes = append(visibleRTRoutes, visibleRoute{id: id, itemTypeID: itemTypeID, workspaceID: *workspaceID})
		}
	}()
	if err := rtRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channel request types: %w", err)
	}
	for _, route := range visibleRTRoutes {
		if err := s.collectBoundCustomFieldIDs(ctx, cfIDs, "request_type", route.id, route.workspaceID, route.itemTypeID); err != nil {
			return nil, err
		}
	}

	// Visible form-mode asset reports → their custom-typed configured fields.
	arRows, err := s.db.QueryContext(ctx, `
		SELECT ar.id, ar.item_type_id, ar.workspace_id, ar.visibility_group_ids, ar.visibility_org_ids
		FROM asset_reports ar
		WHERE ar.channel_id = ? AND ar.is_active = true AND ar.run_mode = 'form'
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel asset reports: %w", err)
	}
	var visibleARRoutes []visibleRoute
	func() {
		defer arRows.Close()
		for arRows.Next() {
			var id int
			var itemTypeID, workspaceID *int
			var groups, orgs sql.NullString
			if err := arRows.Scan(&id, &itemTypeID, &workspaceID, &groups, &orgs); err != nil {
				continue
			}
			if !isAdmin && !portalRowVisible(groups, orgs, userGroupIDs, customerOrgID) {
				continue
			}
			if itemTypeID != nil && workspaceID != nil && containsInt(channelConfig.PortalWorkspaceIDs, *workspaceID) {
				visibleARRoutes = append(visibleARRoutes, visibleRoute{id: id, itemTypeID: *itemTypeID, workspaceID: *workspaceID})
			}
		}
	}()
	if err := arRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channel asset reports: %w", err)
	}
	for _, route := range visibleARRoutes {
		if err := s.collectBoundCustomFieldIDs(ctx, cfIDs, "asset_report", route.id, route.workspaceID, route.itemTypeID); err != nil {
			return nil, err
		}
	}

	return cfIDs, nil
}

// collectBoundCustomFieldIDs admits only custom fields present on the route's
// effective create screen. This keeps legacy or forged field rows from
// exposing definitions outside the portal/form workspace configuration.
func (s *PortalService) collectBoundCustomFieldIDs(ctx context.Context, out map[int]struct{}, ownerType string, ownerID, workspaceID, itemTypeID int) error {
	createScreenID, err := repository.NewScreenRepository(s.db).GetCreateScreenID(workspaceID, itemTypeID)
	if err != nil || createScreenID == nil {
		return err
	}
	screenFields, err := repository.NewScreenRepository(s.db).ListFields(*createScreenID)
	if err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(screenFields))
	for _, field := range screenFields {
		if field.FieldType == "custom" && field.FieldName != "" &&
			isPublicFormFillableCustomFieldType(field.CustomFieldType) {
			allowed[field.FieldIdentifier] = struct{}{}
		}
	}

	var query string
	switch ownerType {
	case "request_type":
		query = "SELECT field_identifier FROM request_type_fields WHERE field_type = 'custom' AND request_type_id = ?"
	case "asset_report":
		query = "SELECT field_identifier FROM asset_report_fields WHERE field_type = 'custom' AND asset_report_id = ?"
	default:
		return fmt.Errorf("unsupported public form owner %q", ownerType)
	}
	rows, err := s.db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return fmt.Errorf("collect bound custom field ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			continue
		}
		if _, ok := allowed[identifier]; !ok {
			continue
		}
		id, err := strconv.Atoi(identifier)
		if err != nil || id <= 0 {
			continue
		}
		out[id] = struct{}{}
	}
	return rows.Err()
}

func containsInt(values []int, candidate int) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

// portalRowVisible mirrors models.RequestType.IsVisibleTo / AssetReport.IsVisibleTo
// against the JSON-encoded visibility columns. Returns true when no
// restrictions are set or when the caller matches any allowed group/org.
func portalRowVisible(groupsCol, orgsCol sql.NullString, userGroupIDs []int, customerOrgID *int) bool {
	var allowGroups, allowOrgs []int
	if groupsCol.Valid && groupsCol.String != "" {
		if err := json.Unmarshal([]byte(groupsCol.String), &allowGroups); err != nil {
			return false
		}
	}
	if orgsCol.Valid && orgsCol.String != "" {
		if err := json.Unmarshal([]byte(orgsCol.String), &allowOrgs); err != nil {
			return false
		}
	}
	if len(allowGroups) == 0 && len(allowOrgs) == 0 {
		return true
	}
	for _, gid := range allowGroups {
		for _, ug := range userGroupIDs {
			if gid == ug {
				return true
			}
		}
	}
	if customerOrgID != nil {
		for _, oid := range allowOrgs {
			if oid == *customerOrgID {
				return true
			}
		}
	}
	return false
}
