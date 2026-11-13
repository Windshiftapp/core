package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

// GetOrCreatePersonalWorkspace gets or creates a personal workspace for a user.
//
// Unlike the regular Create handler, this path intentionally does NOT require
// the global workspace.create permission. A personal "Todo List" workspace is
// a per-user baseline provisioned on first access — gating it on workspace.create
// would strand users whose admin never granted that permission. Authentication
// is the only gate by design; do not add a canCreateWorkspace check here.
func (h *WorkspaceHandler) GetOrCreatePersonalWorkspace(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID
	userName := user.Username
	if userName == "" {
		userName = "User"
	}

	now := time.Now()
	var workspace models.Workspace
	changed := false // created or reactivated — drives cache invalidation
	created := false // freshly inserted — drives the 201 status

	err := database.WithTx(h.db, func(tx database.Tx) error {
		// One active personal workspace per owner — the partial unique index
		// on (owner_id) WHERE is_personal guarantees it at the schema level.
		var timeProjectName sql.NullString
		err := tx.QueryRow(`
			SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
			       tp.name as time_project_name
			FROM workspaces w
			LEFT JOIN time_projects tp ON w.time_project_id = tp.id
			WHERE w.is_personal = true AND w.owner_id = ? AND w.active = true
		`, userID).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
			&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt,
			&timeProjectName)

		if err == nil {
			workspace.TimeProjectName = timeProjectName.String
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		// An owner can reach an inactive personal workspace only through
		// deprovisioning (SCIM). Get-or-create semantics: reactivate it.
		var inactiveID int64
		err = tx.QueryRow(`
			SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ? AND active = false
		`, userID).Scan(&inactiveID)
		if err == nil {
			if _, err := tx.Exec(`UPDATE workspaces SET active = true, updated_at = ? WHERE id = ?`, now, inactiveID); err != nil {
				return err
			}
			err := tx.QueryRow(`
				SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
				       tp.name as time_project_name
				FROM workspaces w
				LEFT JOIN time_projects tp ON w.time_project_id = tp.id
				WHERE w.id = ?
			`, inactiveID).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
				&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt,
				&timeProjectName)
			if err != nil {
				return err
			}
			workspace.TimeProjectName = timeProjectName.String
			changed = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		// Create. The key derives from the owner ID: alphanumeric, within the
		// 2-10 key contract, and collision-free except for the (vanishingly
		// unlikely) case of a regular workspace already owning "P<id>".
		displayName := userName
		if user.FirstName != "" {
			displayName = user.FirstName
		}
		workspaceName := displayName + "'s Todo List"
		description := "Personal todo list and task management"

		for _, candidate := range personalWorkspaceKeyCandidates(userID) {
			// Postgres aborts the whole transaction on a failed statement, so
			// each key attempt needs its own savepoint to retry from.
			const attempt = "personal_key_attempt"
			if _, err := tx.Exec("SAVEPOINT " + attempt); err != nil {
				return err
			}
			err := tx.QueryRow(`
				INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				RETURNING id
			`, workspaceName, candidate, description, true, nil, true, userID, now, now).Scan(&workspace.ID)
			if err == nil {
				if _, err := tx.Exec("RELEASE SAVEPOINT " + attempt); err != nil {
					return err
				}
				workspace.Name = workspaceName
				workspace.Key = candidate
				workspace.Description = description
				workspace.Active = true
				workspace.IsPersonal = true
				workspace.OwnerID = &userID
				workspace.CreatedAt = now
				workspace.UpdatedAt = now
				changed = true
				created = true
				if err := h.repo.CreateItemSequenceTx(tx, int64(workspace.ID)); err != nil {
					slog.Warn("failed to create item sequence for personal workspace", slog.String("component", "workspaces"), slog.Int64("workspace_id", int64(workspace.ID)), slog.Any("error", err))
				}
				return nil
			}
			if _, rbErr := tx.Exec("ROLLBACK TO SAVEPOINT " + attempt); rbErr != nil {
				return rbErr
			}
			if !database.IsUniqueConstraintError(err) {
				return err
			}
			// Key taken (or a concurrent creator won the owner race):
			// fall through to the next key candidate; if the owner race,
			// the re-select below finds the winner.
		}

		// Every key candidate collided — either a pathological regular-workspace
		// naming pattern or a concurrent first-access race. Re-read; the race
		// winner exists by now.
		err = tx.QueryRow(`
			SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
			       tp.name as time_project_name
			FROM workspaces w
			LEFT JOIN time_projects tp ON w.time_project_id = tp.id
			WHERE w.is_personal = true AND w.owner_id = ? AND w.active = true
		`, userID).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
			&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt,
			&timeProjectName)
		if err != nil {
			return fmt.Errorf("personal workspace key exhausted and no existing workspace found: %w", err)
		}
		workspace.TimeProjectName = timeProjectName.String
		return nil
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.cacheInvalidator.Apply(services.AuthorizationInvalidation{
		UserIDs:                 []int{userID},
		ActiveWorkspacesChanged: changed,
		WorkspaceKeysChanged:    changed,
	}); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if created {
		respondJSONCreated(w, workspace)
		return
	}
	respondJSONOK(w, workspace)
}

// personalWorkspaceKeyCandidates lists the key candidates for a personal
// workspace: P<userID> first, then letter-suffixed variants for the
// pathological case where a regular workspace already owns the base key.
//
// Fallbacks use letters only, so every suffixed candidate ends in a letter and
// can never equal another user's base key (P + decimal digits) or another
// user's fallback (the trailing letter pins the suffix position; the digit run
// pins the owner). Collisions therefore cannot cascade across users — only a
// regular workspace owning the exact candidate triggers the next one, via the
// savepoint retry. All candidates satisfy ^[A-Z0-9]+$ and the 10-char cap for
// user IDs up to 9 digits.
func personalWorkspaceKeyCandidates(userID int) []string {
	base := fmt.Sprintf("P%d", userID)
	candidates := []string{base}
	for _, suffix := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"} {
		if candidate := base + suffix; len(candidate) <= 10 {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}
