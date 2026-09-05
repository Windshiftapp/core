// Package v2 provides the canonical API v2 contract for session and bearer clients.
package v2

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
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

//go:embed contract-metadata.json
var contractMetadataJSON []byte

// AuthClass states whether a route is public or needs a principal.
type AuthClass string

const (
	AuthPublic        AuthClass = "public"
	AuthAuthenticated AuthClass = "authenticated"
)

// Exposure states which authentication boundary publishes an operation.
type Exposure string

const (
	ExposureBoth    Exposure = "both"
	ExposureSession Exposure = "session"
	ExposureBearer  Exposure = "bearer"
)

// ResponseShape identifies the transport envelope used by an operation.
type ResponseShape string

const (
	ResponseRaw          ResponseShape = "raw"
	ResponseDirect       ResponseShape = "direct"
	ResponseDocument     ResponseShape = "document"
	ResponsePage         ResponseShape = "page"
	ResponseMetadata     ResponseShape = "metadata"
	ResponsePageMetadata ResponseShape = "page_metadata"
	ResponseEmpty        ResponseShape = "empty"
)

// Route describes one canonical operation and its authentication boundaries.
type Route struct {
	Method            string
	Path              string
	OperationID       string
	Tag               string
	Summary           string
	Description       string
	Auth              AuthClass
	Exposure          Exposure
	Scopes            []string
	RequestType       reflect.Type
	ResponseType      reflect.Type
	MetadataType      reflect.Type
	RequestMediaType  string
	ResponseMediaType string
	SuccessStatus     int
	ResponseShape     ResponseShape
	Parameters        []ParameterMetadata
	DocumentedErrors  []int
}

// ParameterMetadata is the public contract for one operation parameter.
type ParameterMetadata struct {
	Name        string
	In          string
	Description string
	Required    bool
	Schema      map[string]any
}

type route struct {
	Route
	handler Handler
}

type routeBuilder struct {
	routes    []route
	transport transport
	exposure  Exposure
}

func (b *routeBuilder) metadata(method, path string, auth AuthClass, scopes []string) Route {
	return Route{
		Method:   method,
		Path:     path,
		Auth:     auth,
		Exposure: b.exposure,
		Scopes:   slices.Clone(scopes),
	}
}

func (b *routeBuilder) sessionOnly(register func()) {
	previous := b.exposure
	b.exposure = ExposureSession
	defer func() { b.exposure = previous }()
	register()
}

func (b *routeBuilder) SessionRead[Response any](path string, operation readOperation[Response]) {
	b.sessionOnly(func() {
		b.Read(path, AuthAuthenticated, nil, operation)
	})
}

func (b *routeBuilder) SessionPage[Response any](path string, operation pageOperation[Response]) {
	b.sessionOnly(func() {
		b.Page(path, AuthAuthenticated, nil, operation)
	})
}

func (b *routeBuilder) SessionJSON[Request, Response any](method, path string, status int, patch bool, operation jsonOperation[Request, Response]) {
	b.sessionOnly(func() {
		b.JSON(method, path, status, patch, AuthAuthenticated, nil, operation)
	})
}

func (b *routeBuilder) SessionCommand(method, path string, operation commandOperation) {
	b.sessionOnly(func() {
		b.Command(method, path, AuthAuthenticated, nil, operation)
	})
}

func (b *routeBuilder) Read[Response any](path string, auth AuthClass, scopes []string, operation readOperation[Response]) {
	metadata := b.metadata(http.MethodGet, path, auth, scopes)
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = http.StatusOK
	metadata.ResponseShape = ResponseDocument
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: b.transport.Read(http.StatusOK, operation),
	})
}

func (b *routeBuilder) Page[Response any](path string, auth AuthClass, scopes []string, operation pageOperation[Response]) {
	metadata := b.metadata(http.MethodGet, path, auth, scopes)
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = http.StatusOK
	metadata.ResponseShape = ResponsePage
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: b.transport.Page(operation),
	})
}

func (b *routeBuilder) PageMetadata[Response, Meta any](path string, auth AuthClass, scopes []string, operation pageMetadataOperation[Response, Meta]) {
	metadata := b.metadata(http.MethodGet, path, auth, scopes)
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.MetadataType = reflect.TypeFor[Meta]()
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = http.StatusOK
	metadata.ResponseShape = ResponsePageMetadata
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: b.transport.PageMetadata(operation),
	})
}

func (b *routeBuilder) Metadata[Response, Meta any](path string, auth AuthClass, scopes []string, operation metadataOperation[Response, Meta]) {
	metadata := b.metadata(http.MethodGet, path, auth, scopes)
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.MetadataType = reflect.TypeFor[Meta]()
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = http.StatusOK
	metadata.ResponseShape = ResponseMetadata
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: b.transport.Metadata(operation),
	})
}

func (b *routeBuilder) JSON[Request, Response any](method, path string, status int, patch bool, auth AuthClass, scopes []string, operation jsonOperation[Request, Response]) {
	metadata := b.metadata(method, path, auth, scopes)
	metadata.RequestType = reflect.TypeFor[Request]()
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.RequestMediaType = "application/json"
	if patch {
		metadata.RequestMediaType = "application/merge-patch+json"
	}
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = status
	metadata.ResponseShape = ResponseDocument
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: b.transport.JSON(status, patch, operation),
	})
}

func (b *routeBuilder) Raw(method, path string, auth AuthClass, scopes []string, handler Handler) {
	metadata := b.metadata(method, path, auth, scopes)
	metadata.ResponseShape = ResponseRaw
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: handler,
	})
}

// RawResponse registers a handler that writes a direct response without the
// standard data envelope while retaining its public contract metadata.
func (b *routeBuilder) RawResponse[Response any](method, path string, status int, mediaType string, auth AuthClass, scopes []string, handler Handler) {
	metadata := b.metadata(method, path, auth, scopes)
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.ResponseMediaType = mediaType
	metadata.SuccessStatus = status
	metadata.ResponseShape = ResponseDirect
	b.routes = append(b.routes, route{Route: metadata, handler: handler})
}

// RawDocument registers a handler that parses a non-JSON request itself and
// writes the standard data envelope.
func (b *routeBuilder) RawDocument[Request, Response any](method, path string, status int, requestMediaType string, auth AuthClass, scopes []string, handler Handler) {
	metadata := b.metadata(method, path, auth, scopes)
	metadata.RequestType = reflect.TypeFor[Request]()
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.RequestMediaType = requestMediaType
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = status
	metadata.ResponseShape = ResponseDocument
	b.routes = append(b.routes, route{Route: metadata, handler: handler})
}

func (b *routeBuilder) Command(method, path string, auth AuthClass, scopes []string, operation commandOperation) {
	metadata := b.metadata(method, path, auth, scopes)
	metadata.SuccessStatus = http.StatusNoContent
	metadata.ResponseShape = ResponseEmpty
	b.routes = append(b.routes, route{
		Route:   metadata,
		handler: b.transport.Command(operation),
	})
}

func (b *routeBuilder) Action[Response any](method, path string, status int, auth AuthClass, scopes []string, operation actionOperation[Response]) {
	metadata := b.metadata(method, path, auth, scopes)
	metadata.ResponseType = reflect.TypeFor[Response]()
	metadata.ResponseMediaType = "application/json"
	metadata.SuccessStatus = status
	metadata.ResponseShape = ResponseDocument
	b.routes = append(b.routes, route{
		Route:   metadata,
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
	ListPage(repository.WorklogDetailFilter) ([]models.Worklog, int, error)
}

type timeAccess interface {
	CanBookTimeOnProject(int, int) (bool, error)
	CanEditWorklog(int, int) (bool, error)
	CanViewProject(int, int) (bool, error)
	AccessibleTimeProjectIDs(int) ([]int, error)
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

// BearerInventory returns the operations published by the public REST API.
func BearerInventory() []Route {
	inventory := Inventory()
	result := make([]Route, 0, len(inventory))
	for _, item := range inventory {
		if item.Exposure == ExposureBoth || item.Exposure == ExposureBearer {
			result = append(result, item)
		}
	}
	return result
}

func buildRoutes(deps Deps) []route {
	builder := routeBuilder{exposure: ExposureBoth}
	builder.RawResponse[map[string]any](http.MethodGet, "/openapi.json", http.StatusOK, "application/json", AuthPublic, nil, serveOpenAPI(apispec.V2SpecJSON))
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
	applyEmbeddedContractMetadata(builder.routes, contractMetadataJSON)
	return builder.routes
}

func applyEmbeddedContractMetadata(routes []route, document []byte) {
	var spec struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Parameters map[string]ParameterMetadata `json:"parameters"`
		} `json:"components"`
	}
	if json.Unmarshal(document, &spec) != nil {
		return
	}
	for index := range routes {
		item := spec.Paths[routes[index].Path]
		var operation struct {
			Tag         []string                   `json:"tags"`
			Summary     string                     `json:"summary"`
			Description string                     `json:"description"`
			Parameters  []json.RawMessage          `json:"parameters"`
			Responses   map[string]json.RawMessage `json:"responses"`
		}
		raw := item[strings.ToLower(routes[index].Method)]
		_ = json.Unmarshal(raw, &operation)
		routes[index].OperationID = canonicalOperationID(routes[index].Method, routes[index].Path)
		if len(operation.Tag) > 0 {
			routes[index].Tag = operation.Tag[0]
		}
		routes[index].Summary = operation.Summary
		routes[index].Description = semanticRouteDescription(routes[index].Method, operation.Summary)
		routes[index].Parameters = contractParameters(item["parameters"], operation.Parameters, spec.Components.Parameters)
		for status := range operation.Responses {
			code, err := strconv.Atoi(status)
			if err == nil && code >= 400 {
				routes[index].DocumentedErrors = append(routes[index].DocumentedErrors, code)
			}
		}
		sharedErrors := []int{http.StatusInternalServerError}
		if routes[index].Auth == AuthAuthenticated {
			sharedErrors = append(sharedErrors, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests)
		}
		if routes[index].RequestType != nil {
			sharedErrors = append(sharedErrors, http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType)
		}
		if strings.Contains(routes[index].Path, "{") {
			sharedErrors = append(sharedErrors, http.StatusNotFound)
		}
		for _, code := range sharedErrors {
			if !slices.Contains(routes[index].DocumentedErrors, code) {
				routes[index].DocumentedErrors = append(routes[index].DocumentedErrors, code)
			}
		}
		slices.Sort(routes[index].DocumentedErrors)
		applyParameterCorrections(&routes[index].Route)
	}
}

func contractParameters(pathRaw json.RawMessage, operationRaw []json.RawMessage, components map[string]ParameterMetadata) []ParameterMetadata {
	var pathParameters []json.RawMessage
	_ = json.Unmarshal(pathRaw, &pathParameters)
	result := make([]ParameterMetadata, 0, len(pathParameters)+len(operationRaw))
	for _, raw := range append(pathParameters, operationRaw...) {
		var reference struct {
			Ref string `json:"$ref"`
		}
		_ = json.Unmarshal(raw, &reference)
		if strings.HasPrefix(reference.Ref, "#/components/parameters/") {
			result = appendUniqueParameter(result, components[strings.TrimPrefix(reference.Ref, "#/components/parameters/")])
			continue
		}
		var parameter ParameterMetadata
		if json.Unmarshal(raw, &parameter) == nil && parameter.Name != "" {
			result = appendUniqueParameter(result, parameter)
		}
	}
	return result
}

func appendUniqueParameter(parameters []ParameterMetadata, candidate ParameterMetadata) []ParameterMetadata {
	for _, parameter := range parameters {
		if parameter.Name == candidate.Name && parameter.In == candidate.In {
			return parameters
		}
	}
	return append(parameters, candidate)
}

func applyParameterCorrections(route *Route) {
	if route.ResponseShape == ResponsePage || route.ResponseShape == ResponsePageMetadata {
		upsertParameter(route, integerQuery("page", "One-based page number.", 0, defaultPage))
		upsertParameter(route, integerQuery("page_size", "Maximum number of resources to return.", maxPageSize, defaultPageSize))
	}
	switch route.Method + " " + route.Path {
	case "GET /items/{item_id}/comments", "GET /workspaces/{workspace_id}/agent-runs", "GET /items/{item_id}/agent-runs", "GET /agent-runs/{run_id}/events":
		filtered := route.Parameters[:0]
		for _, parameter := range route.Parameters {
			if parameter.In != "query" || (parameter.Name != "before" && parameter.Name != "before_id" && parameter.Name != "since" && parameter.Name != "since_id" && parameter.Name != "after_id") {
				filtered = append(filtered, parameter)
			}
		}
		route.Parameters = filtered
		upsertParameter(route, ParameterMetadata{Name: "cursor", In: "query", Description: "Opaque continuation cursor returned by the preceding response. Cursors are operation-specific and cannot be combined with removed timestamp or ID boundaries.", Schema: map[string]any{"type": "string", "minLength": 1}})
		defaultSize := 50
		if route.Path == "/agent-runs/{run_id}/events" {
			defaultSize = 200
		}
		upsertParameter(route, integerQuery("page_size", "Maximum number of resources to return.", 200, defaultSize))
	case "POST /items/batch", "POST /assets/summaries", "POST /milestones/test-statistics", "POST /iterations/progress":
		removeQueryParameter(route, "ids")
	}

	switch route.Method + " " + route.Path {
	case "GET /items/{item_id}":
		if !slices.Contains(route.DocumentedErrors, http.StatusBadRequest) {
			route.DocumentedErrors = append(route.DocumentedErrors, http.StatusBadRequest)
		}
		upsertParameter(route, ParameterMetadata{
			Name:        "item_id",
			In:          "path",
			Required:    true,
			Description: "Positive integer item ID, or a KEY-NUMBER item key when no integer ID is available. Numeric values are always resolved as IDs.",
			Schema: map[string]any{"oneOf": []any{
				map[string]any{"type": "integer", "minimum": 1, "example": 123},
				map[string]any{"type": "string", "pattern": "^[A-Za-z0-9]{2,10}-[1-9][0-9]*$", "example": "WI-1238"},
			}},
		})
	case "GET /query-language/values":
		source := enumQuery("source", "Completion value catalog to search.",
			"workspaces", "statuses", "status_categories", "priorities", "users", "milestones", "iterations", "projects", "item_types", "labels")
		source.Required = true
		upsertParameter(route, source)
		valueField := enumQuery("value_field", "Resource field returned as the query-language value.", "id", "name", "key")
		valueField.Required = true
		upsertParameter(route, valueField)
		upsertParameter(route, stringQuery("q", "Case-insensitive value-label search text."))
		upsertParameter(route, integerQuery("limit", "Maximum number of completion values to return.", 100, defaultQLCompletionValueLimit))
	case "GET /milestones", "GET /workspaces/{workspace_id}/milestones":
		upsertParameter(route, positiveIDQuery("category_id", "Restricts results to one milestone category."))
		upsertParameter(route, stringQuery("status", "Restricts results to a milestone status."))
		upsertParameter(route, enumQuery("sort", "Sort field; prefix with '-' for descending order. ID is the stable final tie-breaker.", "position", "-position", "name", "-name", "target_date", "-target_date", "status", "-status", "created_at", "-created_at", "updated_at", "-updated_at"))
	case "GET /iterations", "GET /workspaces/{workspace_id}/iterations":
		upsertParameter(route, positiveIDQuery("type_id", "Restricts results to one iteration type."))
		upsertParameter(route, stringQuery("status", "Restricts results to an iteration status."))
		upsertParameter(route, enumQuery("sort", "Sort field; prefix with '-' for descending order. ID is the stable final tie-breaker.", "start_date", "-start_date", "end_date", "-end_date", "name", "-name", "status", "-status", "created_at", "-created_at", "updated_at", "-updated_at"))
	case "GET /time/worklogs":
		upsertParameter(route, positiveIDQuery("project_id", "Restricts worklogs to one time project."))
		fallthrough
	case "GET /items/{item_id}/worklogs", "GET /time/projects/{project_id}/worklogs":
		upsertParameter(route, dateQuery("from", "Includes worklogs on or after this civil date."))
		upsertParameter(route, dateQuery("to", "Includes worklogs on or before this civil date."))
	case "GET /asset-sets/{asset_set_id}/assets":
		for _, name := range []string{"type_id", "category_id", "status_id"} {
			upsertParameter(route, stringQuery(name, "Restricts assets by "+strings.ReplaceAll(name, "_", " ")+"."))
		}
		upsertParameter(route, booleanQuery("include_subcategories", "Whether category filtering includes descendant categories.", true))
		upsertParameter(route, stringQuery("search", "Case-insensitive asset text search."))
		upsertParameter(route, stringQuery("ql", "Asset query-language expression applied after structured filters."))
	case "GET /asset-sets/{asset_set_id}/categories":
		upsertParameter(route, booleanQuery("tree", "Whether to return categories as a hierarchy.", false))
	case "GET /workspaces/{workspace_id}/test-cases":
		upsertParameter(route, stringQuery("q", "Case-insensitive test-case search."))
		upsertParameter(route, positiveIDQuery("label_id", "Restricts test cases to one label."))
		upsertParameter(route, positiveIDQuery("folder_id", "Restricts test cases to one folder; null selects unfiled cases."))
		upsertParameter(route, booleanQuery("all", "Whether to ignore the folder filter.", false))
	case "GET /workspaces/{workspace_id}/test-runs":
		for _, name := range []string{"assignee_id", "template_id", "plan_id"} {
			upsertParameter(route, positiveIDQuery(name, "Restricts test runs by "+strings.ReplaceAll(name, "_", " ")+"."))
		}
		upsertParameter(route, booleanQuery("unassigned", "Restricts results to unassigned test runs.", false))
		upsertParameter(route, booleanQuery("include_ended", "Whether ended test runs are included.", false))
	case "GET /workspaces/{workspace_id}/test-coverage/requirements", "GET /collections/{collection_id}/test-coverage/requirements":
		upsertParameter(route, enumQuery("covered", "Restricts results by coverage state.", "true", "false"))
		upsertParameter(route, positiveIDQuery("item_type_id", "Restricts requirements to one item type."))
		upsertParameter(route, stringQuery("search", "Case-insensitive requirement search."))
	case "GET /approvals/mine":
		upsertParameter(route, stringQuery("status", "Restricts approvals to one decision status."))
	case "GET /workspaces/{workspace_id}/pages/search":
		upsertParameter(route, stringQuery("q", "Page title and content search text."))
		upsertParameter(route, integerQuery("limit", "Maximum number of matches.", 100, 20))
	case "GET /time/projects", "GET /workspaces/{workspace_id}/time-projects":
		upsertParameter(route, stringQuery("status", "Restricts time projects to one status."))
	case "GET /link-types":
		upsertParameter(route, booleanQuery("include_inactive", "Whether inactive link types are included.", false))
	case "GET /links/batch":
		removeQueryParameter(route, "after_id")
		upsertParameter(route, stringQuery("ids", "Comma-separated item IDs. At most 100 IDs are recommended to remain within common proxy URL limits; duplicates use their first occurrence."))
		upsertParameter(route, stringQuery("ql", "Item query-language selector. Mutually exclusive with ids."))
		upsertParameter(route, ParameterMetadata{Name: "cursor", In: "query", Description: "Opaque per-item link cursor from next_cursor. Valid only with the same single explicit item ID and without ql.", Schema: map[string]any{"type": "string", "minLength": 1}})
		upsertParameter(route, enumQuery("sort", "Sort field; prefix with '-' for descending order.", "key", "-key", "created_at", "-created_at", "updated_at", "-updated_at"))
		upsertParameter(route, booleanQuery("include_custom_fields", "Whether linked item summaries include custom fields.", false))
	case "GET /links/search":
		upsertParameter(route, stringQuery("q", "Linkable resource search text."))
		upsertParameter(route, stringQuery("type", "Restricts matches to one entity type."))
		upsertParameter(route, integerQuery("limit", "Maximum number of matches.", 100, 20))
		upsertParameter(route, stringQuery("item_type_ids", "Comma-separated positive item-type IDs."))
	case "POST /agent-runs/{run_id}/cancel":
		upsertParameter(route, booleanQuery("force", "Whether to force the persisted run into the canceled state when cooperative cancellation cannot reach the worker.", false))
	case "GET /collections":
		upsertParameter(route, positiveIDQuery("workspace_id", "Restricts collections to one workspace."))
		upsertParameter(route, positiveIDQuery("category_id", "Restricts collections to one category."))
	case "GET /collections/{collection_id}/board-configuration/bootstrap":
		upsertParameter(route, positiveIDQuery("workspace_id", "Workspace used for defaults when the collection has no saved board configuration."))
	case "GET /condition-sets", "GET /approval-sets":
		upsertParameter(route, positiveIDQuery("workflow_id", "Restricts results to one workflow."))
	case "GET /workspaces/{workspace_id}/test-reports/summary":
		upsertParameter(route, positiveIDQuery("milestone_id", "Restricts the report to one milestone."))
		upsertParameter(route, integerQuery("days", "Number of recent civil days included in trend calculations.", 365, 30))
	case "DELETE /asset-management-sets/{asset_set_id}/roles/{assignment_id}":
		upsertParameter(route, enumQuery("type", "Assignment principal type.", "user", "group"))
	case "POST /milestones/{milestone_id}/release":
		upsertParameter(route, ParameterMetadata{Name: "Idempotency-Key", In: "header", Description: "Caller-generated key that makes retries return the original release result.", Schema: map[string]any{"type": "string", "minLength": 1}})
	case "GET /openapi.json":
		upsertParameter(route, ParameterMetadata{Name: "If-None-Match", In: "header", Description: "Returns 304 when this value matches the current specification ETag.", Schema: map[string]any{"type": "string"}})
	}

	switch route.Method + " " + route.Path {
	case "GET /items/{item_id}":
		route.Description = "Returns one authorization-checked item. A positive integer path value is resolved as the immutable item ID; only a non-numeric KEY-NUMBER value falls back to case-insensitive workspace-key lookup. Malformed references return 400, while missing or inaccessible items return the same 404 contract."
	case "POST /items/batch", "POST /assets/summaries", "POST /milestones/test-statistics", "POST /iterations/progress":
		route.Description = "Returns a bounded projection for up to 500 IDs. IDs are deduplicated by first occurrence; visible matches preserve request order, and missing or unauthorized resources are omitted without revealing which case applied. Results reflect committed state at request time and are all-or-nothing on computation failure."
	case "GET /links/batch":
		route.Description = "Returns one-hop links for at most 100 explicit item IDs, or for one paged query-language selection. Explicit IDs retain first-occurrence order. Each item is capped independently and exposes an operation-bound next_cursor when more links exist; inaccessible links are omitted."
	case "GET /items/{item_id}/comments":
		route.Description = "Returns an authorization-filtered comment feed with at most 200 entries. Opaque next_cursor and refresh_cursor values preserve the exclusive timestamp-and-ID boundary without exposing its representation; partial comment pages are never returned on failure."
	case "GET /agent-runs/{run_id}/events":
		route.Description = "Returns up to 200 visible run events after an operation-bound opaque cursor. Events use stable ascending ID order, next_cursor resumes exclusively after the final returned event, and partial pages are never returned on failure."
	case "GET /workspaces/{workspace_id}/agent-runs", "GET /items/{item_id}/agent-runs":
		route.Description = "Returns at most 200 visible agent runs in stable newest-first ID order. next_cursor resumes exclusively before the final returned run; private verification runs and unauthorized runs are omitted."
	case "GET /items/{item_id}/detail-summary", "GET /workspaces/{workspace_key}/items/{item_number}/detail-summary":
		route.Description = "Returns an authorization-checked item-detail bootstrap projection from committed state. Independent optional sections report deterministic section_errors when they fail, so partial data is distinguishable from an empty section."
	case "GET /workspaces/{workspace_id}/test-reports/summary":
		route.Description = "Computes an authorization-checked test summary from committed runs within the bounded 1-to-365-day window. The response is atomic and includes overall, trend, recent-failure, and recent-blocked projections."
	}
}

func removeQueryParameter(route *Route, name string) {
	filtered := route.Parameters[:0]
	for _, parameter := range route.Parameters {
		if parameter.In != "query" || parameter.Name != name {
			filtered = append(filtered, parameter)
		}
	}
	route.Parameters = filtered
}

func stringQuery(name, description string) ParameterMetadata {
	return ParameterMetadata{Name: name, In: "query", Description: description, Schema: map[string]any{"type": "string"}}
}

func positiveIDQuery(name, description string) ParameterMetadata {
	return ParameterMetadata{Name: name, In: "query", Description: description, Schema: map[string]any{"type": "integer", "minimum": 1}}
}

func integerQuery(name, description string, maximum, fallback int) ParameterMetadata {
	schema := map[string]any{"type": "integer", "minimum": 1, "default": fallback}
	if maximum > 0 {
		schema["maximum"] = maximum
	}
	return ParameterMetadata{Name: name, In: "query", Description: description, Schema: schema}
}

func booleanQuery(name, description string, fallback bool) ParameterMetadata {
	return ParameterMetadata{Name: name, In: "query", Description: description, Schema: map[string]any{"type": "boolean", "default": fallback}}
}

func dateQuery(name, description string) ParameterMetadata {
	return ParameterMetadata{Name: name, In: "query", Description: description, Schema: map[string]any{"type": "string", "format": "date"}}
}

func enumQuery(name, description string, values ...string) ParameterMetadata {
	return ParameterMetadata{Name: name, In: "query", Description: description, Schema: map[string]any{"type": "string", "enum": values}}
}

func upsertParameter(route *Route, parameter ParameterMetadata) {
	for index := range route.Parameters {
		if route.Parameters[index].Name == parameter.Name && route.Parameters[index].In == parameter.In {
			route.Parameters[index] = parameter
			return
		}
	}
	route.Parameters = append(route.Parameters, parameter)
}

func canonicalOperationID(method, path string) string {
	result := strings.ToLower(method)
	for _, part := range strings.FieldsFunc(path, func(value rune) bool {
		return value == '/' || value == '-' || value == '_' || value == '{' || value == '}'
	}) {
		if part == "" {
			continue
		}
		result += strings.ToUpper(part[:1]) + part[1:]
	}
	return result
}

func semanticRouteDescription(method, summary string) string {
	summary = strings.TrimSuffix(strings.TrimSpace(summary), ".")
	switch method {
	case http.MethodGet:
		return summary + ". Results include only resources visible to the authenticated caller; collections use their documented stable order and bounds."
	case http.MethodPost, http.MethodPut:
		return summary + ". The server validates resource ownership and command preconditions before persisting changes. Unknown JSON fields are rejected when the operation accepts a body."
	case http.MethodPatch:
		return summary + ". The operation uses JSON Merge Patch: omitted fields remain unchanged and documented nullable fields accept null. Unknown fields are rejected."
	case http.MethodDelete:
		return summary + ". The server verifies resource ownership and deletion preconditions before removing the resource."
	default:
		return summary + "."
	}
}

func validateRouteScopes(routes []route) error {
	for _, item := range routes {
		switch item.Exposure {
		case ExposureBoth, ExposureSession, ExposureBearer:
		default:
			return fmt.Errorf("v2: route %s %s has unknown exposure %q", item.Method, item.Path, item.Exposure)
		}
		bearerExposed := item.Exposure == ExposureBoth || item.Exposure == ExposureBearer
		switch item.Auth {
		case AuthPublic:
			if len(item.Scopes) != 0 {
				return fmt.Errorf("v2: public route %s %s declares token scopes", item.Method, item.Path)
			}
		case AuthAuthenticated:
			if !bearerExposed && len(item.Scopes) != 0 {
				return fmt.Errorf("v2: session-only route %s %s declares token scopes", item.Method, item.Path)
			}
			if bearerExposed && len(item.Scopes) == 0 {
				return fmt.Errorf("v2: authenticated route %s %s declares no token scope", item.Method, item.Path)
			}
			if bearerExposed {
				if err := tokenauth.ValidateScopes(item.Scopes); err != nil {
					return fmt.Errorf("v2: route %s %s: %w", item.Method, item.Path, err)
				}
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
			if session && item.Exposure == ExposureBearer || !session && item.Exposure == ExposureSession {
				continue
			}
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
