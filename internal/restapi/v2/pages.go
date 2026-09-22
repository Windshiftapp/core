package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerPageRoutes(builder *routeBuilder, deps Deps) {
	pages := deps.PageApplication
	builder.Read("/workspaces/{workspace_id}/pages", AuthAuthenticated, []string{"pages:read"}, listPages(pages))
	builder.Read("/pages/titles", AuthAuthenticated, []string{"pages:read"}, listPageTitlesAcrossWorkspaces(pages))
	builder.Read("/workspaces/{workspace_id}/pages/effective-levels", AuthAuthenticated, []string{"pages:read"}, listPageEffectiveLevels(pages))
	builder.Read("/workspaces/{workspace_id}/pages/archived", AuthAuthenticated, []string{"pages:read"}, listArchivedPages(pages))
	builder.Read("/workspaces/{workspace_id}/pages/search", AuthAuthenticated, []string{"pages:read"}, searchPages(pages))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/pages", http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, createPage(pages))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}", AuthAuthenticated, []string{"pages:read"}, getPage(pages))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/pages/{page_id}", http.StatusOK, true, AuthAuthenticated, []string{"pages:write"}, updatePage(pages))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/pages/{page_id}", AuthAuthenticated, []string{"pages:delete"}, archivePage(pages))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/move", http.StatusOK, false, AuthAuthenticated, []string{"pages:write"}, movePage(pages))
	builder.Action(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/unarchive", http.StatusOK, AuthAuthenticated, []string{"pages:write"}, unarchivePage(pages))
	builder.Page("/workspaces/{workspace_id}/pages/{page_id}/history", AuthAuthenticated, []string{"pages:read"}, listPageHistory(deps))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}/history/{revision_id}", AuthAuthenticated, []string{"pages:read"}, getPageRevision(pages))
	builder.Action(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/history/{revision_id}/restore", http.StatusOK, AuthAuthenticated, []string{"pages:write"}, restorePageRevision(pages))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}/permissions", AuthAuthenticated, []string{"pages:read"}, getPagePermissions(pages))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}/publication", AuthAuthenticated, []string{"pages:read"}, getPagePublication(deps))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/permissions", http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, grantPagePermission(pages))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/pages/{page_id}/permissions/{permission_id}", AuthAuthenticated, []string{"pages:write"}, revokePagePermission(pages))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/pages/{page_id}/inheritance", http.StatusOK, true, AuthAuthenticated, []string{"pages:write"}, setPageInheritance(pages))
}

type createPageRequest struct {
	ParentID *int            `json:"parent_id"`
	Title    string          `json:"title"`
	Content  string          `json:"content"`
	Metadata json.RawMessage `json:"metadata"`
	IsHome   bool            `json:"is_home"`
}

type patchPageRequest struct {
	Title               Optional[string]          `json:"title"`
	Content             Optional[string]          `json:"content"`
	Metadata            Optional[json.RawMessage] `json:"metadata"`
	ExpectedContentHash Optional[string]          `json:"expected_content_hash"`
}

type movePageRequest struct {
	DestinationWorkspaceID *int `json:"destination_workspace_id"`
	ParentID               *int `json:"parent_id"`
	PrevSiblingID          *int `json:"prev_sibling_id"`
	NextSiblingID          *int `json:"next_sibling_id"`
}

type grantPagePermissionRequest struct {
	PrincipalType   string `json:"principal_type"`
	PrincipalID     int    `json:"principal_id"`
	PermissionLevel string `json:"permission_level"`
}

type pageInheritanceRequest struct {
	InheritPermissions bool `json:"inherit_permissions"`
}

type archivedPageDTO struct {
	ID             int       `json:"id"`
	Title          string    `json:"title"`
	Slug           string    `json:"slug"`
	Path           string    `json:"path"`
	Depth          int       `json:"depth"`
	ArchivedAt     time.Time `json:"archived_at"`
	ArchivedBy     *int      `json:"archived_by"`
	ArchivedByName string    `json:"archived_by_name"`
}

func listPages(pages pageApplication) readOperation[[]models.Page] {
	return func(r *http.Request) ([]models.Page, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		result, err := pages.List(user.ID, workspaceID)
		return result, pageError(err)
	}
}

// maxTitleWorkspaces bounds one titles request; consumers needing more
// workspace sources must batch their lookups.
const maxTitleWorkspaces = 50

// listPageTitlesAcrossWorkspaces serves the portal customize panel's
// id+title lookups for several KB source workspaces in one request (WI-1447)
// instead of one full-page-list fetch per workspace.
func listPageTitlesAcrossWorkspaces(pages pageApplication) readOperation[[]services.PageTitleRow] {
	return func(r *http.Request) ([]services.PageTitleRow, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		workspaceIDs := parseWorkspaceIDList(r.URL.Query().Get("workspace_ids"), maxTitleWorkspaces)
		if len(workspaceIDs) == 0 {
			return []services.PageTitleRow{}, nil
		}
		return pages.ListTitlesAcrossWorkspaces(user.ID, workspaceIDs)
	}
}

// parseWorkspaceIDList parses a comma-separated id list, deduplicates, and
// caps the result. Malformed entries are skipped.
func parseWorkspaceIDList(raw string, limit int) []int {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[int]bool)
	ids := make([]int, 0, limit)
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 1 || seen[value] {
			continue
		}
		seen[value] = true
		ids = append(ids, value)
		if len(ids) >= limit {
			break
		}
	}
	return ids
}

func listArchivedPages(pages pageApplication) readOperation[[]archivedPageDTO] {
	return func(r *http.Request) ([]archivedPageDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		rows, err := pages.ListArchived(user.ID, workspaceID)
		if err != nil {
			return nil, pageError(err)
		}
		result := make([]archivedPageDTO, len(rows))
		for i, row := range rows {
			result[i] = archivedPageDTO{row.ID, row.Title, row.Slug, row.Path, row.Depth, row.ArchivedAt, row.ArchivedBy, row.ArchivedByName}
		}
		return result, nil
	}
}

func searchPages(pages pageApplication) readOperation[[]models.Page] {
	return func(r *http.Request) ([]models.Page, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		limit, err := parsePositiveInt(r, "limit", 20, 100)
		if err != nil {
			return nil, err
		}
		result, err := pages.Search(user.ID, workspaceID, r.URL.Query().Get("q"), limit)
		return result, pageError(err)
	}
}

func createPage(pages pageApplication) jsonOperation[createPageRequest, models.Page] {
	return func(r *http.Request, input createPageRequest) (models.Page, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.Create(auditActor(r, user), services.CreatePageInput{WorkspaceID: workspaceID, ParentID: input.ParentID, Title: input.Title, Content: input.Content, Metadata: input.Metadata, IsHome: input.IsHome})
		return derefPage(page), pageError(err)
	}
}

func getPage(pages pageApplication) readOperation[models.Page] {
	return func(r *http.Request) (models.Page, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.Get(user.ID, workspaceID, pageID)
		return derefPage(page), pageError(err)
	}
}

func updatePage(pages pageApplication) jsonOperation[patchPageRequest, models.Page] {
	return func(r *http.Request, input patchPageRequest) (models.Page, error) {
		if input.Title.Null || input.Content.Null || input.Metadata.Null || input.ExpectedContentHash.Null {
			return models.Page{}, newError(http.StatusBadRequest, "invalid_request", "Page fields cannot be null")
		}
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.Update(auditActor(r, user), workspaceID, services.PageApplicationUpdateInput{ID: pageID, Title: optionalValue(input.Title), Content: optionalValue(input.Content), Metadata: optionalValue(input.Metadata), ExpectedContentHash: optionalValue(input.ExpectedContentHash)})
		return derefPage(page), pageError(err)
	}
}

func archivePage(pages pageApplication) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return err
		}
		_, err = pages.Archive(auditActor(r, user), workspaceID, pageID)
		return pageError(err)
	}
}

func movePage(pages pageApplication) jsonOperation[movePageRequest, models.Page] {
	return func(r *http.Request, input movePageRequest) (models.Page, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.Move(auditActor(r, user), workspaceID, pageID, input.DestinationWorkspaceID, input.ParentID, input.PrevSiblingID, input.NextSiblingID)
		return derefPage(page), pageError(err)
	}
}

func unarchivePage(pages pageApplication) actionOperation[models.Page] {
	return func(r *http.Request) (models.Page, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.Unarchive(auditActor(r, user), workspaceID, pageID)
		return derefPage(page), pageError(err)
	}
}

func listPageHistory(deps Deps) pageOperation[models.PageRevision] {
	return func(r *http.Request) ([]models.PageRevision, Pagination, int, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		revisions, total, err := deps.PageApplication.ListHistory(user.ID, workspaceID, pageID, page.PageSize, page.Offset)
		if err != nil {
			return nil, page, total, pageError(err)
		}
		// Revision authors are identities: hide everyone except the viewer,
		// admins, and viewers with the user-directory permission (legacy
		// filterPageRevisionAuthors).
		isAdmin, _ := deps.SystemAdmins.IsSystemAdmin(user.ID)
		hasListPermission, _ := deps.GlobalPermission.HasGlobalPermission(user.ID, models.PermissionUserList)
		filterPageRevisionAuthors(revisions, user.ID, isAdmin, hasListPermission)
		return revisions, page, total, nil
	}
}

func filterPageRevisionAuthors(revisions []models.PageRevision, userID int, isAdmin, hasListPermission bool) {
	for i := range revisions {
		author := revisions[i].Author
		if author == nil || author.ID == userID || isAdmin || (hasListPermission && author.IsActive) {
			continue
		}
		revisions[i].Author = nil
	}
}

func getPageRevision(pages pageApplication) readOperation[models.PageRevision] {
	return func(r *http.Request) (models.PageRevision, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.PageRevision{}, err
		}
		revisionID, err := pathID(r, "revision_id")
		if err != nil {
			return models.PageRevision{}, err
		}
		revision, err := pages.GetRevision(user.ID, workspaceID, pageID, revisionID)
		if revision == nil {
			return models.PageRevision{}, pageError(err)
		}
		return *revision, pageError(err)
	}
}

func restorePageRevision(pages pageApplication) actionOperation[models.Page] {
	return func(r *http.Request) (models.Page, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.Page{}, err
		}
		revisionID, err := pathID(r, "revision_id")
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.Restore(auditActor(r, user), workspaceID, pageID, revisionID)
		return derefPage(page), pageError(err)
	}
}

// listPageEffectiveLevels returns the caller's effective page permission
// level for every live visible page in the workspace, keyed by page ID
// ("admin" | "edit" | "view"). The sidebar gates per-row actions from this
// single payload instead of querying each page.
func listPageEffectiveLevels(pages pageApplication) readOperation[map[string]string] {
	return func(r *http.Request) (map[string]string, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		levels, err := pages.EffectiveLevels(user.ID, workspaceID)
		if err != nil {
			return nil, pageError(err)
		}
		out := make(map[string]string, len(levels))
		for id, level := range levels {
			out[strconv.Itoa(id)] = level
		}
		return out, nil
	}
}

func getPagePermissions(pages pageApplication) readOperation[services.PagePermissionsResult] {
	return func(r *http.Request) (services.PagePermissionsResult, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return services.PagePermissionsResult{}, err
		}
		result, err := pages.GetPermissions(user.ID, workspaceID, pageID)
		return result, pageError(err)
	}
}

// pagePublicationDTO reports whether a page is published through portal
// knowledge bases. Portals lists the portal titles exposing it; always an
// array so consumers never null-check.
type pagePublicationDTO struct {
	PubliclyViewable bool     `json:"publicly_viewable"`
	Portals          []string `json:"portals"`
}

// getPagePublication resolves the portal knowledge-base publication state of
// a page the caller can view. Editors use it to surface a visible
// "publicly viewable" marker on published pages.
func getPagePublication(deps Deps) readOperation[pagePublicationDTO] {
	return func(r *http.Request) (pagePublicationDTO, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return pagePublicationDTO{}, err
		}
		result := pagePublicationDTO{Portals: []string{}}
		// Route through the permission-checked page read so existence of an
		// unpublished page never leaks through this endpoint either.
		page, err := deps.PageApplication.Get(user.ID, workspaceID, pageID)
		if err != nil {
			return pagePublicationDTO{}, pageError(err)
		}
		if deps.PagePublication == nil {
			return result, nil
		}
		viewable, portals, err := deps.PagePublication.PagePublication(page)
		if err != nil {
			return pagePublicationDTO{}, err
		}
		result.PubliclyViewable = viewable
		result.Portals = portals
		if result.Portals == nil {
			result.Portals = []string{}
		}
		return result, nil
	}
}

func grantPagePermission(pages pageApplication) jsonOperation[grantPagePermissionRequest, models.PagePermission] {
	return func(r *http.Request, input grantPagePermissionRequest) (models.PagePermission, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.PagePermission{}, err
		}
		permission, err := pages.GrantPermission(auditActor(r, user), workspaceID, pageID, input.PrincipalType, input.PrincipalID, input.PermissionLevel)
		if permission == nil {
			return models.PagePermission{}, pageError(err)
		}
		return *permission, pageError(err)
	}
}

func revokePagePermission(pages pageApplication) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return err
		}
		permissionID, err := pathID(r, "permission_id")
		if err != nil {
			return err
		}
		return pageError(pages.RevokePermission(auditActor(r, user), workspaceID, pageID, permissionID))
	}
}

func setPageInheritance(pages pageApplication) jsonOperation[pageInheritanceRequest, models.Page] {
	return func(r *http.Request, input pageInheritanceRequest) (models.Page, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return models.Page{}, err
		}
		page, err := pages.SetInheritance(auditActor(r, user), workspaceID, pageID, input.InheritPermissions)
		return derefPage(page), pageError(err)
	}
}

func pageTarget(r *http.Request) (user *models.User, workspaceID, pageID int, err error) {
	user, workspaceID, err = principalAndWorkspace(r)
	if err != nil {
		return nil, 0, 0, err
	}
	pageID, err = pathID(r, "page_id")
	return user, workspaceID, pageID, err
}

func derefPage(page *models.Page) models.Page {
	if page == nil {
		return models.Page{}
	}
	return *page
}

func pageError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrPageNotFound), errors.Is(err, services.ErrPageParentNotFound), errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Page was not found")
	case errors.Is(err, services.ErrPageContentConflict), errors.Is(err, services.ErrPageUniqueConflict), errors.Is(err, services.ErrPagePermissionDuplicate):
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, services.ErrPageTitleRequired), errors.Is(err, services.ErrPageNoChanges), errors.Is(err, services.ErrPageParentMismatch), errors.Is(err, services.ErrPageCycle), errors.Is(err, services.ErrPageDepthExceeded), errors.Is(err, services.ErrPageRevisionMismatch), errors.Is(err, services.ErrPageMetadataInvalid), errors.Is(err, services.ErrPageInvalidPrincipal), errors.Is(err, services.ErrPageInvalidLevel), errors.Is(err, services.ErrPageGrantPrincipalNotFound):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrPageMutationForbidden):
		return newError(http.StatusNotFound, "not_found", "Page was not found")
	default:
		return internalError(err)
	}
}
