package handlers

import (
	"strconv"
	"strings"
)

// buildWorkspaceMap creates a mapping of workspace identifiers for VQL evaluation
func (h *WorkspaceHandler) buildWorkspaceMap() (map[string]int, error) {
	workspaceMap := make(map[string]int)

	rows, err := h.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			return nil, err
		}

		workspaceMap[strconv.Itoa(id)] = id
		workspaceMap[strings.ToLower(name)] = id
		workspaceMap[strings.ToLower(key)] = id
	}

	return workspaceMap, nil
}
