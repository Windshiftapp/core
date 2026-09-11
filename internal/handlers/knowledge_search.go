package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/services"
)

type KnowledgeSearchHandler struct {
	retrieval *services.KnowledgeRetrievalService
}

func NewKnowledgeSearchHandler(retrieval *services.KnowledgeRetrievalService) *KnowledgeSearchHandler {
	return &KnowledgeSearchHandler{retrieval: retrieval}
}

func (h *KnowledgeSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	query := r.URL.Query().Get("q")
	results, err := h.retrieval.Search(services.SearchInput{
		UserID: user.ID, WorkspaceID: workspaceID, Query: query, Limit: limit,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if results == nil {
		results = []services.KnowledgeResult{}
	}
	respondJSONOK(w, map[string]any{"results": results, "query": query})
}
