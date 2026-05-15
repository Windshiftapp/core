package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
)

// AgentOwnership describes whether a user is an agent and who owns it.
type AgentOwnership struct {
	IsAgent bool
	OwnerID *int
	Exists  bool
}

// APITokenPolicyRepository provides supporting reads for API-token policy handlers.
type APITokenPolicyRepository struct {
	db database.Database
}

// NewAPITokenPolicyRepository creates an APITokenPolicyRepository.
func NewAPITokenPolicyRepository(db database.Database) *APITokenPolicyRepository {
	return &APITokenPolicyRepository{db: db}
}

// LoadAgentOwnership reads the agent flag + owner link for a user ID.
func (r *APITokenPolicyRepository) LoadAgentOwnership(userID int) (AgentOwnership, error) {
	var out AgentOwnership
	var owner sql.NullInt64
	err := r.db.QueryRow(
		"SELECT COALESCE(is_agent, false), agent_owner_user_id FROM users WHERE id = ?",
		userID,
	).Scan(&out.IsAgent, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentOwnership{Exists: false}, nil
	}
	if err != nil {
		return AgentOwnership{}, fmt.Errorf("load agent ownership for user %d: %w", userID, err)
	}
	out.Exists = true
	if owner.Valid {
		v := int(owner.Int64)
		out.OwnerID = &v
	}
	return out, nil
}

// GetSystemSetting returns a system setting value, or ok=false when absent.
func (r *APITokenPolicyRepository) GetSystemSetting(key string) (value string, ok bool, err error) {
	err = r.db.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get system setting %q: %w", key, err)
	}
	return value, true, nil
}

// UserInAnyGroup reports whether userID is in any groupIDs.
func (r *APITokenPolicyRepository) UserInAnyGroup(userID int, groupIDs []int) (bool, error) {
	if len(groupIDs) == 0 {
		return false, nil
	}
	placeholders := make([]string, len(groupIDs))
	args := make([]interface{}, 0, len(groupIDs)+1)
	args = append(args, userID)
	for i, gid := range groupIDs {
		placeholders[i] = "?"
		args = append(args, gid)
	}
	var count int
	query := "SELECT COUNT(*) FROM group_members WHERE user_id = ? AND group_id IN (" + strings.Join(placeholders, ",") + ")"
	if err := r.db.QueryRow(query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("check user %d allowed groups: %w", userID, err)
	}
	return count > 0, nil
}
