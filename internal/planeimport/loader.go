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
	// DefaultStatusMap maps Plane state NAMES to status-category builtin keys.
	// It is the fallback for contracts that carry no state_group; the group
	// field always wins when present (PlaneStateGroupCategories). An empty
	// map selects DefaultPlaneStatusMap.
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
	UnmappedStates     []string  `json:"unmapped_state_names,omitempty"`
	UnmappedPriorities []string  `json:"unmapped_priorities,omitempty"`
	CreatedCategories  []string  `json:"created_status_categories,omitempty"`
	CreatedStatuses    []string  `json:"created_statuses,omitempty"`
	CreatedPriorities  []string  `json:"created_priorities,omitempty"`
	CreatedLabels      []string  `json:"created_labels,omitempty"`
	TransitionsAdded   int       `json:"workflow_transitions_added"`
	IssuesConverged    int       `json:"issues_converged_to_contract"`
	IssuesLeftEdited   int       `json:"issues_left_alone_edited_since_import"`
	LabelLinksAdded    int       `json:"label_links_added"`
	Collisions         []string  `json:"collisions,omitempty"`
	NumberGaps         []int     `json:"number_gaps_preserved,omitempty"`
	MaxNumber          int       `json:"max_number_after_import"`
	Batches            int       `json:"batches"`
}

// DefaultPlaneStatusMap maps Plane state NAMES to Windshift status-category
// builtin keys. It is the fallback for contracts that do not carry each
// issue's state_group; a contract that does carry groups uses
// PlaneStateGroupCategories, which is what Plane itself defines. Names are
// used only when the group is absent, because Plane users rename states freely
// while the group is fixed by Plane's own state model.
var DefaultPlaneStatusMap = map[string]string{
	"Todo":        "to_do",
	"Backlog":     "to_do",
	"Untriaged":   "to_do",
	"Triage":      "to_do",
	"Blocked":     "to_do",
	"In Progress": "in_progress",
	"Started":     "in_progress",
	"Done":        "done",
	"Completed":   "done",
	// The key is the source's own state name, not prose: Plane spells it with
	// two l's, and renaming the key would silently unmap the state.
	"Cancelled": "cancelled", //nolint:misspell // Plane's state name is source data
}

// PlaneStateGroupCategories maps Plane's state GROUP (the field Plane itself
// uses to drive board behavior) onto Windshift status-category builtin keys.
// This is the mapping the fidelity contract runs on: a state lands in the
// category its group implies, never in whatever bucket a name guess suggests.
// Windshift seeds no category for Plane's cancel-outcome group; the vocabulary
// pass creates it when the source needs it (is_completed=true: abandoned work
// is finished work, the burndown must stop counting it).
var PlaneStateGroupCategories = map[string]string{
	"backlog":   "to_do",
	"unstarted": "to_do",
	"triage":    "to_do",
	"started":   "in_progress",
	"completed": "done",
	"cancelled": "cancelled", //nolint:misspell // Plane's group name is source data
}

// Loader runs the import. One Loader holds one database handle and the
// resolved mapping tables for its run.
type Loader struct {
	db               database.Database
	workspaceID      int
	opts             Options
	statusID         map[string]int // Windshift status id by EXACT status name (1:1 with source state names)
	statusIsNew      map[string]struct{}
	categoryID       map[string]int    // status-category id by builtin_key (targets for created statuses)
	labelID          map[string]int    // Windshift label id by lowercased name
	labelByPlane     map[string]int    // Plane label uuid -> Windshift label id
	labelNameByPlane map[string]string // Plane label uuid -> Windshift label name (mapping metadata)
	prioID           map[string]int    // resolved global priority id by Plane priority name
	userID           map[string]int    // resolved Windshift user id by Plane user uuid
	meta             *Meta
	issues           []*Issue
	comments         []*Comment
	users            []UserRow
	states           []StateRow
	labels           []LabelRow
	typeID           map[string]int      // resolved item type id by builtin_key ("task"/"subtask")
	prioSeen         map[string]struct{} // dedup for unmapped-priority report lines
	stateSeen        map[string]struct{} // dedup for unmapped-state report lines (its own map: a state named like a priority must not hide either)
	deferred         []planeID           // child/parent plane uuid pairs resolved after batching
	report           Report
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
	return &Loader{db: db, opts: opts,
		prioSeen: map[string]struct{}{}, stateSeen: map[string]struct{}{},
		statusIsNew: map[string]struct{}{}, categoryID: map[string]int{},
		labelID: map[string]int{}, labelByPlane: map[string]int{}, labelNameByPlane: map[string]string{},
		report: Report{},
	}
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
		case "state":
			if rec.State == nil {
				return fmt.Errorf("contract line %d: state record without payload", line)
			}
			l.states = append(l.states, *rec.State)
		case "label":
			if rec.Label == nil {
				return fmt.Errorf("contract line %d: label record without payload", line)
			}
			l.labels = append(l.labels, *rec.Label)
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

	// Vocabulary: whatever the source catalog names and the destination lacks
	// gets created here — statuses 1:1 with the project's states, the
	// canceled-outcome category when the project has that group, the urgent
	// priority only when an issue carries it, labels — and new statuses are
	// wired into the effective workflow so they are reachable, not islands.
	if err := l.ensureVocabulary(); err != nil {
		return err
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
			l.dryRunBatch(batch, commentsByIssue, existing, takenNumbers, numberToPlaneID)
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
		if err := l.parentPass(existing); err != nil {
			return nil, err
		}
	}

	l.finalizeReport(takenNumbers)
	l.report.FinishedAt = time.Now()
	return &l.report, nil
}

// vocabularyState is one source state the destination must be able to place
// issues in: the name arrives 1:1, the category is implied by Plane's own
// group field.
type vocabularyState struct {
	name     string
	category string // status-category builtin key
}

// ensureVocabulary makes the destination hold the source's catalog.
//
// Statuses and categories are global rows shared by every workspace —
// creating the source project's own status rows is the product's own
// custom-status feature (POST /statuses, POST /status-categories), not a
// workaround; the Jira importer materializing whole workflows into these same
// tables is the in-repo precedent. Windshift seeds three categories and four
// priorities; Plane vocabularies routinely use more, and collapsing them onto
// the seeded trio is exactly what a faithful migration must not do.
//
// Everything runs in ONE transaction. On a dry run the pass is rolled back
// after the inserts succeeded: --dry-run still proves the writes land against
// the live schema and reports precisely what the run would create.
func (l *Loader) ensureVocabulary() error {
	tx, err := l.db.Begin()
	if err != nil {
		return fmt.Errorf("begin vocabulary pass: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	l.statusID = map[string]int{}
	vocab := map[string]vocabularyState{}
	for _, s := range l.states {
		vocab[s.Name] = vocabularyState{s.Name, l.stateCategory(s.Name, s.Group)}
	}
	for _, iss := range l.issues {
		if iss.StateName == "" {
			continue
		}
		if _, ok := vocab[iss.StateName]; !ok {
			vocab[iss.StateName] = vocabularyState{iss.StateName, l.stateCategory(iss.StateName, iss.StateGroup)}
		}
	}
	// A state whose GROUP neither map knows is created under to_do on a guess.
	// That guess is a fact the operator must see, so it is reported by name —
	// same field, same rule as everything else the importer drops or infers.
	reportGuessedCategory := func(name, group string) {
		if name == "" {
			return
		}
		if _, ok := PlaneStateGroupCategories[strings.ToLower(strings.TrimSpace(group))]; ok {
			return
		}
		if _, named := l.opts.DefaultStatusMap[name]; named {
			return
		}
		if _, dup := l.stateSeen[name]; dup {
			return
		}
		l.stateSeen[name] = struct{}{}
		l.report.UnmappedStates = append(l.report.UnmappedStates, name)
	}
	for _, s := range l.states {
		reportGuessedCategory(s.Name, s.Group)
	}
	for _, iss := range l.issues {
		reportGuessedCategory(iss.StateName, iss.StateGroup)
	}
	names := make([]string, 0, len(vocab))
	for n := range vocab {
		names = append(names, n)
	}
	sort.Strings(names)

	l.categoryID = map[string]int{}
	for _, n := range names {
		builtin := vocab[n].category
		if _, have := l.categoryID[builtin]; have {
			continue
		}
		id, err := l.resolveOrCreateCategory(tx, builtin)
		if err != nil {
			return err
		}
		l.categoryID[builtin] = id
	}

	// Statuses: 1:1 by name. An existing row of the same name is ADOPTED —
	// statuses are global, whoever named that row first owns it — a missing
	// one is created under its group's category.
	for _, n := range names {
		var id int
		err := tx.QueryRow(`SELECT id FROM statuses WHERE name = ? ORDER BY is_default DESC, id LIMIT 1`, n).Scan(&id)
		if err == nil {
			l.statusID[n] = id
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("look up status %q: %w", n, err)
		}
		cat := l.categoryID[vocab[n].category]
		if err := tx.QueryRow(`
			INSERT INTO statuses (name, description, category_id, is_default)
			VALUES (?, NULL, ?, false) RETURNING id`, n, cat).Scan(&id); err != nil {
			return fmt.Errorf("create status %q under category %s: %w", n, vocab[n].category, err)
		}
		l.statusID[n] = id
		l.statusIsNew[n] = struct{}{}
		l.report.CreatedStatuses = append(l.report.CreatedStatuses, n)
	}

	if err := l.ensureUrgentPriority(tx); err != nil {
		return err
	}
	if err := l.ensureLabels(tx); err != nil {
		return err
	}
	if err := l.ensureTransitions(tx); err != nil {
		return err
	}

	if l.opts.DryRun {
		return nil // the deferred Rollback IS the dry run: writes proven, nothing persists
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit vocabulary pass: %w", err)
	}
	return nil
}

// stateCategory names the status-category builtin key a source state belongs
// in. Plane's group field decides; the name map is the fallback for contracts
// that predate state_group. A state neither map knows is still created — under
// to_do — and the guess is reported by name in ensureVocabulary, never folded
// into done by guesswork.
func (l *Loader) stateCategory(name, group string) string {
	if builtin, ok := PlaneStateGroupCategories[strings.ToLower(strings.TrimSpace(group))]; ok {
		return builtin
	}
	if builtin, ok := l.opts.DefaultStatusMap[name]; ok {
		return builtin
	}
	return "to_do"
}

// categoryNames are the human labels Windshift seeds for its builtin category
// keys. A deployment that renamed or pre-created a category is matched by
// builtin key first, by seeded name second, before the importer creates one.
var categoryNames = map[string]string{
	"to_do": "To Do", "in_progress": "In Progress", "done": "Done",
	"cancelled": "Cancelled", //nolint:misspell // Plane's group name spelled into a row label
}

func (l *Loader) resolveOrCreateCategory(tx database.Tx, builtin string) (int, error) {
	hint := builtin
	if named, ok := categoryNames[builtin]; ok {
		hint = named
	}
	var id int
	err := tx.QueryRow(`SELECT id FROM status_categories WHERE builtin_key = ? OR LOWER(name) = LOWER(?) ORDER BY id LIMIT 1`, builtin, hint).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("look up status category %q: %w", builtin, err)
	}
	completed := builtin == "done" || builtin == "cancelled" //nolint:misspell // Plane's group name is source data
	if err := tx.QueryRow(`
		INSERT INTO status_categories (builtin_key, name, color, description, is_default, is_completed)
		VALUES (?, ?, ?, ?, false, ?) RETURNING id`,
		builtin, hint, l.categoryColorFor(builtin),
		"Created by the Plane importer: the source vocabulary names this state group.", completed).Scan(&id); err != nil {
		return 0, fmt.Errorf("create status category %q: %w", builtin, err)
	}
	l.report.CreatedCategories = append(l.report.CreatedCategories, hint)
	return id, nil
}

// categoryColorFor picks the color of a category the importer creates: the
// source's own state color when the contract carries one for a state in that
// group, otherwise neutral slate — stated, not invented per board.
func (l *Loader) categoryColorFor(builtin string) string {
	group := ""
	for g, c := range PlaneStateGroupCategories {
		if c == builtin {
			group = g
			break
		}
	}
	for _, s := range l.states {
		if s.Group == group && s.Color != "" {
			return planeColorHex(s.Color)
		}
	}
	return "#6B7280"
}

// ensureUrgentPriority gives Plane's `urgent` a row instead of a NULL.
// Windshift seeds critical/high/medium/low; Plane's scale has a rung above
// critical. Dropping urgent to NULL loses a fact the source states; mapping it
// onto Critical asserts a severity nobody chose. The row is created ONLY when
// the contract actually carries an urgent issue — the importer does not
// decorate a deployment with vocabulary its sources never used.
func (l *Loader) ensureUrgentPriority(tx database.Tx) error {
	used := false
	for _, iss := range l.issues {
		if iss.Priority == "urgent" {
			used = true
			break
		}
	}
	if !used {
		return nil
	}
	if _, ok := l.prioID["urgent"]; ok {
		return nil
	}
	var id int
	err := tx.QueryRow(`SELECT id FROM priorities WHERE builtin_key = 'urgent' OR LOWER(name) = 'urgent' ORDER BY sort_order, id LIMIT 1`).Scan(&id)
	if err == nil {
		l.prioID["urgent"] = id
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("look up urgent priority: %w", err)
	}
	if err := tx.QueryRow(`
		INSERT INTO priorities (builtin_key, name, description, is_default, sort_order)
		VALUES ('urgent', 'Urgent', 'Created by the Plane importer: the source scale has a rung above Critical.', false, 0)
		RETURNING id`).Scan(&id); err != nil {
		return fmt.Errorf("create urgent priority: %w", err)
	}
	l.prioID["urgent"] = id
	l.report.CreatedPriorities = append(l.report.CreatedPriorities, "urgent")
	return nil
}

// ensureLabels materializes the project's label vocabulary. Labels are a
// global catalog keyed case-insensitively on name; an existing label is
// adopted, a missing one created with the source's color.
func (l *Loader) ensureLabels(tx database.Tx) error {
	l.labelID = map[string]int{}
	l.labelByPlane = map[string]int{}
	l.labelNameByPlane = map[string]string{}
	for _, lb := range l.labels {
		key := strings.ToLower(lb.Name)
		id, ok := l.labelID[key]
		if !ok {
			var err error
			id, err = l.resolveOrCreateLabel(tx, lb.Name, lb.Color)
			if err != nil {
				return err
			}
			l.labelID[key] = id
		}
		l.labelByPlane[lb.ID] = id
		l.labelNameByPlane[lb.ID] = lb.Name
	}
	return nil
}

func (l *Loader) resolveOrCreateLabel(tx database.Tx, name, color string) (int, error) {
	var id int
	err := tx.QueryRow(`SELECT id FROM labels WHERE LOWER(name) = LOWER(?) ORDER BY id LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("look up label %q: %w", name, err)
	}
	if err := tx.QueryRow(`INSERT INTO labels (name, color) VALUES (?, ?) RETURNING id`, name, planeColorHex(color)).Scan(&id); err != nil {
		return 0, fmt.Errorf("create label %q: %w", name, err)
	}
	l.report.CreatedLabels = append(l.report.CreatedLabels, name)
	return id, nil
}

// planeColorHex normalizes the two color shapes the source stores ("#RRGGBB"
// and bare hex) into the "#"-prefixed form the destination columns carry.
func planeColorHex(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return "#3B82F6"
	}
	if !strings.HasPrefix(c, "#") {
		return "#" + c
	}
	return c
}

// ensureTransitions makes every newly created status reachable and exitable in
// the workflow the workspace actually resolves (configuration set first, then
// the global default). A status no transition reaches is an island: items land
// on it, then nothing may move them off it. Plane's boards move issues between
// states freely, so the added edges mirror the source rather than inventing a
// stricter machine. Every edge is checked before insert — the table has no
// unique constraint, and a rerun must converge, not accumulate.
func (l *Loader) ensureTransitions(tx database.Tx) error {
	if len(l.statusIsNew) == 0 {
		return nil
	}
	var workflowID int
	err := tx.QueryRow(`
		SELECT COALESCE(
			(SELECT cs.workflow_id FROM workspace_configuration_sets wcs
				JOIN configuration_sets cs ON cs.id = wcs.configuration_set_id
				WHERE wcs.workspace_id = ? AND cs.workflow_id IS NOT NULL LIMIT 1),
			(SELECT id FROM workflows WHERE is_default = true ORDER BY id LIMIT 1))`,
		l.workspaceID).Scan(&workflowID)
	if err == sql.ErrNoRows {
		return nil // no workflow configured: the app already allows every transition
	}
	if err != nil {
		return fmt.Errorf("resolve effective workflow: %w", err)
	}

	universe := make([]int, 0, len(l.statusID))
	seen := map[int]bool{}
	for _, id := range l.statusID {
		if !seen[id] {
			seen[id] = true
			universe = append(universe, id)
		}
	}
	sort.Ints(universe)

	for name := range l.statusIsNew {
		id := l.statusID[name]
		var reachable int
		err := tx.QueryRow(`SELECT 1 FROM workflow_transitions WHERE workflow_id = ? AND to_status_id = ? AND from_all_statuses = true LIMIT 1`, workflowID, id).Scan(&reachable)
		if err == sql.ErrNoRows {
			if _, err := tx.ExecWrite(`
				INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, from_all_statuses)
				VALUES (?, NULL, ?, true)`, workflowID, id); err != nil {
				return fmt.Errorf("allow entry to status %q: %w", name, err)
			}
			l.report.TransitionsAdded++
		} else if err != nil {
			return fmt.Errorf("check entry to status %q: %w", name, err)
		}
		for _, target := range universe {
			if target == id {
				continue
			}
			var edge int
			err := tx.QueryRow(`SELECT 1 FROM workflow_transitions WHERE workflow_id = ? AND to_status_id = ? AND from_status_id = ? LIMIT 1`, workflowID, target, id).Scan(&edge)
			if err == sql.ErrNoRows {
				if _, err := tx.ExecWrite(`
					INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, from_all_statuses)
					VALUES (?, ?, ?, false)`, workflowID, id, target); err != nil {
					return fmt.Errorf("allow exit from status %q to status %d: %w", name, target, err)
				}
				l.report.TransitionsAdded++
			} else if err != nil {
				return fmt.Errorf("check exit from status %q: %w", name, err)
			}
		}
	}
	return nil
}
