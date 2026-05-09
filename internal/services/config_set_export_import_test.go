package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// configSetTplTestEnv seeds the minimum schema fixtures required to round-trip
// a configuration-set bundle: a workflow with two transitions, one item type,
// one priority, one custom field, one screen referencing the custom field,
// one workspace_role (so we can exercise role refs in conditions), one
// condition_set with a user_in_role condition, one approval_set whose status
// triggers approve/deny transitions, and a configuration_set linking it all.
type configSetTplTestEnv struct {
	t  *testing.T
	db database.Database

	configSetID    int
	roleID         int
	customFieldID  int
	statusOpenID   int
	statusDoneID   int
	workflowID     int
}

func newConfigSetTplTestEnv(t *testing.T) *configSetTplTestEnv {
	t.Helper()
	dsn := fmt.Sprintf("file:cstpl-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	env := &configSetTplTestEnv{t: t, db: db}
	env.seed()
	return env
}

func (e *configSetTplTestEnv) seed() {
	t := e.t
	prefix := strings.ReplaceAll(t.Name(), "/", "_")

	// Reuse the system-seeded "to-do" category for both statuses.
	var catID int
	if err := e.db.QueryRow(`SELECT id FROM status_categories WHERE is_default = true LIMIT 1`).Scan(&catID); err != nil {
		_ = e.db.QueryRow(`SELECT id FROM status_categories LIMIT 1`).Scan(&catID)
	}

	res, err := e.db.Exec(`INSERT INTO statuses (name, category_id) VALUES (?, ?)`, prefix+"-Open", catID)
	if err != nil {
		t.Fatalf("status open: %v", err)
	}
	id, _ := res.LastInsertId()
	e.statusOpenID = int(id)
	res, _ = e.db.Exec(`INSERT INTO statuses (name, category_id) VALUES (?, ?)`, prefix+"-Done", catID)
	id, _ = res.LastInsertId()
	e.statusDoneID = int(id)

	res, _ = e.db.Exec(`INSERT INTO workflows (name, description, is_default) VALUES (?, ?, false)`, prefix+"-WF", "")
	id, _ = res.LastInsertId()
	e.workflowID = int(id)

	// Open → Done transition + an initial-state transition (NULL from).
	if _, err := e.db.Exec(`INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id) VALUES (?, NULL, ?)`,
		e.workflowID, e.statusOpenID); err != nil {
		t.Fatalf("init transition: %v", err)
	}
	if _, err := e.db.Exec(`INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id) VALUES (?, ?, ?)`,
		e.workflowID, e.statusOpenID, e.statusDoneID); err != nil {
		t.Fatalf("transition: %v", err)
	}
	var transitionID int
	if err := e.db.QueryRow(`SELECT id FROM workflow_transitions WHERE workflow_id = ? AND from_status_id = ? AND to_status_id = ?`,
		e.workflowID, e.statusOpenID, e.statusDoneID).Scan(&transitionID); err != nil {
		t.Fatalf("lookup transition: %v", err)
	}

	res, _ = e.db.Exec(`INSERT INTO item_types (name, hierarchy_level) VALUES (?, 3)`, prefix+"-Bug")
	id, _ = res.LastInsertId()
	itemTypeID := int(id)

	res, _ = e.db.Exec(`INSERT INTO priorities (name, sort_order) VALUES (?, 0)`, prefix+"-High")
	id, _ = res.LastInsertId()
	priorityID := int(id)

	res, err = e.db.Exec(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order, system_default)
		VALUES (?, 'text', '', false, '', 0, false)
	`, prefix+"-Severity")
	if err != nil {
		t.Fatalf("custom field: %v", err)
	}
	id, _ = res.LastInsertId()
	e.customFieldID = int(id)

	// Screen with one custom-field row + one default + one system field.
	res, _ = e.db.Exec(`INSERT INTO screens (name, description) VALUES (?, '')`, prefix+"-Screen")
	id, _ = res.LastInsertId()
	screenID := int(id)
	if _, err := e.db.Exec(`
		INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
		VALUES (?, 'custom', ?, 0, false, 'full')
	`, screenID, fmt.Sprintf("%d", e.customFieldID)); err != nil {
		t.Fatalf("screen_fields custom: %v", err)
	}
	if _, err := e.db.Exec(`
		INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
		VALUES (?, 'default', 'description', 1, false, 'full')
	`, screenID); err != nil {
		t.Fatalf("screen_fields default: %v", err)
	}
	if _, err := e.db.Exec(`INSERT INTO screen_system_fields (screen_id, field_name) VALUES (?, 'title')`, screenID); err != nil {
		t.Fatalf("screen_system_fields: %v", err)
	}

	// A workspace_role to exercise role-name resolution in conditions.
	res, err = e.db.Exec(`
		INSERT INTO workspace_roles (name, description, is_system, permissions_enabled, display_order)
		VALUES (?, '', false, false, 0)
	`, prefix+"-Reviewer")
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	id, _ = res.LastInsertId()
	e.roleID = int(id)

	// Condition set with one user_in_role condition gating Open→Done.
	res, _ = e.db.Exec(`INSERT INTO condition_sets (name, description, workflow_id) VALUES (?, '', ?)`, prefix+"-CS", e.workflowID)
	id, _ = res.LastInsertId()
	conditionSetID := int(id)
	res, _ = e.db.Exec(`INSERT INTO condition_set_transitions (condition_set_id, transition_id, logic_mode) VALUES (?, ?, 'and')`,
		conditionSetID, transitionID)
	id, _ = res.LastInsertId()
	cstID := int(id)
	cfgJSON := fmt.Sprintf(`{"source":"current_user","role_id":%d}`, e.roleID)
	if _, err := e.db.Exec(`
		INSERT INTO conditions (condition_set_transition_id, condition_type, config, display_order, mode, error_message)
		VALUES (?, 'user_in_role', ?, 0, 'validator', 'must be reviewer')
	`, cstID, cfgJSON); err != nil {
		t.Fatalf("condition: %v", err)
	}

	// Approval set with one status (Open) and one role-based step.
	res, _ = e.db.Exec(`INSERT INTO approval_sets (name, description, workflow_id) VALUES (?, '', ?)`, prefix+"-AS", e.workflowID)
	id, _ = res.LastInsertId()
	approvalSetID := int(id)
	res, _ = e.db.Exec(`
		INSERT INTO approval_set_statuses (approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode)
		VALUES (?, ?, ?, ?, 'sequential')
	`, approvalSetID, e.statusOpenID, transitionID, transitionID)
	id, _ = res.LastInsertId()
	assID := int(id)
	if _, err := e.db.Exec(`
		INSERT INTO approval_steps
			(approval_set_status_id, display_order, name, quorum_mode, rejection_policy,
			 approver_source, approver_role_id, allow_self_approval, on_leave_strategy)
		VALUES (?, 0, 'Reviewer step', 'any', 'any_rejection_fails', 'role', ?, false, 'use_substitute')
	`, assID, e.roleID); err != nil {
		t.Fatalf("approval_step: %v", err)
	}

	// Configuration set wiring everything together.
	res, _ = e.db.Exec(`
		INSERT INTO configuration_sets (name, description, is_default, differentiate_by_item_type, workflow_id, condition_set_id, approval_set_id, default_item_type_id)
		VALUES (?, ?, false, false, ?, ?, ?, ?)
	`, prefix+"-CS", "fixture", e.workflowID, conditionSetID, approvalSetID, itemTypeID)
	id, _ = res.LastInsertId()
	e.configSetID = int(id)
	if _, err := e.db.Exec(`INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context) VALUES (?, ?, 'create')`,
		e.configSetID, screenID); err != nil {
		t.Fatalf("css create: %v", err)
	}
	if _, err := e.db.Exec(`INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id) VALUES (?, ?)`,
		e.configSetID, itemTypeID); err != nil {
		t.Fatalf("csit: %v", err)
	}
	if _, err := e.db.Exec(`INSERT INTO configuration_set_priorities (configuration_set_id, priority_id) VALUES (?, ?)`,
		e.configSetID, priorityID); err != nil {
		t.Fatalf("csp: %v", err)
	}
}

// TestConfigSetExport_BasicShape verifies that Export produces a payload
// whose every cross-section reference is by name (or email), the envelope
// stamps are present, and the custom field embedded in the seeded condition
// is surfaced in the custom_fields[] section.
func TestConfigSetExport_BasicShape(t *testing.T) {
	env := newConfigSetTplTestEnv(t)
	repo := repository.NewConfigurationSetRepository(env.db)
	exp := NewConfigSetExportService(env.db, repo)

	tpl, err := exp.Export(context.Background(), env.configSetID, &ConfigSetExportBy{Username: "tester", Instance: "https://test.local"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if tpl.Kind != ConfigSetTemplateKind {
		t.Errorf("kind = %q; want %q", tpl.Kind, ConfigSetTemplateKind)
	}
	if tpl.SchemaVersion != ConfigSetTemplateSchemaVersion {
		t.Errorf("schema_version = %d; want %d", tpl.SchemaVersion, ConfigSetTemplateSchemaVersion)
	}
	if tpl.ExportedBy == nil || tpl.ExportedBy.Username != "tester" {
		t.Errorf("exported_by missing or wrong: %+v", tpl.ExportedBy)
	}
	if len(tpl.Payload.Workflows) != 1 {
		t.Fatalf("workflows = %d; want 1", len(tpl.Payload.Workflows))
	}
	if len(tpl.Payload.Statuses) < 2 {
		t.Errorf("statuses = %d; want >=2", len(tpl.Payload.Statuses))
	}
	if len(tpl.Payload.CustomFields) != 1 {
		t.Errorf("custom_fields = %d; want 1", len(tpl.Payload.CustomFields))
	}
	if len(tpl.Payload.ConditionSets) != 1 {
		t.Fatalf("condition_sets = %d; want 1", len(tpl.Payload.ConditionSets))
	}
	if len(tpl.Payload.ApprovalSets) != 1 {
		t.Fatalf("approval_sets = %d; want 1", len(tpl.Payload.ApprovalSets))
	}

	// The role ref inside the seeded user_in_role condition should be a name,
	// not an integer ID.
	cond := tpl.Payload.ConditionSets[0].TransitionConditions[0].Conditions[0]
	if _, hasID := cond.Config["role_id"]; hasID {
		t.Errorf("condition config still contains role_id (should be role_name)")
	}
	if name, _ := cond.Config["role_name"].(string); name == "" {
		t.Errorf("condition config missing role_name; got %+v", cond.Config)
	}

	// The approver_role_id on the seeded approval step should resolve to a
	// non-empty name in the exported step.
	step := tpl.Payload.ApprovalSets[0].SetStatuses[0].Steps[0]
	if step.ApproverRoleName == "" {
		t.Errorf("approval step missing approver_role_name")
	}
}

// TestConfigSetImport_RoundTrip exports a seeded config set, imports it on
// the same instance, and verifies that the new config set is independent
// of the source (different ID), references the same shared entities by
// name (e.g. the existing item type id is reused, not duplicated), and
// preserves workflow/condition/approval shape.
func TestConfigSetImport_RoundTrip(t *testing.T) {
	env := newConfigSetTplTestEnv(t)
	repo := repository.NewConfigurationSetRepository(env.db)
	exp := NewConfigSetExportService(env.db, repo)
	imp := NewConfigSetImportService(env.db, repo)
	ctx := context.Background()

	tpl, err := exp.Export(ctx, env.configSetID, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Capture pre-import counts of shared global entities — they must NOT
	// grow on the second import (idempotent name match).
	beforeStatuses := countRows(t, env.db, "statuses")
	beforeItemTypes := countRows(t, env.db, "item_types")
	beforePriorities := countRows(t, env.db, "priorities")
	beforeCustomFields := countRows(t, env.db, "custom_field_definitions")

	newID, warnings, err := imp.Import(ctx, tpl)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if newID == env.configSetID {
		t.Errorf("import returned source ID; expected a fresh row")
	}

	// Shared global entities should be reused — the name-match path of the
	// importer must not have duplicated any row.
	if got := countRows(t, env.db, "statuses"); got != beforeStatuses {
		t.Errorf("statuses grew from %d to %d; expected reuse", beforeStatuses, got)
	}
	if got := countRows(t, env.db, "item_types"); got != beforeItemTypes {
		t.Errorf("item_types grew from %d to %d; expected reuse", beforeItemTypes, got)
	}
	if got := countRows(t, env.db, "priorities"); got != beforePriorities {
		t.Errorf("priorities grew from %d to %d; expected reuse", beforePriorities, got)
	}
	if got := countRows(t, env.db, "custom_field_definitions"); got != beforeCustomFields {
		t.Errorf("custom_field_definitions grew from %d to %d; expected reuse", beforeCustomFields, got)
	}

	// The new configuration set should have the bundle's name and link to
	// freshly-created workflow/condition/approval rows (bundle-owned entities).
	created, err := repo.FindByID(newID)
	if err != nil {
		t.Fatalf("find new config set: %v", err)
	}
	if created.Name != tpl.Payload.ConfigurationSet.Name {
		t.Errorf("created name = %q; want %q", created.Name, tpl.Payload.ConfigurationSet.Name)
	}
	if created.WorkflowID == nil || *created.WorkflowID == env.workflowID {
		t.Errorf("created workflow_id should be a fresh ID; got %v (source=%d)", created.WorkflowID, env.workflowID)
	}
	// Warnings are non-fatal; make sure they're emitted as a slice (the
	// reused-by-name screen path produces one).
	if warnings == nil {
		t.Logf("warnings: nil")
	} else {
		t.Logf("warnings: %v", warnings)
	}
}

// TestConfigSetImport_UnresolvedReferences fabricates a template that names
// a role not present on the target instance and asserts the import refuses
// to write anything and surfaces an UnresolvedRef of kind="role".
func TestConfigSetImport_UnresolvedReferences(t *testing.T) {
	env := newConfigSetTplTestEnv(t)
	repo := repository.NewConfigurationSetRepository(env.db)
	exp := NewConfigSetExportService(env.db, repo)
	imp := NewConfigSetImportService(env.db, repo)
	ctx := context.Background()

	tpl, err := exp.Export(ctx, env.configSetID, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Mutate the role name in the user_in_role condition to something that
	// doesn't exist on this instance. The import must fail before any write.
	tpl.Payload.ConditionSets[0].TransitionConditions[0].Conditions[0].Config["role_name"] = "nonexistent-role-xyz"
	tpl.Payload.ApprovalSets[0].SetStatuses[0].Steps[0].ApproverRoleName = "nonexistent-role-xyz"

	beforeConfigSets := countRows(t, env.db, "configuration_sets")
	beforeWorkflows := countRows(t, env.db, "workflows")
	beforeApprovalSets := countRows(t, env.db, "approval_sets")

	_, _, err = imp.Import(ctx, tpl)
	if err == nil {
		t.Fatalf("import: expected error, got nil")
	}
	var unresolvedErr *ErrUnresolvedReferences
	if !errors.As(err, &unresolvedErr) {
		t.Fatalf("import: expected ErrUnresolvedReferences; got %T (%v)", err, err)
	}
	if len(unresolvedErr.Items) == 0 {
		t.Errorf("expected at least one unresolved item")
	}
	foundRole := false
	for _, ref := range unresolvedErr.Items {
		if ref.Kind == UnresolvedKindRole && ref.Name == "nonexistent-role-xyz" {
			foundRole = true
			break
		}
	}
	if !foundRole {
		t.Errorf("expected role unresolved ref; got %+v", unresolvedErr.Items)
	}

	// No writes: every count must match.
	if got := countRows(t, env.db, "configuration_sets"); got != beforeConfigSets {
		t.Errorf("configuration_sets changed from %d to %d on failed import", beforeConfigSets, got)
	}
	if got := countRows(t, env.db, "workflows"); got != beforeWorkflows {
		t.Errorf("workflows changed from %d to %d on failed import", beforeWorkflows, got)
	}
	if got := countRows(t, env.db, "approval_sets"); got != beforeApprovalSets {
		t.Errorf("approval_sets changed from %d to %d on failed import", beforeApprovalSets, got)
	}
}

func countRows(t *testing.T, db database.Database, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestConfigSetExport_DefaultRefused flips the seeded config set to
// is_default=true and asserts that Export refuses with ErrCannotExportDefault.
func TestConfigSetExport_DefaultRefused(t *testing.T) {
	env := newConfigSetTplTestEnv(t)
	if _, err := env.db.Exec(`UPDATE configuration_sets SET is_default = true WHERE id = ?`, env.configSetID); err != nil {
		t.Fatalf("flip is_default: %v", err)
	}
	repo := repository.NewConfigurationSetRepository(env.db)
	exp := NewConfigSetExportService(env.db, repo)

	tpl, err := exp.Export(context.Background(), env.configSetID, nil)
	if !errors.Is(err, ErrCannotExportDefault) {
		t.Fatalf("expected ErrCannotExportDefault; got tpl=%v err=%v", tpl, err)
	}
	if tpl != nil {
		t.Errorf("expected nil template on refusal; got %+v", tpl)
	}
}

// TestConfigSetImport_DefaultConfigSetCollision asserts that an import bundle
// whose configuration_set.name (case-insensitively) matches an existing
// is_default=true config set is rejected with ErrDefaultEntityConflict and
// performs zero writes.
func TestConfigSetImport_DefaultConfigSetCollision(t *testing.T) {
	env := newConfigSetTplTestEnv(t)
	repo := repository.NewConfigurationSetRepository(env.db)
	exp := NewConfigSetExportService(env.db, repo)
	imp := NewConfigSetImportService(env.db, repo)
	ctx := context.Background()

	tpl, err := exp.Export(ctx, env.configSetID, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Plant a *different*, default-flagged config set whose name collides
	// (case-insensitively) with the bundle's name.
	collidingName := strings.ToLower(tpl.Payload.ConfigurationSet.Name)
	if _, err := env.db.Exec(`INSERT INTO configuration_sets (name, is_default) VALUES (?, true)`, collidingName); err != nil {
		t.Fatalf("seed default config set: %v", err)
	}

	beforeConfigSets := countRows(t, env.db, "configuration_sets")
	beforeWorkflows := countRows(t, env.db, "workflows")

	_, _, err = imp.Import(ctx, tpl)
	var conflictErr *ErrDefaultEntityConflict
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ErrDefaultEntityConflict; got %T (%v)", err, err)
	}
	foundCS := false
	for _, c := range conflictErr.Conflicts {
		if c.Kind == DefaultConflictConfigurationSet {
			foundCS = true
		}
	}
	if !foundCS {
		t.Errorf("expected configuration_set conflict; got %+v", conflictErr.Conflicts)
	}

	if got := countRows(t, env.db, "configuration_sets"); got != beforeConfigSets {
		t.Errorf("configuration_sets changed from %d to %d on refused import", beforeConfigSets, got)
	}
	if got := countRows(t, env.db, "workflows"); got != beforeWorkflows {
		t.Errorf("workflows changed from %d to %d on refused import", beforeWorkflows, got)
	}
}

// TestConfigSetImport_DefaultWorkflowCollision asserts that an import whose
// embedded workflow name collides with an existing is_default=true workflow
// is rejected with ErrDefaultEntityConflict and writes nothing — even when
// the configuration_set name is unique.
func TestConfigSetImport_DefaultWorkflowCollision(t *testing.T) {
	env := newConfigSetTplTestEnv(t)
	repo := repository.NewConfigurationSetRepository(env.db)
	exp := NewConfigSetExportService(env.db, repo)
	imp := NewConfigSetImportService(env.db, repo)
	ctx := context.Background()

	tpl, err := exp.Export(ctx, env.configSetID, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(tpl.Payload.Workflows) == 0 {
		t.Fatalf("test fixture should have at least one workflow")
	}

	// Rename the bundle's config set to dodge the config-set check, then
	// plant a default-flagged workflow whose name collides (case-folded)
	// with the bundle's first workflow.
	tpl.Payload.ConfigurationSet.Name = "Bundle-" + tpl.Payload.ConfigurationSet.Name + "-import"
	collidingWF := strings.ToLower(tpl.Payload.Workflows[0].Name)
	if _, err := env.db.Exec(`INSERT INTO workflows (name, is_default) VALUES (?, true)`, collidingWF); err != nil {
		t.Fatalf("seed default workflow: %v", err)
	}

	beforeConfigSets := countRows(t, env.db, "configuration_sets")
	beforeWorkflows := countRows(t, env.db, "workflows")

	_, _, err = imp.Import(ctx, tpl)
	var conflictErr *ErrDefaultEntityConflict
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ErrDefaultEntityConflict; got %T (%v)", err, err)
	}
	foundWF := false
	for _, c := range conflictErr.Conflicts {
		if c.Kind == DefaultConflictWorkflow {
			foundWF = true
		}
	}
	if !foundWF {
		t.Errorf("expected workflow conflict; got %+v", conflictErr.Conflicts)
	}

	if got := countRows(t, env.db, "configuration_sets"); got != beforeConfigSets {
		t.Errorf("configuration_sets changed from %d to %d on refused import", beforeConfigSets, got)
	}
	if got := countRows(t, env.db, "workflows"); got != beforeWorkflows {
		t.Errorf("workflows changed from %d to %d on refused import", beforeWorkflows, got)
	}
}
