package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// PendingAudit is one additional audit row to persist atomically with a
// definition mutation. The service stamps revision metadata onto every row.
type PendingAudit struct {
	ActionType string
	Details    map[string]any
}

// ActionWriteMeta carries the authenticated editor and the audit events that
// must commit or roll back with the definition mutation.
type ActionWriteMeta struct {
	Actor             AuditActor
	Source            string
	PrimaryActionType string
	PrimaryDetails    map[string]any
	ExtraAudits       []PendingAudit
	// AuthorizeActor, when set, runs inside the write transaction after the
	// action row is locked. currentActor is the row's actor at lock time and
	// effectiveActor is what the revision will record; actorChanging reports
	// whether the patch actually moves the actor. Returning a
	// *PrivilegedActionEditDenied aborts the mutation and writes a safe denied
	// audit row; any other error aborts without an audit.
	AuthorizeActor func(currentActor, effectiveActor *int, actorChanging bool) error
}

// PrivilegedActionEditDenied reports a rejected attempt to change the
// executable configuration of an action with a configured actor without
// action.set_actor. It carries only safe metadata.
type PrivilegedActionEditDenied struct {
	AttemptedFields    []string
	RequiredPermission string
}

func (e *PrivilegedActionEditDenied) Error() string {
	return "action.set_actor required to edit executable configuration of a privileged action"
}

// editorSource resolves the stored editor source label for a revision.
func (m ActionWriteMeta) editorSource() string {
	if m.Actor.Source != "" {
		return m.Actor.Source
	}
	if m.Actor.UserID == 0 {
		return logger.AuditSourceSystem
	}
	return logger.AuditSourceUser
}

// persistActionRevisionTx snapshots the action's executable definition, diffs it
// against the previous revision, inserts the new immutable revision, points the
// action at it, and writes every audit row — all inside the caller's
// transaction. A failure here rolls back the definition mutation with it.
func persistActionRevisionTx(
	ctx context.Context,
	tx database.Tx,
	revisions *repository.ActionRevisionRepository,
	action *models.Action,
	nodes []models.ActionNode,
	edges []models.ActionEdge,
	source string,
	meta ActionWriteMeta,
) error {
	snapshot := models.NewActionDefinitionSnapshot(action, nodes, edges)

	var previous *models.ActionDefinitionSnapshot
	previousRevision, err := revisions.LatestByActionTx(tx, action.ID)
	switch {
	case err == nil:
		previous = &previousRevision.Snapshot
	case errors.Is(err, repository.ErrNotFound):
		previous = nil
	default:
		return fmt.Errorf("load previous action revision: %w", err)
	}

	changedFields := models.DiffActionDefinitionSnapshots(previous, snapshot)
	nextNumber, err := revisions.NextRevisionNumberTx(tx, action.ID)
	if err != nil {
		return err
	}

	revision := &models.ActionDefinitionRevision{
		ActionID:       action.ID,
		WorkspaceID:    action.WorkspaceID,
		RevisionNumber: nextNumber,
		Source:         source,
		EditorUsername: meta.Actor.Username,
		EditorSource:   meta.editorSource(),
		ChangeSummary:  summarizeActionChange(source, changedFields),
		ChangedFields:  changedFields,
		Snapshot:       snapshot,
	}
	if meta.Actor.UserID > 0 {
		editorID := meta.Actor.UserID
		revision.EditorUserID = &editorID
	}

	revisionID, err := revisions.CreateTx(tx, revision)
	if err != nil {
		return err
	}
	revision.ID = revisionID
	if _, err := tx.Exec(`UPDATE actions SET current_revision_id = ? WHERE id = ?`, revisionID, action.ID); err != nil {
		return fmt.Errorf("bind current action revision: %w", err)
	}
	action.CurrentRevisionID = &revisionID

	if err := writeActionAuditsTx(ctx, tx, action, revision, meta); err != nil {
		return err
	}
	return nil
}

func summarizeActionChange(source string, changedFields []string) string {
	if source == models.ActionRevisionSourceBaseline {
		return "baseline snapshot"
	}
	if len(changedFields) == 0 {
		return "no executable change"
	}
	return "changed " + strings.Join(changedFields, ", ")
}

func writeActionAuditsTx(
	ctx context.Context,
	tx database.Tx,
	action *models.Action,
	revision *models.ActionDefinitionRevision,
	meta ActionWriteMeta,
) error {
	base := map[string]any{
		"workspace_id":    action.WorkspaceID,
		"revision_id":     revision.ID,
		"revision_number": revision.RevisionNumber,
		"change_source":   revision.Source,
		"changed_fields":  revision.ChangedFields,
	}
	for key, value := range meta.PrimaryDetails {
		base[key] = value
	}
	if err := writeActionAuditTx(ctx, tx, meta.Actor, meta.PrimaryActionType, action, base); err != nil {
		return err
	}
	for _, extra := range meta.ExtraAudits {
		details := map[string]any{
			"workspace_id":    action.WorkspaceID,
			"revision_id":     revision.ID,
			"revision_number": revision.RevisionNumber,
			"change_source":   revision.Source,
		}
		for key, value := range extra.Details {
			details[key] = value
		}
		if err := writeActionAuditTx(ctx, tx, meta.Actor, extra.ActionType, action, details); err != nil {
			return err
		}
	}
	return nil
}

func writeActionAuditTx(
	ctx context.Context,
	tx database.Tx,
	actor AuditActor,
	actionType string,
	action *models.Action,
	details map[string]any,
) error {
	if actionType == "" {
		return nil
	}
	resourceID := action.ID
	return logger.LogAuditTx(ctx, tx, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		Source:       actor.Source,
		ActionType:   actionType,
		ResourceType: logger.ResourceAutomation,
		ResourceID:   &resourceID,
		ResourceName: action.Name,
		Details:      mergeAuditDetails(details, actor),
		Success:      true,
	})
}
