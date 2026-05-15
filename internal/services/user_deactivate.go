package services

import (
	"fmt"

	"windshift/internal/database"
)

// AgentDeactivationResult captures the side-effects of deactivating an owner
// so the caller can emit audit entries for each affected row.
type AgentDeactivationResult struct {
	AgentIDs         []int // owned agent user IDs that were flipped to inactive
	RevokedAPITokens []int // api_tokens row IDs removed (owner + agents)
}

// ActiveSystemAdminIDs returns the user IDs of every active user who holds the
// 'system.admin' global permission. Used for baking in admin notifications on
// security-relevant SCIM events (e.g. cascaded offboarding) so operators learn
// about integration impact without having to poll the audit log.
func ActiveSystemAdminIDs(db database.Database) ([]int, error) {
	rows, err := db.Query(`
		SELECT DISTINCT ugp.user_id
		FROM user_global_permissions ugp
		JOIN permissions p ON ugp.permission_id = p.id
		JOIN users u ON ugp.user_id = u.id
		WHERE p.permission_key = 'system.admin' AND u.is_active = true
	`)
	if err != nil {
		return nil, fmt.Errorf("load system admins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if scanErr := rows.Scan(&id); scanErr == nil {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system admins: %w", err)
	}
	return ids, nil
}

// DeactivateOwnedAgentsAndTokens propagates an owner's deactivation to their
// agents and revokes every API token held by the owner or their agents. The
// owner's own `users.is_active` row is expected to already be flipped by the
// caller; this function only handles the cascade onto dependents.
//
// Runs in a single transaction so a partial cascade cannot leak live tokens.
func DeactivateOwnedAgentsAndTokens(db database.Database, ownerID int) (AgentDeactivationResult, error) {
	var result AgentDeactivationResult

	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("failed to begin cascade transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Collect owned agent IDs so we can audit and scope the token sweeps.
	agentRows, err := tx.Query(`SELECT id FROM users WHERE agent_owner_user_id = ? AND is_active = true`, ownerID)
	if err != nil {
		return result, fmt.Errorf("failed to load owned agents: %w", err)
	}
	for agentRows.Next() {
		var id int
		if scanErr := agentRows.Scan(&id); scanErr == nil {
			result.AgentIDs = append(result.AgentIDs, id)
		}
	}
	if err := agentRows.Err(); err != nil {
		_ = agentRows.Close()
		return result, fmt.Errorf("failed to iterate owned agents: %w", err)
	}
	_ = agentRows.Close()

	// 2. Flip agents inactive.
	if len(result.AgentIDs) > 0 {
		if _, err = tx.Exec(`UPDATE users SET is_active = false, updated_at = CURRENT_TIMESTAMP WHERE agent_owner_user_id = ?`, ownerID); err != nil {
			return result, fmt.Errorf("failed to deactivate owned agents: %w", err)
		}
	}

	// 3. Collect api_tokens row IDs before we delete them (for audit).
	// api_tokens has no is_active column, so revocation is a hard DELETE.
	userIDs := append([]int{ownerID}, result.AgentIDs...)
	apiTokenRows, err := tx.Query(inClauseQuery(`SELECT id FROM api_tokens WHERE user_id IN (`, len(userIDs)), toIfaceSlice(userIDs)...)
	if err != nil {
		return result, fmt.Errorf("failed to load api_tokens: %w", err)
	}
	for apiTokenRows.Next() {
		var id int
		if scanErr := apiTokenRows.Scan(&id); scanErr == nil {
			result.RevokedAPITokens = append(result.RevokedAPITokens, id)
		}
	}
	if err := apiTokenRows.Err(); err != nil {
		_ = apiTokenRows.Close()
		return result, fmt.Errorf("failed to iterate api_tokens: %w", err)
	}
	_ = apiTokenRows.Close()

	if len(result.RevokedAPITokens) > 0 {
		if _, err = tx.Exec(inClauseQuery(`DELETE FROM api_tokens WHERE user_id IN (`, len(userIDs)), toIfaceSlice(userIDs)...); err != nil {
			return result, fmt.Errorf("failed to revoke api_tokens: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf("failed to commit cascade: %w", err)
	}

	return result, nil
}

// inClauseQuery appends `?,?,?) ...trailing` sized for n placeholders.
func inClauseQuery(prefix string, n int) string {
	if n == 0 {
		return prefix + ")"
	}
	out := prefix
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += "?"
	}
	return out + ")"
}

func toIfaceSlice(ids []int) []interface{} {
	out := make([]interface{}, len(ids))
	for i, v := range ids {
		out[i] = v
	}
	return out
}
