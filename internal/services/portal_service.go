package services

import (
	"context"
	"database/sql"
	"fmt"

	"windshift/internal/database"
)

// PortalService encapsulates database logic for portal requests
type PortalService struct {
	db database.Database
}

// NewPortalService creates a new PortalService
func NewPortalService(db database.Database) *PortalService {
	return &PortalService{db: db}
}

// PortalRequestSummary represents a summarized portal request
type PortalRequestSummary struct {
	ID                  int     `json:"id"`
	WorkspaceID         int     `json:"workspace_id"`
	WorkspaceItemNumber int     `json:"workspace_item_number"`
	WorkspaceName       string  `json:"workspace_name"`
	WorkspaceKey        string  `json:"workspace_key"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	Status              string  `json:"status"`
	Priority            string  `json:"priority"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	ChannelID           *int    `json:"channel_id"`
	RequestTypeID       *int    `json:"request_type_id"`
	RequestTypeName     *string `json:"request_type_name"`
	RequestTypeIcon     *string `json:"request_type_icon"`
	RequestTypeColor    *string `json:"request_type_color"`
	CommentCount        int     `json:"comment_count"`
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
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	AuthorName       string `json:"author_name"`
	AuthorEmail      string `json:"author_email"`
}

// GetRequestsByCreatorID gets requests for internal user (by creator_id)
func (s *PortalService) GetRequestsByCreatorID(ctx context.Context, creatorID int, channelID int) ([]PortalRequestSummary, error) {
	query := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
			i.status_id, i.priority_id, i.created_at, i.updated_at,
			i.channel_id, i.request_type_id,
			w.name AS workspace_name,
			w.key AS workspace_key,
			rt.name AS request_type_name,
			rt.icon AS request_type_icon,
			rt.color AS request_type_color,
			(SELECT COUNT(*) FROM comments WHERE item_id = i.id AND (is_private = false OR is_private IS NULL)) AS comment_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN request_types rt ON i.request_type_id = rt.id
		WHERE i.creator_id = ? AND i.channel_id = ?
		ORDER BY i.created_at DESC
	`

	return s.scanRequestSummaries(ctx, query, creatorID, channelID)
}

// GetRequestsByPortalCustomerID gets requests for portal customer (by creator_portal_customer_id)
func (s *PortalService) GetRequestsByPortalCustomerID(ctx context.Context, portalCustomerID int, channelID int) ([]PortalRequestSummary, error) {
	query := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
			i.status_id, i.priority_id, i.created_at, i.updated_at,
			i.channel_id, i.request_type_id,
			w.name AS workspace_name,
			w.key AS workspace_key,
			rt.name AS request_type_name,
			rt.icon AS request_type_icon,
			rt.color AS request_type_color,
			(SELECT COUNT(*) FROM comments WHERE item_id = i.id AND (is_private = false OR is_private IS NULL)) AS comment_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN request_types rt ON i.request_type_id = rt.id
		WHERE i.creator_portal_customer_id = ? AND i.channel_id = ?
		ORDER BY i.created_at DESC
	`

	return s.scanRequestSummaries(ctx, query, portalCustomerID, channelID)
}

// scanRequestSummaries is a helper to scan request summary rows
func (s *PortalService) scanRequestSummaries(ctx context.Context, query string, args ...interface{}) ([]PortalRequestSummary, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch requests: %w", err)
	}
	defer rows.Close()

	var requests []PortalRequestSummary
	for rows.Next() {
		var req PortalRequestSummary
		var requestTypeName, requestTypeIcon, requestTypeColor sql.NullString
		err := rows.Scan(
			&req.ID, &req.WorkspaceID, &req.WorkspaceItemNumber, &req.Title, &req.Description,
			&req.Status, &req.Priority, &req.CreatedAt, &req.UpdatedAt,
			&req.ChannelID, &req.RequestTypeID,
			&req.WorkspaceName, &req.WorkspaceKey,
			&requestTypeName, &requestTypeIcon, &requestTypeColor,
			&req.CommentCount,
		)
		if err != nil {
			continue
		}

		if requestTypeName.Valid {
			req.RequestTypeName = &requestTypeName.String
		}
		if requestTypeIcon.Valid {
			req.RequestTypeIcon = &requestTypeIcon.String
		}
		if requestTypeColor.Valid {
			req.RequestTypeColor = &requestTypeColor.String
		}

		requests = append(requests, req)
	}

	if requests == nil {
		requests = []PortalRequestSummary{}
	}

	return requests, nil
}

// GetRequestDetail gets request detail with ownership info
func (s *PortalService) GetRequestDetail(ctx context.Context, itemID int) (*PortalRequestDetail, error) {
	query := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
			i.status_id, i.priority_id, i.created_at, i.updated_at,
			i.channel_id, i.request_type_id, i.creator_portal_customer_id, i.creator_id,
			w.name AS workspace_name,
			w.key AS workspace_key,
			rt.name AS request_type_name,
			rt.icon AS request_type_icon,
			rt.color AS request_type_color,
			(SELECT COUNT(*) FROM comments WHERE item_id = i.id AND (is_private = false OR is_private IS NULL)) AS comment_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN request_types rt ON i.request_type_id = rt.id
		WHERE i.id = ?
	`

	var detail PortalRequestDetail
	var requestTypeName, requestTypeIcon, requestTypeColor sql.NullString
	var creatorPortalCustomerID, creatorID sql.NullInt64

	err := s.db.QueryRowContext(ctx, query, itemID).Scan(
		&detail.ID, &detail.WorkspaceID, &detail.WorkspaceItemNumber, &detail.Title, &detail.Description,
		&detail.Status, &detail.Priority, &detail.CreatedAt, &detail.UpdatedAt,
		&detail.ChannelID, &detail.RequestTypeID, &creatorPortalCustomerID, &creatorID,
		&detail.WorkspaceName, &detail.WorkspaceKey,
		&requestTypeName, &requestTypeIcon, &requestTypeColor,
		&detail.CommentCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch request: %w", err)
	}

	if requestTypeName.Valid {
		detail.RequestTypeName = &requestTypeName.String
	}
	if requestTypeIcon.Valid {
		detail.RequestTypeIcon = &requestTypeIcon.String
	}
	if requestTypeColor.Valid {
		detail.RequestTypeColor = &requestTypeColor.String
	}
	if creatorPortalCustomerID.Valid {
		id := int(creatorPortalCustomerID.Int64)
		detail.CreatorPortalCustomerID = &id
	}
	if creatorID.Valid {
		id := int(creatorID.Int64)
		detail.CreatorID = &id
	}

	return &detail, nil
}

// VerifyRequestOwnership verifies that a user owns a request
// Returns true if the user owns the request within the specified channel
func (s *PortalService) VerifyRequestOwnership(ctx context.Context, itemID int, channelID int, internalUserID *int, portalCustomerID *int) (bool, error) {
	detail, err := s.GetRequestDetail(ctx, itemID)
	if err != nil {
		return false, err
	}
	if detail == nil {
		return false, nil
	}

	// Verify channel matches
	if detail.ChannelID == nil || *detail.ChannelID != channelID {
		return false, nil
	}

	// Check ownership based on auth type
	if internalUserID != nil && detail.CreatorID != nil && *detail.CreatorID == *internalUserID {
		return true, nil
	}
	if portalCustomerID != nil && detail.CreatorPortalCustomerID != nil && *detail.CreatorPortalCustomerID == *portalCustomerID {
		return true, nil
	}

	return false, nil
}

// GetRequestComments gets comments for a request (public only)
func (s *PortalService) GetRequestComments(ctx context.Context, itemID int) ([]PortalComment, error) {
	query := `
		SELECT
			c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.created_at, c.updated_at,
			COALESCE(u.first_name || ' ' || u.last_name, pc.name, 'Unknown') AS author_name,
			COALESCE(u.email, pc.email, '') AS author_email
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
			&comment.CreatedAt, &comment.UpdatedAt, &comment.AuthorName, &comment.AuthorEmail,
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
		comments = append(comments, comment)
	}

	if comments == nil {
		comments = []PortalComment{}
	}

	return comments, nil
}
