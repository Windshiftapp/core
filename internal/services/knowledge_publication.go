package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// maxKnowledgeBasePageSources caps how many page sources one portal may
// wire into its knowledge base.
const maxKnowledgeBasePageSources = 20

// KnowledgePublicationService resolves which workspace pages are published
// through portal knowledge bases. A channel manager explicitly wires a
// workspace (or a sub-page subtree) into a portal's knowledge base via the
// channel config; the publication decision made there is the access
// decision — published pages are treated as publicly viewable and are not
// re-filtered through the per-page ACL.
type KnowledgePublicationService struct {
	db    database.Database
	pages *repository.PageRepository
}

// NewKnowledgePublicationService builds the publication resolver.
func NewKnowledgePublicationService(db database.Database) *KnowledgePublicationService {
	return &KnowledgePublicationService{
		db:    db,
		pages: repository.NewPageRepository(db),
	}
}

// PublishedSource is one wired page source with the portal that exposes it.
type PublishedSource struct {
	ChannelID   int
	PortalTitle string
	WorkspaceID int
	RootPageID  *int
}

// PublishedSources returns every page source wired into an enabled portal.
// Disabled portals stop publishing immediately.
func (s *KnowledgePublicationService) PublishedSources() ([]PublishedSource, error) {
	rows, err := s.db.Query(`SELECT id, config FROM channels WHERE type = 'portal' AND status = 'enabled'`)
	if err != nil {
		return nil, fmt.Errorf("list portal channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []PublishedSource
	for rows.Next() {
		var channelID int
		var configJSON string
		if err := rows.Scan(&channelID, &configJSON); err != nil {
			return nil, err
		}
		var config models.ChannelConfig
		if err := parseChannelConfig(configJSON, &config); err != nil {
			return nil, err
		}
		for _, src := range config.KnowledgeBasePageSources {
			out = append(out, PublishedSource{
				ChannelID:   channelID,
				PortalTitle: config.PortalTitle,
				WorkspaceID: src.WorkspaceID,
				RootPageID:  src.RootPageID,
			})
		}
	}
	return out, rows.Err()
}

// sourcesForWorkspace returns the enabled-portal sources that publish the
// given workspace.
func (s *KnowledgePublicationService) sourcesForWorkspace(workspaceID int) ([]PublishedSource, error) {
	all, err := s.PublishedSources()
	if err != nil {
		return nil, err
	}
	var out []PublishedSource
	for _, src := range all {
		if src.WorkspaceID == workspaceID {
			out = append(out, src)
		}
	}
	return out, nil
}

// PagePublication reports whether the page is published through enabled
// portal knowledge bases and which portals expose it.
func (s *KnowledgePublicationService) PagePublication(page *models.Page) (published bool, portals []string, err error) {
	sources, err := s.sourcesForWorkspace(page.WorkspaceID)
	if err != nil {
		return false, nil, err
	}
	viewable := false
	var portalTitles []string
	seenPortals := map[int]bool{}
	for _, src := range sources {
		inScope := false
		if src.RootPageID == nil {
			inScope = true
		} else {
			inScope, err = s.pageInSubtree(page, *src.RootPageID)
			if err != nil {
				return false, nil, err
			}
		}
		if !inScope {
			continue
		}
		viewable = true
		if !seenPortals[src.ChannelID] {
			seenPortals[src.ChannelID] = true
			portalTitles = append(portalTitles, src.PortalTitle)
		}
	}
	return viewable, portalTitles, nil
}

// pageInSubtree reports whether page is the root itself or one of its
// descendants. The root must exist unarchived in the page's workspace — an
// archived root means the wiring no longer publishes anything (archive
// cascades to descendants, so live descendants imply a live root anyway).
func (s *KnowledgePublicationService) pageInSubtree(page *models.Page, rootPageID int) (bool, error) {
	if page.ID == rootPageID {
		return page.ArchivedAt == nil, nil
	}
	root, err := s.pages.GetByID(rootPageID)
	if err != nil {
		if err == repository.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	if root.WorkspaceID != page.WorkspaceID || root.ArchivedAt != nil {
		return false, nil
	}
	prefix := root.Path + strconv.Itoa(root.ID) + "/"
	return strings.HasPrefix(page.Path, prefix), nil
}

// PublishedPageHit is one search hit from published workspace pages.
type PublishedPageHit struct {
	PageID      int
	WorkspaceID int
	Title       string
	HeadingPath string
	Snippet     string
	Score       float64
}

// SearchPublishedPagesForPortal full-text searches the pages published by
// this portal's knowledge-base wiring and returns ranked hits. Only pages
// inside a wired workspace (and inside a wired subtree when the wiring
// names a root) match. Sources come from the given channel config so one
// portal never sees pages wired into a different portal.
func (s *KnowledgePublicationService) SearchPublishedPagesForPortal(config models.ChannelConfig, query string, limit int) ([]PublishedPageHit, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	sources := make([]PublishedSource, 0, len(config.KnowledgeBasePageSources))
	for _, src := range config.KnowledgeBasePageSources {
		sources = append(sources, PublishedSource{
			WorkspaceID: src.WorkspaceID,
			RootPageID:  src.RootPageID,
		})
	}
	if len(sources) == 0 {
		return nil, nil
	}

	// Group whole-workspace and subtree wirings per workspace so each
	// workspace is searched once.
	wholeWorkspaces := map[int]bool{}
	rootsByWorkspace := map[int][]int{}
	for _, src := range sources {
		if src.RootPageID == nil {
			wholeWorkspaces[src.WorkspaceID] = true
		} else {
			rootsByWorkspace[src.WorkspaceID] = append(rootsByWorkspace[src.WorkspaceID], *src.RootPageID)
		}
	}

	rawLimit := limit * 3
	out := make([]PublishedPageHit, 0, limit)
	seen := map[int]bool{}
	appendHits := func(hits []repository.PageChunkSearchResult, workspaceID int, include func(page *models.Page) bool) error {
		for _, hit := range hits {
			if len(out) >= limit {
				return nil
			}
			if seen[hit.PageID] {
				continue
			}
			page, err := s.pages.GetByID(hit.PageID)
			if err != nil {
				if err == repository.ErrNotFound {
					continue
				}
				return err
			}
			if page.ArchivedAt != nil || page.WorkspaceID != workspaceID || !include(page) {
				continue
			}
			seen[hit.PageID] = true
			out = append(out, PublishedPageHit{
				PageID:      hit.PageID,
				WorkspaceID: hit.WorkspaceID,
				Title:       page.Title,
				HeadingPath: hit.HeadingPath,
				Snippet:     hit.Snippet,
				Score:       hit.Score,
			})
		}
		return nil
	}

	for workspaceID := range wholeWorkspaces {
		hits, err := s.pages.SearchChunks(workspaceID, query, rawLimit)
		if err != nil {
			return nil, err
		}
		if err := appendHits(hits, workspaceID, func(*models.Page) bool { return true }); err != nil {
			return nil, err
		}
		if len(out) >= limit {
			return out, nil
		}
	}
	for workspaceID, rootIDs := range rootsByWorkspace {
		roots := make([]*models.Page, 0, len(rootIDs))
		for _, rootID := range rootIDs {
			root, err := s.pages.GetByID(rootID)
			if err == repository.ErrNotFound {
				continue
			}
			if err != nil {
				return nil, err
			}
			if root.ArchivedAt != nil || root.WorkspaceID != workspaceID {
				continue
			}
			roots = append(roots, root)
		}
		if len(roots) == 0 {
			continue
		}
		hits, err := s.pages.SearchChunks(workspaceID, query, rawLimit)
		if err != nil {
			return nil, err
		}
		include := func(page *models.Page) bool {
			for _, root := range roots {
				prefix := root.Path + strconv.Itoa(root.ID) + "/"
				if page.ID == root.ID || strings.HasPrefix(page.Path, prefix) {
					return true
				}
			}
			return false
		}
		if err := appendHits(hits, workspaceID, include); err != nil {
			return nil, err
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// PublishedPagesForPortal lists the live pages this portal's knowledge
// base publishes, for the browse listing. Whole-workspace wirings
// contribute every live page of the workspace; subtree wirings contribute
// the root and its descendants. Pages arrive in each workspace's stable
// tree order (wired workspaces in ascending ID order), and a page wired
// through several sources appears once. Mirrors PublishedPageForPortal's
// inclusion rules so browse, search, and detail always agree.
func (s *KnowledgePublicationService) PublishedPagesForPortal(config models.ChannelConfig) ([]models.Page, error) {
	if len(config.KnowledgeBasePageSources) == 0 {
		return nil, nil
	}

	wholeWorkspaces := map[int]bool{}
	rootsByWorkspace := map[int][]int{}
	workspaceIDs := make([]int, 0, len(config.KnowledgeBasePageSources))
	seenWorkspaces := map[int]bool{}
	for _, src := range config.KnowledgeBasePageSources {
		if src.RootPageID == nil {
			wholeWorkspaces[src.WorkspaceID] = true
		}
		if !seenWorkspaces[src.WorkspaceID] {
			seenWorkspaces[src.WorkspaceID] = true
			workspaceIDs = append(workspaceIDs, src.WorkspaceID)
		}
		if src.RootPageID != nil {
			rootsByWorkspace[src.WorkspaceID] = append(rootsByWorkspace[src.WorkspaceID], *src.RootPageID)
		}
	}
	sort.Ints(workspaceIDs)

	out := make([]models.Page, 0, len(config.KnowledgeBasePageSources))
	seenPages := map[int]bool{}
	for _, workspaceID := range workspaceIDs {
		pages, err := s.pages.ListWorkspaceTreeMeta(workspaceID, false)
		if err != nil {
			return nil, err
		}
		roots := make([]*models.Page, 0, len(rootsByWorkspace[workspaceID]))
		for _, rootID := range rootsByWorkspace[workspaceID] {
			root, err := s.pages.GetByID(rootID)
			if err == repository.ErrNotFound {
				continue
			}
			if err != nil {
				return nil, err
			}
			if root.ArchivedAt != nil || root.WorkspaceID != workspaceID {
				continue
			}
			roots = append(roots, root)
		}

		for _, page := range pages {
			if seenPages[page.ID] {
				continue
			}
			included := wholeWorkspaces[workspaceID]
			if !included {
				for _, root := range roots {
					prefix := root.Path + strconv.Itoa(root.ID) + "/"
					if page.ID == root.ID || strings.HasPrefix(page.Path, prefix) {
						included = true
						break
					}
				}
			}
			if included {
				seenPages[page.ID] = true
				out = append(out, page)
			}
		}
	}
	return out, nil
}

// PublishedPageForPortal returns the page with the given ID when the
// portal channel publishes it (whole-workspace or subtree wiring) and the
// page is not archived. Any other case is ErrNotFound so callers can emit
// the usual 404 without leaking existence.
func (s *KnowledgePublicationService) PublishedPageForPortal(config models.ChannelConfig, pageID int) (*models.Page, error) {
	page, err := s.pages.GetByID(pageID)
	if err != nil {
		return nil, err
	}
	if page.ArchivedAt != nil {
		return nil, repository.ErrNotFound
	}
	for _, src := range config.KnowledgeBasePageSources {
		if src.WorkspaceID != page.WorkspaceID {
			continue
		}
		if src.RootPageID == nil {
			return page, nil
		}
		inSubtree, err := s.pageInSubtree(page, *src.RootPageID)
		if err != nil {
			return nil, err
		}
		if inSubtree {
			return page, nil
		}
	}
	return nil, repository.ErrNotFound
}

// parseChannelConfig decodes a stored channel config JSON blob.
func parseChannelConfig(configJSON string, out *models.ChannelConfig) error {
	if strings.TrimSpace(configJSON) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(configJSON), out); err != nil {
		return fmt.Errorf("decode channel config: %w", err)
	}
	return nil
}
