package handlers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"windshift/internal/models"
)

var personalWorkspaceKeySanitizer = regexp.MustCompile(`[^A-Za-z0-9]+`)

// GetOrCreatePersonalWorkspace gets or creates a personal workspace for a user
func (h *WorkspaceHandler) GetOrCreatePersonalWorkspace(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	userID := user.ID
	userName := user.Username
	if userName == "" {
		userName = "User"
	}

	// Check if personal workspace already exists for this user
	var workspace models.Workspace
	var timeProjectName sql.NullString
	err := h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.is_personal = true AND w.owner_id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at, tp.name
	`, userID).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.ProjectCount, &timeProjectName)

	if err == nil {
		// Personal workspace exists, return it
		workspace.TimeProjectName = timeProjectName.String
		respondJSONOK(w, workspace)
		return
	}

	if err != sql.ErrNoRows {
		// Database error occurred
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Personal workspace doesn't exist, create it
	// Use first name if available, otherwise fall back to username
	displayName := userName
	if user.FirstName != "" {
		displayName = user.FirstName
	}
	workspaceName := displayName + "'s Todo List"

	// Generate slugified workspace key derived from the user's name
	baseKey := h.generatePersonalWorkspaceKey(displayName, userName, userID)

	// Check for uniqueness and add counter if needed
	workspaceKey := baseKey
	counter := 1
	for {
		var exists bool
		checkErr := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE key = ?)", workspaceKey).Scan(&exists)
		if checkErr != nil || !exists {
			break
		}
		workspaceKey = baseKey + "-" + strconv.Itoa(counter)
		counter++
	}

	description := "Personal todo list and task management"

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceName, workspaceKey, description, true, nil, true, userID, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create item number sequence for this workspace (PostgreSQL only, no-op for SQLite)
	if err := h.db.CreateWorkspaceItemSequence(id); err != nil {
		slog.Warn("failed to create item sequence for personal workspace", slog.String("component", "workspaces"), slog.Int64("workspace_id", id), slog.Any("error", err))
	}

	// Return the created personal workspace
	err = h.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at, tp.name
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt, &workspace.ProjectCount, &timeProjectName)

	workspace.TimeProjectName = timeProjectName.String

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONCreated(w, workspace)
}

// generatePersonalWorkspaceKey builds a slug-like key (max ~10 chars) based on user identity.
func (h *WorkspaceHandler) generatePersonalWorkspaceKey(displayName, userName string, userID int) string {
	candidates := []string{displayName, userName}
	for _, candidate := range candidates {
		if key := sanitizePersonalWorkspaceKeyCandidate(candidate); key != "" {
			return key
		}
	}
	return sanitizePersonalWorkspaceKeyCandidate(fmt.Sprintf("USER-%d", userID))
}

func sanitizePersonalWorkspaceKeyCandidate(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	key := personalWorkspaceKeySanitizer.ReplaceAllString(strings.ToUpper(input), "-")
	key = strings.Trim(key, "-")

	// Keep workspace keys reasonably short to match create/update validation expectations.
	const maxKeyLength = 10
	if len(key) > maxKeyLength {
		key = key[:maxKeyLength]
		key = strings.Trim(key, "-")
	}

	if key == "" {
		return ""
	}

	return key
}
