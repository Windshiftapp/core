package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"windshift/internal/constants"
	"windshift/internal/database"
	"windshift/internal/integrations/todoist"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/sso"

	"uuid"
)

// ErrTodoistSyncAlreadyRunning is returned by SyncConfig when another run
// already holds the per-config lock (a manual "Sync now" racing the poller, or
// two manual runs). Callers treat it as a benign skip, not a failure: the
// manual endpoint maps it to 409 and the scheduler quietly moves on.
var ErrTodoistSyncAlreadyRunning = errors.New("todoist sync already running for this config")

// syncLockLease bounds how long one run may hold a config's sync lock. It is a
// safety valve: if a run crashes or hangs without releasing, the lease expires
// and the next run can proceed. It must comfortably exceed a normal
// reconcile (a Sync pull + a batched command write).
const syncLockLease = 10 * time.Minute

// dueDateLayout is the canonical (date-only) representation the sync uses for
// due dates on both sides. Time-of-day is intentionally not synced in v1: it
// avoids timezone churn between Windshift's *time.Time and Todoist's mixed
// date/datetime due object.
const dueDateLayout = "2006-01-02"

// taskState is the canonical, side-agnostic view of a task that the reconciler
// compares. Each of the three inputs to a merge — the Windshift item, the
// Todoist task, and the last-synced snapshot — is projected to a taskState.
// Priority and time-of-day are deliberately out of scope for v1.
type taskState struct {
	Title       string
	Description string
	Due         string // "YYYY-MM-DD" or ""
	Completed   bool
}

// todoistAPI is the slice of the Todoist client the reconciler needs. *todoist.Client
// satisfies it; tests substitute a fake.
type todoistAPI interface {
	Sync(syncToken string, resourceTypes []string) (*todoist.SyncResponse, error)
	ExecuteCommands(commands []todoist.Command) (*todoist.SyncResponse, error)
}

// personalStore is the Windshift-side surface the reconciler needs: list/create/
// update/delete tasks in a personal workspace. The real implementation wraps the
// item repository + CRUD service; tests substitute an in-memory fake.
type personalStore interface {
	ListTasks(workspaceID int) ([]repository.PersonalWorkspaceTask, error)
	CreateTask(workspaceID, userID int, st taskState) (int, error)
	UpdateTask(itemID int, st taskState, fields []string) error
	DeleteTask(itemID int) error
}

// SyncStats summarizes what one reconciliation run changed.
type SyncStats struct {
	CreatedInWS int
	UpdatedInWS int
	DeletedInWS int
	CreatedInTD int
	UpdatedInTD int
	DeletedInTD int
}

// TodoistSyncService runs two-way personal-task sync for a connected user.
type TodoistSyncService struct {
	db         database.Database
	encryption *sso.SecretEncryption
	syncRepo   *repository.TodoistSyncRepository

	// newAPI builds a Todoist API client from a decrypted token. Overridable in tests.
	newAPI func(accessToken string) todoistAPI
	// newStore builds the Windshift-side store. Overridable in tests.
	newStore func() personalStore
}

// NewTodoistSyncService constructs the sync service with production wiring.
func NewTodoistSyncService(db database.Database, encryption *sso.SecretEncryption) *TodoistSyncService {
	s := &TodoistSyncService{
		db:         db,
		encryption: encryption,
		syncRepo:   repository.NewTodoistSyncRepository(db),
	}
	s.newAPI = func(accessToken string) todoistAPI { return todoist.NewClient(accessToken) }
	s.newStore = func() personalStore { return &itemPersonalStore{db: db} }
	return s
}

// SyncConfig runs one reconciliation for a config: it decrypts the user's token,
// builds the API + store, reconciles, and records the outcome (cursor, last
// error) on the config row.
func (s *TodoistSyncService) SyncConfig(cfg models.TodoistSyncConfig) (SyncStats, error) {
	// Per-config admission: only one run may reconcile a config at a time, so a
	// manual sync and the poller can't both observe missing links and create
	// duplicate Todoist tasks. The lease self-heals a crashed holder.
	now := time.Now().UTC()
	acquired, err := s.syncRepo.AcquireSyncLock(cfg.ID, now, now.Add(syncLockLease))
	if err != nil {
		return SyncStats{}, fmt.Errorf("acquire sync lock: %w", err)
	}
	if !acquired {
		return SyncStats{}, ErrTodoistSyncAlreadyRunning
	}
	defer func() {
		if relErr := s.syncRepo.ReleaseSyncLock(cfg.ID); relErr != nil {
			slog.Error("failed to release todoist sync lock", slog.String("component", "todoist-sync"), slog.Any("error", relErr))
		}
	}()

	enc, err := s.syncRepo.GetEncryptedToken(cfg.UserID, cfg.IntegrationProviderID)
	if err != nil {
		return SyncStats{}, fmt.Errorf("load token: %w", err)
	}
	token, err := s.encryption.Decrypt(enc)
	if err != nil {
		return SyncStats{}, fmt.Errorf("decrypt token: %w", err)
	}

	stats, newToken, syncErr := s.reconcile(cfg, s.newAPI(token), s.newStore())

	// Persist the cursor + last error regardless of partial failure so a write
	// hiccup doesn't replay the whole delta next run.
	lastError := ""
	if syncErr != nil {
		lastError = syncErr.Error()
	}
	if newToken == "" {
		newToken = cfg.SyncToken
	}
	if err := s.syncRepo.UpdateSyncState(cfg.ID, newToken, lastError); err != nil {
		slog.Error("failed to persist todoist sync state", slog.String("component", "todoist-sync"), slog.Any("error", err))
	}
	return stats, syncErr
}

// reconcile is the pure-ish core: it diffs the Todoist incremental delta and the
// current personal-workspace tasks against the stored id-map + snapshots, applies
// Windshift-side changes immediately, and batches Todoist-side writes. It returns
// the new Todoist sync cursor so the caller can persist it.
func (s *TodoistSyncService) reconcile(cfg models.TodoistSyncConfig, api todoistAPI, store personalStore) (SyncStats, string, error) {
	var stats SyncStats

	resp, err := api.Sync(cfg.SyncToken, []string{"items"})
	if err != nil {
		return stats, "", fmt.Errorf("todoist sync pull: %w", err)
	}
	newToken := resp.SyncToken

	wsTasks, err := store.ListTasks(cfg.PersonalWorkspaceID)
	if err != nil {
		return stats, newToken, fmt.Errorf("list personal tasks: %w", err)
	}
	links, err := s.syncRepo.ListLinksByUser(cfg.UserID)
	if err != nil {
		return stats, newToken, fmt.Errorf("list task links: %w", err)
	}

	linkByItem := make(map[int]models.TodoistTaskLink, len(links))
	linkByTD := make(map[string]models.TodoistTaskLink, len(links))
	for _, l := range links {
		linkByItem[l.ItemID] = l
		linkByTD[l.TodoistTaskID] = l
	}
	wsByItem := make(map[int]repository.PersonalWorkspaceTask, len(wsTasks))
	for _, t := range wsTasks {
		wsByItem[t.ItemID] = t
	}

	handledItems := make(map[int]bool)
	var cmds []todoist.Command
	// onSuccess closures run only for Todoist commands that the batch reports as
	// "ok", keyed by command UUID — so a failed write never advances a snapshot.
	onSuccess := make(map[string]func(resp *todoist.SyncResponse))

	// --- Pass A: Todoist delta -> Windshift ---
	for i := range resp.Items {
		td := resp.Items[i]
		link, linked := linkByTD[td.ID]

		if td.IsDeleted {
			if linked {
				if err := store.DeleteTask(link.ItemID); err != nil {
					slog.Warn("todoist sync: delete WS task failed", slog.String("component", "todoist-sync"), slog.Any("error", err))
				} else {
					stats.DeletedInWS++
				}
				_ = s.syncRepo.DeleteLink(link.ID)
				handledItems[link.ItemID] = true
			}
			continue
		}

		if !inScope(cfg, td.ProjectID) {
			// A task that left our scope (e.g. moved to another project) is
			// unlinked but its Windshift copy is left intact.
			if linked {
				_ = s.syncRepo.DeleteLink(link.ID)
				handledItems[link.ItemID] = true
			}
			continue
		}

		tdState := stateFromTodoist(td)

		if !linked {
			itemID, err := store.CreateTask(cfg.PersonalWorkspaceID, mustUserID(cfg.UserID), tdState)
			if err != nil {
				slog.Warn("todoist sync: create WS task failed", slog.String("component", "todoist-sync"), slog.Any("error", err))
				continue
			}
			s.saveSnapshot(cfg.UserID, itemID, td.ID, td.ProjectID, tdState)
			stats.CreatedInWS++
			continue
		}

		ws, exists := wsByItem[link.ItemID]
		if !exists {
			// Windshift copy was deleted while Todoist changed: the deletion wins.
			cmd := todoist.NewDeleteItemCommand(td.ID)
			cmds = append(cmds, cmd)
			capturedLink := link
			onSuccess[cmd.UUID] = func(*todoist.SyncResponse) {
				_ = s.syncRepo.DeleteLink(capturedLink.ID)
				stats.DeletedInTD++
			}
			handledItems[link.ItemID] = true
			continue
		}

		s.reconcilePair(cfg, link, stateFromWS(ws), tdState, td.ID, td.ProjectID, store, &cmds, onSuccess, &stats)
		handledItems[link.ItemID] = true
	}

	// --- Pass B: Windshift tasks -> Todoist (items the delta didn't touch) ---
	for _, ws := range wsTasks {
		if handledItems[ws.ItemID] {
			continue
		}
		link, linked := linkByItem[ws.ItemID]
		if !linked {
			st := stateFromWS(ws)
			cmd, tempID := todoist.NewAddItemCommand(addArgs(cfg, st))
			cmds = append(cmds, cmd)
			capturedWS := ws
			onSuccess[cmd.UUID] = func(r *todoist.SyncResponse) {
				realID := r.TempIDMapping[tempID]
				if realID == "" {
					return
				}
				s.saveSnapshot(cfg.UserID, capturedWS.ItemID, realID, cfg.TodoistProjectID, stateFromWS(capturedWS))
				stats.CreatedInTD++
			}
			continue
		}
		// Existing pair the Todoist delta didn't include: Todoist is unchanged, so
		// its current state equals the snapshot. reconcilePair then pushes any
		// Windshift-side change outward (and no-ops when nothing changed).
		s.reconcilePair(cfg, link, stateFromWS(ws), stateFromSnapshot(link), link.TodoistTaskID, link.TodoistProjectID, store, &cmds, onSuccess, &stats)
	}

	// --- Pass C: Windshift deletions (link whose item is gone) ---
	for _, link := range links {
		if handledItems[link.ItemID] {
			continue
		}
		if _, exists := wsByItem[link.ItemID]; exists {
			continue
		}
		cmd := todoist.NewDeleteItemCommand(link.TodoistTaskID)
		cmds = append(cmds, cmd)
		capturedLink := link
		onSuccess[cmd.UUID] = func(*todoist.SyncResponse) {
			_ = s.syncRepo.DeleteLink(capturedLink.ID)
			stats.DeletedInTD++
		}
	}

	if len(cmds) == 0 {
		return stats, newToken, nil
	}

	execResp, err := api.ExecuteCommands(cmds)
	if err != nil {
		return stats, newToken, fmt.Errorf("todoist commands: %w", err)
	}
	var firstCmdErr error
	for _, cmd := range cmds {
		if cmdErr := execResp.CommandError(cmd.UUID); cmdErr != nil {
			if firstCmdErr == nil {
				firstCmdErr = cmdErr
			}
			slog.Warn("todoist sync: command failed", slog.String("component", "todoist-sync"), slog.Any("error", cmdErr))
			continue
		}
		if fn := onSuccess[cmd.UUID]; fn != nil {
			fn(execResp)
		}
	}
	return stats, newToken, firstCmdErr
}

// reconcilePair merges one already-mapped task. It resolves each field with
// last-write-wins against the snapshot, applies divergences to Windshift
// immediately, queues a Todoist update/complete command when Todoist diverges,
// and refreshes the snapshot to the resolved state.
func (s *TodoistSyncService) reconcilePair(
	cfg models.TodoistSyncConfig, link models.TodoistTaskLink,
	ws, td taskState, todoistID, todoistProjectID string,
	store personalStore, cmds *[]todoist.Command, onSuccess map[string]func(*todoist.SyncResponse), stats *SyncStats,
) {
	snap := stateFromSnapshot(link)
	resolved := taskState{
		Title:       resolve(ws.Title, td.Title, snap.Title),
		Description: resolve(ws.Description, td.Description, snap.Description),
		Due:         resolve(ws.Due, td.Due, snap.Due),
		Completed:   resolve(ws.Completed, td.Completed, snap.Completed),
	}

	// Fully in sync: nothing changed on either side. Skip the snapshot rewrite so
	// an unchanged pair costs no DB write per run.
	if ws == resolved && td == resolved {
		return
	}

	// Apply to Windshift where it diverges from the resolved state.
	var changedFields []string
	if ws.Title != resolved.Title {
		changedFields = append(changedFields, "title")
	}
	if ws.Description != resolved.Description {
		changedFields = append(changedFields, "description")
	}
	if ws.Due != resolved.Due {
		changedFields = append(changedFields, "due")
	}
	if ws.Completed != resolved.Completed {
		changedFields = append(changedFields, "completed")
	}
	if len(changedFields) > 0 {
		if err := store.UpdateTask(link.ItemID, resolved, changedFields); err != nil {
			slog.Warn("todoist sync: update WS task failed", slog.String("component", "todoist-sync"), slog.Any("error", err))
		} else {
			stats.UpdatedInWS++
		}
	}

	// Apply to Todoist where it diverges. Content/description/due go in one
	// item_update; completion is a separate complete/uncomplete command.
	tdContentChanged := td.Title != resolved.Title || td.Description != resolved.Description || td.Due != resolved.Due
	tdCompletionChanged := td.Completed != resolved.Completed
	resolvedCopy := resolved

	if tdContentChanged {
		*cmds = append(*cmds, updateCommand(todoistID, resolved))
		stats.UpdatedInTD++
	}
	if tdCompletionChanged {
		var cmd todoist.Command
		if resolved.Completed {
			cmd = todoist.NewCompleteItemCommand(todoistID)
		} else {
			cmd = todoist.NewUncompleteItemCommand(todoistID)
		}
		*cmds = append(*cmds, cmd)
	}

	// Snapshot refresh. When a Todoist write is queued, defer the snapshot to its
	// success so a failed write is retried next run; otherwise save inline.
	save := func() { s.saveSnapshot(cfg.UserID, link.ItemID, todoistID, todoistProjectID, resolvedCopy) }
	switch {
	case tdContentChanged:
		// Attach to the update command's success (the last appended update).
		updateUUID := (*cmds)[len(*cmds)-1].UUID
		if tdCompletionChanged {
			updateUUID = (*cmds)[len(*cmds)-2].UUID
		}
		onSuccess[updateUUID] = func(*todoist.SyncResponse) { save() }
	case tdCompletionChanged:
		onSuccess[(*cmds)[len(*cmds)-1].UUID] = func(*todoist.SyncResponse) { save() }
	default:
		save()
	}
}

// saveSnapshot upserts the id-map row with the agreed state at this sync.
func (s *TodoistSyncService) saveSnapshot(userID string, itemID int, todoistID, projectID string, st taskState) {
	existing, err := s.syncRepo.GetLinkByItemID(userID, itemID)
	id := uuid.New().String()
	if err == nil && existing != nil {
		id = existing.ID
	}
	if upErr := s.syncRepo.UpsertLink(models.TodoistTaskLink{
		ID:               id,
		UserID:           userID,
		ItemID:           itemID,
		TodoistTaskID:    todoistID,
		TodoistProjectID: projectID,
		LastTitle:        st.Title,
		LastDescription:  st.Description,
		LastDue:          st.Due,
		LastCompleted:    st.Completed,
		LastPriority:     todoist.PriorityNormal,
	}); upErr != nil {
		slog.Warn("todoist sync: save snapshot failed", slog.String("component", "todoist-sync"), slog.Any("error", upErr))
	}
}

// --- pure helpers (unit-tested directly) ------------------------------------

// resolve picks the winner for a single field via last-write-wins against the
// snapshot. When both sides changed and disagree, Todoist wins the tie: the Sync
// API exposes no task mtime, so a Todoist change observed in this delta is
// treated as the most recent write.
func resolve[T comparable](ws, td, snap T) T {
	wsChanged := ws != snap
	tdChanged := td != snap
	switch {
	case wsChanged && !tdChanged:
		return ws
	case tdChanged && !wsChanged:
		return td
	case !wsChanged && !tdChanged:
		return snap
	default:
		if ws == td {
			return ws
		}
		return td
	}
}

func inScope(cfg models.TodoistSyncConfig, projectID string) bool {
	if cfg.ScopeMode == models.TodoistScopeProject {
		return projectID == cfg.TodoistProjectID
	}
	return true
}

// stateFromTodoist sanitizes external content before persistence and reconciliation snapshots.
func stateFromTodoist(td todoist.Item) taskState {
	due := ""
	if td.Due != nil {
		due = normalizeDue(td.Due.Date)
	}
	return taskState{
		Title:       sanitize.PlainTextField.Sanitize(td.Content),
		Description: sanitize.RichText.Sanitize(td.Description),
		Due:         due,
		Completed:   td.Checked,
	}
}

func stateFromWS(ws repository.PersonalWorkspaceTask) taskState {
	due := ""
	if ws.DueDate != nil {
		due = ws.DueDate.Format(dueDateLayout)
	}
	return taskState{Title: ws.Title, Description: ws.Description, Due: due, Completed: ws.Completed}
}

func stateFromSnapshot(link models.TodoistTaskLink) taskState {
	return taskState{Title: link.LastTitle, Description: link.LastDescription, Due: normalizeDue(link.LastDue), Completed: link.LastCompleted}
}

// normalizeDue collapses a Todoist due value (date or RFC3339 datetime) to a
// date-only "YYYY-MM-DD" string.
func normalizeDue(d string) string {
	if len(d) >= 10 {
		return d[:10]
	}
	return d
}

func addArgs(cfg models.TodoistSyncConfig, st taskState) todoist.AddItemArgs {
	args := todoist.AddItemArgs{Content: st.Title, Description: st.Description, DueDate: st.Due}
	if cfg.ScopeMode == models.TodoistScopeProject {
		args.ProjectID = cfg.TodoistProjectID
	}
	return args
}

func updateCommand(todoistID string, st taskState) todoist.Command {
	content := st.Title
	description := st.Description
	args := todoist.UpdateItemArgs{ID: todoistID, Content: &content, Description: &description}
	if st.Due != "" {
		args.Due = &todoist.Due{Date: st.Due}
	} else {
		// Clearing a due date: Todoist accepts a null due object.
		args.Due = nil
	}
	return todoist.NewUpdateItemCommand(args)
}

func mustUserID(s string) int {
	id, _ := strconv.Atoi(s)
	return id
}

// --- Windshift-side store implementation ------------------------------------

type itemPersonalStore struct {
	db database.Database
}

func (st *itemPersonalStore) ListTasks(workspaceID int) ([]repository.PersonalWorkspaceTask, error) {
	return repository.NewItemRepository(st.db).ListPersonalWorkspaceTasks(workspaceID)
}

func (st *itemPersonalStore) CreateTask(workspaceID, userID int, s taskState) (int, error) {
	statusID := statusForCompleted(s.Completed)
	params := ItemCreationParams{
		WorkspaceID:      workspaceID,
		Title:            s.Title,
		Description:      s.Description,
		StatusID:         &statusID,
		IsTask:           true,
		AssigneeID:       &userID,
		CreatorID:        &userID,
		ValidatingUserID: userID,
		DueDate:          dueTime(s.Due),
		EventMetadata:    itemevents.Integration("todoist", "integration"),
	}
	id, err := CreateItem(st.db, params)
	return int(id), err
}

func (st *itemPersonalStore) UpdateTask(itemID int, s taskState, fields []string) error {
	update := map[string]any{}
	for _, f := range fields {
		switch f {
		case "title":
			update["title"] = s.Title
		case "description":
			update["description"] = s.Description
		case "due":
			update["due_date"] = dueTime(s.Due)
		case "completed":
			update["status_id"] = statusForCompleted(s.Completed)
		}
	}
	if len(update) == 0 {
		return nil
	}
	tx, err := st.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	repo := repository.NewItemRepository(st.db)
	original, err := repo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return err
	}
	if err := repo.UpdateFields(tx, itemID, update); err != nil {
		return err
	}
	updated, err := repo.FindByIDForUpdate(tx, itemID)
	if err != nil {
		return err
	}
	metadata := itemevents.Integration("todoist", "integration")
	recorder := itemevents.NewRecorder(st.db)
	changes := itemevents.Changes(original, updated)
	if _, statusChanged := update["status_id"]; statusChanged {
		if _, err := recorder.StatusChanged(context.Background(), tx, updated, original.StatusID, updated.StatusID, changes, metadata); err != nil {
			return err
		}
	} else if _, err := recorder.Updated(context.Background(), tx, updated, changes, metadata); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Live-update publish (WI-483): Todoist sync writes item fields directly,
	// bypassing ItemUpdateService.
	kind := ItemChangeUpdated
	if _, ok := update["status_id"]; ok {
		kind = ItemChangeStatus
	}
	PublishItemChange(itemID, kind)
	return nil
}

func (st *itemPersonalStore) DeleteTask(itemID int) error {
	return NewItemCRUDService(st.db).DeleteSingleWithMetadata(itemID, itemevents.Integration("todoist", "integration"))
}

func statusForCompleted(completed bool) int {
	if completed {
		return constants.StatusIDDone
	}
	return constants.StatusIDOpen
}

func dueTime(due string) *time.Time {
	if due == "" {
		return nil
	}
	t, err := time.Parse(dueDateLayout, normalizeDue(due))
	if err != nil {
		return nil
	}
	return &t
}
