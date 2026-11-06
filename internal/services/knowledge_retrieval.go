package services

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"windshift/internal/database"
	"windshift/internal/integrations/docmost"
	"windshift/internal/repository"
)

// KnowledgeSource identifies where a search hit came from.
const (
	KnowledgeSourcePage    = "page"
	KnowledgeSourceLogbook = "logbook"
	KnowledgeSourceDocmost = "docmost"
)

// KnowledgeSourceState values reported per source.
const (
	KnowledgeSourceStateOK                = "ok"
	KnowledgeSourceStateSkipped           = "skipped"
	KnowledgeSourceStateNotConfigured     = "not_configured"
	KnowledgeSourceStateNotConnected      = "not_connected"
	KnowledgeSourceStateReconnectRequired = "reconnect_required"
	KnowledgeSourceStateUnavailable       = "unavailable"
)

// KnowledgeSourceStatus reports whether a source contributed and, when it did
// not, why. It is additive to the search response so a client can prompt a
// reconnect without parsing logs.
type KnowledgeSourceStatus struct {
	Source string `json:"source"`
	State  string `json:"state"`
}

// KnowledgeSearchResponse is the search result plus per-source states.
type KnowledgeSearchResponse struct {
	Results []KnowledgeResult       `json:"results"`
	Sources []KnowledgeSourceStatus `json:"sources,omitempty"`
}

// KnowledgeResult is a single permission-filtered search hit.
type KnowledgeResult struct {
	Source      string  `json:"source"`
	PageID      int     `json:"page_id,omitempty"`
	ChunkID     int     `json:"chunk_id,omitempty"`
	WorkspaceID int     `json:"workspace_id"`
	Title       string  `json:"title"`
	HeadingPath string  `json:"heading_path,omitempty"`
	URL         string  `json:"url,omitempty"`
	Snippet     string  `json:"snippet"`
	Score       float64 `json:"score"`
}

// DocmostKnowledgeSource resolves the acting user's Docmost credential. The
// concrete implementation is *DocmostService; the interface keeps the
// retrieval service testable and makes the fail-closed contract explicit.
type DocmostKnowledgeSource interface {
	Credential(ctx context.Context, userID int) (baseURL, token string, err error)
	MarkInvalid(userID int)
	MarkVerified(userID int)
}

// DocmostSearcher is the search subset of the Docmost client.
type DocmostSearcher interface {
	Search(ctx context.Context, baseURL, token, query string, limit int) ([]docmost.SearchResult, error)
}

// docmostHighlightTagRE strips the <b> markers Docmost wraps around matched
// terms. The snippet is plain text in the JSON response, never HTML.
var docmostHighlightTagRE = regexp.MustCompile(`<[^>]*>`)

// KnowledgeRetrievalService is a thin facade over per-source repositories
// that always runs the appropriate permission check before returning a
// snippet. Adapters (HTTP search endpoint, AI tools) call into this
// service rather than reaching into the page repository directly so the
// re-check on read can never be skipped.
type KnowledgeRetrievalService struct {
	pages         *repository.PageRepository
	pageAuth      *PagePermissionService
	docmost       DocmostKnowledgeSource
	docmostClient DocmostSearcher
}

// NewKnowledgeRetrievalService wires the retrieval facade against the
// existing repository + permission evaluator.
func NewKnowledgeRetrievalService(db database.Database, pageAuth *PagePermissionService) *KnowledgeRetrievalService {
	return &KnowledgeRetrievalService{
		pages:    repository.NewPageRepository(db),
		pageAuth: pageAuth,
	}
}

// SetDocmost enables the authenticated Docmost source. Without it the source
// stays skipped, so a deployment with no Docmost configured is unaffected.
func (s *KnowledgeRetrievalService) SetDocmost(creds DocmostKnowledgeSource, client DocmostSearcher) {
	s.docmost = creds
	s.docmostClient = client
}

// SearchInput is the request shape for unified search.
type SearchInput struct {
	Ctx         context.Context
	UserID      int
	WorkspaceID int
	Query       string
	Limit       int
	Sources     []string // optional whitelist; empty = "all enabled"
}

// Search runs full-text search over the requested sources and returns ranked
// results. It is the compatibility wrapper for callers that do not need the
// per-source states.
func (s *KnowledgeRetrievalService) Search(in SearchInput) ([]KnowledgeResult, error) {
	response, err := s.SearchWithSources(in)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

// SearchWithSources runs full-text search over the requested sources, filters
// every hit by the relevant per-source permission, and reports each source's
// state alongside the results.
func (s *KnowledgeRetrievalService) SearchWithSources(in SearchInput) (KnowledgeSearchResponse, error) {
	if in.Query == "" {
		return KnowledgeSearchResponse{Results: []KnowledgeResult{}}, nil
	}
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 25
	}
	useSource := func(name string) bool {
		if len(in.Sources) == 0 {
			return true
		}
		for _, s := range in.Sources {
			if s == name {
				return true
			}
		}
		return false
	}

	out := []KnowledgeResult{}
	var sources []KnowledgeSourceStatus
	if useSource(KnowledgeSourcePage) {
		pageHits, err := s.searchPages(in)
		if err != nil {
			return KnowledgeSearchResponse{}, err
		}
		out = append(out, pageHits...)
	}
	if useSource(KnowledgeSourceDocmost) {
		docmostHits, state, report := s.searchDocmost(in)
		out = append(out, docmostHits...)
		// A deployment with no Docmost configured reports nothing for the source
		// (skipped), so the response shape is unchanged for those deployments.
		if report {
			sources = append(sources, KnowledgeSourceStatus{Source: KnowledgeSourceDocmost, State: state})
		}
	}
	// Logbook integration intentionally deferred — the bucket-access model
	// lives outside PagePermissionService and is a separate slice. Until
	// then the source whitelist still accepts "logbook" to keep clients
	// forward-compatible; it just returns nothing.
	if len(out) > in.Limit {
		out = out[:in.Limit]
	}
	return KnowledgeSearchResponse{Results: out, Sources: sources}, nil
}

// searchDocmost queries the acting user's Docmost account. It fails closed: a
// missing, unconfigured, or rejected credential yields no Docmost results (and
// never falls back to the anonymous share path) while native pages keep
// working. A token Docmost rejects is flagged so the client can prompt for
// reconnect.
//
// report is false when the source is not part of this deployment (no service
// wired or no enabled provider), so callers omit it from the per-source status
// instead of advertising a source the deployment does not have.
func (s *KnowledgeRetrievalService) searchDocmost(in SearchInput) (results []KnowledgeResult, state string, report bool) {
	if s.docmost == nil || s.docmostClient == nil {
		return nil, KnowledgeSourceStateSkipped, false
	}
	ctx := in.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	baseURL, token, err := s.docmost.Credential(ctx, in.UserID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return nil, KnowledgeSourceStateNotConnected, true
		case errors.Is(err, ErrDocmostNotConfigured):
			return nil, KnowledgeSourceStateNotConfigured, false
		case errors.Is(err, ErrDocmostReconnect):
			return nil, KnowledgeSourceStateReconnectRequired, true
		default:
			return nil, KnowledgeSourceStateUnavailable, true
		}
	}

	hits, err := s.docmostClient.Search(ctx, baseURL, token, in.Query, in.Limit)
	if err != nil {
		if errors.Is(err, docmost.ErrUnauthorized) {
			s.docmost.MarkInvalid(in.UserID)
			return nil, KnowledgeSourceStateReconnectRequired, true
		}
		return nil, KnowledgeSourceStateUnavailable, true
	}
	s.docmost.MarkVerified(in.UserID)

	results = make([]KnowledgeResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, KnowledgeResult{
			Source:      KnowledgeSourceDocmost,
			WorkspaceID: in.WorkspaceID,
			Title:       hit.Title,
			URL:         docmost.PageURL(baseURL, hit.SpaceSlug, hit.SlugID),
			Snippet:     stripHighlightTags(hit.Highlight),
			Score:       hit.Rank,
		})
	}
	return results, KnowledgeSourceStateOK, true
}

// stripHighlightTags turns a Docmost highlight into plain text: strip the
// <b> markers first, then decode entities the page text may contain.
func stripHighlightTags(highlight string) string {
	return strings.TrimSpace(html.UnescapeString(docmostHighlightTagRE.ReplaceAllString(highlight, "")))
}

func (s *KnowledgeRetrievalService) searchPages(in SearchInput) ([]KnowledgeResult, error) {
	// Over-fetch a bit so the post-permission filter doesn't starve the
	// response when several leading hits are inaccessible.
	rawLimit := in.Limit * 3
	hits, err := s.pages.SearchChunks(in.WorkspaceID, in.Query, rawLimit)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// Re-check permission per result. The permission evaluator is cheap
	// for the open-page case (which is what the workspace-tree default
	// produces) and prevents content from a restricted page surfacing
	// through search even if a hostile query somehow matched a hidden
	// chunk.
	out := make([]KnowledgeResult, 0, len(hits))
	cache := make(map[int]bool, len(hits))
	for _, hit := range hits {
		can, ok := cache[hit.PageID]
		if !ok {
			c, err := s.pageAuth.Can(in.UserID, in.WorkspaceID, hit.PageID, PageOpView)
			if err != nil {
				return nil, fmt.Errorf("permission check for page %d: %w", hit.PageID, err)
			}
			can = c
			cache[hit.PageID] = c
		}
		if !can {
			continue
		}
		page, err := s.pages.GetByID(hit.PageID)
		if err != nil {
			return nil, err
		}
		out = append(out, KnowledgeResult{
			Source:      KnowledgeSourcePage,
			PageID:      hit.PageID,
			ChunkID:     hit.ChunkID,
			WorkspaceID: hit.WorkspaceID,
			Title:       page.Title,
			HeadingPath: hit.HeadingPath,
			URL:         fmt.Sprintf("/workspaces/%d/pages/%d", hit.WorkspaceID, hit.PageID),
			Snippet:     hit.Snippet,
			Score:       hit.Score,
		})
		if len(out) >= in.Limit {
			break
		}
	}
	return out, nil
}
