package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/actionevents"
	"windshift/internal/events"
	"windshift/internal/itemevents"
)

const (
	DurableEmailReplyConsumerKey = "email_replies.comments.v1"
	durableEmailReplyCutoverKey  = "email_replies.comments.canonical.v1"
)

// PrepareDurableEmailReplyEngine installs the canonical comment consumer and
// disables the post-commit compatibility call at the same durable boundary.
func PrepareDurableEmailReplyEngine(ctx context.Context, engine *events.Engine, replies *EmailReplyService) error {
	if engine == nil || replies == nil {
		return errors.New("domain event engine and email reply service are required")
	}
	cutover, err := actionevents.CurrentCutover(ctx, replies.db, durableEmailReplyCutoverKey)
	if err != nil {
		return fmt.Errorf("load email reply cutover: %w", err)
	}
	if cutover == nil {
		cutover, err = actionevents.ActivateCutover(ctx, replies.db, durableEmailReplyCutoverKey, "comment email reply")
		if err != nil {
			return err
		}
	}
	if err := engine.Store().ConfigureConsumer(ctx, events.Consumer{
		Key: DurableEmailReplyConsumerKey, HandlerVersion: 1, Active: true,
		StartEventID: cutover.StartEventID, EventTypes: []string{itemevents.CommentCreated},
	}); err != nil {
		return err
	}
	if err := engine.RegisterHandler(DurableEmailReplyConsumerKey, &DurableEmailReplyConsumer{replies: replies}); err != nil {
		return err
	}
	replies.canonical = true
	return nil
}

// DurableEmailReplyConsumer enriches and renders a reply only after its
// committed comment fact has been admitted. The SMTP scheduler owns sending.
type DurableEmailReplyConsumer struct {
	replies *EmailReplyService
}

func (c *DurableEmailReplyConsumer) Handle(ctx context.Context, event events.Event) error {
	if event.Type != itemevents.CommentCreated || event.PayloadVersion != itemevents.PayloadVersion {
		return events.Permanent(fmt.Errorf("unsupported email reply event %s v%d", event.Type, event.PayloadVersion))
	}
	var payload itemevents.CommentCreatedV1
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return events.Permanent(fmt.Errorf("decode email reply comment event: %w", err))
	}
	if payload.IsPrivate || payload.PortalCustomerID != nil || payload.AuthorID == nil || *payload.AuthorID <= 0 || payload.SuppressSideEffects {
		return nil
	}
	var content string
	if err := c.replies.db.QueryRowContext(ctx, `
		SELECT content FROM comments WHERE id = ? AND item_id = ?
	`, payload.CommentID, payload.ItemID).Scan(&content); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("load durable email reply comment: %w", err)
	}
	return c.replies.handleCommentCreated(HandleCommentParams{
		CommentID: int(payload.CommentID), ItemID: payload.ItemID,
		AuthorID: *payload.AuthorID, Content: content,
	}, false)
}
