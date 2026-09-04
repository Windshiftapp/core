package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerPageRoutes(builder *routeBuilder, pages pageApplication) {
	builder.Read("/workspaces/{workspace_id}/pages", AuthAuthenticated, []string{"pages:read"}, listPages(pages))
	builder.Read("/workspaces/{workspace_id}/pages/archived", AuthAuthenticated, []string{"pages:read"}, listArchivedPages(pages))
	builder.Read("/workspaces/{workspace_id}/pages/search", AuthAuthenticated, []string{"pages:read"}, searchPages(pages))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/pages", http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, createPage(pages))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}", AuthAuthenticated, []string{"pages:read"}, getPage(pages))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/pages/{page_id}", http.StatusOK, true, AuthAuthenticated, []string{"pages:write"}, updatePage(pages))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/pages/{page_id}", AuthAuthenticated, []string{"pages:delete"}, archivePage(pages))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/move", http.StatusOK, false, AuthAuthenticated, []string{"pages:write"}, movePage(pages))
	builder.Action(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/unarchive", http.StatusOK, AuthAuthenticated, []string{"pages:write"}, unarchivePage(pages))
	builder.Page("/workspaces/{workspace_id}/pages/{page_id}/history", AuthAuthenticated, []string{"pages:read"}, listPageHistory(pages))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}/history/{revision_id}", AuthAuthenticated, []string{"pages:read"}, getPageRevision(pages))
	builder.Action(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/history/{revision_id}/restore", http.StatusOK, AuthAuthenticated, []string{"pages:write"}, restorePageRevision(pages))
	builder.Read("/workspaces/{workspace_id}/pages/{page_id}/permissions", AuthAuthenticated, []string{"pages:read"}, getPagePermissions(pages))
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

func listPageHistory(pages pageApplication) pageOperation[models.PageRevision] {
	return func(r *http.Request) ([]models.PageRevision, Pagination, int, error) {
		user, workspaceID, pageID, err := pageTarget(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		revisions, total, err := pages.ListHistory(user.ID, workspaceID, pageID, page.PageSize, page.Offset)
		return revisions, page, total, pageError(err)
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
