package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
)

// TransitionRepository serves read-only views of workflow_transitions used
// by governance / override-warning UI. Mutations to transitions still flow
// through the workflow editor's existing handlers.
type TransitionRepository struct {
	db database.Database
}

// NewTransitionRepository creates a TransitionRepository.
func NewTransitionRepository(db database.Database) *TransitionRepository {
	return &TransitionRepository{db: db}
}

// TransitionWithStatuses is the transition row plus the joined-in status
// names. FromStatusID is nullable (initial transitions have no source).
type TransitionWithStatuses struct {
	ID             int
	FromStatusID   *int
	ToStatusID     int
	FromStatusName string // empty when FromStatusID is nil
	ToStatusName   string
}

// GetWithStatusNames loads a transition and the human-readable names of its
// from/to statuses. Returns ErrNotFound when no transition with that id exists.
func (r *TransitionRepository) GetWithStatusNames(id int) (*TransitionWithStatuses, error) {
	var fromID sql.NullInt64
	var toID int
	var fromName sql.NullString
	var toName string
	err := r.db.QueryRow(`
		SELECT wt.from_status_id, wt.to_status_id, fs.name AS from_name, ts.name AS to_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON fs.id = wt.from_status_id
		JOIN statuses ts ON ts.id = wt.to_status_id
		WHERE wt.id = ?
	`, id).Scan(&fromID, &toID, &fromName, &toName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get transition %d: %w", id, err)
	}
	out := &TransitionWithStatuses{
		ID:           id,
		ToStatusID:   toID,
		ToStatusName: toName,
	}
	if fromID.Valid {
		f := int(fromID.Int64)
		out.FromStatusID = &f
	}
	if fromName.Valid {
		out.FromStatusName = fromName.String
	}
	return out, nil
}

// ConditionSetTouch is one condition_set that targets a given transition,
// along with how many individual conditions it carries.
type ConditionSetTouch struct {
	ConditionSetID   int
	ConditionSetName string
	ConditionCount   int
}

// ListConditionSetTouches returns the condition_sets that target a transition,
// regardless of mode, ordered by condition_set name.
func (r *TransitionRepository) ListConditionSetTouches(transitionID int) ([]ConditionSetTouch, error) {
	rows, err := r.db.Query(`
		SELECT cs.id, cs.name, COUNT(c.id) as condition_count
		FROM condition_set_transitions cst
		JOIN condition_sets cs ON cs.id = cst.condition_set_id
		LEFT JOIN conditions c ON c.condition_set_transition_id = cst.id
		WHERE cst.transition_id = ?
		GROUP BY cs.id, cs.name
		ORDER BY cs.name
	`, transitionID)
	if err != nil {
		return nil, fmt.Errorf("list condition_set touches for transition %d: %w", transitionID, err)
	}
	defer func() { _ = rows.Close() }()

	var touches []ConditionSetTouch
	for rows.Next() {
		var ct ConditionSetTouch
		if err := rows.Scan(&ct.ConditionSetID, &ct.ConditionSetName, &ct.ConditionCount); err != nil {
			return nil, fmt.Errorf("scan condition_set touch: %w", err)
		}
		touches = append(touches, ct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate condition_set touches: %w", err)
	}
	return touches, nil
}

// TransitionScreenSummary describes the screen bound to a transition, with
// just enough detail for the governance response and the designer inspector.
// A nil summary means the transition carries no screen.
type TransitionScreenSummary struct {
	ID                 int
	Name               string
	FieldCount         int
	RequiredFieldCount int
}

// GetScreen returns the screen bound to a transition, or nil when the
// transition has no screen. Callers that need to distinguish a missing
// transition should load it first (GetWithStatusNames).
func (r *TransitionRepository) GetScreen(transitionID int) (*TransitionScreenSummary, error) {
	var summary TransitionScreenSummary
	err := r.db.QueryRow(`
		SELECT s.id, s.name,
		       COUNT(sf.id) AS field_count,
		       COALESCE(SUM(CASE WHEN sf.is_required THEN 1 ELSE 0 END), 0) AS required_field_count
		FROM workflow_transitions wt
		JOIN screens s ON s.id = wt.screen_id
		LEFT JOIN screen_fields sf ON sf.screen_id = s.id
		WHERE wt.id = ?
		GROUP BY s.id, s.name
	`, transitionID).Scan(&summary.ID, &summary.Name, &summary.FieldCount, &summary.RequiredFieldCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // "no screen bound" is a real state, distinct from an error
	}
	if err != nil {
		return nil, fmt.Errorf("get screen for transition %d: %w", transitionID, err)
	}
	return &summary, nil
}

// SetScreen binds a screen to a transition, or clears the binding when
// screenID is nil. Returns ErrNotFound when the transition does not exist.
// The caller is responsible for confirming the screen exists first.
func (r *TransitionRepository) SetScreen(transitionID int, screenID *int) error {
	var arg any
	if screenID != nil {
		arg = *screenID
	}
	result, err := r.db.ExecWrite("UPDATE workflow_transitions SET screen_id = ? WHERE id = ?", arg, transitionID)
	if err != nil {
		return fmt.Errorf("bind screen to transition %d: %w", transitionID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("bind screen to transition %d: %w", transitionID, err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ScreenExists reports whether a screen row exists.
func (r *TransitionRepository) ScreenExists(screenID int) (bool, error) {
	var exists bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM screens WHERE id = ?)", screenID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check screen %d: %w", screenID, err)
	}
	return exists, nil
}

// CountTransitionsUsingScreen returns how many transitions are bound to a
// screen. Used by the screens delete guard and the usage affordance.
func (r *TransitionRepository) CountTransitionsUsingScreen(screenID int) (int, error) {
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM workflow_transitions WHERE screen_id = ?", screenID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count transitions using screen %d: %w", screenID, err)
	}
	return count, nil
}

// TransitionScreenField is one field declared on a transition's screen, in
// display order. FieldType is "default" or "custom".
type TransitionScreenField struct {
	FieldType       string
	FieldIdentifier string
	IsRequired      bool
}

// GetScreenFields returns the fields declared on the screen bound to a
// transition, ordered by display order. Returns nil when the transition has
// no screen. The authoritative list is screen_fields; the legacy
// screen_system_fields table is not consulted.
func (r *TransitionRepository) GetScreenFields(transitionID int) ([]TransitionScreenField, error) {
	rows, err := r.db.Query(`
		SELECT sf.field_type, sf.field_identifier, sf.is_required
		FROM workflow_transitions wt
		JOIN screen_fields sf ON sf.screen_id = wt.screen_id
		WHERE wt.id = ?
		ORDER BY sf.display_order, sf.id
	`, transitionID)
	if err != nil {
		return nil, fmt.Errorf("list screen fields for transition %d: %w", transitionID, err)
	}
	defer func() { _ = rows.Close() }()

	var fields []TransitionScreenField
	for rows.Next() {
		var field TransitionScreenField
		if err := rows.Scan(&field.FieldType, &field.FieldIdentifier, &field.IsRequired); err != nil {
			return nil, fmt.Errorf("scan screen field: %w", err)
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate screen fields: %w", err)
	}
	return fields, nil
}

// ScreensForTransitions returns the screen bound to each transition, keyed by
// transition id. Transitions without a screen are absent. One query keeps the
// available-transitions read off an N+1 path.
func (r *TransitionRepository) ScreensForTransitions(transitionIDs []int) (map[int]TransitionScreenSummary, error) {
	out := map[int]TransitionScreenSummary{}
	if len(transitionIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(transitionIDs))
	args := make([]any, len(transitionIDs))
	for i, id := range transitionIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(fmt.Sprintf(`
		SELECT wt.id, s.id, s.name,
		       COUNT(sf.id) AS field_count,
		       COALESCE(SUM(CASE WHEN sf.is_required THEN 1 ELSE 0 END), 0) AS required_field_count
		FROM workflow_transitions wt
		JOIN screens s ON s.id = wt.screen_id
		LEFT JOIN screen_fields sf ON sf.screen_id = s.id
		WHERE wt.id IN (%s)
		GROUP BY wt.id, s.id, s.name
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("list screens for transitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var transitionID int
		var summary TransitionScreenSummary
		if err := rows.Scan(&transitionID, &summary.ID, &summary.Name, &summary.FieldCount, &summary.RequiredFieldCount); err != nil {
			return nil, fmt.Errorf("scan transition screen: %w", err)
		}
		out[transitionID] = summary
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transition screens: %w", err)
	}
	return out, nil
}
