package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// approvalTestEnv is a fully-initialized in-memory DB plus the entities the
// approval flow needs: a workspace, a workflow with three statuses (Open,
// Review, Approved, Rejected), four transitions (Open→Review, Review→Approved,
// Review→Rejected, Review→Open), an item starting in Review, and a configured
// ApprovalService.
//
// Tests build approval sets on top of this base and drive the service.
type approvalTestEnv struct {
	t               *testing.T
	db              database.Database
	workflowService *WorkflowService
	permService     *PermissionService
	leaveRepo       *repository.LeaveRepository
	approvalService *ApprovalService

	workspaceID         int
	itemID              int
	itemTypeID          int
	configurationSetID  int
	workflowID          int
	statusOpenID        int
	statusReviewID      int
	statusApprovedID    int
	statusRejectedID    int
	transitionOpenToReview     int
	transitionReviewToApproved int
	transitionReviewToRejected int
	transitionReviewToOpen     int

	requestor int
	approver1 int
	approver2 int
	approver3 int
}

func newApprovalTestEnv(t *testing.T) *approvalTestEnv {
	t.Helper()

	dsn := fmt.Sprintf("file:approvals-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	env := &approvalTestEnv{t: t, db: db}
	permService, err := NewPermissionService(db, DefaultPermissionCacheConfig())
	if err != nil {
		t.Fatalf("permission service: %v", err)
	}
	env.permService = permService
	env.workflowService = NewWorkflowService(db)
	env.leaveRepo = repository.NewLeaveRepository(db)
	env.approvalService = NewApprovalService(db, env.permService, env.leaveRepo, env.workflowService)

	env.bootstrap()
	return env
}

// bootstrap creates the workspace, users, statuses, workflow, transitions,
// item, configuration set, and ties them together. It does NOT create the
// approval set — tests do that themselves with the desired step config.
func (e *approvalTestEnv) bootstrap() {
	t := e.t
	t.Helper()

	// Three users.
	e.requestor = e.insertUser("alice@example.com", "alice")
	e.approver1 = e.insertUser("bob@example.com", "bob")
	e.approver2 = e.insertUser("carol@example.com", "carol")
	e.approver3 = e.insertUser("dave@example.com", "dave")

	// Workspace (non-personal so the approval pipeline runs).
	res, err := e.db.Exec(`INSERT INTO workspaces (name, key, active, is_personal) VALUES (?, ?, 1, 0)`, "TestWS", "TWS")
	if err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	wid, _ := res.LastInsertId()
	e.workspaceID = int(wid)

	// Workflow + statuses + transitions. Initialize() seeds default categories;
	// reuse them so statuses satisfy FK to status_categories.
	var todoCat int
	if err := e.db.QueryRow(`SELECT id FROM status_categories WHERE is_default = true LIMIT 1`).Scan(&todoCat); err != nil {
		// fall back to any category
		_ = e.db.QueryRow(`SELECT id FROM status_categories LIMIT 1`).Scan(&todoCat)
	}

	res, err = e.db.Exec(`INSERT INTO workflows (name, description, is_default) VALUES ('test-wf', '', false)`)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	wfID, _ := res.LastInsertId()
	e.workflowID = int(wfID)

	// Statuses must be globally unique by name; namespace by test name.
	prefix := strings.ReplaceAll(t.Name(), "/", "_")
	e.statusOpenID = e.insertStatus(prefix+"-Open", todoCat)
	e.statusReviewID = e.insertStatus(prefix+"-Review", todoCat)
	e.statusApprovedID = e.insertStatus(prefix+"-Approved", todoCat)
	e.statusRejectedID = e.insertStatus(prefix+"-Rejected", todoCat)

	e.transitionOpenToReview = e.insertTransition(e.workflowID, e.statusOpenID, e.statusReviewID)
	e.transitionReviewToApproved = e.insertTransition(e.workflowID, e.statusReviewID, e.statusApprovedID)
	e.transitionReviewToRejected = e.insertTransition(e.workflowID, e.statusReviewID, e.statusRejectedID)
	e.transitionReviewToOpen = e.insertTransition(e.workflowID, e.statusReviewID, e.statusOpenID)

	// Item type + configuration set with workspace_configuration_sets link, so
	// GetApprovalSetIDForItem can resolve. item_types.name is globally unique;
	// namespace by test.
	res, err = e.db.Exec(`INSERT INTO item_types (name, hierarchy_level) VALUES (?, 3)`, prefix+"-Task")
	if err != nil {
		t.Fatalf("insert item_type: %v", err)
	}
	itID, _ := res.LastInsertId()
	e.itemTypeID = int(itID)

	res, err = e.db.Exec(`INSERT INTO configuration_sets (name, workflow_id) VALUES ('cs', ?)`, e.workflowID)
	if err != nil {
		t.Fatalf("insert configuration_set: %v", err)
	}
	csID, _ := res.LastInsertId()
	e.configurationSetID = int(csID)
	if _, err := e.db.Exec(`INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id) VALUES (?, ?)`,
		e.workspaceID, e.configurationSetID); err != nil {
		t.Fatalf("link wcs: %v", err)
	}

	// Item, starting in Review — that's the approval-bound status the tests gate.
	res, err = e.db.Exec(`
		INSERT INTO items (workspace_id, workspace_item_number, item_type_id, title, description,
		                   status_id, assignee_id, creator_id, custom_field_values)
		VALUES (?, 1, ?, 'Test', '', ?, ?, ?, '{}')
	`, e.workspaceID, e.itemTypeID, e.statusReviewID, e.approver1, e.requestor)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}
	iid, _ := res.LastInsertId()
	e.itemID = int(iid)
}

func (e *approvalTestEnv) insertUser(email, username string) int {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO users (email, username, first_name, last_name) VALUES (?, ?, ?, '')`, email, username, username)
	if err != nil {
		e.t.Fatalf("insert user %s: %v", email, err)
	}
	uid, _ := res.LastInsertId()
	return int(uid)
}

func (e *approvalTestEnv) insertStatus(name string, categoryID int) int {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO statuses (name, category_id) VALUES (?, ?)`, name, categoryID)
	if err != nil {
		e.t.Fatalf("insert status %s: %v", name, err)
	}
	sid, _ := res.LastInsertId()
	return int(sid)
}

func (e *approvalTestEnv) insertTransition(workflowID, fromID, toID int) int {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id) VALUES (?, ?, ?)`,
		workflowID, fromID, toID)
	if err != nil {
		e.t.Fatalf("insert transition: %v", err)
	}
	tid, _ := res.LastInsertId()
	return int(tid)
}

// approvalSetSpec is a compact builder for the test's approval set + status + steps.
type approvalSetSpec struct {
	stepMode string
	steps    []approvalStepSpec
}

type approvalStepSpec struct {
	displayOrder int
	name         string
	quorumMode   string
	quorumCount  *int
	rejection    string
	source       string
	userIDs      []int // used when source == "user" — first id only
	roleID       *int
	allowSelf    bool
	onLeave      string
	escalateAfter   *int
	escalateAction  string
	escalateTarget  string
	escalateUserID  *int
	maxEscalations  *int
}

// createApprovalSet inserts an approval_sets row, attaches an approval_set_status
// to env.statusReviewID with the configured approve/deny transitions, then writes
// step rows. Returns the approval_set_id.
//
// Multiple steps with source="user" share env.approver1/2/3 in order — caller
// can override per spec via userIDs.
func (e *approvalTestEnv) createApprovalSet(spec approvalSetSpec) int {
	t := e.t
	t.Helper()
	res, err := e.db.Exec(`INSERT INTO approval_sets (name, workflow_id) VALUES ('test-set', ?)`, e.workflowID)
	if err != nil {
		t.Fatalf("insert approval_set: %v", err)
	}
	setID64, _ := res.LastInsertId()
	setID := int(setID64)

	stepMode := spec.stepMode
	if stepMode == "" {
		stepMode = models.ApprovalStepModeSequential
	}
	res, err = e.db.Exec(`
		INSERT INTO approval_set_statuses
			(approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode)
		VALUES (?, ?, ?, ?, ?)
	`, setID, e.statusReviewID, e.transitionReviewToApproved, e.transitionReviewToRejected, stepMode)
	if err != nil {
		t.Fatalf("insert approval_set_status: %v", err)
	}
	assID64, _ := res.LastInsertId()
	assID := int(assID64)

	for _, step := range spec.steps {
		var userID interface{}
		if step.source == models.ApprovalSourceUser && len(step.userIDs) > 0 {
			userID = step.userIDs[0]
		}
		_, err := e.db.Exec(`
			INSERT INTO approval_steps (approval_set_status_id, display_order, name,
				quorum_mode, quorum_count, rejection_policy,
				approver_source, approver_user_id, approver_role_id, allow_self_approval,
				on_leave_strategy,
				escalation_after_hours, escalation_action,
				escalation_target_source, escalation_target_user_id, max_escalations)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			assID, step.displayOrder, step.name,
			defaultStr(step.quorumMode, models.ApprovalQuorumModeAny), step.quorumCount, defaultStr(step.rejection, models.ApprovalRejectionPolicyAnyFails),
			step.source, userID, step.roleID, boolToInt(step.allowSelf),
			defaultStr(step.onLeave, models.ApprovalOnLeaveUseSubstitute),
			step.escalateAfter, defaultStr(step.escalateAction, ""),
			defaultStr(step.escalateTarget, ""), step.escalateUserID, step.maxEscalations,
		)
		if err != nil {
			t.Fatalf("insert approval_step: %v", err)
		}
	}

	// Wire the approval set into the workspace's configuration set.
	if _, err := e.db.Exec(`UPDATE configuration_sets SET approval_set_id = ? WHERE id = ?`, setID, e.configurationSetID); err != nil {
		t.Fatalf("attach approval_set to configuration_set: %v", err)
	}
	return setID
}

// userStep returns a single-user-source step spec for compactness.
func userStep(order int, name string, userID int, opts ...func(*approvalStepSpec)) approvalStepSpec {
	s := approvalStepSpec{
		displayOrder: order,
		name:         name,
		quorumMode:   models.ApprovalQuorumModeAny,
		source:       models.ApprovalSourceUser,
		userIDs:      []int{userID},
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intPtr(i int) *int { return &i }

// itemStatusID returns the current status_id of env.itemID.
func (e *approvalTestEnv) itemStatusID() int {
	e.t.Helper()
	var sid sql.NullInt64
	if err := e.db.QueryRow(`SELECT status_id FROM items WHERE id = ?`, e.itemID).Scan(&sid); err != nil {
		e.t.Fatalf("item status: %v", err)
	}
	if !sid.Valid {
		return 0
	}
	return int(sid.Int64)
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

func TestApproval_Sequential_QuorumAny_Approves(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		stepMode: models.ApprovalStepModeSequential,
		steps: []approvalStepSpec{
			userStep(0, "First", env.approver1),
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if req == nil {
		t.Fatal("RequestApproval returned nil — approval-set-status not resolved")
	}
	if req.Status != models.ApprovalRequestStatusPending {
		t.Fatalf("request status = %s, want pending", req.Status)
	}
	if got := len(req.StepInstances); got != 1 {
		t.Fatalf("step instances = %d, want 1", got)
	}
	if got := len(req.StepInstances[0].Approvers); got != 1 ||
		req.StepInstances[0].Approvers[0].UserID == nil ||
		*req.StepInstances[0].Approvers[0].UserID != env.approver1 {
		t.Fatalf("step pool = %#v, want [%d]", req.StepInstances[0].Approvers, env.approver1)
	}

	_, out, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "ok", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide approve: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("after final approve, request = %s, want approved", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusApprovedID {
		t.Fatalf("item status = %d, want %d (Approved)", got, env.statusApprovedID)
	}
}

func TestApproval_Sequential_RejectFiresDeny(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	_, out, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionReject, "no", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide reject: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusRejected {
		t.Fatalf("request = %s, want rejected", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusRejectedID {
		t.Fatalf("item status = %d, want %d (Rejected)", got, env.statusRejectedID)
	}
}

func TestApproval_Sequential_TwoSteps_AdvanceAndApprove(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			userStep(0, "First", env.approver1),
			userStep(1, "Second", env.approver2),
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if got := len(req.StepInstances); got != 2 {
		t.Fatalf("step instances = %d, want 2", got)
	}
	if req.StepInstances[1].StartedAt != nil {
		t.Fatalf("step 2 should not be started yet")
	}

	_, _, err = env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide step1: %v", err)
	}

	out, err := env.approvalService.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusPending {
		t.Fatalf("after step1, request = %s, want pending", out.Status)
	}
	if out.StepInstances[1].StartedAt == nil {
		t.Fatal("step 2 should have started after step 1 approved")
	}
	if got := len(out.StepInstances[1].Approvers); got != 1 {
		t.Fatalf("step 2 pool = %d, want 1", got)
	}

	_, out, err = env.approvalService.Decide(context.Background(), req.ID, env.approver2, models.ApprovalDecisionApprove, "", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide step2: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("after step2, request = %s, want approved", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusApprovedID {
		t.Fatalf("item status = %d, want approved", got)
	}
}

func TestApproval_Parallel_AllApprove(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		stepMode: models.ApprovalStepModeParallel,
		steps: []approvalStepSpec{
			userStep(0, "Eng", env.approver1),
			userStep(1, "Legal", env.approver2),
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	for i, si := range req.StepInstances {
		if si.StartedAt == nil {
			t.Fatalf("parallel step %d not started at request time", i)
		}
	}

	_, out, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide eng: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusPending {
		t.Fatalf("request = %s, want pending after first parallel approve", out.Status)
	}

	_, out, err = env.approvalService.Decide(context.Background(), req.ID, env.approver2, models.ApprovalDecisionApprove, "", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide legal: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("request = %s, want approved", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusApprovedID {
		t.Fatalf("item status = %d, want approved", got)
	}
}

func TestApproval_Parallel_OneRejectsCancelsPeers(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		stepMode: models.ApprovalStepModeParallel,
		steps: []approvalStepSpec{
			userStep(0, "Eng", env.approver1),
			userStep(1, "Legal", env.approver2),
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	_, out, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionReject, "no", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide reject: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusRejected {
		t.Fatalf("request = %s, want rejected", out.Status)
	}
	// Peer step should be skipped, not pending.
	for _, si := range out.StepInstances {
		if si.ID == req.StepInstances[0].ID {
			continue
		}
		if si.Status != models.ApprovalStepStatusSkipped {
			t.Fatalf("peer step %d = %s, want skipped", si.ID, si.Status)
		}
	}
	if got := env.itemStatusID(); got != env.statusRejectedID {
		t.Fatalf("item status = %d, want rejected", got)
	}
}

func TestApproval_Quorum_Count(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder: 0,
				name:         "Three of three",
				quorumMode:   models.ApprovalQuorumModeCount,
				quorumCount:  intPtr(2),
				source:       models.ApprovalSourceRole,
				roleID:       intPtr(1), // wired below via direct insert
			},
		},
	})

	// Seed a workspace role and assign approver1+approver2+approver3 to it.
	res, err := env.db.Exec(`INSERT INTO workspace_roles (name) VALUES ('reviewer')`)
	if err != nil {
		t.Fatalf("insert role: %v", err)
	}
	roleID64, _ := res.LastInsertId()
	roleID := int(roleID64)
	if _, err := env.db.Exec(`UPDATE approval_steps SET approver_role_id = ? WHERE approver_role_id = 1`, roleID); err != nil {
		t.Fatalf("update role id on step: %v", err)
	}
	for _, uid := range []int{env.approver1, env.approver2, env.approver3} {
		if _, err := env.db.Exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?, ?, ?)`, uid, env.workspaceID, roleID); err != nil {
			t.Fatalf("assign role: %v", err)
		}
	}

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if got := len(req.StepInstances[0].Approvers); got != 3 {
		t.Fatalf("pool = %d, want 3", got)
	}

	// 1/3 → still pending.
	_, out, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide #1: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusPending {
		t.Fatalf("after 1/3 approves, request = %s, want pending", out.Status)
	}
	// 2/3 → quorum hit.
	_, out, err = env.approvalService.Decide(context.Background(), req.ID, env.approver2, models.ApprovalDecisionApprove, "", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide #2: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("after 2/3 approves, request = %s, want approved", out.Status)
	}
}

func TestApproval_Decision_DoubleVoteRejected(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder: 0,
				name:         "All three",
				quorumMode:   models.ApprovalQuorumModeAll,
				source:       models.ApprovalSourceRole,
				roleID:       intPtr(1),
			},
		},
	})
	res, err := env.db.Exec(`INSERT INTO workspace_roles (name) VALUES ('reviewer')`)
	if err != nil {
		t.Fatalf("insert role: %v", err)
	}
	rid64, _ := res.LastInsertId()
	roleID := int(rid64)
	if _, err := env.db.Exec(`UPDATE approval_steps SET approver_role_id = ? WHERE approver_role_id = 1`, roleID); err != nil {
		t.Fatalf("update role: %v", err)
	}
	for _, uid := range []int{env.approver1, env.approver2, env.approver3} {
		if _, err := env.db.Exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?, ?, ?)`, uid, env.workspaceID, roleID); err != nil {
			t.Fatalf("assign role: %v", err)
		}
	}

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if _, _, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "", DecideOptions{}); err != nil {
		t.Fatalf("first vote: %v", err)
	}
	if _, _, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "", DecideOptions{}); err == nil {
		t.Fatal("expected double-vote to be rejected by unique index")
	}
}

func TestApproval_OnLeave_UsesSubstitute(t *testing.T) {
	env := newApprovalTestEnv(t)
	// approver1 is on leave with approver3 as substitute.
	now := time.Now()
	if _, err := env.db.Exec(`
		INSERT INTO user_leave_periods (user_id, substitute_user_id, start_date, end_date, reason, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'pto', 1, ?, ?)
	`, env.approver1, env.approver3, now.Add(-24*time.Hour), now.Add(24*time.Hour), now, now); err != nil {
		t.Fatalf("seed leave: %v", err)
	}

	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if got := len(req.StepInstances[0].Approvers); got != 1 {
		t.Fatalf("pool = %d, want 1 substitute", got)
	}
	app := req.StepInstances[0].Approvers[0]
	if app.UserID == nil || *app.UserID != env.approver3 {
		t.Fatalf("approver = %v, want substitute %d", app.UserID, env.approver3)
	}
	if app.SubstitutedForUserID == nil || *app.SubstitutedForUserID != env.approver1 {
		t.Fatalf("substituted_for_user_id = %v, want %d", app.SubstitutedForUserID, env.approver1)
	}
}

func TestApproval_Escalate_AutoReject(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder:    0,
				name:            "First",
				quorumMode:      models.ApprovalQuorumModeAny,
				source:          models.ApprovalSourceUser,
				userIDs:         []int{env.approver1},
				escalateAfter:   intPtr(1),
				escalateAction:  models.ApprovalEscalationActionAutoReject,
			},
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	si := req.StepInstances[0]

	if err := env.approvalService.Escalate(context.Background(), si.ID, 0, "timeout"); err != nil {
		t.Fatalf("Escalate: %v", err)
	}

	out, err := env.approvalService.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusRejected {
		t.Fatalf("request = %s, want rejected", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusRejectedID {
		t.Fatalf("item status = %d, want rejected", got)
	}
}

func TestApproval_Escalate_SkipStep(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder:   0,
				name:           "Skippable",
				quorumMode:     models.ApprovalQuorumModeAny,
				source:         models.ApprovalSourceUser,
				userIDs:        []int{env.approver1},
				escalateAfter:  intPtr(1),
				escalateAction: models.ApprovalEscalationActionSkipStep,
			},
			userStep(1, "Required", env.approver2),
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if err := env.approvalService.Escalate(context.Background(), req.StepInstances[0].ID, 0, "timeout"); err != nil {
		t.Fatalf("Escalate: %v", err)
	}

	out, err := env.approvalService.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	// First step: escalated; second step should now be active.
	if out.StepInstances[0].Status != models.ApprovalStepStatusEscalated {
		t.Fatalf("step1 = %s, want escalated", out.StepInstances[0].Status)
	}
	if out.StepInstances[1].StartedAt == nil {
		t.Fatal("step2 should have started")
	}

	if _, _, err := env.approvalService.Decide(context.Background(), req.ID, env.approver2, models.ApprovalDecisionApprove, "", DecideOptions{}); err != nil {
		t.Fatalf("Decide step2: %v", err)
	}
	out, _ = env.approvalService.GetRequest(req.ID)
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("after step2 approve, request = %s, want approved", out.Status)
	}
}

func TestApproval_Cancel_PendingRequest(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})
	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if err := env.approvalService.Cancel(context.Background(), req.ID, env.requestor, "changed mind", "manual"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	out, _ := env.approvalService.GetRequest(req.ID)
	if out.Status != models.ApprovalRequestStatusCancelled {
		t.Fatalf("request = %s, want cancelled", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusOpenID {
		t.Fatalf("item status not reverted: got %d, want statusOpenID=%d", got, env.statusOpenID)
	}
}

// Cancel records reverted_to_status_id in the audit decision metadata when the
// item is rolled back, and writes an item_history row for the revert.
func TestApproval_Cancel_RevertsItemStatusAndAudit(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})
	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if err := env.approvalService.Cancel(context.Background(), req.ID, env.requestor, "", "manual"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// item_history row for the revert.
	var histCount int
	if err := env.db.QueryRow(`
		SELECT COUNT(*) FROM item_history
		WHERE item_id = ? AND field_name = 'status_id' AND new_value = ?
	`, env.itemID, fmt.Sprintf("%d", env.statusOpenID)).Scan(&histCount); err != nil {
		t.Fatalf("item_history query: %v", err)
	}
	if histCount == 0 {
		t.Fatalf("expected item_history row for revert; found none")
	}

	// Audit metadata records reverted_to_status_id.
	var meta sql.NullString
	if err := env.db.QueryRow(`
		SELECT metadata FROM approval_decisions WHERE approval_request_id = ? AND decision = 'cancel'
	`, req.ID).Scan(&meta); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if !meta.Valid || !strings.Contains(meta.String, "reverted_to_status_id") {
		t.Fatalf("audit metadata missing reverted_to_status_id: %v", meta.String)
	}
}

// When the item has drifted to a different status before Cancel runs (e.g., a
// user manually transitioned out via a non-gated path), Cancel must still mark
// the request cancelled but skip the revert and record the reason.
func TestApproval_Cancel_NoRevertWhenItemDrifted(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})
	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	// Drift the item to a different status outside the approval flow.
	if _, err := env.db.Exec(`UPDATE items SET status_id = ? WHERE id = ?`, env.statusApprovedID, env.itemID); err != nil {
		t.Fatalf("drift item: %v", err)
	}

	if err := env.approvalService.Cancel(context.Background(), req.ID, env.requestor, "", "manual"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := env.itemStatusID(); got != env.statusApprovedID {
		t.Fatalf("item status flipped under drift: got %d, want %d", got, env.statusApprovedID)
	}
	var meta sql.NullString
	if err := env.db.QueryRow(`
		SELECT metadata FROM approval_decisions WHERE approval_request_id = ? AND decision = 'cancel'
	`, req.ID).Scan(&meta); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if !meta.Valid || !strings.Contains(meta.String, "status_drift") {
		t.Fatalf("audit metadata missing status_drift skip reason: %v", meta.String)
	}
}

// When from_status_id is NULL (pre-migration row, or the prior status was
// deleted via the ON DELETE SET NULL FK), Cancel must skip the revert and
// record pre_migration in the audit.
func TestApproval_Cancel_NoRevertWhenFromStatusNull(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})
	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	if _, err := env.db.Exec(`UPDATE approval_requests SET from_status_id = NULL WHERE id = ?`, req.ID); err != nil {
		t.Fatalf("null from_status_id: %v", err)
	}

	if err := env.approvalService.Cancel(context.Background(), req.ID, env.requestor, "", "manual"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := env.itemStatusID(); got != env.statusReviewID {
		t.Fatalf("item status changed despite NULL from-status: got %d, want %d", got, env.statusReviewID)
	}
	var meta sql.NullString
	if err := env.db.QueryRow(`
		SELECT metadata FROM approval_decisions WHERE approval_request_id = ? AND decision = 'cancel'
	`, req.ID).Scan(&meta); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if !meta.Valid || !strings.Contains(meta.String, "pre_migration") {
		t.Fatalf("audit metadata missing pre_migration skip reason: %v", meta.String)
	}
}

// Cancel's revert calls CommitTransition directly (not PerformTransition), so
// it must NOT open a fresh approval gate even if the from-status itself is
// configured to require approval.
func TestApproval_Cancel_RevertDoesNotReopenGate(t *testing.T) {
	env := newApprovalTestEnv(t)
	// Configure approval gates on BOTH the destination status (Review) and the
	// origin status (Open). The Open gate would normally trigger when an item
	// enters that status; the revert must bypass it.
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})
	// Add a second approval_set_status row targeting Open. We re-use the same
	// approval_set + steps.
	var setID int
	if err := env.db.QueryRow(`SELECT approval_set_id FROM approval_set_statuses WHERE status_id = ?`, env.statusReviewID).Scan(&setID); err != nil {
		t.Fatalf("lookup set id: %v", err)
	}
	if _, err := env.db.Exec(`
		INSERT INTO approval_set_statuses (approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, is_active)
		VALUES (?, ?, ?, ?, 'sequential', 1)
	`, setID, env.statusOpenID, env.transitionOpenToReview, env.transitionOpenToReview); err != nil {
		t.Fatalf("seed Open gate: %v", err)
	}

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if err := env.approvalService.Cancel(context.Background(), req.ID, env.requestor, "", "manual"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// The original request is cancelled and item is back at Open. Critically,
	// no new pending request should exist.
	var pendingCount int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM approval_requests WHERE item_id = ? AND status = 'pending'`, env.itemID).Scan(&pendingCount); err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if pendingCount != 0 {
		t.Fatalf("revert opened a new approval gate: %d pending requests after cancel", pendingCount)
	}
	if got := env.itemStatusID(); got != env.statusOpenID {
		t.Fatalf("item not reverted: got %d, want statusOpenID=%d", got, env.statusOpenID)
	}
}

// Portal-customer creator: when an item's creator_portal_customer_id is set,
// the 'creator' approver source resolves the customer (not creator_id) and the
// snapshot writes a portal_customer_id row. DecideAsCustomer drives the request
// to completion — proves the polymorphic-creator + portal-decide path
// end-to-end without requiring a portal handler.
func TestApproval_Creator_PortalCustomer(t *testing.T) {
	env := newApprovalTestEnv(t)

	// Seed a portal customer + retag the item as customer-created.
	res, err := env.db.Exec(`INSERT INTO portal_customers (name, email) VALUES ('Acme', 'acme@example.com')`)
	if err != nil {
		t.Fatalf("insert portal_customer: %v", err)
	}
	cid64, _ := res.LastInsertId()
	customerID := int(cid64)

	if _, err := env.db.Exec(`UPDATE items SET creator_id = NULL, creator_portal_customer_id = ? WHERE id = ?`, customerID, env.itemID); err != nil {
		t.Fatalf("flip item to customer-created: %v", err)
	}

	// approver_source='creator' with allow_self_approval so the customer can
	// approve their own request (the canonical happy path).
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder:   0,
				name:           "Customer sign-off",
				quorumMode:     models.ApprovalQuorumModeAny,
				source:         models.ApprovalSourceCreator,
				allowSelf:      true,
			},
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if got := len(req.StepInstances); got != 1 {
		t.Fatalf("step instances = %d, want 1", got)
	}
	if got := len(req.StepInstances[0].Approvers); got != 1 {
		t.Fatalf("pool size = %d, want 1", got)
	}
	app := req.StepInstances[0].Approvers[0]
	if app.UserID != nil {
		t.Fatalf("user_id = %v, want nil for customer-creator", app.UserID)
	}
	if app.PortalCustomerID == nil || *app.PortalCustomerID != customerID {
		t.Fatalf("portal_customer_id = %v, want %d", app.PortalCustomerID, customerID)
	}

	// Customer decides via the portal-flavored entry point.
	_, out, err := env.approvalService.DecideAsCustomer(context.Background(), req.ID, customerID, models.ApprovalDecisionApprove, "all good", DecideOptions{})
	if err != nil {
		t.Fatalf("DecideAsCustomer: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("request = %s, want approved", out.Status)
	}
	if got := env.itemStatusID(); got != env.statusApprovedID {
		t.Fatalf("item status = %d, want approved", got)
	}

	// Audit row carries actor_portal_customer_id, NOT actor_user_id, for the approve.
	var foundApprove bool
	for _, d := range out.Decisions {
		if d.Decision == models.ApprovalDecisionApprove {
			foundApprove = true
			if d.ActorUserID != nil {
				t.Fatalf("approve actor_user_id = %v, want nil for customer-actor", d.ActorUserID)
			}
			if d.ActorPortalCustomerID == nil || *d.ActorPortalCustomerID != customerID {
				t.Fatalf("approve actor_portal_customer_id = %v, want %d", d.ActorPortalCustomerID, customerID)
			}
		}
	}
	if !foundApprove {
		t.Fatal("expected an 'approve' audit row")
	}
}

// A customer who's NOT in the active pool gets the standard "not an active
// approver" error from DecideAsCustomer — same gate as the user path.
func TestApproval_PortalCustomerNotInPool(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})
	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	res, err := env.db.Exec(`INSERT INTO portal_customers (name, email) VALUES ('Stranger', 'stranger@example.com')`)
	if err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	strangerID64, _ := res.LastInsertId()
	strangerID := int(strangerID64)

	if _, _, err := env.approvalService.DecideAsCustomer(context.Background(), req.ID, strangerID, models.ApprovalDecisionApprove, "", DecideOptions{}); err == nil {
		t.Fatal("expected error for customer not in pool")
	}
}

// Custom (label-only) workspace roles must still resolve their members for
// approval routing — only the permission cache should ignore them. This test
// is the contract: flipping permissions_enabled=FALSE on the role used by
// approver_source='role' does NOT shrink the approver pool.
func TestApproval_LabelOnlyRole_StillResolvesApprovers(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder: 0,
				name:         "Reviewers",
				quorumMode:   models.ApprovalQuorumModeAny,
				source:       models.ApprovalSourceRole,
				roleID:       intPtr(1),
			},
		},
	})

	// Seed a label-only role (permissions_enabled = FALSE) and assign three users.
	res, err := env.db.Exec(`INSERT INTO workspace_roles (name, permissions_enabled) VALUES ('label-only-reviewers', 0)`)
	if err != nil {
		t.Fatalf("insert label-only role: %v", err)
	}
	roleID64, _ := res.LastInsertId()
	roleID := int(roleID64)
	if _, err := env.db.Exec(`UPDATE approval_steps SET approver_role_id = ? WHERE approver_role_id = 1`, roleID); err != nil {
		t.Fatalf("rewire approver_role_id: %v", err)
	}
	for _, uid := range []int{env.approver1, env.approver2, env.approver3} {
		if _, err := env.db.Exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?, ?, ?)`, uid, env.workspaceID, roleID); err != nil {
			t.Fatalf("assign role: %v", err)
		}
	}

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if got := len(req.StepInstances[0].Approvers); got != 3 {
		t.Fatalf("approver pool from label-only role = %d, want 3 (the flag must NOT affect routing)", got)
	}

	// Decision still works — the pool is real even though the role grants no permissions.
	_, out, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "ok", DecideOptions{})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if out.Status != models.ApprovalRequestStatusApproved {
		t.Fatalf("request = %s, want approved", out.Status)
	}
}

// Permission cache must IGNORE permissions attached to label-only roles. Pair
// a permission to the role; even though the user is assigned to the role,
// HasWorkspacePermission must return false because permissions_enabled=FALSE.
func TestPermissionCache_LabelOnlyRole_DoesNotGrantPermissions(t *testing.T) {
	env := newApprovalTestEnv(t)

	// Use a permission that is NOT in the everyone-Viewer fallback set. The
	// default workspace is "open" (no explicit Viewer assignments → everyone
	// has Viewer perms via the fallback at permission_cache.go:1006-1012),
	// which would mask any leak through item.delete. item.delete is in the
	// Administrator role only, so the label-only role is the ONLY path.
	var permID int
	if err := env.db.QueryRow(`SELECT id FROM permissions WHERE permission_key = 'item.delete' LIMIT 1`).Scan(&permID); err != nil {
		t.Fatalf("lookup permission item.delete: %v", err)
	}

	// Two roles: one label-only, one permission-bearing — both grant item.delete.
	r1, err := env.db.Exec(`INSERT INTO workspace_roles (name, permissions_enabled) VALUES ('label-only', 0)`)
	if err != nil {
		t.Fatalf("insert label-only role: %v", err)
	}
	labelRoleID64, _ := r1.LastInsertId()
	labelRoleID := int(labelRoleID64)
	if _, err := env.db.Exec(`INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)`, labelRoleID, permID); err != nil {
		t.Fatalf("attach perm to label role: %v", err)
	}

	r2, err := env.db.Exec(`INSERT INTO workspace_roles (name, permissions_enabled) VALUES ('permission-bearing', 1)`)
	if err != nil {
		t.Fatalf("insert permission role: %v", err)
	}
	permRoleID64, _ := r2.LastInsertId()
	permRoleID := int(permRoleID64)
	if _, err := env.db.Exec(`INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)`, permRoleID, permID); err != nil {
		t.Fatalf("attach perm to perm role: %v", err)
	}

	// approver1 → label-only role only. approver2 → both roles.
	if _, err := env.db.Exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?, ?, ?)`, env.approver1, env.workspaceID, labelRoleID); err != nil {
		t.Fatalf("assign label role to approver1: %v", err)
	}
	if _, err := env.db.Exec(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id) VALUES (?, ?, ?), (?, ?, ?)`,
		env.approver2, env.workspaceID, labelRoleID,
		env.approver2, env.workspaceID, permRoleID); err != nil {
		t.Fatalf("assign roles to approver2: %v", err)
	}

	// approver1 has the role assigned but the permission must NOT come through.
	hasView1, err := env.permService.HasWorkspacePermission(env.approver1, env.workspaceID, "item.delete")
	if err != nil {
		t.Fatalf("HasWorkspacePermission approver1: %v", err)
	}
	if hasView1 {
		t.Fatal("approver1 (label-only role only) should NOT have item.delete; permission cache leaked the flag")
	}

	// approver2 has the permission via the permission-bearing role.
	hasView2, err := env.permService.HasWorkspacePermission(env.approver2, env.workspaceID, "item.delete")
	if err != nil {
		t.Fatalf("HasWorkspacePermission approver2: %v", err)
	}
	if !hasView2 {
		t.Fatal("approver2 should have item.delete via permission-bearing role")
	}

	// HasWorkspaceRole should still return TRUE for both — role assignment
	// tracking is independent of the flag.
	hasRole1, err := env.permService.HasWorkspaceRole(env.approver1, env.workspaceID, labelRoleID)
	if err != nil || !hasRole1 {
		t.Fatalf("HasWorkspaceRole(approver1, label-only) = %v, %v; want true", hasRole1, err)
	}
}

// UserHasActivePoolMembershipOnItem is the gate for approver-derived item.view
// access: returns true while the user is in an is_active=true row of a pending
// step on a pending request, and flips to false once the step or request
// closes. The contract: access ends with the decision. No long-lived
// "I once approved this" grant.
func TestApproval_UserHasActivePoolMembershipOnItem(t *testing.T) {
	env := newApprovalTestEnv(t)
	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{userStep(0, "First", env.approver1)},
	})

	// Before request creation: no membership.
	got, err := env.approvalService.UserHasActivePoolMembershipOnItem(env.approver1, env.itemID)
	if err != nil {
		t.Fatalf("pre-request membership: %v", err)
	}
	if got {
		t.Fatal("expected no membership before request opened")
	}

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	// Request is pending, step active, approver1 in pool → true.
	got, err = env.approvalService.UserHasActivePoolMembershipOnItem(env.approver1, env.itemID)
	if err != nil {
		t.Fatalf("active membership: %v", err)
	}
	if !got {
		t.Fatal("expected approver1 to have active pool membership while request is pending")
	}

	// approver2 is NOT in the pool → false even though request is pending.
	got, err = env.approvalService.UserHasActivePoolMembershipOnItem(env.approver2, env.itemID)
	if err != nil {
		t.Fatalf("non-member membership: %v", err)
	}
	if got {
		t.Fatal("approver2 should not have membership — never in pool")
	}

	// requestor is the triggerer, not an approver → false.
	got, err = env.approvalService.UserHasActivePoolMembershipOnItem(env.requestor, env.itemID)
	if err != nil {
		t.Fatalf("requestor membership: %v", err)
	}
	if got {
		t.Fatal("requestor (triggerer) should not have approver-pool membership")
	}

	// Decide approve → request becomes approved, step closes. Membership flips.
	if _, _, err := env.approvalService.Decide(context.Background(), req.ID, env.approver1, models.ApprovalDecisionApprove, "", DecideOptions{}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	got, err = env.approvalService.UserHasActivePoolMembershipOnItem(env.approver1, env.itemID)
	if err != nil {
		t.Fatalf("post-decide membership: %v", err)
	}
	if got {
		t.Fatal("expected approver1 to lose membership after the request closed (request status != pending)")
	}
}

// PortalCustomerHasActivePoolMembershipOnItem mirrors the user-side gate for
// portal customers added directly to an approval pool — the case the
// "internal users without workspace access (e.g. a finance reviewer reachable
// only via portal customer link)" comment describes.
func TestApproval_PortalCustomerHasActivePoolMembershipOnItem(t *testing.T) {
	env := newApprovalTestEnv(t)

	// Mirror TestApproval_Creator_PortalCustomer: the customer becomes a pool
	// member by being the item creator with approver_source=creator.
	res, err := env.db.Exec(`INSERT INTO portal_customers (name, email) VALUES ('Reviewer', 'rev@example.com')`)
	if err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	cid64, _ := res.LastInsertId()
	customerID := int(cid64)

	if _, err := env.db.Exec(`UPDATE items SET creator_id = NULL, creator_portal_customer_id = ? WHERE id = ?`, customerID, env.itemID); err != nil {
		t.Fatalf("flip item to customer-created: %v", err)
	}

	env.createApprovalSet(approvalSetSpec{
		steps: []approvalStepSpec{
			{
				displayOrder: 0,
				name:         "Customer review",
				quorumMode:   models.ApprovalQuorumModeAny,
				source:       models.ApprovalSourceCreator,
				allowSelf:    true,
			},
		},
	})

	req, err := env.approvalService.RequestApproval(context.Background(), env.itemID, env.statusReviewID, env.statusOpenID, env.requestor)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	// Customer is in the active pool while the request is pending.
	got, err := env.approvalService.PortalCustomerHasActivePoolMembershipOnItem(customerID, env.itemID)
	if err != nil {
		t.Fatalf("active membership: %v", err)
	}
	if !got {
		t.Fatal("expected portal customer to have active pool membership while request is pending")
	}

	// A different (unrelated) customer must not get a positive answer.
	res2, err := env.db.Exec(`INSERT INTO portal_customers (name, email) VALUES ('Stranger', 'stranger@example.com')`)
	if err != nil {
		t.Fatalf("insert stranger customer: %v", err)
	}
	stranger64, _ := res2.LastInsertId()
	got, err = env.approvalService.PortalCustomerHasActivePoolMembershipOnItem(int(stranger64), env.itemID)
	if err != nil {
		t.Fatalf("stranger membership: %v", err)
	}
	if got {
		t.Fatal("stranger customer should not have approver-pool membership")
	}

	// Decide → request approved → membership ends.
	if _, _, err := env.approvalService.DecideAsCustomer(context.Background(), req.ID, customerID, models.ApprovalDecisionApprove, "", DecideOptions{}); err != nil {
		t.Fatalf("DecideAsCustomer: %v", err)
	}
	got, err = env.approvalService.PortalCustomerHasActivePoolMembershipOnItem(customerID, env.itemID)
	if err != nil {
		t.Fatalf("post-decide membership: %v", err)
	}
	if got {
		t.Fatal("expected customer to lose membership after the request closed")
	}
}
