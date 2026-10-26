package planeimport

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/repository"
)

// Options configures one import run.
type Options struct {
	// WorkspaceKey is the destination workspace key (e.g. "VECTIS").
	WorkspaceKey string
	// SourceSlug, when set, must equal the contract meta's workspace_slug —
	// a cheap guard against pointing the wrong dump file at a workspace.
	SourceSlug string
	// ContractPath is the JSONL file produced by the dumper.
	ContractPath string
	// UsersPath is the users sidecar (JSON array of UserRow). Optional.
	UsersPath string
	// Aliases maps Plane email -> Windshift email before user resolution.
	// Without an alias, a Plane author whose email differs from their
	// Windshift email cannot be matched and is reported unmapped.
	Aliases map[string]string
	// Strict fails the run when an author cannot be resolved, instead of
	// leaving creator_id NULL and counting the row in the report.
	Strict bool
	// DryRun parses, maps, and reports without writing anything.
	DryRun bool
	// BatchSize is the number of issues committed per transaction.
	BatchSize int
	// DefaultStatusMap maps Plane state names to Windshift status names.
	// Empty selects DefaultPlaneStatusMap.
	DefaultStatusMap map[string]string
}

// Report is the reconciliation output of one run. Every count is measured
// during the run; zero means "none seen", not "not checked".
type Report struct {
	WorkspaceKey       string    `json:"workspace_key"`
	DryRun             bool      `json:"dry_run"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	IssuesImported     int       `json:"issues_imported"`
	IssuesExisting     int       `json:"issues_skipped_existing"`
	IssuesSoftDeleted  int       `json:"issues_skipped_soft_deleted"`
	IssuesDraft        int       `json:"issues_skipped_draft"`
	CommentsImported   int       `json:"comments_imported"`
	CommentsExisting   int       `json:"comments_skipped_existing"`
	CommentsSkipped    int       `json:"comments_skipped_unmapped_issue"`
	ParentsLinkedLate  int       `json:"parents_linked_in_second_pass"`
	HTMLFallbacks      int       `json:"descriptions_html_fallback"`
	UnmappedUsers      []string  `json:"unmapped_users,omitempty"`
	MissingStatuses    []string  `json:"missing_statuses,omitempty"`
	UnmappedStates     []string  `json:"unmapped_state_names,omitempty"`
	UnmappedPriorities []string  `json:"unmapped_priorities,omitempty"`
	Collisions         []string  `json:"collisions,omitempty"`
	NumberGaps         []int     `json:"number_gaps_preserved,omitempty"`
	MaxNumber          int       `json:"max_number_after_import"`
	Batches            int       `json:"batches"`
}

// DefaultPlaneStatusMap maps Plane state names to Windshift status builtin
// keys. Cancelled maps to done: Windshift's default workflow has no cancelled
// category, and the report keeps the source state on every row via
// plane_import_mappings.metadata_json, so the distinction survives where it
// matters. Keys are builtin keys (stable across installs; the user-visible
// names differ between fresh seeds and provisioned workspaces), so a renamed
// default status still resolves. Unknown names map onto the to_do bucket and
// are reported so provisioning can catch them.
var DefaultPlaneStatusMap = map[string]string{
	"Todo":        "to_do",
	"Backlog":     "to_do",
	"Untriaged":   "to_do",
	"Blocked":     "to_do",
	"In Progress": "in_progress",
	"Started":     "in_progress",
	"Done":        "done",
	"Completed":   "done",
	"Cancelled":   "done",
}

// Loader runs the import. One Loader holds one database handle and the
// resolved mapping tables for its run.
type Loader struct {
	db          database.Database
	workspaceID int
	opts        Options
	statusID    map[string]int // resolved Windshift status id by lowercased status name
	prioID      map[string]int // resolved global priority id by Plane priority name
	userID      map[string]int // resolved Windshift user id by Plane user uuid
	meta        *Meta
	issues      []*Issue
	comments    []*Comment
	users       []UserRow
	typeID      map[string]int      // resolved item type id by builtin_key ("task"/"subtask")
	prioSeen    map[string]struct{} // dedup for unmapped-priority report lines
	stateSeen   map[string]struct{} // dedup for unmapped-state report lines (its own map: a state named like a priority must not hide either)
	deferred    []planeID           // child/parent plane uuid pairs resolved after batching
	report      Report
}

// planeID is a child/parent pair whose parent was not yet mapped when the
// child was inserted. The parent pass resolves them once every batch committed.
type planeID struct{ child, parent string }

// NewLoader opens the destination database and prepares the run. The
// connection string is Windshift's own (POSTGRES_CONNECTION_STRING shape);
// Initialize is called so the plane_import_mappings migration lands before
// any import writes.
func NewLoader(db database.Database, opts Options) *Loader {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 200
	}
	if opts.DefaultStatusMap == nil {
		opts.DefaultStatusMap = DefaultPlaneStatusMap
	}
	return &Loader{db: db, opts: opts, prioSeen: map[string]struct{}{}, stateSeen: map[string]struct{}{}, report: Report{}}
}

// LoadContract reads the JSONL contract and the users sidecar into memory.
// 10k-row contracts parse in well under a second; the format is line-oriented
// so a producer can stream it.
func (l *Loader) LoadContract() error {
	f, err := os.Open(l.opts.ContractPath)
	if err != nil {
		return fmt.Errorf("open contract: %w", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20) // descriptions can be large
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var rec ContractRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return fmt.Errorf("contract line %d: %w", line, err)
		}
		switch rec.Type {
		case "meta":
			if rec.Meta == nil {
				return fmt.Errorf("contract line %d: meta record without meta", line)
			}
			if l.meta != nil {
				return fmt.Errorf("contract line %d: second meta record", line)
			}
			l.meta = rec.Meta
			if rec.Meta.SchemaVersion != ContractSchemaVersion {
				return fmt.Errorf("contract schema version %d, this build reads %d",
					rec.Meta.SchemaVersion, ContractSchemaVersion)
			}
		case "issue":
			if rec.Issue == nil {
				return fmt.Errorf("contract line %d: issue record without payload", line)
			}
			l.issues = append(l.issues, rec.Issue)
		case "comment":
			if rec.Comment == nil {
				return fmt.Errorf("contract line %d: comment record without payload", line)
			}
			l.comments = append(l.comments, rec.Comment)
		case "user":
			if rec.User == nil {
				return fmt.Errorf("contract line %d: user record without payload", line)
			}
			l.users = append(l.users, *rec.User)
		default:
			// Unknown record types are future-proofing slots, not errors.
			continue
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read contract: %w", err)
	}
	if l.meta == nil {
		return fmt.Errorf("contract has no meta record")
	}
	if l.opts.UsersPath != "" {
		data, err := os.ReadFile(l.opts.UsersPath)
		if err != nil {
			return fmt.Errorf("read users sidecar: %w", err)
		}
		if err := json.Unmarshal(data, &l.users); err != nil {
			return fmt.Errorf("parse users sidecar: %w", err)
		}
	}
	sort.SliceStable(l.issues, func(i, j int) bool {
		return l.issues[i].SequenceID < l.issues[j].SequenceID
	})
	return nil
}

// Resolve maps source facts onto destination identities: the workspace, the
// statuses the run needs, priorities, and every author referenced by the
// contract. Missing pieces are recorded on the report; with Options.Strict a
// missing author fails the run.
func (l *Loader) Resolve() error {
	l.report.WorkspaceKey = l.opts.WorkspaceKey
	l.report.StartedAt = time.Now()

	if l.opts.SourceSlug != "" && l.meta != nil && l.meta.WorkspaceSlug != l.opts.SourceSlug {
		return fmt.Errorf("contract is from Plane workspace %q, --workspace-slug says %q",
			l.meta.WorkspaceSlug, l.opts.SourceSlug)
	}

	var wsID int
	if err := l.db.QueryRow(`SELECT id FROM workspaces WHERE key = ?`, l.opts.WorkspaceKey).Scan(&wsID); err != nil {
		return fmt.Errorf("workspace %q not found: %w", l.opts.WorkspaceKey, err)
	}
	l.workspaceID = wsID

	// Statuses: resolve the builtin keys the contract needs. Statuses are
	// global rows (workspace scoping lives in configuration sets /
	// workflows), and builtin keys survive renamed defaults.
	need := map[string]struct{}{}
	for _, iss := range l.issues {
		if iss.DeletedAt != nil || iss.IsDraft {
			continue
		}
		need[l.mapStateName(iss.StateName)] = struct{}{}
	}
	l.statusID = map[string]int{}
	for key := range need {
		// Statuses are global rows (workspace scoping lives in configuration
		// sets / workflows). Resolve through the status CATEGORY builtin key:
		// the categories (to_do / in_progress / done) are stable across
		// versions, while the status rows under them have been renamed
		// between releases (Open -> To Do), so keying on the status row's own
		// builtin_key is version-fragile.
		var id int
		err := l.db.QueryRow(`
			SELECT s.id FROM statuses s
			JOIN status_categories c ON c.id = s.category_id
			WHERE c.builtin_key = ?
			ORDER BY s.is_default DESC, s.id ASC LIMIT 1`, key).Scan(&id)
		if err == sql.ErrNoRows {
			l.report.MissingStatuses = append(l.report.MissingStatuses, key)
			continue
		}
		if err != nil {
			return fmt.Errorf("resolve status %q: %w", key, err)
		}
		l.statusID[key] = id
	}
	if len(l.report.MissingStatuses) > 0 {
		sort.Strings(l.report.MissingStatuses)
		return fmt.Errorf("destination lacks statuses (builtin keys): %s", strings.Join(l.report.MissingStatuses, ", "))
	}

	// Item types: Plane issues map onto the default task/subtask types so the
	// hierarchy semantics survive (a child becomes a sub-task, as in Plane).
	l.typeID = map[string]int{}
	for _, builtin := range []string{"task", "subtask"} {
		var id int
		if err := l.db.QueryRow(`SELECT id FROM item_types WHERE builtin_key = ?`, builtin).Scan(&id); err == nil {
			l.typeID[builtin] = id
		}
	}

	// Priorities: global table, resolved by builtin_key. Plane's "none"
	// deliberately has no mapping — Windshift's default set has no none row,
	// and inventing one (silent Critical) would misstate every unestimated
	// ticket. Unmapped priorities import with priority_id NULL and are
	// named in the report.
	l.prioID = map[string]int{}
	for _, pk := range []string{"critical", "high", "medium", "low", "urgent"} {
		var id int
		if err := l.db.QueryRow(`SELECT id FROM priorities WHERE builtin_key = ?`, pk).Scan(&id); err == nil {
			l.prioID[pk] = id
		}
	}

	// Users: Plane uuid -> Windshift user id, via email after aliasing.
	byEmail := map[string]string{}
	for _, u := range l.users {
		byEmail[u.ID] = strings.ToLower(strings.TrimSpace(u.Email))
	}
	l.userID = map[string]int{}
	unmapped := map[string]struct{}{}
	for _, iss := range l.issues {
		for _, uid := range []string{iss.CreatedByID} {
			if uid == "" {
				continue
			}
			if _, done := l.userID[uid]; done {
				continue
			}
			id, ok := l.resolveUser(uid, byEmail, unmapped)
			if ok {
				l.userID[uid] = id
			}
		}
	}
	for _, c := range l.comments {
		if _, done := l.userID[c.CreatedByID]; done || c.CreatedByID == "" {
			continue
		}
		if id, ok := l.resolveUser(c.CreatedByID, byEmail, unmapped); ok {
			l.userID[c.CreatedByID] = id
		}
	}
	l.report.UnmappedUsers = sortedKeys(unmapped)
	if l.opts.Strict && len(l.report.UnmappedUsers) > 0 {
		return fmt.Errorf("strict mode: unmapped Plane authors: %s", strings.Join(l.report.UnmappedUsers, ", "))
	}
	return nil
}

// resolveUser maps one Plane user uuid to a Windshift user id. The chain is:
// sidecar email -> alias table -> users.email lookup. A miss is not fatal:
// the row imports with creator_id NULL and the uuid is named in the report,
// because dropping an imported ticket's history over one missing alias is
// worse than an honest gap in the author column.
func (l *Loader) resolveUser(planeUID string, byEmail map[string]string, unmapped map[string]struct{}) (int, bool) {
	email, ok := byEmail[planeUID]
	if !ok {
		unmapped[planeUID+" (no sidecar row)"] = struct{}{}
		return 0, false
	}
	if alias, has := l.opts.Aliases[email]; has {
		email = strings.ToLower(strings.TrimSpace(alias))
	}
	var id int
	err := l.db.QueryRow(`SELECT id FROM users WHERE LOWER(email) = LOWER(?)`, email).Scan(&id)
	if err == sql.ErrNoRows {
		unmapped[planeUID+" <"+email+">"] = struct{}{}
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	return id, true
}

// IssueCount reports how many issue records the loaded contract holds.
func (l *Loader) IssueCount() int { return len(l.issues) }

// CommentCount reports how many comment records the loaded contract holds.
func (l *Loader) CommentCount() int { return len(l.comments) }

// Run executes the import. Issues load in ascending sequence order inside
// per-batch transactions: numbering is allocated by the repository's MAX+1
// path (dense and gap-correct because the source order is ascending), each
// row's creator and timestamps come from the contract, parent links attach in
// the same pass because every parent has a lower sequence id, and created
// facts are batch-recorded with the source chronology as their occurred_at.
func (l *Loader) Run(ctx context.Context) (*Report, error) {
	if l.meta == nil {
		return nil, fmt.Errorf("LoadContract must run before Run")
	}
	l.report.DryRun = l.opts.DryRun

	existing, err := l.loadExistingMappings()
	if err != nil {
		return nil, err
	}
	takenNumbers, err := l.loadTakenNumbers()
	if err != nil {
		return nil, err
	}
	numberToPlaneID := map[int]string{}

	repo := repository.NewItemRepository(l.db)
	recorder := itemevents.NewRecorder(l.db)

	// Comments grouped by issue so each comment lands in its issue's batch.
	commentsByIssue := map[string][]*Comment{}
	for _, c := range l.comments {
		commentsByIssue[c.IssueID] = append(commentsByIssue[c.IssueID], c)
	}

	for start := 0; start < len(l.issues); start += l.opts.BatchSize {
		end := start + l.opts.BatchSize
		if end > len(l.issues) {
			end = len(l.issues)
		}
		batch := l.issues[start:end]
		if l.opts.DryRun {
			if err := l.dryRunBatch(batch, commentsByIssue, existing, takenNumbers, numberToPlaneID); err != nil {
				return nil, err
			}
			l.report.Batches++
			continue
		}
		if err := l.runBatch(ctx, batch, commentsByIssue, existing, takenNumbers, numberToPlaneID, repo, recorder); err != nil {
			return nil, err
		}
		l.report.Batches++
	}

	// Second pass for parent links whose source parent had not been mapped
	// yet (measured: some Plane parents carry a HIGHER sequence_id than the
	// child — the in-pass assumption does not hold universally).
	if !l.opts.DryRun && len(l.deferred) > 0 {
		if err := l.parentPass(existing, repo); err != nil {
			return nil, err
		}
	}

	l.finalizeReport(takenNumbers)
	l.report.FinishedAt = time.Now()
	return &l.report, nil
}
