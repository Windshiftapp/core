package repository

import (
	"database/sql"
	"errors"
	"fmt"

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
	return touches, nil
}
