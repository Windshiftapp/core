package planeimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// fallbackStatusID is the status used when a source state has no status row
// at write time — the belt for the post-resolve race where a status someone
// created is deleted mid-run by hand. Resolve's vocabulary pass normally
// guarantees every contract state has a row; this is what keeps a row from
// landing on a null status when that guarantee is broken underneath it.
func (l *Loader) fallbackStatusID() int {
	var id int
	if err := l.db.QueryRow(`SELECT id FROM statuses WHERE is_default = true ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		return 0
	}
	return id
}

// loadExistingMappings reads the idempotency map for this workspace: Plane
// uuid -> Windshift item id, and Plane comment uuid -> Windshift comment id.
// Rows are keyed by (workspace, entity_type, external_id) so reruns after a
// partial failure add only what is missing.
func (l *Loader) loadExistingMappings() (map[string]map[string]int, error) {
	out := map[string]map[string]int{"issue": {}, "comment": {}}
	rows, err := l.db.Query(`SELECT entity_type, external_id, windshift_id FROM plane_import_mappings WHERE workspace_id = ?`, l.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load plane import mappings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var entity, externalID string
		var windshiftID int
		if err := rows.Scan(&entity, &externalID, &windshiftID); err != nil {
			return nil, fmt.Errorf("scan plane import mapping: %w", err)
		}
		if bucket, ok := out[entity]; ok {
			bucket[externalID] = windshiftID
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read plane import mappings: %w", err)
	}
	return out, nil
}

// loadTakenNumbers returns the set of item numbers present in the workspace.
// A source number that is taken by a row the importer does not own (no mapping
// row) is a collision: importing would corrupt number parity, so the run
// refuses it loudly instead of silently renumbering.
func (l *Loader) loadTakenNumbers() (map[int]string, error) {
	out := map[int]string{}
	rows, err := l.db.Query(`SELECT workspace_item_number FROM items WHERE workspace_id = ?`, l.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load taken item numbers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan taken item number: %w", err)
		}
		out[n] = ""
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read taken item numbers: %w", err)
	}
	return out, nil
}

// rowQuerier is satisfied by both the database handle and an open
// transaction: the read half of convergence runs through whichever is in hand.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// writer is the write half: a transaction in the live pass, nil on a dry run
// (the comparison and the counts are still computed, nothing is written).
type writer interface {
	ExecWrite(query string, args ...any) (sql.Result, error)
}

// convergeIssue moves an already-imported row onto what the contract says it
// should be — status per state, priority per source rung, labels linked. It
// only touches rows nobody edited since the import: `updated_at == created_at`
// is the proof, because every product write moves updated_at. A row the
// operator has since edited is their decision, and the import defers to it
// (counted in the report, not silently overwritten).
func (l *Loader) convergeIssue(i *Issue, itemID int, q rowQuerier, w writer) error {
	var curStatus, curPrio sql.NullInt64
	var updatedAt time.Time
	if err := q.QueryRow(`SELECT status_id, priority_id, updated_at FROM items WHERE id = ?`, itemID).Scan(&curStatus, &curPrio, &updatedAt); err != nil {
		return fmt.Errorf("read item %d for convergence: %w", itemID, err)
	}
	// The proof of "nobody edited since import": the import wrote the
	// contract's updated_at into the row, and every product write moves
	// updated_at. Equality against the CONTRACT value (not against
	// created_at) is what distinguishes "Plane edited it before the import"
	// — which the import faithfully carried over — from "someone edited it in
	// Windshift after the import" — which the import must defer to.
	if !updatedAt.Equal(i.UpdatedAt) {
		l.report.IssuesLeftEdited++
		return nil
	}
	wantStatus, known := l.statusID[i.StateName]
	if !known {
		l.reportUnmappedState(i.StateName)
	}
	var wantPriority *int
	if id, ok := l.prioID[i.Priority]; ok && i.Priority != "" && i.Priority != "none" {
		wantPriority = &id
	}
	if i.Priority != "" && i.Priority != "none" && wantPriority == nil {
		// A priority the destination cannot name: the row keeps whatever it
		// has and the name reaches the report — dedup'd with the insert
		// path's lines so one value produces one line per run.
		if _, seen := l.prioSeen[i.Priority]; !seen {
			l.prioSeen[i.Priority] = struct{}{}
			l.report.UnmappedPriorities = append(l.report.UnmappedPriorities, i.Priority)
		}
	}
	statusDiffers := known && (!curStatus.Valid || curStatus.Int64 != int64(wantStatus))
	priorityDiffers := (wantPriority == nil) != !curPrio.Valid ||
		(wantPriority != nil && curPrio.Valid && curPrio.Int64 != int64(*wantPriority))
	if statusDiffers || priorityDiffers {
		if w != nil {
			var newStatus any
			if curStatus.Valid {
				newStatus = curStatus.Int64
			}
			if known {
				newStatus = wantStatus
			}
			newPriority := any(nil)
			if wantPriority != nil {
				newPriority = *wantPriority
			}
			if _, err := w.ExecWrite(`UPDATE items SET status_id = ?, priority_id = ? WHERE id = ?`, newStatus, newPriority, itemID); err != nil {
				return fmt.Errorf("converge item %d to contract state %q: %w", itemID, i.StateName, err)
			}
		}
		l.report.IssuesConverged++
	}
	return l.attachIssueLabels(i.LabelIDs, itemID, q, w)
}

// attachIssueLabels links every label the source applied and no link carries
// yet. Existing links stay untouched — someone else's label on the row is
// their call — and labels the contract never defined are reported, not
// invented.
func (l *Loader) attachIssueLabels(planeIDs []string, itemID int, q rowQuerier, w writer) error {
	for _, planeID := range planeIDs {
		labelID, ok := l.labelByPlane[planeID]
		if !ok {
			l.report.Collisions = append(l.report.Collisions,
				fmt.Sprintf("item %d references source label %s the contract never carried", itemID, planeID))
			continue
		}
		var have int
		err := q.QueryRow(`SELECT 1 FROM item_labels WHERE item_id = ? AND label_id = ?`, itemID, labelID).Scan(&have)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check label %d on item %d: %w", labelID, itemID, err)
		}
		if w != nil {
			if _, err := w.ExecWrite(`INSERT INTO item_labels (item_id, label_id) VALUES (?, ?)`, itemID, labelID); err != nil {
				return fmt.Errorf("link label %d to item %d: %w", labelID, itemID, err)
			}
		}
		l.report.LabelLinksAdded++
	}
	return nil
}

// labelNamesFor lists the destination label names a source issue's label ids
// resolve to, for the mapping metadata. Unknown ids are carried verbatim —
// provenance records what the source said, not what could be resolved.
func (l *Loader) labelNamesFor(planeIDs []string) []string {
	if len(planeIDs) == 0 {
		return nil
	}
	out := make([]string, 0, len(planeIDs))
	for _, id := range planeIDs {
		if name, ok := l.labelNameByPlane[id]; ok {
			out = append(out, name)
			continue
		}
		out = append(out, "unresolved:"+id)
	}
	return out
}

// issueDescription converts one issue body, falling back to Plane's HTML
// render when the TipTap document is absent or unparseable. The fallback
// count lands in the report — an honest importer says which bodies it could
// not convert, not just that it converted everything.
func (l *Loader) issueDescription(i *Issue) string {
	if md, ok := ConvertDescriptionJSON(i.DescriptionJSON); ok && md != "" {
		return md
	}
	if strings.TrimSpace(i.DescriptionJSON) == "" && strings.TrimSpace(i.DescriptionHTML) == "" {
		return ""
	}
	l.report.HTMLFallbacks++
	return ConvertDescriptionFromHTML(i.DescriptionHTML)
}

// reportUnmappedState records a Plane state name the status map does not cover,
// once per name. It dedups on its own map: sharing the priority map would let
// a state named like a priority key (or the reverse) silence a report line,
// and this tool's whole promise is that nothing is dropped quietly.
func (l *Loader) reportUnmappedState(name string) {
	if _, seen := l.stateSeen[name]; seen {
		return
	}
	l.stateSeen[name] = struct{}{}
	l.report.UnmappedStates = append(l.report.UnmappedStates, name)
}

// buildItem assembles the models.Item the repository inserts. Every field the
// fidelity contract promises is set here explicitly: number, creator, status,
// dates, story points, and both timestamps. It cannot fail: an unmapped status
// or priority lands on the fallback with a report line, never on an error.
func (l *Loader) buildItem(i *Issue, number, parentWindshiftID int) *models.Item {
	creatorID := l.userID[i.CreatedByID]
	statusID, ok := l.statusID[i.StateName]
	if !ok {
		l.reportUnmappedState(i.StateName)
		statusID = l.fallbackStatusID()
	}
	var priorityID *int
	if id, ok := l.prioID[i.Priority]; ok {
		priorityID = &id
	} else if i.Priority != "" && i.Priority != "none" {
		if _, seen := l.prioSeen[i.Priority]; !seen {
			l.prioSeen[i.Priority] = struct{}{}
			l.report.UnmappedPriorities = append(l.report.UnmappedPriorities, i.Priority)
		}
	}
	item := &models.Item{
		WorkspaceID:         l.workspaceID,
		WorkspaceItemNumber: number,
		Title:               i.Name,
		Description:         l.issueDescription(i),
		StatusID:            &statusID,
		PriorityID:          priorityID,
		CreatorID:           ptrInt(creatorID),
		ReporterID:          ptrInt(creatorID),
		ParentID:            ptrInt(parentWindshiftID),
		DueDate:             cloneTime(i.TargetDate),
		StartDate:           cloneTime(i.StartDate),
		StoryPoints:         storyPointsFromEstimate(i.EstimatePoint),
		CreatedAt:           i.CreatedAt,
		UpdatedAt:           i.UpdatedAt,
	}
	builtin := "task"
	if parentWindshiftID != 0 {
		builtin = "subtask"
	}
	if id, ok := l.typeID[builtin]; ok {
		item.ItemTypeID = &id
	}
	return item
}

// dryRunBatch validates one batch without writing: mapping collisions, taken
// numbers, state/user resolution. It mutates the same bookkeeping maps the
// write path uses so the report is identical in shape.
func (l *Loader) dryRunBatch(batch []*Issue, commentsByIssue map[string][]*Comment, existing map[string]map[string]int, takenNumbers, numberToPlaneID map[int]string) {
	for _, i := range batch {
		switch {
		case i.DeletedAt != nil:
			l.report.IssuesSoftDeleted++
			continue
		case i.IsDraft:
			l.report.IssuesDraft++
			continue
		}
		_, alreadyMapped := existing["issue"][i.ID]
		if alreadyMapped {
			l.report.IssuesExisting++
			// Dry-run convergence: read-only comparison, same counters the
			// write pass would produce. A read error here is a broken handle
			// the write pass will hit loudly too.
			_ = l.convergeIssue(i, existing["issue"][i.ID], l.db, nil)
			// Comments of an already-imported issue are re-skipped on a rerun;
			// count them as existing so the dry-run mirrors what Run would do.
			for _, c := range commentsByIssue[i.ID] {
				if c.DeletedAt != nil {
					continue
				}
				if _, done := existing["comment"][c.ID]; done {
					l.report.CommentsExisting++
				} else {
					l.report.CommentsImported++
				}
			}
			continue
		}
		if owner, taken := takenNumbers[i.SequenceID]; taken && owner != i.ID {
			l.report.Collisions = append(l.report.Collisions,
				fmt.Sprintf("number %d already used (plane issue %s claims it)", i.SequenceID, i.ID))
			continue
		}
		takenNumbers[i.SequenceID] = i.ID
		numberToPlaneID[i.SequenceID] = i.ID
		// An author Resolve() could not map is already named in the report;
		// the creator column stays NULL rather than getting an invented user.
		l.report.IssuesImported++
		if n := i.SequenceID; n > l.report.MaxNumber {
			l.report.MaxNumber = n
		}
		_ = l.attachIssueLabels(i.LabelIDs, i.SequenceID, l.db, nil)
		for _, c := range commentsByIssue[i.ID] {
			if c.DeletedAt != nil {
				continue
			}
			l.report.CommentsImported++
		}
	}
}

// runBatch imports one batch inside a single transaction. A batch is the
// durability unit: a crash replays the batch cleanly because every write in
// it is keyed on source uuids that a rerun checks before inserting.
func (l *Loader) runBatch(
	ctx context.Context,
	batch []*Issue,
	commentsByIssue map[string][]*Comment,
	existing map[string]map[string]int,
	takenNumbers map[int]string,
	numberToPlaneID map[int]string,
	repo *repository.ItemRepository,
	recorder *itemevents.Recorder,
) error {
	tx, err := l.db.Begin()
	if err != nil {
		return fmt.Errorf("begin import batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var records []itemevents.CreateRecord
	for _, i := range batch {
		switch {
		case i.DeletedAt != nil:
			l.report.IssuesSoftDeleted++
			continue
		case i.IsDraft:
			l.report.IssuesDraft++
			continue
		}
		if _, done := existing["issue"][i.ID]; done {
			l.report.IssuesExisting++
			// A mapped row is not automatically a correct row: an import from
			// an earlier build of this tool (or an interrupted run) can leave
			// it on the collapsed status. Convergence repairs untouched rows
			// and defers to edited ones.
			if err := l.convergeIssue(i, existing["issue"][i.ID], tx, tx); err != nil {
				return err
			}
			// An already-imported issue's comments still need counting: on a
			// partial-replay rerun a mapped issue can own comments the previous
			// run never reached.
			for _, c := range commentsByIssue[i.ID] {
				if c.DeletedAt != nil {
					continue
				}
				if _, cDone := existing["comment"][c.ID]; cDone {
					l.report.CommentsExisting++
				} else {
					if err := l.insertComment(tx, existing, idOf(existing, i.ID), c); err != nil {
						return err
					}
				}
			}
			continue
		}
		if owner, taken := takenNumbers[i.SequenceID]; taken && owner != i.ID {
			return fmt.Errorf("collision: number %d already used (plane issue %s claims it) — aborting batch", i.SequenceID, i.ID)
		}
		parentID := 0
		if i.ParentID != nil {
			// Usual case: Plane parents carry a lower sequence_id, so they are
			// already mapped (previous run) or already inserted (this pass).
			// Measured exceptions exist (a parent newer than its child), so an
			// unresolvable parent is DEFERRED to the after-batch parent pass
			// rather than dropped or aborted.
			if mapped, ok := existing["issue"][*i.ParentID]; ok {
				parentID = mapped
			} else if owner := numberToPlaneID[i.ParentSequence]; owner != "" && owner == *i.ParentID {
				parentID = existing["issue"][owner]
			} else {
				l.deferred = append(l.deferred, planeID{i.ID, *i.ParentID})
			}
		}
		item := l.buildItem(i, i.SequenceID, parentID)
		id, err := repo.Create(tx, item)
		if err != nil {
			return fmt.Errorf("insert plane issue %s (number %d): %w", i.ID, i.SequenceID, err)
		}
		item.ID = id
		if err := l.recordMapping(tx, "issue", i.ID, id, map[string]any{
			"plane_state": i.StateName, "plane_priority": i.Priority,
			"sequence_id": i.SequenceID, "labels": l.labelNamesFor(i.LabelIDs),
		}); err != nil {
			return err
		}
		if err := l.attachIssueLabels(i.LabelIDs, id, tx, tx); err != nil {
			return err
		}
		existing["issue"][i.ID] = id
		takenNumbers[i.SequenceID] = i.ID
		numberToPlaneID[i.SequenceID] = i.ID
		l.report.IssuesImported++
		if i.SequenceID > l.report.MaxNumber {
			l.report.MaxNumber = i.SequenceID
		}

		metadata := itemevents.Import("plane")
		metadata.OccurredAt = i.CreatedAt // the created fact carries the source chronology
		records = append(records, itemevents.CreateRecord{
			Item: item, Metadata: metadata,
		})

		// Comments for this issue land in the same transaction.
		for _, c := range commentsByIssue[i.ID] {
			if c.DeletedAt != nil {
				continue
			}
			if err := l.insertComment(tx, existing, id, c); err != nil {
				return err
			}
		}
	}

	if len(records) > 0 {
		if _, err := recorder.CreatedBatch(ctx, tx, records); err != nil {
			return fmt.Errorf("record import events: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import batch: %w", err)
	}
	return nil
}

func idOf(existing map[string]map[string]int, planeID string) int {
	return existing["issue"][planeID]
}

// parentPass attaches parent links that the in-pass resolution could not
// reach — the deferred pairs. Parents are resolved through the same mapping
// map, so a rerun that re-imported the parent also fixes the child.
func (l *Loader) parentPass(existing map[string]map[string]int) error {
	for _, pair := range l.deferred {
		childID, ok := existing["issue"][pair.child]
		if !ok {
			continue // child was not imported by this workspace/run — nothing to link
		}
		parentID, ok := existing["issue"][pair.parent]
		if !ok {
			l.report.Collisions = append(l.report.Collisions,
				fmt.Sprintf("issue %s has parent %s which the contract never carried", pair.child, pair.parent))
			continue
		}
		tx, err := l.db.Begin()
		if err != nil {
			return fmt.Errorf("begin parent pass: %w", err)
		}
		// A targeted parent_id write, not repo.Update: the repository's
		// Update stamps updated_at with import time, and this tool promises
		// un-stamped history — a linked child would otherwise read as "edited
		// at import" and every later convergence run would defer to a Windshift
		// edit that never happened.
		if _, err := tx.ExecWrite(`UPDATE items SET parent_id = ? WHERE id = ?`, parentID, childID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("link child %d to parent %d: %w", childID, parentID, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit parent pass: %w", err)
		}
		l.report.ParentsLinkedLate++
	}
	return nil
}

// insertComment writes one comment row with the contract's author and
// timestamps, records its idempotency mapping, and counts it.
func (l *Loader) insertComment(tx database.Tx, existing map[string]map[string]int, itemID int, c *Comment) error {
	if _, done := existing["comment"][c.ID]; done {
		l.report.CommentsExisting++
		return nil
	}
	content := c.Text
	if md, ok := ConvertDescriptionJSON(c.CommentJSON); ok && md != "" {
		content = md
	} else if strings.TrimSpace(content) == "" {
		content = ConvertDescriptionFromHTML(c.CommentHTML)
	}
	if content == "" {
		return nil
	}
	var authorID any
	if id, ok := l.userID[c.CreatedByID]; ok {
		authorID = id
	}
	var commentID int
	if err := tx.QueryRow(`
		INSERT INTO comments (item_id, author_id, content, is_private, created_at, updated_at)
		VALUES (?, ?, ?, false, ?, ?) RETURNING id`,
		itemID, authorID, content, c.CreatedAt, c.UpdatedAt).Scan(&commentID); err != nil {
		return fmt.Errorf("insert plane comment %s: %w", c.ID, err)
	}
	if err := l.recordMapping(tx, "comment", c.ID, commentID, map[string]any{"plane_issue_present": true}); err != nil {
		return err
	}
	existing["comment"][c.ID] = commentID
	l.report.CommentsImported++
	return nil
}

// recordMapping writes one idempotency/provenance row. The metadata_json
// keeps the source state name, so a ticket the source closed by abandoning it
// stays visibly that way even though Windshift files it under Done.
func (l *Loader) recordMapping(tx database.Tx, entity, externalID string, windshiftID int, metadata any) error {
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal mapping metadata: %w", err)
	}
	_, err = tx.ExecWrite(`
		INSERT INTO plane_import_mappings (workspace_id, entity_type, external_id, windshift_id, metadata_json)
		VALUES (?, ?, ?, ?, ?)`,
		l.workspaceID, entity, externalID, windshiftID, string(metaJSON))
	if err != nil {
		return fmt.Errorf("record %s mapping %s: %w", entity, externalID, err)
	}
	return nil
}

// finalizeReport computes the preserved gap list from the taken numbers so
// the operator can see, at a glance, exactly which numbers the import left
// reserved and why (soft-deletes stay in the contract's counts).
func (l *Loader) finalizeReport(takenNumbers map[int]string) {
	if l.report.MaxNumber == 0 {
		return
	}
	var gaps []int
	for n := 1; n <= l.report.MaxNumber; n++ {
		if _, ok := takenNumbers[n]; !ok {
			gaps = append(gaps, n)
		}
	}
	l.report.NumberGaps = gaps
	if l.report.UnmappedUsers == nil {
		l.report.UnmappedUsers = []string{}
	}
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ptrInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

// storyPointsFromEstimate maps Plane's estimate_point value (a numeric string
// on its Fibonacci scale) to Windshift story points. A null estimate stays
// null; an unparseable value is dropped rather than invented.
func storyPointsFromEstimate(raw *string) *float64 {
	if raw == nil {
		return nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return nil
	}
	return &v
}
