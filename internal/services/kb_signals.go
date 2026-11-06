package services

import (
	"context"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// KB event types recorded in kb_events. They are the raw usage signals for
// the portal knowledge base; keep the set aligned with the table's CHECK
// constraint.
const (
	KBEventSearch     = "search"
	KBEventNoResult   = "no_result"
	KBEventView       = "view"
	KBEventDeflection = "deflection"
)

// KB view sources. Views coming from a ticket conversation are assisted
// reads; search/browse views are self-service moments eligible for the
// deflection signal.
const (
	KBViewSourceSearch = "search"
	KBViewSourceBrowse = "browse"
	KBViewSourceTicket = "ticket"
)

// KBSignalService records knowledge-base usage events. Insert-only by
// design: rows carry IDs and the submitted query — never page content —
// so unauthorized bodies can never leak through analytics.
type KBSignalService struct {
	db database.Database
}

// NewKBSignalService builds the signal recorder.
func NewKBSignalService(db database.Database) *KBSignalService {
	return &KBSignalService{db: db}
}

// KBEventInput describes one usage event. All fields except ChannelID and
// EventType are optional context.
type KBEventInput struct {
	ChannelID        int
	PortalCustomerID *int
	EventType        string
	PageID           *int
	WorkspaceID      *int
	Source           string
	Query            string
}

// RecordKBEvent persists one event. Failures are logged by callers, never
// surfaced to the portal: analytics must not break the user-facing path.
func (s *KBSignalService) RecordKBEvent(ctx context.Context, in KBEventInput) error {
	query := strings.TrimSpace(in.Query)
	if len(query) > 256 {
		query = query[:256]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kb_events (channel_id, portal_customer_id, event_type, page_id, workspace_id, source, query)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.ChannelID, in.PortalCustomerID, in.EventType, in.PageID, in.WorkspaceID, in.Source, query,
	)
	if err != nil {
		return fmt.Errorf("insert kb_event: %w", err)
	}
	return nil
}

// PortalCustomerHasOpenRequests reports whether the customer has at least one
// request on the portal that has not reached a completed status category.
// An "open" request means the customer may still be waiting on staff, so an
// article view in that state is not treated as self-service deflection. The
// request scope mirrors the portal: its own channel plus linked intakes.
func (s *KBSignalService) PortalCustomerHasOpenRequests(ctx context.Context, channelID, portalCustomerID int) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM items i
		 JOIN statuses s ON s.id = i.status_id
		 JOIN status_categories sc ON sc.id = s.category_id
		 WHERE `+repository.PortalItemScopeSQL("i")+` AND i.creator_portal_customer_id = ?
		   AND COALESCE(sc.is_completed, false) = false`,
		channelID, channelID, portalCustomerID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count open portal requests: %w", err)
	}
	return count > 0, nil
}
