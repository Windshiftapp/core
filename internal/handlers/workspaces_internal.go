package handlers

// buildWorkspaceMap creates a mapping of workspace identifiers for VQL evaluation
func (h *WorkspaceHandler) buildWorkspaceMap() (map[string]int, error) {
	return h.repo.BuildWorkspaceMap()
}
