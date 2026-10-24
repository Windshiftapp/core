package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// ErrItemMergeConflict marks a merge request that cannot proceed because a
// duplicate already points at a different canonical, or the merge would fold
// a ticket into its own descendant. Handlers surface it as a conflict.
var ErrItemMergeConflict = errors.New("item merge conflict")

// lockCanonicalMergeTarget re-checks the canonical's merge identity inside
// the merge transaction and fences it against concurrent merges: PostgreSQL
// locks the row for the transaction; SQLite serializes through the write
// lock via a no-op conditional update. A canonical that became a merged
// duplicate in the meantime fails the merge instead of closing a redirect
// cycle (WI-1567).
func lockCanonicalMergeTarget(ctx context.Context, tx database.Tx, driver string, targetID int) error {
	if database.IsPostgresDriver(driver) {
		var id int
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM items WHERE id = ? AND merged_into_item_id IS NULL FOR UPDATE`, targetID,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: the canonical ticket became a merged duplicate", ErrItemMergeConflict)
		}
		return err
	}
	res, err := tx.Exec(`UPDATE items SET updated_at = updated_at WHERE id = ? AND merged_into_item_id IS NULL`, targetID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: the canonical ticket became a merged duplicate", ErrItemMergeConflict)
	}
	return nil
}

// ItemLifecycleEmitter receives post-commit side effects for lifecycle
// mutations. *EventCoordinator satisfies it.
type ItemLifecycleEmitter interface {
	EmitItemUpdated(original, updated *models.Item, statusChanged, assigneeChanged bool, actorUserID int, fieldChanges []repository.HistoryEntry, actorUsername ...string)
	EmitItemCreated(item *models.Item, actorUserID int, actorUsername ...string)
}

// ItemLifecycleService implements support-ticket lifecycle operations on the
// generic item model: merging duplicates into a canonical ticket and
// splitting a request into a subticket. Callers retain transport scope
// checks; this service owns workspace authorization, transactional moves,
// durable facts, and cache/live-view invalidation.
type ItemLifecycleService struct {
	db       database.Database
	perm     *PermissionService
	items    *repository.ItemRepository
	comments *CommentService
	emitter  ItemLifecycleEmitter
}

func NewItemLifecycleService(db database.Database, perm *PermissionService) *ItemLifecycleService {
	return &ItemLifecycleService{
		db:       db,
		perm:     perm,
		items:    repository.NewItemRepository(db),
		comments: NewCommentService(db),
	}
}

// SetEmitter wires post-commit side effects (notifications, webhooks,
// automation). Optional: lifecycle facts are durable without it.
func (s *ItemLifecycleService) SetEmitter(emitter ItemLifecycleEmitter) {
	s.emitter = emitter
}

// ItemMergeSourceResult reports what happened to one duplicate.
type ItemMergeSourceResult struct {
	SourceItemID     int  `json:"source_item_id"`
	TargetItemID     int  `json:"target_item_id"`
	Merged           bool `json:"merged"`
	MovedComments    int  `json:"moved_comments"`
	MovedAttachments int  `json:"moved_attachments"`
	MovedLinks       int  `json:"moved_links"`
	CommentsPrivate  bool `json:"comments_private"`
	// ContentKeptOnSource reports that files and email threads stayed on the
	// merged duplicate because the requesters differ.
	ContentKeptOnSource bool `json:"content_kept_on_source,omitempty"`
}

// ItemMergeResult summarizes one merge request.
type ItemMergeResult struct {
	Target  *models.Item            `json:"-"`
	Sources []ItemMergeSourceResult `json:"sources"`
}

// ItemMergeInput names the canonical ticket and the duplicates folded into
// it. Merging is all-or-nothing across the requested sources.
type ItemMergeInput struct {
	ActorUserID   int
	ActorUsername string
	TargetItemID  int
	SourceItemIDs []int
}

// Merge folds duplicate tickets into a canonical one. Comments, attachments,
// item links, watchers, email threading rows, and pending outbound replies
// move to the canonical; each duplicate keeps pointing at it via
// items.merged_into_item_id. Re-running a completed merge is a no-op for
// those sources; a duplicate already pointing elsewhere conflicts.
func (s *ItemLifecycleService) Merge(ctx context.Context, input ItemMergeInput) (*ItemMergeResult, error) {
	if len(input.SourceItemIDs) == 0 {
		return nil, &validation.ValidationError{Field: "source_item_ids", Message: "At least one duplicate item is required"}
	}

	target, err := s.items.FindByID(input.TargetItemID)
	if err != nil {
		return nil, err
	}
	if err := s.requireEdit(input.ActorUserID, target.WorkspaceID); err != nil {
		return nil, err
	}
	var targetMergedInto *int
	if err := s.db.QueryRowContext(ctx,
		`SELECT merged_into_item_id FROM items WHERE id = ?`, target.ID,
	).Scan(&targetMergedInto); err != nil {
		return nil, fmt.Errorf("load merge state for target %d: %w", target.ID, err)
	}
	if targetMergedInto != nil {
		return nil, fmt.Errorf("%w: the canonical ticket is itself a merged duplicate", ErrItemMergeConflict)
	}

	// Stable source order, no duplicates within the request.
	seen := make(map[int]bool, len(input.SourceItemIDs))
	ordered := make([]int, 0, len(input.SourceItemIDs))
	for _, id := range input.SourceItemIDs {
		if id == input.TargetItemID {
			return nil, &validation.ValidationError{Field: "source_item_ids", Message: "The canonical item cannot be merged into itself"}
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, id)
	}

	type plannedSource struct {
		item          *models.Item
		alreadyMerged bool
		sameRequester bool
	}

	sources := make([]plannedSource, 0, len(ordered))
	for _, id := range ordered {
		source, err := s.items.FindByID(id)
		if err != nil {
			return nil, err
		}
		if err := s.requireEdit(input.ActorUserID, source.WorkspaceID); err != nil {
			return nil, err
		}
		if source.WorkspaceID != target.WorkspaceID {
			return nil, &validation.ValidationError{Field: "source_item_ids", Message: "All items must be in the same workspace as the canonical ticket"}
		}
		// Folding a ticket into its own descendant would create a redirect
		// cycle. Follow the live parent_id chain: items.path is only written
		// by cross-workspace moves and template clones and stays '/' for
		// normally created items.
		targetIsDescendant, err := s.items.IsDescendantContext(ctx, id, input.TargetItemID)
		if err != nil {
			return nil, err
		}
		if targetIsDescendant {
			return nil, fmt.Errorf("%w: cannot merge an item into its own subticket", ErrItemMergeConflict)
		}
		plan := plannedSource{item: source, sameRequester: sameRequester(source, target)}
		var mergedInto *int
		if err := s.db.QueryRowContext(ctx,
			`SELECT merged_into_item_id FROM items WHERE id = ?`, id,
		).Scan(&mergedInto); err != nil {
			return nil, fmt.Errorf("load merge state for item %d: %w", id, err)
		}
		if mergedInto != nil {
			if *mergedInto != input.TargetItemID {
				return nil, fmt.Errorf("%w: item %d is already merged into a different ticket", ErrItemMergeConflict, id)
			}
			plan.alreadyMerged = true
		}
		if !plan.alreadyMerged {
			var children int
			if err := s.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM items WHERE parent_id = ?`, id,
			).Scan(&children); err != nil {
				return nil, fmt.Errorf("count subtickets of item %d: %w", id, err)
			}
			if children > 0 {
				return nil, &validation.ValidationError{Field: "source_item_ids", Message: fmt.Sprintf("Item %d still has subtickets; split or reparent them before merging", id)}
			}
		}
		sources = append(sources, plan)
	}

	result := &ItemMergeResult{Target: target, Sources: make([]ItemMergeSourceResult, 0, len(sources))}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		// Re-check the canonical's merge identity under a transaction fence so
		// concurrent opposite-direction merges cannot both succeed and leave
		// mutually redirecting tickets (WI-1567).
		if err := lockCanonicalMergeTarget(ctx, tx, s.db.GetDriverName(), target.ID); err != nil {
			return err
		}
		recorder := itemevents.NewRecorder(s.db)
		now := time.Now().UTC()
		for _, plan := range sources {
			out := ItemMergeSourceResult{
				SourceItemID: plan.item.ID,
				TargetItemID: target.ID,
				Merged:       !plan.alreadyMerged,
			}
			if plan.alreadyMerged {
				result.Sources = append(result.Sources, out)
				continue
			}

			var movedAttachments, movedLinks int64
			// Comments follow the canonical. When the requesters differ, moved
			// comments become private so the canonical's requester never sees
			// another customer's content in the portal; agents keep the full
			// thread with per-comment attribution.
			// thread with per-comment attribution.
			movedComments, err := s.comments.MoveCommentsToItem(tx, int64(plan.item.ID), int64(target.ID), !plan.sameRequester)
			if err != nil {
				return fmt.Errorf("move comments from item %d: %w", plan.item.ID, err)
			}
			out.CommentsPrivate = !plan.sameRequester && movedComments > 0
			if plan.sameRequester {
				// One requester: files and email threads follow the canonical.
				res, err := tx.Exec(`UPDATE attachments SET item_id = ? WHERE item_id = ? AND COALESCE(entity_type, 'item') = 'item'`, target.ID, plan.item.ID)
				if err != nil {
					return fmt.Errorf("move attachments from item %d: %w", plan.item.ID, err)
				}
				movedAttachments, _ = res.RowsAffected()
			} else {
				// Different requesters: files keep their original requester
				// provenance on the merged duplicate. There is no per-file
				// visibility flag, so moving customer files onto another
				// customer's request would expose them through portal reads and
				// downloads (WI-1566). Agents reach them through the source
				// ticket, which stays in place as a redirect.
				out.ContentKeptOnSource = true
			}

			// Re-point item links onto the canonical, dropping links that
			// would collapse into duplicates or self-links.
			links, err := s.sourceItemLinks(tx, plan.item.ID)
			if err != nil {
				return err
			}
			for _, link := range links {
				newSourceID, newTargetID := link.SourceID, link.TargetID
				if link.SourceType == "item" && link.SourceID == plan.item.ID {
					newSourceID = target.ID
				}
				if link.TargetType == "item" && link.TargetID == plan.item.ID {
					newTargetID = target.ID
				}
				if newSourceID == newTargetID && link.SourceType == link.TargetType {
					continue // self-link after the fold; drop it
				}
				if _, err := tx.Exec(`
					INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at)
					SELECT ?, ?, ?, ?, ?, created_by, created_at FROM item_links WHERE id = ?
					ON CONFLICT DO NOTHING
				`, link.LinkTypeID, link.SourceType, newSourceID, link.TargetType, newTargetID, link.ID); err != nil {
					return fmt.Errorf("re-point item link %d: %w", link.ID, err)
				}
				if _, err := tx.Exec(`DELETE FROM item_links WHERE id = ?`, link.ID); err != nil {
					return fmt.Errorf("drop old item link %d: %w", link.ID, err)
				}
				movedLinks++
			}

			// Watchers keep following the content: skip ones already watching
			// the canonical, move the rest.
			if _, err := tx.Exec(`
				DELETE FROM item_watches
				WHERE item_id = ? AND user_id IN (SELECT user_id FROM item_watches WHERE item_id = ?)
			`, plan.item.ID, target.ID); err != nil {
				return fmt.Errorf("deduplicate watchers for item %d: %w", plan.item.ID, err)
			}
			if _, err := tx.Exec(`UPDATE item_watches SET item_id = ? WHERE item_id = ?`, target.ID, plan.item.ID); err != nil {
				return fmt.Errorf("move watchers from item %d: %w", plan.item.ID, err)
			}

			// Email threads tracked on the duplicate append to the canonical so
			// it cannot accumulate independent replies — but only when both
			// tickets share a requester. Otherwise the original sender's
			// replies would land as public comments on another customer's
			// request; the thread stays with its requester on the merged
			// duplicate (WI-1566).
			if plan.sameRequester {
				if _, err := tx.Exec(`UPDATE email_message_tracking SET item_id = ? WHERE item_id = ?`, target.ID, plan.item.ID); err != nil {
					return fmt.Errorf("re-point email threads from item %d: %w", plan.item.ID, err)
				}
				// Intake receipts are one per item
				// (uq_email_reply_outbox_confirmation), so the canonical's own
				// receipt wins and the duplicate's extra one is dropped before
				// the move. Everything that remains then lives on the canonical,
				// so mail delivered after the merge tracks back to it.
				if _, err := tx.Exec(`
					DELETE FROM email_reply_outbox
					WHERE item_id = ? AND kind = 'intake_confirmation'
					  AND EXISTS (SELECT 1 FROM email_reply_outbox WHERE item_id = ? AND kind = 'intake_confirmation')
				`, plan.item.ID, target.ID); err != nil {
					return fmt.Errorf("drop duplicate intake receipt for item %d: %w", plan.item.ID, err)
				}
				if _, err := tx.Exec(`UPDATE email_reply_outbox SET item_id = ? WHERE item_id = ?`, target.ID, plan.item.ID); err != nil {
					return fmt.Errorf("re-point pending replies from item %d: %w", plan.item.ID, err)
				}
				// External participants follow the content too, or a source-only
				// participant loses canonical portal access and reply notifications.
				// Dedupe first: the unique (item_id, portal_customer_id) index
				// would otherwise reject a customer on both tickets.
				if _, err := tx.Exec(`
					DELETE FROM item_participants
					WHERE item_id = ? AND portal_customer_id IN (SELECT portal_customer_id FROM item_participants WHERE item_id = ?)
				`, plan.item.ID, target.ID); err != nil {
					return fmt.Errorf("deduplicate participants for item %d: %w", plan.item.ID, err)
				}
				if _, err := tx.Exec(`UPDATE item_participants SET item_id = ? WHERE item_id = ?`, target.ID, plan.item.ID); err != nil {
					return fmt.Errorf("move participants from item %d: %w", plan.item.ID, err)
				}
			}

			// The conditional update closes the race between the pre-checks and
			// this transaction: a concurrent merge of the same duplicate rolls
			// this one back instead of overwriting the pointer.
			res, err := tx.Exec(`
				UPDATE items SET merged_into_item_id = ?, updated_at = ?
				WHERE id = ? AND merged_into_item_id IS NULL
			`, target.ID, now, plan.item.ID)
			if err != nil {
				return fmt.Errorf("mark item %d merged: %w", plan.item.ID, err)
			}
			if rows, _ := res.RowsAffected(); rows == 0 {
				return fmt.Errorf("%w: item %d was merged concurrently", ErrItemMergeConflict, plan.item.ID)
			}

			if err := s.items.RecordHistory(tx, repository.HistoryEntry{
				ItemID: plan.item.ID, UserID: input.ActorUserID,
				FieldName: "merged_into_item_id", OldValueNull: true,
				NewValue: fmt.Sprintf("%d", target.ID), ChangedAt: now,
			}); err != nil {
				return err
			}
			// The canonical records its half of the fold so its own history
			// explains where the duplicate's content came from.
			if err := s.items.RecordHistory(tx, repository.HistoryEntry{
				ItemID: target.ID, UserID: input.ActorUserID,
				FieldName: "merged_item_id", OldValueNull: true,
				NewValue: fmt.Sprintf("%d", plan.item.ID), ChangedAt: now,
			}); err != nil {
				return err
			}

			// The duplicate is a redirect now: abandon its running SLA cycles so
			// armed warning/breach jobs stop firing for a ticket whose content
			// moved to the canonical (WI-1528).
			if _, err := tx.Exec(`
				UPDATE item_sla_cycles
				SET status = 'abandoned', abandon_reason = 'merged', stopped_at = ?,
				    paused = false, pause_started_at = NULL, next_deadline_at = NULL,
				    remaining_at_pause_ms = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE item_id = ? AND status = 'ongoing'
			`, now, plan.item.ID); err != nil {
				return fmt.Errorf("abandon SLA cycles for merged item %d: %w", plan.item.ID, err)
			}

			out.MovedComments = int(movedComments)
			out.MovedAttachments = int(movedAttachments)
			out.MovedLinks = int(movedLinks)
			result.Sources = append(result.Sources, out)

			if _, err := recorder.Merged(ctx, tx, target.WorkspaceID, itemevents.MergedV1{
				SourceItemID: plan.item.ID, TargetItemID: target.ID,
				MovedComments: out.MovedComments, MovedAttachments: out.MovedAttachments,
				MovedLinks: out.MovedLinks, CommentsPrivate: out.CommentsPrivate,
			}, itemevents.User(input.ActorUserID, "application")); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.invalidateAfterMutation(target.WorkspaceID, append(ordered, input.TargetItemID))
	if s.emitter != nil {
		if updated, err := s.items.FindByID(input.TargetItemID); err == nil && updated != nil {
			s.emitter.EmitItemUpdated(target, updated, false, false, input.ActorUserID, nil, input.ActorUsername)
		}
		// Emit for each duplicate too: its merged_into_item_id changed, and
		// automations/webhooks must learn the ticket was folded away instead
		// of only seeing the canonical (WI-1528).
		for _, id := range ordered {
			source, err := s.items.FindByID(id)
			if err != nil || source == nil {
				continue
			}
			s.emitter.EmitItemUpdated(source, source, false, false, input.ActorUserID, nil, input.ActorUsername)
		}
	}
	emitServiceAudit(s.db, AuditActor{UserID: input.ActorUserID, Username: input.ActorUsername},
		logger.ActionItemMerge, logger.ResourceItem, &input.TargetItemID, target.Title, map[string]any{
			"source_item_ids": input.SourceItemIDs,
			"sources":         len(result.Sources),
		})
	return result, nil
}

// ItemSplitInput carves a subticket out of a source ticket with explicit
// customer/agent ownership. ItemTypeID is optional: when nil the child type
// is derived from the source type's hierarchy.
type ItemSplitInput struct {
	ActorUserID      int
	ActorUsername    string
	SourceItemID     int
	Title            string
	Description      string
	ItemTypeID       *int
	CommentIDs       []int
	AttachmentIDs    []int
	AssigneeID       *int
	PortalCustomerID *int
}

// ItemSplitResult reports the created subticket.
type ItemSplitResult struct {
	Source           *models.Item `json:"-"`
	Split            *models.Item `json:"-"`
	SplitItemID      int          `json:"split_item_id"`
	MovedComments    int          `json:"moved_comments"`
	MovedAttachments int          `json:"moved_attachments"`
}

// resolveSplitChildType picks the item type for a split child. An explicit
// request is honored only when it is a legal child of the source type — the
// next hierarchy level down or the generic sub-task sentinel. When the caller
// omits a type, the first next-level type in catalog order is used, falling
// back to the generic sub-task. Copying the source type is deliberately not a
// fallback: a regular type may not parent its own level, so an Initiative's
// subticket is an Epic, never another Initiative.
func resolveSplitChildType(db database.Database, source *models.Item, requested *int) (*int, error) {
	if source.ItemTypeID == nil {
		return nil, &validation.ValidationError{Field: "item_type_id", Message: "Source item has no item type"}
	}
	// The source type may predate the workspace's config-set binding (for
	// example the global default), so resolve the hierarchy from the full
	// catalog and only constrain the child candidates to workspace-allowed
	// types.
	types, err := repository.NewItemTypeRepository(db).List(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve split child type: %w", err)
	}

	findType := func(id int) *models.ItemType {
		for i := range types {
			if types[i].ID == id {
				return &types[i]
			}
		}
		return nil
	}

	sourceType := findType(*source.ItemTypeID)
	if sourceType == nil {
		return nil, &validation.ValidationError{Field: "item_type_id", Message: "Source item type was not found"}
	}
	if sourceType.HierarchyLevel == models.HierarchyLevelGenericSubtask {
		return nil, &validation.ValidationError{Field: "item_type_id", Message: "Generic sub-tasks are terminal and cannot have child items"}
	}

	allowed := func(id int) bool {
		ok, err := IsItemTypeAllowedInWorkspace(db, source.WorkspaceID, id)
		return err == nil && ok
	}

	var nextLevel, generic *int
	for i := range types {
		switch types[i].HierarchyLevel {
		case sourceType.HierarchyLevel + 1:
			if nextLevel == nil && allowed(types[i].ID) {
				nextLevel = &types[i].ID
			}
		case models.HierarchyLevelGenericSubtask:
			if generic == nil && allowed(types[i].ID) {
				generic = &types[i].ID
			}
		}
	}

	if requested != nil {
		requestedType := findType(*requested)
		if requestedType == nil {
			return nil, &validation.ValidationError{Field: "item_type_id", Message: "Item type was not found"}
		}
		legal := requestedType.HierarchyLevel == sourceType.HierarchyLevel+1 ||
			requestedType.HierarchyLevel == models.HierarchyLevelGenericSubtask
		if !legal {
			return nil, &validation.ValidationError{Field: "item_type_id", Message: "Item type is not a valid child of the source item type"}
		}
		if !allowed(*requested) {
			return nil, &validation.ValidationError{Field: "item_type_id", Message: "Item type is not allowed in this workspace"}
		}
		return requested, nil
	}
	if nextLevel != nil {
		return nextLevel, nil
	}
	if generic != nil {
		return generic, nil
	}
	return nil, &validation.ValidationError{Field: "item_type_id", Message: "No child item type is available for this item type"}
}

// Split creates a child ticket of the source and moves exactly the selected
// comments and attachments into it. Ownership is explicit: the caller sets
// the agent (assignee) and, for customer-facing splits, the requester.
func (s *ItemLifecycleService) Split(ctx context.Context, input ItemSplitInput) (*ItemSplitResult, error) {
	source, err := s.items.FindByID(input.SourceItemID)
	if err != nil {
		return nil, err
	}
	if err := s.requireEdit(input.ActorUserID, source.WorkspaceID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Title) == "" {
		return nil, &validation.ValidationError{Field: "title", Message: "A title is required for the new subticket"}
	}
	if len(input.CommentIDs) == 0 && len(input.AttachmentIDs) == 0 {
		return nil, &validation.ValidationError{Field: "comment_ids", Message: "Select at least one comment or attachment to split out"}
	}
	childTypeID, err := resolveSplitChildType(s.db, source, input.ItemTypeID)
	if err != nil {
		return nil, err
	}

	// The moved counts are reported after commit, so they live outside the
	// AfterCreate hook. The child goes through the shared creation API; the
	// moves and the split fact commit with it in the same transaction.
	var movedComments, movedAttachments int64
	childID64, err := CreateItem(s.db, ItemCreationParams{
		WorkspaceID:             source.WorkspaceID,
		Title:                   strings.TrimSpace(input.Title),
		Description:             input.Description,
		ItemTypeID:              childTypeID,
		ParentID:                &source.ID,
		AssigneeID:              input.AssigneeID,
		CreatorID:               &input.ActorUserID,
		CreatorPortalCustomerID: input.PortalCustomerID,
		ChannelID:               source.ChannelID,
		SkipAssigneeTrigger:     true,
		SkipPublish:             true,
		EventMetadata:           itemEventMetadata(input.ActorUserID, "application", nil),
		AfterCreate: func(ctx context.Context, tx database.Tx, newChildID int) error {
			if len(input.CommentIDs) > 0 {
				var moved int64
				moved, err = s.comments.MoveCommentsByID(tx, int64(source.ID), int64(newChildID), int64s(input.CommentIDs))
				movedComments = moved
				if err != nil {
					return fmt.Errorf("move comments to split item: %w", err)
				}
				if int(movedComments) != len(input.CommentIDs) {
					return &validation.ValidationError{Field: "comment_ids", Message: "One or more comments do not belong to this item"}
				}
			}
			if len(input.AttachmentIDs) > 0 {
				query := fmt.Sprintf(`
					UPDATE attachments SET item_id = ? WHERE item_id = ? AND COALESCE(entity_type, 'item') = 'item' AND id IN (%s)
				`, placeholderList(input.AttachmentIDs))
				args := append([]any{newChildID, source.ID}, intSliceToAny(input.AttachmentIDs)...)
				res, err := tx.Exec(query, args...)
				if err != nil {
					return fmt.Errorf("move attachments to split item: %w", err)
				}
				movedAttachments, _ = res.RowsAffected()
				if int(movedAttachments) != len(input.AttachmentIDs) {
					return &validation.ValidationError{Field: "attachment_ids", Message: "One or more attachments do not belong to this item"}
				}
			}

			now := time.Now().UTC()
			if err := s.items.RecordHistory(tx, repository.HistoryEntry{
				ItemID: source.ID, UserID: input.ActorUserID,
				FieldName: "split_item_id", OldValueNull: true,
				NewValue: fmt.Sprintf("%d", newChildID), ChangedAt: now,
			}); err != nil {
				return err
			}

			_, err := itemevents.NewRecorder(s.db).Split(ctx, tx, source.WorkspaceID, itemevents.SplitV1{
				SourceItemID: source.ID, SplitItemID: newChildID,
				MovedComments: int(movedComments), MovedAttachments: int(movedAttachments),
				AssigneeID: input.AssigneeID, PortalCustomerID: input.PortalCustomerID,
			}, itemevents.User(input.ActorUserID, "application"))
			return err
		},
	})
	if err != nil {
		return nil, err
	}
	childID := int(childID64)

	child, err := s.items.FindByID(childID)
	if err != nil {
		return nil, err
	}
	source, err = s.items.FindByID(source.ID)
	if err != nil {
		return nil, err
	}

	PublishItemChange(childID)
	PublishItemChange(source.ID)
	PublishWorkspaceChange(source.WorkspaceID)
	repository.InvalidateItemListCountCache(s.db, source.WorkspaceID)
	if s.emitter != nil {
		s.emitter.EmitItemCreated(child, input.ActorUserID, input.ActorUsername)
		s.emitter.EmitItemUpdated(source, source, false, false, input.ActorUserID, nil, input.ActorUsername)
	}

	splitID := childID
	emitServiceAudit(s.db, AuditActor{UserID: input.ActorUserID, Username: input.ActorUsername},
		logger.ActionItemSplit, logger.ResourceItem, &splitID, input.Title, map[string]any{
			"source_item_id":    input.SourceItemID,
			"moved_comments":    movedComments,
			"moved_attachments": movedAttachments,
		})

	return &ItemSplitResult{
		Source: source, Split: child, SplitItemID: childID,
		MovedComments: int(movedComments), MovedAttachments: int(movedAttachments),
	}, nil
}

func (s *ItemLifecycleService) requireEdit(userID, workspaceID int) error {
	if s.perm == nil {
		return ErrItemForbidden
	}
	allowed, err := s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrItemForbidden
	}
	return nil
}

// ItemMergeRedirect names the canonical ticket a merged duplicate points at.
type ItemMergeRedirect struct {
	TargetItemID int `json:"target_item_id"`
}

// GetMergeRedirect returns the redirect for one item. View permission on the
// item is required; a missing or unmerged item reads as
// repository.ErrNotFound so callers can 404 without leaking existence.
func (s *ItemLifecycleService) GetMergeRedirect(ctx context.Context, userID, itemID int) (*ItemMergeRedirect, error) {
	item, err := s.items.FindByID(itemID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.perm.HasWorkspacePermission(userID, item.WorkspaceID, models.PermissionItemView)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, repository.ErrNotFound
	}
	var target *int
	if err := s.db.QueryRowContext(ctx,
		`SELECT merged_into_item_id FROM items WHERE id = ?`, itemID,
	).Scan(&target); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("load merge redirect for item %d: %w", itemID, err)
	}
	if target == nil {
		return nil, repository.ErrNotFound
	}
	return &ItemMergeRedirect{TargetItemID: *target}, nil
}

func sameRequester(a, b *models.Item) bool {
	if a.CreatorPortalCustomerID == nil && b.CreatorPortalCustomerID == nil {
		return true
	}
	if a.CreatorPortalCustomerID == nil || b.CreatorPortalCustomerID == nil {
		return false
	}
	return *a.CreatorPortalCustomerID == *b.CreatorPortalCustomerID
}

// sourceItemLinks lists item links attached to the given item on either side.
type sourceItemLinkRow struct {
	ID         int
	LinkTypeID int
	SourceType string
	SourceID   int
	TargetType string
	TargetID   int
}

func (s *ItemLifecycleService) sourceItemLinks(tx database.Tx, itemID int) ([]sourceItemLinkRow, error) {
	rows, err := tx.Query(`
		SELECT id, link_type_id, source_type, source_id, target_type, target_id
		FROM item_links
		WHERE (source_type = 'item' AND source_id = ?) OR (target_type = 'item' AND target_id = ?)
	`, itemID, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item links for item %d: %w", itemID, err)
	}
	defer rows.Close()

	var out []sourceItemLinkRow
	for rows.Next() {
		var row sourceItemLinkRow
		if err := rows.Scan(&row.ID, &row.LinkTypeID, &row.SourceType, &row.SourceID, &row.TargetType, &row.TargetID); err != nil {
			return nil, fmt.Errorf("scan item link: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *ItemLifecycleService) invalidateAfterMutation(workspaceID int, itemIDs []int) {
	repository.InvalidateItemListCountCache(s.db, workspaceID)
	PublishWorkspaceChange(workspaceID)
	for _, id := range itemIDs {
		PublishItemChange(id)
	}
}

// placeholderList renders an IN (...) placeholder list for the given arity.
func placeholderList(ids []int) string {
	parts := make([]string, len(ids))
	for i := range ids {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func intSliceToAny(ids []int) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// int64s widens the API-facing int slice for comment move calls.
func int64s(ids []int) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}
