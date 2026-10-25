package planeimport

import (
	"context"
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

// fallbackStatusID is the status used when a source state maps to a name the
// workspace does not have: the workspace default status (In Progress in the
// default data), so an unmapped state never lands on a null status. Resolve()
// normally refuses to run with missing statuses; this is the belt for the
// post-resolve race where a status disappears mid-run.
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

// mapStateName maps a Plane state name to the Windshift status builtin key
// the run targets. Configured entries win; anything else lands on to_do and
// is named in the report, because "start the work somewhere" beats "fail the
// row" for a custom Plane workflow, and the mapping metadata preserves the
// real state for whoever reconciles later.
func (l *Loader) mapStateName(planeState string) string {
	if mapped, ok := l.opts.DefaultStatusMap[planeState]; ok {
		return mapped
	}
	return "to_do"
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

// buildItem assembles the models.Item the repository inserts. Every field the
// fidelity contract promises is set here explicitly: number, creator, status,
// dates, story points, and both timestamps.
func (l *Loader) buildItem(i *Issue, number int, parentWindshiftID int, existing map[string]map[string]int) (*models.Item, error) {
	creatorID := l.userID[i.CreatedByID]
	if _, configured := l.opts.DefaultStatusMap[i.StateName]; !configured {
		key := "unmapped-state:" + i.StateName
		if _, seen := l.prioSeen[key]; !seen {
			l.prioSeen[key] = struct{}{}
			l.report.UnmappedStates = append(l.report.UnmappedStates, "state "+i.StateName)
		}
	}
	statusKey := l.mapStateName(i.StateName)
	statusID, ok := l.statusID[statusKey]
	if !ok {
		key := "unmapped-state:" + i.StateName
		if _, seen := l.prioSeen[key]; !seen {
			l.prioSeen[key] = struct{}{}
			l.report.UnmappedStates = append(l.report.UnmappedStates, "state "+i.StateName)
		}
		statusID = l.fallbackStatusID()
	}
	var priorityID *int
	if id, ok := l.prioID[i.Priority]; ok {
		priorityID = &id
	} else if i.Priority != "" && i.Priority != "none" {
		key := "unmapped:" + i.Priority
		if _, seen := l.prioSeen[key]; !seen {
			l.prioSeen[key] = struct{}{}
			l.report.UnmappedStates = append(l.report.UnmappedStates, "priority "+i.Priority)
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
	return item, nil
}

// dryRunBatch validates one batch without writing: mapping collisions, taken
// numbers, state/user resolution. It mutates the same bookkeeping maps the
// write path uses so the report is identical in shape.
func (l *Loader) dryRunBatch(batch []*Issue, commentsByIssue map[string][]*Comment, existing map[string]map[string]int, takenNumbers map[int]string, numberToPlaneID map[int]string) error {
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
		if _, ok := l.userID[i.CreatedByID]; !ok {
			// resolveUser already recorded the uuid in the report; the creator
			// column stays NULL and honest about it.
		}
		l.report.IssuesImported++
		if n := i.SequenceID; n > l.report.MaxNumber {
			l.report.MaxNumber = n
		}
		for _, c := range commentsByIssue[i.ID] {
			if c.DeletedAt != nil {
				continue
			}
			l.report.CommentsImported++
		}
	}
	return nil
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
			// Every Plane parent has a lower sequence_id than its child, so the
			// parent is either already mapped from a previous run or was inserted
			// earlier in this same pass; numberToPlaneID tracks the latter.
			if mapped, ok := existing["issue"][*i.ParentID]; ok {
				parentID = mapped
			} else if owner := numberToPlaneID[i.ParentSequence]; owner != "" && owner == *i.ParentID {
				parentID = existing["issue"][owner]
			} else if i.ParentSequence > 0 {
				l.report.Collisions = append(l.report.Collisions,
					fmt.Sprintf("issue %d parent %s not imported before the child", i.SequenceID, *i.ParentID))
			}
		}
		item, err := l.buildItem(i, i.SequenceID, parentID, existing)
		if err != nil {
			return err
		}
		id, err := repo.Create(tx, item)
		if err != nil {
			return fmt.Errorf("insert plane issue %s (number %d): %w", i.ID, i.SequenceID, err)
		}
		item.ID = id
		if err := l.recordMapping(tx, "issue", i.ID, id, map[string]any{
			"plane_state": i.StateName, "plane_priority": i.Priority,
			"sequence_id": i.SequenceID,
		}); err != nil {
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

// insertComment writes one comment row with the contract's author and
// timestamps, records its idempotency mapping, and counts it.
func (l *Loader) insertComment(tx database.Tx, existing map[string]map[string]int, itemID int, c *Comment) error {
	if _, done := existing["comment"][c.ID]; done {
		l.report.CommentsExisting++
		return nil
	}
	content := c.Text
	if md, ok := ConvertDescriptionJSON(c.DeltaJSON); ok && md != "" {
		content = md
	} else if strings.TrimSpace(content) == "" {
		content = ConvertDescriptionFromHTML(c.DeltaHTML)
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
// keeps the source state name so a cancelled Plane ticket remains visibly
// cancelled even though Windshift files it under Done.
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
