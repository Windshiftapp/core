package repository

import (
	"fmt"
	"strings"

	"windshift/internal/database"
)

// DeprovisionCascade reports the rows a deprovisioning transaction touched so
// callers can audit each impact and evict validation caches after commit.
type DeprovisionCascade struct {
	OwnedAgentIDs       []int // every owned agent, including agents already inactive
	DeactivatedAgentIDs []int // owned agents flipped inactive by this call
	RevokedAPITokenIDs  []int // api_tokens row IDs removed (owner + agents)
}

// HasImpact reports whether the cascade touched any rows.
func (c DeprovisionCascade) HasImpact() bool {
	return len(c.DeactivatedAgentIDs) > 0 || len(c.RevokedAPITokenIDs) > 0
}

// DeactivateOwnedAgentsAndTokensTx deactivates every agent owned by the user
// and deletes all API tokens belonging to the owner or their agents, on the
// caller's transaction. Deprovisioning writes (SCIM DELETE/PATCH/PUT and the
// standalone deactivation service) share this helper so the security cascade
// commits atomically with the resource write it belongs to. Repeated calls
// are safe: already-inactive agents are collected but not re-flipped, and
// deleting already-deleted tokens is a no-op.
func DeactivateOwnedAgentsAndTokensTx(tx database.Tx, ownerID int) (DeprovisionCascade, error) {
	var cascade DeprovisionCascade

	// Collect every owned agent so token revocation also covers agents that
	// were already inactive.
	agentRows, err := tx.Query(`SELECT id, is_active FROM users WHERE agent_owner_user_id = ?`, ownerID)
	if err != nil {
		return cascade, fmt.Errorf("failed to load owned agents: %w", err)
	}
	for agentRows.Next() {
		var id int
		var active bool
		if scanErr := agentRows.Scan(&id, &active); scanErr != nil {
			_ = agentRows.Close()
			return cascade, fmt.Errorf("failed to scan owned agent: %w", scanErr)
		}
		cascade.OwnedAgentIDs = append(cascade.OwnedAgentIDs, id)
		if active {
			cascade.DeactivatedAgentIDs = append(cascade.DeactivatedAgentIDs, id)
		}
	}
	if err := agentRows.Err(); err != nil {
		_ = agentRows.Close()
		return cascade, fmt.Errorf("failed to iterate owned agents: %w", err)
	}
	_ = agentRows.Close()

	if len(cascade.DeactivatedAgentIDs) > 0 {
		if _, err := tx.Exec(`UPDATE users SET is_active = false, updated_at = CURRENT_TIMESTAMP WHERE agent_owner_user_id = ? AND is_active = true`, ownerID); err != nil {
			return cascade, fmt.Errorf("failed to deactivate owned agents: %w", err)
		}
	}

	// api_tokens has no is_active column, so revocation is a hard DELETE.
	// Row IDs are collected first so the caller can evict validation-cache
	// entries after commit (the cache is keyed by token hash, not user).
	userIDs := append([]int{ownerID}, cascade.OwnedAgentIDs...)
	tokenRows, err := tx.Query(scopedInQuery(`SELECT id FROM api_tokens WHERE user_id IN (`, len(userIDs)), scopedInArgs(userIDs)...)
	if err != nil {
		return cascade, fmt.Errorf("failed to load api_tokens: %w", err)
	}
	for tokenRows.Next() {
		var id int
		if scanErr := tokenRows.Scan(&id); scanErr != nil {
			_ = tokenRows.Close()
			return cascade, fmt.Errorf("failed to scan api_token: %w", scanErr)
		}
		cascade.RevokedAPITokenIDs = append(cascade.RevokedAPITokenIDs, id)
	}
	if err := tokenRows.Err(); err != nil {
		_ = tokenRows.Close()
		return cascade, fmt.Errorf("failed to iterate api_tokens: %w", err)
	}
	_ = tokenRows.Close()

	if len(cascade.RevokedAPITokenIDs) > 0 {
		if _, err := tx.Exec(scopedInQuery(`DELETE FROM api_tokens WHERE user_id IN (`, len(userIDs)), scopedInArgs(userIDs)...); err != nil {
			return cascade, fmt.Errorf("failed to revoke api_tokens: %w", err)
		}
	}

	return cascade, nil
}

// scopedInQuery renders `prefix?,?,...,?)` sized for n placeholders.
func scopedInQuery(prefix string, n int) string {
	return prefix + strings.TrimSuffix(strings.Repeat("?, ", n), ", ") + ")"
}

func scopedInArgs(ids []int) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
