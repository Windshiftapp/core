package models

import (
	"encoding/json"
	"time"
)

// Custom widget visibility. Workspace-visible widgets are readable by every
// member of the workspace; private widgets belong to one user.
const (
	CustomWidgetVisibilityPrivate   = "private"
	CustomWidgetVisibilityWorkspace = "workspace"
)

// CustomWidget is a user-authored dashboard widget rendered from an OpenUI
// program. Program and Queries are opaque to the API layer: the frontend
// renderer interprets the program, and the data endpoint interprets Queries.
type CustomWidget struct {
	ID             int             `json:"id"`
	WorkspaceID    *int            `json:"workspace_id,omitempty"`
	OwnerUserID    *int            `json:"owner_user_id,omitempty"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Program        string          `json:"program"`
	LangVersion    string          `json:"lang_version"`
	LibraryVersion string          `json:"library_version"`
	Queries        json.RawMessage `json:"queries"`
	Visibility     string          `json:"visibility"`
	CreatedBy      int             `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
