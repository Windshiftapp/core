// Package v2 provides the canonical API v2 contract for session and bearer clients.
package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	apispec "windshift/api"
	tokenauth "windshift/internal/auth"
	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

const (
	sessionPrefix = "/api/v2"
	restPrefix    = "/rest/api/v2"
)

// AuthClass states whether a route is public or needs a principal.
type AuthClass string

const (
	AuthPublic        AuthClass = "public"
	AuthAuthenticated AuthClass = "authenticated"
)

// Route describes one canonical operation mounted at both v2 prefixes.
type Route struct {
	Method string
	Path   string
	Auth   AuthClass
	Scopes []string
}

type route struct {
	Route
	handler Handler
}

type routeBuilder struct {
	routes    []route
	transport transport
}

func (b *routeBuilder) Read[Response any](path string, auth AuthClass, scopes []string, operation readOperation[Response]) {
	b.routes = append(b.routes, route{
		Route: Route{
			Method: http.MethodGet,
			Path:   path,
			Auth:   auth,
			Scopes: slices.Clone(scopes),
		},
		handler: b.transport.Read(http.StatusOK, operation),
	})
}

func (b *routeBuilder) Page[Response any](path string, auth AuthClass, scopes []string, operation pageOperation[Response]) {
	b.routes = append(b.routes, route{
		Route: Route{
			Method: http.MethodGet,
			Path:   path,
			Auth:   auth,
			Scopes: slices.Clone(scopes),
		},
		handler: b.transport.Page(operation),
	})
}

func (b *routeBuilder) PageMetadata[Response, Meta any](path string, auth AuthClass, scopes []string, operation pageMetadataOperation[Response, Meta]) {
	b.routes = append(b.routes, route{
		Route:   Route{Method: http.MethodGet, Path: path, Auth: auth, Scopes: slices.Clone(scopes)},
		handler: b.transport.PageMetadata(operation),
	})
}

func (b *routeBuilder) Metadata[Response, Meta any](path string, auth AuthClass, scopes []string, operation metadataOperation[Response, Meta]) {
	b.routes = append(b.routes, route{
		Route:   Route{Method: http.MethodGet, Path: path, Auth: auth, Scopes: slices.Clone(scopes)},
		handler: b.transport.Metadata(operation),
	})
}

func (b *routeBuilder) JSON[Request, Response any](method, path string, status int, patch bool, auth AuthClass, scopes []string, operation jsonOperation[Request, Response]) {
	b.routes = append(b.routes, route{
		Route: Route{
			Method: method,
			Path:   path,
			Auth:   auth,
			Scopes: slices.Clone(scopes),
		},
		handler: b.transport.JSON(status, patch, operation),
	})
}

func (b *routeBuilder) Raw(method, path string, auth AuthClass, scopes []string, handler Handler) {
	b.routes = append(b.routes, route{
		Route: Route{
			Method: method,
			Path:   path,
			Auth:   auth,
			Scopes: slices.Clone(scopes),
		},
		handler: handler,
	})
}

func (b *routeBuilder) Command(method, path string, auth AuthClass, scopes []string, operation commandOperation) {
	b.routes = append(b.routes, route{
		Route:   Route{Method: method, Path: path, Auth: auth, Scopes: slices.Clone(scopes)},
		handler: b.transport.Command(operation),
	})
}

func (b *routeBuilder) Action[Response any](method, path string, status int, auth AuthClass, scopes []string, operation actionOperation[Response]) {
	b.routes = append(b.routes, route{
		Route:   Route{Method: method, Path: path, Auth: auth, Scopes: slices.Clone(scopes)},
		handler: b.transport.Action(status, operation),
	})
}

type tokenAuthenticator interface {
	ValidateToken(string) (*models.User, *models.APIToken, error)
	CheckTokenPermissions(*models.APIToken, []string) bool
}

type csrfValidator interface {
	Allows(*http.Request) bool
}

type concurrencyLimiter interface {
	Acquire(int) (func(), bool)
}

type objectLocalizer interface {
	LocalizeResponse(context.Context, string, string, any) error
}

type labelApplication interface {
	Create(services.AuditActor, string, string) (*models.Label, error)
	Update(services.AuditActor, int, services.LabelUpdate) (*models.Label, error)
	Delete(services.AuditActor, int) error
	ListForItem(int) ([]models.Label, error)
	SetForItem(int, []int) ([]models.Label, error)
	AddToItem(int, int) ([]models.Label, error)
	RemoveFromItem(int, int) error
}

type itemReader interface {
	FindByID(int) (*models.Item, error)
}

type resourceAccess interface {
	CanViewWorkspace(int, int) (bool, error)
	CanEditWorkspace(int, int) (bool, error)
	CanAdminWorkspace(int, int) (bool, error)
}

type preferencesApplication interface {
	GetSnapshot(int) (services.UserPreferencesSnapshot, error)
	UpdateSnapshot(int, services.UserPreferencesPatch) (services.UserPreferencesSnapshot, error)
}

type recurrenceApplication interface {
	Get(int) (*models.RecurrenceRule, error)
	Create(int, int, int, models.CreateRecurrenceRequest, ...services.AuditActor) (*models.RecurrenceRule, error)
	UpdateWithPatch(int, services.RecurrenceUpdate, ...services.AuditActor) (*models.RecurrenceRule, error)
	Delete(int, ...services.AuditActor) error
	ListInstances(int, int, int) (*services.RecurrenceInstances, error)
	ForceGenerate(int, ...services.AuditActor) (int, error)
	ListByWorkspace(int) ([]*models.RecurrenceRule, error)
	Preview(models.RRulePreviewRequest) (*services.RecurrencePreview, error)
}

type itemDiagramApplication interface {
	List(int) ([]models.ItemDiagram, error)
	Get(int) (*models.ItemDiagram, error)
	Create(int, string, string, int) (*models.ItemDiagram, error)
	Patch(int, *string, *string, int) (*models.ItemDiagram, error)
	Delete(int, int) (*models.ItemDiagram, error)
}

type pageReader interface {
	GetByID(int) (*models.Page, error)
}

type pageApplication interface {
	List(int, int) ([]models.Page, error)
	Get(int, int, int) (*models.Page, error)
	Search(int, int, string, int) ([]models.Page, error)
	ListArchived(int, int) ([]repository.ArchivedPageRow, error)
	ListHistory(int, int, int, int, int) ([]models.PageRevision, int, error)
	GetRevision(int, int, int, int) (*models.PageRevision, error)
	GetPermissions(int, int, int) (services.PagePermissionsResult, error)
	Create(services.AuditActor, services.CreatePageInput) (*models.Page, error)
	Update(services.AuditActor, int, services.PageApplicationUpdateInput) (*models.Page, error)
	Move(services.AuditActor, int, int, *int, *int, *int, *int) (*models.Page, error)
	Archive(services.AuditActor, int, int) (*models.Page, error)
	Restore(services.AuditActor, int, int, int) (*models.Page, error)
	GrantPermission(services.AuditActor, int, int, string, int, string) (*models.PagePermission, error)
	RevokePermission(services.AuditActor, int, int, int) error
	SetInheritance(services.AuditActor, int, int, bool) (*models.Page, error)
	Unarchive(services.AuditActor, int, int) (*models.Page, error)
}

type pageDiagramApplication interface {
	Create(services.AuditActor, services.CreatePageDiagramInput) (*services.PageDiagram, error)
	List(services.AuditActor, int) ([]services.PageDiagram, error)
	Get(services.AuditActor, int, int) (*services.PageDiagram, error)
	Update(services.AuditActor, services.UpdatePageDiagramInput) (*services.PageDiagram, error)
}

type pageAccess interface {
	HasWorkspacePermissionFor(int, int, string) (bool, error)
	Can(int, int, int, string) (bool, error)
}

type pageLabelApplication interface {
	List(int) ([]models.PageLabel, error)
	Get(int, int) (*models.PageLabel, error)
	Create(int, string, string, ...services.AuditActor) (*models.PageLabel, error)
	Update(int, int, services.PageLabelUpdate, ...services.AuditActor) (*models.PageLabel, error)
	Delete(int, int, ...services.AuditActor) (*models.PageLabel, error)
	ListForPage(int) ([]models.PageLabel, error)
	SetForPage(int, int, []int) ([]models.PageLabel, error)
	AddToPage(int, int, int) ([]models.PageLabel, error)
	RemoveFromPage(int, int) error
}

type worklogApplication interface {
	Create(int, services.WorklogMutationInput) (*services.WorklogMutationResult, error)
	Update(int, int, services.WorklogMutationInput) (*services.WorklogMutationResult, error)
	Delete(int) error
	Get(int) (*models.Worklog, error)
	ListMine(repository.WorklogListFilter) ([]models.Worklog, int, error)
	List(repository.WorklogDetailFilter) ([]models.Worklog, error)
}

type timeAccess interface {
	CanBookTimeOnProject(int, int) (bool, error)
	CanEditWorklog(int, int) (bool, error)
	CanViewProject(int, int) (bool, error)
	IsTimeProjectManager(int, int) (bool, error)
}

type systemAdministrator interface {
	IsSystemAdmin(int) (bool, error)
}

type groupApplication interface {
	ListPage(int, int) ([]models.TeamGroup, int, error)
	Get(int, bool) (*models.TeamGroup, error)
	Create(string, string, int) (*services.GroupMutationResult, error)
	Update(int, services.GroupUpdateInput) (*services.GroupMutationResult, error)
	Delete(int) (*repository.GroupDeleteSnapshot, error)
}

type adminUserApplication interface {
	ListAdmin(services.PaginationParams) ([]models.User, int, error)
	GetByID(int) (*models.User, error)
	GetGroupIDs(int) ([]int, error)
	UpdateAdmin(int, services.AdminUserUpdate) (*services.AdminUserUpdateResult, error)
}

type commentApplication interface {
	Get(int) (*services.CommentWithDetails, error)
	GetFeedByItemID(int, bool, services.CommentFeedOptions) (*services.CommentFeedPage, error)
	CountFeedByItemID(int) (int, error)
	UserCanReadItemAsApprover(context.Context, int, int) (bool, error)
	Create(services.CreateCommentParams) (*services.CreateCommentResult, error)
	UpdateWithEffects(services.UpdateCommentParams) (*models.Comment, error)
	DeleteWithEffects(services.DeleteCommentParams) error
}

type commentAccess interface {
	HasWorkspacePermission(int, int, string) (bool, error)
	HasGlobalPermission(int, string) (bool, error)
}

type attachmentApplication interface {
	UploadPolicy() (services.ItemAttachmentUploadPolicy, error)
	ListItemAttachments(int, int, int, int) ([]models.Attachment, int, error)
	GetItemAttachment(int, int) (*models.Attachment, error)
	OpenItemAttachment(int, int, bool) (*services.ItemAttachmentBinary, error)
	UploadItemAttachment(services.ItemAttachmentUploadInput) (models.AttachmentUploadResponse, error)
	DeleteItemAttachment(int, int) error
}

type pageAttachmentApplication interface {
	UploadPageAttachment(services.PageAttachmentUploadInput) (models.AttachmentUploadResponse, error)
}

type collectionApplication interface {
	List(services.CollectionListParams) ([]models.Collection, int, error)
	Get(int, int) (*models.Collection, error)
	Create(services.AuditActor, models.Collection) (*models.Collection, error)
	Update(services.AuditActor, int, services.CollectionUpdate) (*models.Collection, error)
	UpdateSharing(services.AuditActor, int, services.CollectionSharingUpdate) (*models.Collection, error)
	Delete(services.AuditActor, int) error
	ListCategories() ([]models.CollectionCategory, error)
	GetCategory(int) (*models.CollectionCategory, error)
	CreateCategory(services.AuditActor, models.CollectionCategory) (*models.CollectionCategory, error)
	PatchCategory(services.AuditActor, int, services.CollectionCategoryPatch) (*models.CollectionCategory, error)
	DeleteCategory(services.AuditActor, int) error
	GetBoardConfiguration(int, services.BoardConfigurationScope) (*models.BoardConfiguration, error)
	GetBoardConfigurationBootstrap(context.Context, int, services.BoardConfigurationScope, *int) (*services.BoardConfigurationBootstrap, error)
	PutBoardConfiguration(services.AuditActor, services.BoardConfigurationScope, models.BoardConfigurationRequest) (*models.BoardConfiguration, error)
	DeleteBoardConfiguration(services.AuditActor, services.BoardConfigurationScope) error
}

type planningApplication interface {
	ListMilestones(int, services.MilestoneListParams) ([]services.MilestoneResult, int, error)
	GetMilestone(int, int) (*services.MilestoneResult, error)
	CreateMilestone(int, services.AuditActor, services.CreateMilestoneParams) (*services.MilestoneResult, error)
	UpdateMilestone(int, services.AuditActor, services.UpdateMilestoneParams) (*services.MilestoneResult, error)
	DeleteMilestone(int, services.AuditActor, int) error
	ReorderMilestones(int, services.AuditActor, services.MilestoneScope, []int) error
	GetMilestoneProgress(int, int) (*services.MilestoneProgressReport, error)
	GetMilestoneTestStatistics(int, int) (*services.MilestoneTestStats, error)
	GetMilestoneTestStatisticsBatch(int, []int) (map[int]*services.MilestoneTestStats, error)
	ReleaseMilestone(context.Context, int, services.AuditActor, int, services.ReleaseMilestoneInput) (*services.MilestoneResult, error)
	ListIterations(int, services.IterationListParams) ([]services.IterationResult, int, error)
	GetIteration(int, int) (*services.IterationResult, error)
	CreateIteration(int, services.AuditActor, services.CreateIterationParams) (*services.IterationResult, error)
	UpdateIteration(int, services.AuditActor, services.UpdateIterationParams) (*services.IterationResult, error)
	DeleteIteration(int, services.AuditActor, int) error
	CompleteIteration(context.Context, int, int, *int) (*services.CompleteIterationResult, error)
	GetIterationProgress(int, int) (*services.IterationProgressReport, error)
	GetIterationBurndown(int, int) (*services.IterationBurndownData, error)
	GetIterationProgressBatch(int, []int) (map[int]*services.IterationProgressReport, error)
}

type linkApplication interface {
	ListLinkTypes(bool) ([]models.LinkType, error)
	ListLinksForEntityWithChecks(int, string, int) ([]models.ItemLink, []models.ItemLink, error)
	ListBatch(context.Context, int, services.BatchLinkParams) ([]services.BatchItemLinks, int, error)
	CreateManagedLink(int, services.CreateItemLinkParams) (*models.ItemLink, error)
	DeleteLinkWithChecks(int, int) error
	SearchLinkable(int, string, string, int, []int) ([]models.LinkableItem, error)
	ListFieldLinks(int, int, int) ([]models.ItemLink, error)
}

type agentRunApplication interface {
	ListForWorkspace(context.Context, int, int, int, int) ([]*models.AgentRun, error)
	ListForItem(context.Context, int, int, int, int) ([]*models.AgentRun, error)
	Get(context.Context, int, int) (*models.AgentRun, error)
	Usage(context.Context, int, int) (repository.RunUsageTotals, error)
	Events(context.Context, int, int, int, int) ([]*models.AgentRunEvent, error)
	Rerun(context.Context, int, int) (bool, error)
	Cancel(context.Context, int, int, bool) (services.AgentRunCancelResult, error)
}

type actionApplication interface {
	ListTemplates() []services.ActionTemplateSummary
	ApplyTemplate(context.Context, int, int, services.AuditActor, string) (*services.ApplyToWorkspaceResult, error)
	Catalog(int, int) (services.ActionCatalog, error)
	List(int, int) ([]*models.Action, error)
	Get(int, int, int) (*models.Action, error)
	Create(int, int, services.AuditActor, models.CreateActionRequest) (*models.Action, error)
	Update(int, int, int, services.AuditActor, models.UpdateActionRequest) (*models.Action, error)
	Delete(int, int, int, services.AuditActor) error
	Logs(int, int, int, int, int) ([]*models.ActionExecutionLog, int, error)
	Execute(int, int, int, int) (models.ActionExecutionStatus, error)
}

// Deps lists every required v2 dependency. Construction has no fallbacks.
type Deps struct {
	Mux                *http.ServeMux
	Tokens             tokenAuthenticator
	Users              userReader
	Statuses           statusReader
	Workflows          workflowReader
	Configuration      configurationReader
	ObjectTranslations objectLocalizer
	Catalog            catalogReader
	CatalogMutations   catalogMutationApplication
	Workspaces         workspaceApplication
	ItemTemplates      itemTemplateApplication
	Labels             labelApplication
	Items              itemReader
	Access             resourceAccess
	Preferences        preferencesApplication
	Recurrence         recurrenceApplication
	ItemDiagrams       itemDiagramApplication
	Pages              pageReader
	PageApplication    pageApplication
	PageDiagrams       pageDiagramApplication
	PageAccess         pageAccess
	PageLabels         pageLabelApplication
	Worklogs           worklogApplication
	TimeAccess         timeAccess
	TimeProjects       timeProjectApplication
	Timers             timerApplication
	SystemAdmins       systemAdministrator
	Groups             groupApplication
	AdminUsers         adminUserApplication
	Comments           commentApplication
	CommentAccess      commentAccess
	Attachments        attachmentApplication
	PageAttachments    pageAttachmentApplication
	Collections        collectionApplication
	Planning           planningApplication
	Links              linkApplication
	AgentRuns          agentRunApplication
	AgentSkills        agentSkillReader
	ConditionSets      conditionSetApplication
	Governance         governanceApplication
	Actions            actionApplication
	TestManagement     *services.TestManagementApplicationService
	Assets             *services.AssetApplicationService
	ItemApplication    *services.ItemApplicationService
	ItemDetail         *services.ItemDetailApplicationService
	SessionMiddleware  func(http.Handler) http.Handler
	CORS               Middleware
	CSRF               csrfValidator
	Concurrency        concurrencyLimiter
}

// RegisterRoutes validates dependencies and mounts the canonical inventory twice.
func RegisterRoutes(deps Deps) error {
	if deps.Mux == nil {
		return errors.New("v2: Mux is required")
	}
	if deps.Tokens == nil {
		return errors.New("v2: Tokens is required")
	}
	if deps.Users == nil {
		return errors.New("v2: Users is required")
	}
	if deps.Statuses == nil {
		return errors.New("v2: Statuses is required")
	}
	if deps.Workflows == nil {
		return errors.New("v2: Workflows is required")
	}
	if deps.Configuration == nil {
		return errors.New("v2: Configuration is required")
	}
	if deps.Catalog == nil {
		return errors.New("v2: Catalog is required")
	}
	if deps.CatalogMutations == nil {
		return errors.New("v2: CatalogMutations is required")
	}
	if deps.Workspaces == nil {
		return errors.New("v2: Workspaces is required")
	}
	if deps.ItemTemplates == nil {
		return errors.New("v2: ItemTemplates is required")
	}
	if deps.Labels == nil {
		return errors.New("v2: Labels is required")
	}
	if deps.Items == nil {
		return errors.New("v2: Items is required")
	}
	if deps.Access == nil {
		return errors.New("v2: Access is required")
	}
	if deps.Preferences == nil {
		return errors.New("v2: Preferences is required")
	}
	if deps.Recurrence == nil {
		return errors.New("v2: Recurrence is required")
	}
	if deps.ItemDiagrams == nil {
		return errors.New("v2: ItemDiagrams is required")
	}
	if deps.Pages == nil {
		return errors.New("v2: Pages is required")
	}
	if deps.PageApplication == nil {
		return errors.New("v2: PageApplication is required")
	}
	if deps.PageDiagrams == nil {
		return errors.New("v2: PageDiagrams is required")
	}
	if deps.PageAccess == nil {
		return errors.New("v2: PageAccess is required")
	}
	if deps.PageLabels == nil {
		return errors.New("v2: PageLabels is required")
	}
	if deps.Worklogs == nil {
		return errors.New("v2: Worklogs is required")
	}
	if deps.TimeAccess == nil {
		return errors.New("v2: TimeAccess is required")
	}
	if deps.TimeProjects == nil {
		return errors.New("v2: TimeProjects is required")
	}
	if deps.Timers == nil {
		return errors.New("v2: Timers is required")
	}
	if deps.SystemAdmins == nil {
		return errors.New("v2: SystemAdmins is required")
	}
	if deps.Groups == nil {
		return errors.New("v2: Groups is required")
	}
	if deps.AdminUsers == nil {
		return errors.New("v2: AdminUsers is required")
	}
	if deps.Comments == nil {
		return errors.New("v2: Comments is required")
	}
	if deps.CommentAccess == nil {
		return errors.New("v2: CommentAccess is required")
	}
	if deps.Attachments == nil {
		return errors.New("v2: Attachments is required")
	}
	if deps.PageAttachments == nil {
		return errors.New("v2: PageAttachments is required")
	}
	if deps.Collections == nil {
		return errors.New("v2: Collections is required")
	}
	if deps.Planning == nil {
		return errors.New("v2: Planning is required")
	}
	if deps.Links == nil {
		return errors.New("v2: Links is required")
	}
	if deps.AgentRuns == nil {
		return errors.New("v2: AgentRuns is required")
	}
	if deps.AgentSkills == nil {
		return errors.New("v2: AgentSkills is required")
	}
	if deps.ConditionSets == nil {
		return errors.New("v2: ConditionSets is required")
	}
	if deps.Governance == nil {
		return errors.New("v2: Governance is required")
	}
	if deps.Actions == nil {
		return errors.New("v2: Actions is required")
	}
	if deps.TestManagement == nil {
		return errors.New("v2: TestManagement is required")
	}
	if deps.Assets == nil {
		return errors.New("v2: Assets is required")
	}
	if deps.ItemApplication == nil {
		return errors.New("v2: ItemApplication is required")
	}
	if deps.ItemDetail == nil {
		return errors.New("v2: ItemDetail is required")
	}
	if deps.SessionMiddleware == nil {
		return errors.New("v2: SessionMiddleware is required")
	}
	if deps.Concurrency == nil {
		return errors.New("v2: Concurrency is required")
	}
	if deps.CORS == nil {
		return errors.New("v2: CORS is required")
	}

	routes := buildRoutes(deps)
	if err := validateRouteScopes(routes); err != nil {
		return err
	}
	registerMount(deps.Mux, sessionPrefix, routes, deps, true)
	registerMount(deps.Mux, restPrefix, routes, deps, false)
	return nil
}

// Inventory returns a copy of the canonical route metadata.
func Inventory() []Route {
	routes := buildRoutes(Deps{})
	inventory := make([]Route, len(routes))
	for i, item := range routes {
		inventory[i] = item.Route
		inventory[i].Scopes = slices.Clone(item.Scopes)
	}
	return inventory
}

func buildRoutes(deps Deps) []route {
	var builder routeBuilder
	builder.Raw(http.MethodGet, "/openapi.json", AuthPublic, nil, serveOpenAPI(apispec.V2SpecJSON))
	builder.Read("/users/me", AuthAuthenticated, []string{"users:read"}, getCurrentUser(deps.Users))
	registerCatalogRoutes(&builder, deps)
	registerScopedCatalogRoutes(&builder, deps.Catalog, deps.Workspaces, deps.ItemTemplates)
	registerLabelRoutes(&builder, deps)
	registerPreferenceRoutes(&builder, deps.Preferences)
	registerRecurrenceRoutes(&builder, deps)
	registerItemDiagramRoutes(&builder, deps)
	registerPageDiagramRoutes(&builder, deps)
	registerPageLabelRoutes(&builder, deps)
	registerWorklogRoutes(&builder, deps)
	registerTimeRoutes(&builder, deps)
	registerAdminRoutes(&builder, deps)
	registerPageRoutes(&builder, deps.PageApplication)
	registerCommentRoutes(&builder, deps)
	registerAttachmentRoutes(&builder, deps)
	registerCollectionRoutes(&builder, deps.Collections)
	registerPlanningRoutes(&builder, deps.Planning)
	registerLinkRoutes(&builder, deps.Links, deps.CatalogMutations)
	registerAgentRunRoutes(&builder, deps.AgentRuns)
	registerAgentSkillRoutes(&builder, deps.AgentSkills)
	registerGovernanceRoutes(&builder, deps.ConditionSets, deps.Governance)
	registerActionRoutes(&builder, deps.Actions)
	registerTestManagementRoutes(&builder, deps.TestManagement)
	registerAssetRoutes(&builder, deps.Assets)
	registerItemRoutes(&builder, deps.ItemApplication, deps.ItemDetail)
	return builder.routes
}

func validateRouteScopes(routes []route) error {
	for _, item := range routes {
		switch item.Auth {
		case AuthPublic:
			if len(item.Scopes) != 0 {
				return fmt.Errorf("v2: public route %s %s declares token scopes", item.Method, item.Path)
			}
		case AuthAuthenticated:
			if len(item.Scopes) == 0 {
				return fmt.Errorf("v2: authenticated route %s %s declares no token scope", item.Method, item.Path)
			}
			if err := tokenauth.ValidateScopes(item.Scopes); err != nil {
				return fmt.Errorf("v2: route %s %s: %w", item.Method, item.Path, err)
			}
		default:
			return fmt.Errorf("v2: route %s %s has unknown auth class %q", item.Method, item.Path, item.Auth)
		}
	}
	return nil
}

func registerMount(mux *http.ServeMux, prefix string, routes []route, deps Deps, session bool) {
	dispatch := Handler(func(w http.ResponseWriter, r *http.Request) error {
		if session && hasCRWBearer(r.Header.Get("Authorization")) {
			return newError(http.StatusUnauthorized, "invalid_token", "Bearer credentials are not accepted on the session API")
		}
		path := strings.TrimPrefix(r.URL.Path, prefix)
		if path == "" {
			path = "/"
		}

		var allowed []string
		var selected *route
		var selectedValues map[string]string
		bestSpecificity := -1
		for i := range routes {
			item := &routes[i]
			pathValues, matched := matchPath(item.Path, path)
			if !matched {
				continue
			}
			if !slices.Contains(allowed, item.Method) {
				allowed = append(allowed, item.Method)
			}
			if item.Method != r.Method || routeSpecificity(item.Path) <= bestSpecificity {
				continue
			}
			selected = item
			selectedValues = pathValues
			bestSpecificity = routeSpecificity(item.Path)
		}
		if selected != nil {
			for name, value := range selectedValues {
				r.SetPathValue(name, value)
			}
			handler := selected.handler
			if selected.Auth == AuthAuthenticated {
				if session {
					handler = limitConcurrency(deps.Concurrency)(handler)
					if deps.CSRF != nil && !isSafeMethod(selected.Method) {
						handler = requireCSRF(deps.CSRF)(handler)
					}
					handler = requireSession(handler)
				} else {
					handler = limitConcurrency(deps.Concurrency)(handler)
					handler = requireBearer(deps.Tokens, selected.Scopes)(handler)
				}
			}
			return handler(w, r)
		}
		if len(allowed) > 0 {
			return methodNotAllowed(allowed)(w, r)
		}
		return notFound(w, r)
	})

	mounted := Adapt(deps.CORS(dispatch))
	if session {
		mounted = deps.SessionMiddleware(mounted)
	}
	// Method-specific subtree mounts coexist with the SPA's "GET /" fallback
	// under Go 1.27's ServeMux conflict rules. Dispatch still owns route-level
	// method selection so it can return the canonical 405 response.
	for _, method := range []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	} {
		mux.Handle(method+" "+prefix, mounted)
		mux.Handle(method+" "+prefix+"/", mounted)
	}
}

func routeSpecificity(pattern string) int {
	score := 0
	for _, part := range strings.Split(strings.Trim(pattern, "/"), "/") {
		if !strings.HasPrefix(part, "{") {
			score++
		}
	}
	return score
}

func matchPath(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if len(patternParts) != len(pathParts) {
		return nil, false
	}
	values := make(map[string]string)
	for i, part := range patternParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			if name == "" || pathParts[i] == "" {
				return nil, false
			}
			values[name] = pathParts[i]
			continue
		}
		if part != pathParts[i] {
			return nil, false
		}
	}
	return values, true
}

func requireCSRF(validator csrfValidator) Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			if !validator.Allows(r) {
				return newError(http.StatusForbidden, "csrf_failed", "Cross-site request blocked")
			}
			return next(w, r)
		}
	}
}

func limitConcurrency(limiter concurrencyLimiter) Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			user, ok := r.Context().Value(contextkeys.User).(*models.User)
			if !ok || user == nil {
				return newError(http.StatusUnauthorized, "authentication_required", "Authentication is required")
			}
			release, ok := limiter.Acquire(user.ID)
			if !ok {
				apiErr := newError(http.StatusTooManyRequests, "rate_limited", "Too many concurrent requests")
				apiErr.Headers = http.Header{"Retry-After": {"1"}}
				return apiErr
			}
			defer release()
			return next(w, r)
		}
	}
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func requireSession(next Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		user, ok := r.Context().Value(contextkeys.User).(*models.User)
		if !ok || user == nil {
			return newError(http.StatusUnauthorized, "authentication_required", "Authentication is required")
		}
		return next(w, r)
	}
}

func hasCRWBearer(header string) bool {
	return strings.HasPrefix(header, "Bearer crw_")
}

func requireBearer(tokens tokenAuthenticator, scopes []string) Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			header := r.Header.Get("Authorization")
			if header == "" {
				return bearerError("authentication_required", "Bearer authentication is required")
			}
			if !strings.HasPrefix(header, "Bearer ") {
				return bearerError("invalid_token", "Authorization must use the Bearer scheme")
			}
			raw := strings.TrimPrefix(header, "Bearer ")
			if !strings.HasPrefix(raw, "crw_") {
				return bearerError("invalid_token", "Bearer token is invalid")
			}
			user, token, err := tokens.ValidateToken(raw)
			if err != nil || user == nil || token == nil {
				return bearerError("invalid_token", "Bearer token is invalid")
			}
			if !tokens.CheckTokenPermissions(token, scopes) {
				apiErr := newError(http.StatusForbidden, "insufficient_permission", "Token lacks a required scope")
				apiErr.Details = map[string]any{"required": slices.Clone(scopes)}
				return apiErr
			}
			ctx := context.WithValue(r.Context(), contextkeys.User, user)
			ctx = context.WithValue(ctx, contextkeys.APIToken, token)
			ctx = context.WithValue(ctx, contextkeys.AuthMethod, "bearer")
			return next(w, r.WithContext(ctx))
		}
	}
}

func bearerError(code, message string) *Error {
	apiErr := newError(http.StatusUnauthorized, code, message)
	apiErr.Headers = http.Header{"WWW-Authenticate": {`Bearer realm="windshift-api-v2"`}}
	return apiErr
}

func methodNotAllowed(methods []string) Handler {
	return func(http.ResponseWriter, *http.Request) error {
		apiErr := newError(http.StatusMethodNotAllowed, "method_not_allowed", "Method is not allowed for this resource")
		apiErr.Headers = http.Header{"Allow": {strings.Join(methods, ", ")}}
		return apiErr
	}
}

func notFound(http.ResponseWriter, *http.Request) error {
	return newError(http.StatusNotFound, "not_found", "Resource not found")
}
