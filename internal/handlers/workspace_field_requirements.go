package handlers

import (
	"windshift/internal/database"
	"database/sql"
	"encoding/json"
	"windshift/internal/models"
	"net/http"
	"strconv"

)

type WorkspaceFieldRequirementHandler struct {
	db database.Database
}

func NewWorkspaceFieldRequirementHandler(db database.Database) *WorkspaceFieldRequirementHandler {
	return &WorkspaceFieldRequirementHandler{db: db}
}

func (h *WorkspaceFieldRequirementHandler) GetByWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT wfr.id, wfr.workspace_id, wfr.custom_field_id, wfr.is_required,
		       cfd.name, cfd.field_type, w.name
		FROM workspace_field_requirements wfr
		JOIN custom_field_definitions cfd ON wfr.custom_field_id = cfd.id
		JOIN workspaces w ON wfr.workspace_id = w.id
		WHERE wfr.workspace_id = ?
		ORDER BY cfd.display_order, cfd.name
	`
	
	rows, err := h.db.Query(query, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type WorkspaceFieldRequirement struct {
		ID            int    `json:"id"`
		WorkspaceID   int    `json:"workspace_id"`
		CustomFieldID int    `json:"custom_field_id"`
		IsRequired    bool   `json:"is_required"`
		FieldName     string `json:"field_name"`
		FieldType     string `json:"field_type"`
		WorkspaceName string `json:"workspace_name"`
	}

	var requirements []WorkspaceFieldRequirement
	for rows.Next() {
		var req WorkspaceFieldRequirement
		err := rows.Scan(&req.ID, &req.WorkspaceID, &req.CustomFieldID, &req.IsRequired,
			&req.FieldName, &req.FieldType, &req.WorkspaceName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		requirements = append(requirements, req)
	}

	// Always return an array, even if empty
	if requirements == nil {
		requirements = []WorkspaceFieldRequirement{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requirements)
}

func (h *WorkspaceFieldRequirementHandler) SetRequirement(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	var req struct {
		CustomFieldID int  `json:"custom_field_id"`
		IsRequired    bool `json:"is_required"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if workspace exists
	var workspaceExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", workspaceID).Scan(&workspaceExists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !workspaceExists {
		http.Error(w, "Workspace not found", http.StatusBadRequest)
		return
	}

	// Check if custom field exists
	var fieldExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM custom_field_definitions WHERE id = ?)", req.CustomFieldID).Scan(&fieldExists)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !fieldExists {
		http.Error(w, "Custom field not found", http.StatusBadRequest)
		return
	}

	// Insert or update the requirement (upsert using delete then insert for cross-database compatibility)
	_, err = h.db.ExecWrite(`DELETE FROM workspace_field_requirements WHERE workspace_id = ? AND custom_field_id = ?`, workspaceID, req.CustomFieldID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = h.db.ExecWrite(`
		INSERT INTO workspace_field_requirements (workspace_id, custom_field_id, is_required)
		VALUES (?, ?, ?)
	`, workspaceID, req.CustomFieldID, req.IsRequired)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (h *WorkspaceFieldRequirementHandler) RemoveRequirement(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("workspaceId"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}
	
	fieldID, err := strconv.Atoi(r.PathValue("fieldId"))
	if err != nil {
		http.Error(w, "Invalid field ID", http.StatusBadRequest)
		return
	}

	_, err = h.db.ExecWrite(`
		DELETE FROM workspace_field_requirements 
		WHERE workspace_id = ? AND custom_field_id = ?
	`, workspaceID, fieldID)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceFieldRequirementHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT cfd.id, cfd.name, cfd.field_type, cfd.description, cfd.required, 
		       cfd.options, cfd.display_order, cfd.created_at, cfd.updated_at,
		       COALESCE(wfr.is_required, 0) as is_required
		FROM custom_field_definitions cfd
		LEFT JOIN workspace_field_requirements wfr ON cfd.id = wfr.custom_field_id AND wfr.workspace_id = ?
		ORDER BY cfd.display_order, cfd.name
	`
	
	rows, err := h.db.Query(query, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type FieldWithRequirement struct {
		models.CustomFieldDefinition
		IsRequired bool `json:"is_required"`
	}

	var fields []FieldWithRequirement
	for rows.Next() {
		var field FieldWithRequirement
		var optionsJSON sql.NullString
		
		err := rows.Scan(&field.ID, &field.Name, &field.FieldType, &field.Description,
			&field.Required, &optionsJSON, &field.DisplayOrder, 
			&field.CreatedAt, &field.UpdatedAt, &field.IsRequired)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Set options string
		if optionsJSON.Valid {
			field.Options = optionsJSON.String
		}
		
		fields = append(fields, field)
	}

	// Always return an array, even if empty
	if fields == nil {
		fields = []FieldWithRequirement{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fields)
}