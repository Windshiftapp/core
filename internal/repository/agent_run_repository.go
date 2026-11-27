package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/redact"
)

// AgentRunRepository persists coding-agent runs and their event streams.
// RunService is the only writer in normal operation; the admin
// "Agent runs" UI is the dominant reader.
type AgentRunRepository struct {
	db database.Database
}

type AgentPROwnership struct {
	ItemID            int
	AgentRunID        int
	BindingID         int
	TriggeredByUserID int
	HeadRepo          string
	HeadBranch        string
}

func (r *AgentRunRepository) PRContinuationOwner(ctx context.Context, workspaceID int, repoSlug string, prNumber int) (*AgentPROwnership, error) {
	var owner AgentPROwnership
	err := r.db.QueryRowContext(ctx, `
		SELECT o.item_id, o.agent_run_id, o.binding_id, COALESCE(o.triggered_by_user_id,0), o.head_repo, o.head_branch
		FROM agent_pr_ownerships o
		JOIN workspace_repositories wr ON wr.id = o.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wsc.workspace_id = ? AND wr.repository_name = ? AND o.pr_number = ?
		ORDER BY o.updated_at DESC LIMIT 1
	`, workspaceID, repoSlug, prNumber).Scan(&owner.ItemID, &owner.AgentRunID, &owner.BindingID, &owner.TriggeredByUserID, &owner.HeadRepo, &owner.HeadBranch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &owner, nil
}

func (r *AgentRunRepository) LatestForBindingItem(ctx context.Context, bindingID, itemID int) (*models.AgentRun, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM agent_runs WHERE binding_id=? AND item_id=? ORDER BY id DESC LIMIT 1`, bindingID, itemID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

// NewAgentRunRepository constructs a new repository.
func NewAgentRunRepository(db database.Database) *AgentRunRepository {
	return &AgentRunRepository{db: db}
}

// Insert creates a new agent_runs row in the queued state and returns the
// new ID. Fields managed by the DB (id, queued_at, created_at, updated_at)
// are populated from defaults; the caller is responsible for workspace_id,
// item_id, and binding_id.
func (r *AgentRunRepository) Insert(ctx context.Context, run *models.AgentRun) (int, error) {
	status := run.Status
	if status == "" {
		status = models.AgentRunStatusQueued
	}
	jobKind := run.JobKind
	if jobKind == "" {
		jobKind = models.JobKindCodingAgent
	}
	triggerJSON, err := marshalTrigger(run.Trigger)
	if err != nil {
		return 0, fmt.Errorf("marshal agent_run trigger: %w", err)
	}
	// RETURNING id (not LastInsertId) for Postgres compatibility.
	var id int64
	err = r.db.QueryRowWriteContext(ctx, `
		INSERT INTO agent_runs(
			workspace_id, item_id, binding_id, target_pool_id, job_kind, job_image,
			is_ephemeral, status, triggered_by_user_id, trigger_json, acting_user_id,
			root_initiator_user_id, immediate_trigger_user_id, parent_run_id,
			chain_depth, session_id, profile_version, grants_json, profile_snapshot_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		run.WorkspaceID, nullIntArg(run.ItemID), nullIntArg(run.BindingID),
		nullIntArg(run.TargetPoolID), jobKind, nullStringArg(run.JobImage), run.IsEphemeral, status,
		nullIntArg(run.TriggeredByUserID), triggerJSON, nullIntArg(run.ActingUserID),
		nullIntArg(run.RootInitiatorUserID), nullIntArg(run.ImmediateTriggerUserID),
		nullIntArg(run.ParentRunID), run.ChainDepth, nullStringArg(run.SessionID),
		nullPositiveIntArg(run.ProfileVersion), nullStringArg(run.GrantsJSON),
		nullStringArg(run.ProfileSnapshotJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert agent_run: %w", err)
	}
	return int(id), nil
}

// marshalTrigger renders a run's trigger context to the nullable JSON column
// value. A nil or empty trigger stores SQL NULL so old rows and trigger-less
// runs read back as nil rather than "null"/"{}".
func marshalTrigger(t *models.RunTrigger) (any, error) {
	if t == nil || (t.Kind == "" && t.Instruction == "" && t.CommentID == 0 && t.AuthorID == 0) {
		return nil, nil
	}
	// Defense in depth: the instruction is sourced from sanitized, capped
	// comment content, but bound it again here so no caller can persist an
	// unbounded prompt payload into trigger_json (the column feeds the agent's
	// initial prompt). Copy first — never mutate the caller's trigger.
	if len(t.Instruction) > maxTriggerInstructionBytes {
		clamped := *t
		clamped.Instruction = truncateInstruction(clamped.Instruction)
		t = &clamped
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// maxTriggerInstructionBytes bounds the persisted trigger instruction. Matches
// the 256 KiB long-text ceiling the comment sanitizer already enforces.
const maxTriggerInstructionBytes = 256 * 1024

// truncateInstruction cuts s to at most maxTriggerInstructionBytes bytes
// without splitting a multi-byte rune.
func truncateInstruction(s string) string {
	if len(s) <= maxTriggerInstructionBytes {
		return s
	}
	s = s[:maxTriggerInstructionBytes]
	for s != "" && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func nullPositiveIntArg(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

// scanTrigger decodes the nullable trigger_json column into a RunTrigger.
// Returns nil for SQL NULL / empty so callers see a clean absence.
func scanTrigger(col sql.NullString) (*models.RunTrigger, error) {
	if !col.Valid || col.String == "" || col.String == "null" {
		return nil, nil
	}
	var t models.RunTrigger
	if err := json.Unmarshal([]byte(col.String), &t); err != nil {
		return nil, fmt.Errorf("decode agent_run trigger_json: %w", err)
	}
	return &t, nil
}

// Get loads a single run by ID. Returns sql.ErrNoRows if it does not exist.
func (r *AgentRunRepository) Get(ctx context.Context, id int) (*models.AgentRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, item_id, binding_id, status, queued_at, started_at, ended_at,
		       container_id, runner_id, target_pool_id, job_kind, job_image, is_ephemeral, error,
		       triggered_by_user_id, trigger_json, acting_user_id, root_initiator_user_id,
		       immediate_trigger_user_id, parent_run_id, chain_depth, session_id,
		       profile_version, grants_json, profile_snapshot_json, created_at, updated_at
		FROM agent_runs WHERE id = ?
	`, id)

	run := &models.AgentRun{}
	var itemID, bindingID, runnerID, targetPoolID, triggeredBy sql.NullInt64
	var actingUserID, rootUserID, immediateUserID, parentRunID, profileVersion sql.NullInt64
	var startedAt, endedAt sql.NullTime
	var containerID, jobImage, errMsg, triggerJSON, sessionID, grantsJSON, profileSnapshotJSON sql.NullString

	if err := row.Scan(
		&run.ID, &run.WorkspaceID, &itemID, &bindingID, &run.Status,
		&run.QueuedAt, &startedAt, &endedAt,
		&containerID, &runnerID, &targetPoolID, &run.JobKind, &jobImage, &run.IsEphemeral, &errMsg,
		&triggeredBy, &triggerJSON, &actingUserID, &rootUserID, &immediateUserID,
		&parentRunID, &run.ChainDepth, &sessionID, &profileVersion, &grantsJSON,
		&profileSnapshotJSON, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	trigger, err := scanTrigger(triggerJSON)
	if err != nil {
		return nil, err
	}
	run.Trigger = trigger
	if triggeredBy.Valid {
		v := int(triggeredBy.Int64)
		run.TriggeredByUserID = &v
	}
	setOptionalInt := func(col sql.NullInt64, dst **int) {
		if col.Valid {
			v := int(col.Int64)
			*dst = &v
		}
	}
	setOptionalInt(actingUserID, &run.ActingUserID)
	setOptionalInt(rootUserID, &run.RootInitiatorUserID)
	setOptionalInt(immediateUserID, &run.ImmediateTriggerUserID)
	setOptionalInt(parentRunID, &run.ParentRunID)
	if profileVersion.Valid {
		run.ProfileVersion = int(profileVersion.Int64)
	}
	run.SessionID = sessionID.String
	run.GrantsJSON = grantsJSON.String
	run.ProfileSnapshotJSON = profileSnapshotJSON.String
	if jobImage.Valid {
		run.JobImage = jobImage.String
	}
	if itemID.Valid {
		v := int(itemID.Int64)
		run.ItemID = &v
	}
	if bindingID.Valid {
		v := int(bindingID.Int64)
		run.BindingID = &v
	}
	if runnerID.Valid {
		v := int(runnerID.Int64)
		run.RunnerID = &v
	}
	if targetPoolID.Valid {
		v := int(targetPoolID.Int64)
		run.TargetPoolID = &v
	}
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if endedAt.Valid {
		run.EndedAt = &endedAt.Time
	}
	if containerID.Valid {
		run.ContainerID = containerID.String
	}
	if errMsg.Valid {
		run.Error = errMsg.String
	}
	return run, nil
}

// CountForBindingSince returns the number of runs created against the
// given binding at or after `since`. Used to enforce a binding's
// max_runs_per_day budget before admitting a new run. Returns 0 when
// bindingID is 0 — callers that don't have a binding shouldn't be
// enforcing a per-binding budget in the first place.
func (r *AgentRunRepository) CountForBindingSince(ctx context.Context, bindingID int, since time.Time) (int, error) {
	if bindingID == 0 {
		return 0, nil
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_runs
		WHERE binding_id = ? AND created_at >= ?
	`, bindingID, since)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count runs for binding: %w", err)
	}
	return n, nil
}

// CountActiveForBindingItem returns how many queued or running runs the
// binding currently has on the given item. The comment-@mention trigger's
// dedup check (WI-264): a mention of an agent that is already working on
// the item must not stack a second run.
func (r *AgentRunRepository) CountActiveForBindingItem(ctx context.Context, bindingID, itemID int) (int, error) {
	if bindingID == 0 || itemID == 0 {
		return 0, nil
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_runs
		WHERE binding_id = ? AND item_id = ? AND status IN (?, ?)
	`, bindingID, itemID, models.AgentRunStatusQueued, models.AgentRunStatusRunning)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count active runs for binding item: %w", err)
	}
	return n, nil
}

// MarkRunning transitions a run from queued to running and stamps started_at.
// Callers must hold their admission-control slot before invoking this.
func (r *AgentRunRepository) MarkRunning(ctx context.Context, id int, containerID string, now time.Time) error {
	_, err := r.MarkRunningIfQueued(ctx, id, containerID, now)
	return err
}

// MarkRunningIfQueued is MarkRunning with the CAS outcome surfaced: it
// reports whether the queued→running transition actually happened. The
// in-process queue consumer uses it to skip a dequeued job whose row left
// 'queued' while it sat on the in-memory channel — canceled via the API
// (WI-341) — instead of executing it anyway.
func (r *AgentRunRepository) MarkRunningIfQueued(ctx context.Context, id int, containerID string, now time.Time) (transitioned bool, err error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, started_at = ?, container_id = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`,
		models.AgentRunStatusRunning, now, nullStringArg(containerID), now,
		id, models.AgentRunStatusQueued,
	)
	if err != nil {
		return false, fmt.Errorf("failed to mark agent_run running: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to mark agent_run running: rows affected: %w", err)
	}
	return n > 0, nil
}

// CancelQueued atomically cancels a run that is still queued, using the same
// status-guarded CAS as ClaimQueuedForRunner / FinalizeRunning: the UPDATE only matches
// while status is 'queued', so it can never race a claim — either this wins
// and the run is terminal before anyone executes it (ClaimQueuedForRunner's
// own CAS then skips the row), or a claim won first, zero rows match, and the
// caller falls through to the claimed-run cancel paths. Reports whether the
// transition happened (WI-341).
func (r *AgentRunRepository) CancelQueued(ctx context.Context, id int, now time.Time) (transitioned bool, err error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, ended_at = ?, error = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`,
		models.AgentRunStatusCanceled, now, "canceled while queued", now,
		id, models.AgentRunStatusQueued,
	)
	if err != nil {
		return false, fmt.Errorf("cancel queued: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cancel queued: rows affected: %w", err)
	}
	return n > 0, nil
}

// ListActiveIDsForBinding returns queued/running runs for lifecycle cleanup.
// Binding archive calls the ordinary RunService cancellation path for each ID.
func (r *AgentRunRepository) ListActiveIDsForBinding(ctx context.Context, bindingID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id
		FROM agent_runs
		WHERE binding_id = ? AND status IN (?, ?)
		ORDER BY id
	`, bindingID, models.AgentRunStatusQueued, models.AgentRunStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list active runs for binding: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// StandardQueueKey identifies one independently serialized Standard-agent
// queue. Runs for different profile/item pairs may execute concurrently.
type StandardQueueKey struct {
	BindingID int
	ItemID    int
}

// ClaimNextStandard atomically claims the oldest queued Standard run for one
// profile/item pair, but only while no run for that pair is already running.
func (r *AgentRunRepository) ClaimNextStandard(ctx context.Context, bindingID, itemID int, now time.Time) (*models.AgentRun, error) {
	const maxAttempts = 16
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var id int
		err := r.db.QueryRowContext(ctx, `
			SELECT id FROM agent_runs
			WHERE binding_id = ? AND item_id = ? AND job_kind = ? AND status = ?
			ORDER BY queued_at, id
			LIMIT 1
		`, bindingID, itemID, models.JobKindStandardAgent, models.AgentRunStatusQueued).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("select Standard run for claim: %w", err)
		}
		res, err := r.db.ExecWriteContext(ctx, `
			UPDATE agent_runs
			SET status = ?, started_at = ?, updated_at = ?
			WHERE id = ? AND status = ? AND job_kind = ?
			  AND NOT EXISTS (
				SELECT 1 FROM agent_runs active
				WHERE active.binding_id = agent_runs.binding_id
				  AND active.item_id = agent_runs.item_id
				  AND active.job_kind = ?
				  AND active.status = ?
			  )
		`, models.AgentRunStatusRunning, now, now, id,
			models.AgentRunStatusQueued, models.JobKindStandardAgent,
			models.JobKindStandardAgent, models.AgentRunStatusRunning)
		if err != nil {
			return nil, fmt.Errorf("claim Standard run: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("claim Standard run rows affected: %w", err)
		}
		if n > 0 {
			return r.Get(ctx, id)
		}
		// Another process either claimed this row or currently owns the serial
		// slot. A running owner means this queue is intentionally unavailable.
		var running int
		if err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM agent_runs
			WHERE binding_id = ? AND item_id = ? AND job_kind = ? AND status = ?
		`, bindingID, itemID, models.JobKindStandardAgent, models.AgentRunStatusRunning).Scan(&running); err != nil {
			return nil, fmt.Errorf("check Standard serial owner: %w", err)
		}
		if running > 0 {
			return nil, nil
		}
	}
	return nil, errors.New("claim Standard run: contention limit reached")
}

// ListQueuedStandardKeys returns queues that should be resumed after startup.
func (r *AgentRunRepository) ListQueuedStandardKeys(ctx context.Context) ([]StandardQueueKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT binding_id, item_id
		FROM agent_runs
		WHERE job_kind = ? AND status = ? AND binding_id IS NOT NULL AND item_id IS NOT NULL
		ORDER BY binding_id, item_id
	`, models.JobKindStandardAgent, models.AgentRunStatusQueued)
	if err != nil {
		return nil, fmt.Errorf("list queued Standard queues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StandardQueueKey
	for rows.Next() {
		var key StandardQueueKey
		if err := rows.Scan(&key.BindingID, &key.ItemID); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// HasQueuedStandardRun reports whether one Standard queue still has queued
// work. The targeted EXISTS keeps the worker-exit recheck off the whole-table
// DISTINCT that ListQueuedStandardKeys performs for startup resume.
func (r *AgentRunRepository) HasQueuedStandardRun(ctx context.Context, bindingID, itemID int) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM agent_runs
			WHERE job_kind = ? AND status = ? AND binding_id = ? AND item_id = ?
		)
	`, models.JobKindStandardAgent, models.AgentRunStatusQueued, bindingID, itemID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check queued Standard run: %w", err)
	}
	return exists, nil
}

// FailOrphanedStandardRuns makes restart behavior explicit: in-process runs
// cannot be resumed mid-LLM call, while their queued successors remain durable.
func (r *AgentRunRepository) FailOrphanedStandardRuns(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, ended_at = ?, error = ?, updated_at = ?
		WHERE job_kind = ? AND status = ?
	`, models.AgentRunStatusFailed, now, "Standard agent interrupted by server restart", now,
		models.JobKindStandardAgent, models.AgentRunStatusRunning)
	if err != nil {
		return 0, fmt.Errorf("fail orphaned Standard runs: %w", err)
	}
	return res.RowsAffected()
}

// ListRunningStandard returns in-process calls left running in durable state,
// used during startup repair before they are failed.
func (r *AgentRunRepository) ListRunningStandard(ctx context.Context) ([]*models.AgentRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM agent_runs
		WHERE job_kind = ? AND status = ?
		ORDER BY id
	`, models.JobKindStandardAgent, models.AgentRunStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list running Standard runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*models.AgentRun, 0, len(ids))
	for _, id := range ids {
		run, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

// RunningStandardForActorItem resolves agent-to-agent lineage while the
// parent is still executing its agent-authored final comment.
func (r *AgentRunRepository) RunningStandardForActorItem(ctx context.Context, actingUserID, itemID int) (*models.AgentRun, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM agent_runs
		WHERE job_kind = ? AND acting_user_id = ? AND item_id = ? AND status = ?
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`, models.JobKindStandardAgent, actingUserID, itemID, models.AgentRunStatusRunning).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find Standard parent run: %w", err)
	}
	return r.Get(ctx, id)
}

// ListActiveStandardIDsForBinding supports profile archive cancellation
// without touching Coding/Legacy runner work.
func (r *AgentRunRepository) ListActiveStandardIDsForBinding(ctx context.Context, bindingID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM agent_runs
		WHERE binding_id = ? AND job_kind = ? AND status IN (?, ?)
		ORDER BY id
	`, bindingID, models.JobKindStandardAgent, models.AgentRunStatusQueued, models.AgentRunStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list active Standard runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClaimQueuedForRunner atomically assigns the oldest pool run and marks it
// running. Caller-selected round-robin chooses the runner; status CAS preserves
// FIFO behavior across SQLite and Postgres.
func (r *AgentRunRepository) ClaimQueuedForRunner(ctx context.Context, poolID, nextRunnerID int, now time.Time) (*models.AgentRun, error) {
	const maxAttempts = 16
	for attempt := 0; attempt < maxAttempts; attempt++ {
		row := r.db.QueryRowContext(ctx, `
			SELECT ar.id
			FROM agent_runs ar
			JOIN action_capabilities ac ON ac.id = ar.target_pool_id
			WHERE ar.status = ?
			  AND ar.target_pool_id = ?
			  AND ac.capability_type = ?
			  AND ac.is_enabled = true
			  AND (
				ac.applies_to_all_workspaces = true
				OR EXISTS (
					SELECT 1 FROM action_capability_workspaces acw
					WHERE acw.capability_id = ac.id
					  AND acw.workspace_id = ar.workspace_id
				)
			  )
			ORDER BY ar.queued_at ASC
			LIMIT 1
		`, models.AgentRunStatusQueued, poolID, models.CapabilityRunnerPool)
		var id int
		switch err := row.Scan(&id); err {
		case sql.ErrNoRows:
			return nil, nil
		case nil:
			// fall through to the guarded claim
		default:
			return nil, fmt.Errorf("claim queued: select candidate: %w", err)
		}

		res, err := r.db.ExecWriteContext(ctx, `
			UPDATE agent_runs
			SET status = ?, runner_id = ?, started_at = ?, updated_at = ?
			WHERE id = ? AND status = ?
			  AND EXISTS (
				SELECT 1 FROM action_capabilities ac
				WHERE ac.id = agent_runs.target_pool_id
				  AND ac.capability_type = ?
				  AND ac.is_enabled = true
				  AND (
					ac.applies_to_all_workspaces = true
					OR EXISTS (
						SELECT 1 FROM action_capability_workspaces acw
						WHERE acw.capability_id = ac.id
						  AND acw.workspace_id = agent_runs.workspace_id
					)
				  )
			  )
		`,
			models.AgentRunStatusRunning, nextRunnerID, now, now,
			id, models.AgentRunStatusQueued, models.CapabilityRunnerPool,
		)
		if err != nil {
			return nil, fmt.Errorf("claim queued: mark running: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("claim queued: rows affected: %w", err)
		}
		if n == 1 {
			return r.Get(ctx, id)
		}
		// Lost the race for this candidate; try the next queued run.
	}
	// Heavy contention exhausted the retry budget; the caller polls again.
	return nil, nil
}

// CountQueuedForPool returns the number of queued runs targeted at the given
// pool — the per-pool queue depth an autoscaler scales on (WI-141).
func (r *AgentRunRepository) CountQueuedForPool(ctx context.Context, poolID int) (int, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_runs WHERE status = ? AND target_pool_id = ?
	`, models.AgentRunStatusQueued, poolID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count queued for pool: %w", err)
	}
	return n, nil
}

// CountRunningForPool returns how many runs are currently running on the
// given pool — used to enforce the pool's max-concurrency quota (WI-147)
// before handing out another claim.
func (r *AgentRunRepository) CountRunningForPool(ctx context.Context, poolID int) (int, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_runs WHERE status = ? AND target_pool_id = ?
	`, models.AgentRunStatusRunning, poolID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count running for pool: %w", err)
	}
	return n, nil
}

// RequestCancel flags a running run for cancellation. The runner that owns
// the run learns via its heartbeat and aborts. Idempotent no-op (zero rows)
// when the run is not running or is already flagged.
func (r *AgentRunRepository) RequestCancel(ctx context.Context, runID int, now time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs SET cancel_requested_at = ?, updated_at = ?
		WHERE id = ? AND status = ? AND cancel_requested_at IS NULL
	`, now, now, runID, models.AgentRunStatusRunning)
	if err != nil {
		return fmt.Errorf("request cancel: %w", err)
	}
	return nil
}

// ForceCancelRunning is the admin escape hatch for a phantom running run.
// Its status CAS cannot clobber a concurrent terminal transition.
func (r *AgentRunRepository) ForceCancelRunning(ctx context.Context, runID int, now time.Time) (bool, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, ended_at = ?, error = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`,
		models.AgentRunStatusCanceled, now, "force-canceled by admin", now,
		runID, models.AgentRunStatusRunning,
	)
	if err != nil {
		return false, fmt.Errorf("force cancel running: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("force cancel running: rows affected: %w", err)
	}
	return n > 0, nil
}

// ListAbortableRuns returns the ids of runs the given runner is executing
// that have been flagged for cancellation, so the heartbeat handler can tell
// the runner which jobs to abort.
func (r *AgentRunRepository) ListAbortableRuns(ctx context.Context, runnerInstanceID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM agent_runs
		WHERE runner_id = ? AND status = ? AND cancel_requested_at IS NOT NULL
	`, runnerInstanceID, models.AgentRunStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list abortable runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan abortable run: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReapStaleRuns fails any running run whose owning runner has gone stale —
// revoked, or with a heartbeat older than staleBefore (or never seen and
// registered before staleBefore). It is the liveness backstop for remote
// runs whose runner died mid-execution (WI-141). Returns the number reaped.
func (r *AgentRunRepository) ReapStaleRuns(ctx context.Context, staleBefore, now time.Time) (int, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, error = ?, ended_at = ?, updated_at = ?
		WHERE status = ?
		  AND runner_id IS NOT NULL
		  AND runner_id IN (
		    SELECT id FROM runner_instances
		    WHERE status = ?
		       OR (last_heartbeat_at IS NOT NULL AND last_heartbeat_at < ?)
		       OR (last_heartbeat_at IS NULL AND registered_at < ?)
		  )
	`,
		models.AgentRunStatusFailed, "runner lease expired (missed heartbeat)", now, now,
		models.AgentRunStatusRunning,
		models.RunnerInstanceStatusRevoked, staleBefore, staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("reap stale runs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reap stale runs: rows affected: %w", err)
	}
	return int(n), nil
}

// ReapOrphanedLocalRuns reconciles local queued/running rows before startup.
// Their in-memory workers died with the prior process, so they cannot complete.
func (r *AgentRunRepository) ReapOrphanedLocalRuns(ctx context.Context, now time.Time) (int, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, error = ?, ended_at = ?, updated_at = ?
		WHERE status IN (?, ?)
		  AND runner_id IS NULL
		  AND target_pool_id IS NULL
	`,
		models.AgentRunStatusFailed, "orphaned by restart", now, now,
		models.AgentRunStatusQueued, models.AgentRunStatusRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("reap orphaned local runs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reap orphaned local runs: rows affected: %w", err)
	}
	return int(n), nil
}

// ReapOverdueRuns fails any run that has sat in 'running' since before
// startedBefore, regardless of runner heartbeat. It is the max-run-duration
// backstop (WI-331): a healthy runner whose terminal report was lost keeps
// heartbeating, so ReapStaleRuns never fires and the phantom run would hold a
// pool-concurrency slot and the binding's per-item dedup forever. The same
// bound also covers a claim whose response was lost on the wire (the run is
// 'running' with started_at stamped but no runner ever executes it). A run
// with no started_at (shouldn't happen — every queued→running transition
// stamps it) falls back to queued_at so it cannot dodge the bound. Returns
// the number reaped.
func (r *AgentRunRepository) ReapOverdueRuns(ctx context.Context, startedBefore, now time.Time) (int, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, error = ?, ended_at = ?, updated_at = ?
		WHERE status = ?
		  AND ((started_at IS NOT NULL AND started_at < ?)
		    OR (started_at IS NULL AND queued_at < ?))
	`,
		models.AgentRunStatusFailed, "run exceeded the maximum allowed duration (reaped by the orchestrator backstop)", now, now,
		models.AgentRunStatusRunning, startedBefore, startedBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("reap overdue runs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reap overdue runs: rows affected: %w", err)
	}
	return int(n), nil
}

// StaleQueuedPoolRun is one remote-pool run that has sat queued (unclaimed)
// past the stall threshold. The lease reaper surfaces these so "assigned the
// ticket but nothing happens" is diagnosable from the server log and the
// run's own event stream.
type StaleQueuedPoolRun struct {
	RunID    int
	PoolID   int
	ItemID   *int
	QueuedAt time.Time
}

// ListStaleQueuedPoolRuns returns remote-pool runs still queued since before
// olderThan, oldest first. Local (in-process) runs are excluded — they are
// consumed by the worker pool immediately and have no claim hop to stall on.
func (r *AgentRunRepository) ListStaleQueuedPoolRuns(ctx context.Context, olderThan time.Time) ([]StaleQueuedPoolRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, target_pool_id, item_id, queued_at FROM agent_runs
		WHERE status = ? AND target_pool_id IS NOT NULL AND queued_at < ?
		ORDER BY queued_at ASC
	`, models.AgentRunStatusQueued, olderThan)
	if err != nil {
		return nil, fmt.Errorf("list stale queued pool runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StaleQueuedPoolRun
	for rows.Next() {
		var run StaleQueuedPoolRun
		var itemID sql.NullInt64
		if err := rows.Scan(&run.RunID, &run.PoolID, &itemID, &run.QueuedAt); err != nil {
			return nil, fmt.Errorf("scan stale queued pool run: %w", err)
		}
		if itemID.Valid {
			v := int(itemID.Int64)
			run.ItemID = &v
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// HasEvent reports whether the run already has an event of the given type —
// used to emit one-time warning events without duplicating them every sweep.
func (r *AgentRunRepository) HasEvent(ctx context.Context, runID int, eventType string) (bool, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM agent_run_events WHERE run_id = ? AND type = ?)
	`, runID, eventType)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("has event: %w", err)
	}
	return exists, nil
}

// SetGrants snapshots a run's access-layer grants and binds the run to the
// minted run-token that authorizes them (WI-144). Called from the claim path
// once the grants are derived from the binding.
func (r *AgentRunRepository) SetGrants(ctx context.Context, runID, tokenID int, grants *models.RunGrants, now time.Time) error {
	b, err := json.Marshal(grants)
	if err != nil {
		return fmt.Errorf("set grants: marshal: %w", err)
	}
	if _, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs SET grants_json = ?, run_token_id = ?, updated_at = ?
		WHERE id = ?
	`, string(b), tokenID, now, runID); err != nil {
		return fmt.Errorf("set grants: %w", err)
	}
	return nil
}

// GetRunAuthz returns what a broker needs to authorize a request for a run:
// the id of the token bound to the run (0 if none), the run's grants (nil if
// unset), and the run's current status. Brokers verify the presented token's
// id matches, the status is running, and the resource is in the grants.
func (r *AgentRunRepository) GetRunAuthz(ctx context.Context, runID int) (tokenID, workspaceID int, grants *models.RunGrants, status string, err error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT run_token_id, workspace_id, grants_json, status FROM agent_runs WHERE id = ?
	`, runID)
	var tid sql.NullInt64
	var grantsJSON sql.NullString
	if err := row.Scan(&tid, &workspaceID, &grantsJSON, &status); err != nil {
		return 0, 0, nil, "", err
	}
	if grantsJSON.Valid && grantsJSON.String != "" {
		grants = &models.RunGrants{}
		if err := json.Unmarshal([]byte(grantsJSON.String), grants); err != nil {
			return 0, 0, nil, "", fmt.Errorf("get run authz: unmarshal grants: %w", err)
		}
	}
	return int(tid.Int64), workspaceID, grants, status, nil
}

// GetRunByTokenID resolves the run bound to a given run-token id — used by
// the git broker, where the run id is not in the URL (the clone URL is
// stable/repo-scoped) so the presented token is what identifies the run.
func (r *AgentRunRepository) GetRunByTokenID(ctx context.Context, tokenID int) (runID, workspaceID int, grants *models.RunGrants, status string, err error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, grants_json, status FROM agent_runs WHERE run_token_id = ?
	`, tokenID)
	var grantsJSON sql.NullString
	if err := row.Scan(&runID, &workspaceID, &grantsJSON, &status); err != nil {
		return 0, 0, nil, "", notFoundOrWrap(err, "get run by token")
	}
	if grantsJSON.Valid && grantsJSON.String != "" {
		grants = &models.RunGrants{}
		if err := json.Unmarshal([]byte(grantsJSON.String), grants); err != nil {
			return 0, 0, nil, "", fmt.Errorf("get run by token: unmarshal grants: %w", err)
		}
	}
	return runID, workspaceID, grants, status, nil
}

// SetContainerID records the spawned container id on an existing run row.
// MarkRunning's queued→running transition is intentionally guarded by
// status, so the runner uses this separate path once it actually has a
// container handle to stamp (which may be after MarkRunning has already
// flipped the status).
func (r *AgentRunRepository) SetContainerID(ctx context.Context, id int, containerID string) error {
	if containerID == "" {
		return nil
	}
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET container_id = ?, updated_at = ?
		WHERE id = ?
	`, containerID, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to set agent_run container_id: %w", err)
	}
	return nil
}

// Finalize stamps a terminal status + ended_at + error message. Status must
// be one of the terminal values (see IsAgentRunTerminal). errMsg is stored
// verbatim; pass "" for successful runs.
func (r *AgentRunRepository) Finalize(ctx context.Context, id int, status, errMsg string, now time.Time) error {
	if !models.IsAgentRunTerminal(status) {
		return fmt.Errorf("agent_run finalize: %q is not a terminal status", status)
	}
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, ended_at = ?, error = ?, updated_at = ?
		WHERE id = ?
	`,
		status, now, nullStringArg(errMsg), now, id,
	)
	if err != nil {
		return fmt.Errorf("failed to finalize agent_run: %w", err)
	}
	return nil
}

// FinalizeRunning is the compare-and-swap finalize for the untrusted remote
// path (WI-168): it stamps a terminal status only if the run is still
// 'running', and reports whether the transition actually happened. A remote
// runner credential can therefore not rewrite a run that has already
// finalized (or been canceled by the orchestrator) — the UPDATE matches zero
// rows and transitioned is false, so the caller can skip re-firing terminal
// events / the post-run PR hook. Trusted in-process finalization uses the
// unconditional Finalize.
func (r *AgentRunRepository) FinalizeRunning(ctx context.Context, id int, status, errMsg string, now time.Time) (transitioned bool, err error) {
	if !models.IsAgentRunTerminal(status) {
		return false, fmt.Errorf("agent_run finalize: %q is not a terminal status", status)
	}
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, ended_at = ?, error = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`,
		status, now, nullStringArg(errMsg), now, id, models.AgentRunStatusRunning,
	)
	if err != nil {
		return false, fmt.Errorf("failed to finalize agent_run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read finalize result: %w", err)
	}
	return n > 0, nil
}

// AppendEvent records one entry on the run's event stream. payloadJSON must
// be a JSON document (valid object, array, or scalar); the column type is
// JSONB on Postgres and TEXT on SQLite, but we treat it as opaque here.
//
// payloadJSON is scrubbed before persistence so token-bearing output (URL
// credentials, bearer headers, env assignments, broker tokens, JSON secret
// fields, etc.) cannot leak into the event stream that's visible to every item
// viewer in the workspace.
func (r *AgentRunRepository) AppendEvent(ctx context.Context, runID int, eventType, payloadJSON string) error {
	if payloadJSON == "" {
		payloadJSON = "{}"
	}
	_, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO agent_run_events(run_id, type, payload_json)
		VALUES (?, ?, ?)
	`, runID, eventType, redact.String(payloadJSON))
	if err != nil {
		return fmt.Errorf("failed to append agent_run_event: %w", err)
	}
	return nil
}

// ListForWorkspace returns the most recent N runs in the workspace,
// newest first. Used by the workspace-admin runs list. beforeID is for
// cursor pagination ("give me runs with id < beforeID"); pass 0 for the
// first page. Empty result is not an error.
func (r *AgentRunRepository) ListForWorkspace(ctx context.Context, workspaceID, limit, beforeID int) ([]*models.AgentRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return r.listRuns(ctx, "workspace_id", workspaceID, limit, beforeID)
}

// ListForItem returns the most recent N runs triggered against the given
// work item, newest first — the item detail "Agent log" tab (WI-260).
// Same cursor semantics as ListForWorkspace.
func (r *AgentRunRepository) ListForItem(ctx context.Context, itemID, limit, beforeID int) ([]*models.AgentRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return r.listRuns(ctx, "item_id", itemID, limit, beforeID)
}

func (r *AgentRunRepository) listRuns(ctx context.Context, scopeColumn string, scopeID, limit, beforeID int) ([]*models.AgentRun, error) {
	query := `
		SELECT id, workspace_id, item_id, binding_id, status, queued_at, started_at, ended_at,
		       container_id, error, triggered_by_user_id, job_kind, is_ephemeral, acting_user_id,
		       root_initiator_user_id, immediate_trigger_user_id, parent_run_id,
		       chain_depth, session_id, profile_version, created_at, updated_at
		FROM agent_runs
		WHERE ` + scopeColumn + ` = ? AND is_ephemeral = ?
	`
	args := []any{scopeID, false}
	if beforeID > 0 {
		query += " AND id < ?"
		args = append(args, beforeID)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by %s: %w", scopeColumn, err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.AgentRun
	for rows.Next() {
		run := &models.AgentRun{}
		var itemID, bindingID, triggeredBy, actingUserID, rootUserID, immediateUserID, parentRunID, profileVersion sql.NullInt64
		var startedAt, endedAt sql.NullTime
		var containerID, errMsg, sessionID sql.NullString
		if err := rows.Scan(
			&run.ID, &run.WorkspaceID, &itemID, &bindingID, &run.Status,
			&run.QueuedAt, &startedAt, &endedAt,
			&containerID, &errMsg, &triggeredBy, &run.JobKind, &run.IsEphemeral, &actingUserID,
			&rootUserID, &immediateUserID, &parentRunID, &run.ChainDepth,
			&sessionID, &profileVersion, &run.CreatedAt, &run.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan run row: %w", err)
		}
		if itemID.Valid {
			v := int(itemID.Int64)
			run.ItemID = &v
		}
		if triggeredBy.Valid {
			v := int(triggeredBy.Int64)
			run.TriggeredByUserID = &v
		}
		if bindingID.Valid {
			v := int(bindingID.Int64)
			run.BindingID = &v
		}
		setOptionalInt := func(col sql.NullInt64, dst **int) {
			if col.Valid {
				v := int(col.Int64)
				*dst = &v
			}
		}
		setOptionalInt(actingUserID, &run.ActingUserID)
		setOptionalInt(rootUserID, &run.RootInitiatorUserID)
		setOptionalInt(immediateUserID, &run.ImmediateTriggerUserID)
		setOptionalInt(parentRunID, &run.ParentRunID)
		if profileVersion.Valid {
			run.ProfileVersion = int(profileVersion.Int64)
		}
		run.SessionID = sessionID.String
		if startedAt.Valid {
			run.StartedAt = &startedAt.Time
		}
		if endedAt.Valid {
			run.EndedAt = &endedAt.Time
		}
		if containerID.Valid {
			run.ContainerID = containerID.String
		}
		if errMsg.Valid {
			run.Error = errMsg.String
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ListEventsAfter is the polling-style stream the UI uses to tail a
// run's event log: returns up to `limit` events with id > afterID,
// ordered by id ASC (insertion order). Empty result means "no new
// events since afterID."
func (r *AgentRunRepository) ListEventsAfter(ctx context.Context, runID, afterID, limit int) ([]*models.AgentRunEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, ts, type, payload_json
		FROM agent_run_events
		WHERE run_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, runID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events after: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.AgentRunEvent
	for rows.Next() {
		ev := &models.AgentRunEvent{}
		if err := rows.Scan(&ev.ID, &ev.RunID, &ev.Timestamp, &ev.Type, &ev.PayloadJSON); err != nil {
			return nil, fmt.Errorf("scan event row: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// ListEvents returns events for a run ordered chronologically (by id ASC,
// which matches insertion order in both backends). Used by the SSE backfill
// when a client connects mid-run.
func (r *AgentRunRepository) ListEvents(ctx context.Context, runID int) ([]*models.AgentRunEvent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, ts, type, payload_json
		FROM agent_run_events
		WHERE run_id = ?
		ORDER BY id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent_run_events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.AgentRunEvent
	for rows.Next() {
		ev := &models.AgentRunEvent{}
		if err := rows.Scan(&ev.ID, &ev.RunID, &ev.Timestamp, &ev.Type, &ev.PayloadJSON); err != nil {
			return nil, fmt.Errorf("failed to scan agent_run_event: %w", err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent_run_events: %w", err)
	}
	return out, nil
}
