// Package routes provides domain-based route registration for the API.
package routes

import (
	"net/http"

	"windshift/internal/handlers"
	"windshift/internal/middleware"
	"windshift/internal/router"
)

// RateLimiter defines the interface for rate limiting middleware.
type RateLimiter interface {
	Limit(http.Handler) http.Handler
}

// Deps contains all dependencies needed for route registration.
type Deps struct {
	// Route groups
	API       *router.RouteGroup
	SCIMGroup *router.RouteGroup
	Mux       *http.ServeMux // For plugin routes that need raw mux access

	// Middleware
	AuthMiddleware       *middleware.AuthMiddleware
	PermissionMiddleware *middleware.PermissionMiddleware
	SCIMAuthMiddleware   *middleware.SCIMAuthMiddleware
	CSRFMiddleware       *middleware.CSRFMiddleware
	DisableCSRF          bool

	// Rate limiters
	LoginRateLimiter     RateLimiter
	AuthRateLimiter      RateLimiter
	FIDORateLimiter      RateLimiter
	SSORateLimiter       RateLimiter  // Rate limiter for SSO login/callback endpoints
	SCIMRateLimiter      RateLimiter  // Rate limiter for SCIM provisioning endpoints (10 req/sec)
	PortalSubmitLimiter  RateLimiter
	PortalSearchLimiter  RateLimiter
	EmailVerifyLimiter   RateLimiter
	SetupLimiter         RateLimiter

	// Handler groups organized by domain
	Auth        AuthHandlers
	SCIM        SCIMHandlers
	SCM         SCMHandlers
	Items       ItemHandlers
	Workspaces  WorkspaceHandlers
	Users       UserHandlers
	Admin       AdminHandlers
	Planning    PlanningHandlers
	TimeTracking TimeTrackingHandlers
	TestMgmt    TestManagementHandlers
	Channels    ChannelHandlers
	Portal      PortalHandlers
	Assets      AssetHandlers
	Collections CollectionHandlers
	Misc        MiscHandlers
}

// AuthHandlers groups authentication-related handlers.
type AuthHandlers struct {
	Auth     *handlers.AuthHandler
	SSO      *handlers.SSOHandler
	WebAuthn *handlers.WebAuthnHandler
}

// SCIMHandlers groups SCIM-related handlers.
type SCIMHandlers struct {
	SCIM      *handlers.SCIMHandler
	SCIMToken *handlers.SCIMTokenHandler
}

// SCMHandlers groups source code management handlers.
type SCMHandlers struct {
	Provider      *handlers.SCMProviderHandler
	Workspace     *handlers.SCMWorkspaceHandler
	ItemLinks     *handlers.SCMItemLinksHandler
	UserToken     *handlers.UserSCMTokenHandler
	EmailProvider *handlers.EmailProviderHandler
}

// ItemHandlers groups item-related handlers.
type ItemHandlers struct {
	Item       *handlers.ItemHandler
	Recurrence *handlers.RecurrenceHandler
	Comment    *handlers.CommentHandler
	Attachment *handlers.AttachmentHandler // May be nil if attachments disabled
	AttachmentSettings *handlers.AttachmentSettingsHandler // May be nil
	Diagram    *handlers.DiagramHandler
	ItemLink   *handlers.ItemLinkHandler
	LinkType   *handlers.LinkTypeHandler
}

// WorkspaceHandlers groups workspace-related handlers.
type WorkspaceHandlers struct {
	Workspace            *handlers.WorkspaceHandler
	FieldRequirement     *handlers.WorkspaceFieldRequirementHandler
	Screen               *handlers.ScreenHandler
	ConfigSet            *handlers.ConfigurationSetHandler
	ConfigSetNotification *handlers.ConfigurationSetNotificationHandler
	NotificationSettings *handlers.NotificationSettingsHandler
	ItemType             *handlers.ItemTypeHandler
	Priority             *handlers.PriorityHandler
	HierarchyLevel       *handlers.EnumHandler
	RequestType          *handlers.RequestTypeHandler
	StatusCategory       *handlers.EnumHandler
	Status               *handlers.EnumHandler
	StatusLegacy         *handlers.StatusHandler
	Workflow             *handlers.WorkflowHandler
}

// UserHandlers groups user-related handlers.
type UserHandlers struct {
	User          *handlers.UserHandler
	Group         *handlers.GroupHandler
	Permission    *handlers.PermissionHandler
	PermissionSet *handlers.PermissionSetHandler
	WorkspaceRole *handlers.WorkspaceRoleHandler
	Credential    *handlers.CredentialHandler
	AppToken      *handlers.AppTokenHandler
	APIToken      *handlers.ApiTokenHandler
}

// AdminHandlers groups admin-related handlers.
type AdminHandlers struct {
	SecuritySettings *handlers.SecuritySettingsHandler
	Theme            *handlers.ThemeHandler
	UserPreferences  *handlers.UserPreferencesHandler
	JiraImport       *handlers.JiraImportHandler
	Plugin           *handlers.PluginHandler
	Setup            *handlers.SetupHandler
	System           *handlers.SystemHandler
}

// PlanningHandlers groups planning-related handlers.
type PlanningHandlers struct {
	MilestoneCategory *handlers.EnumHandler
	Milestone         *handlers.MilestoneHandler
	IterationType     *handlers.EnumHandler
	Iteration         *handlers.IterationHandler
	PersonalLabel     *handlers.PersonalLabelHandler
}

// TimeTrackingHandlers groups time tracking handlers.
type TimeTrackingHandlers struct {
	Customer        *handlers.EnumHandler
	ProjectCategory *handlers.TimeProjectCategoryHandler
	Project         *handlers.TimeProjectHandler
	Worklog         *handlers.TimeWorklogHandler
	ActiveTimer     *handlers.ActiveTimerHandler
}

// TestManagementHandlers groups test management handlers.
type TestManagementHandlers struct {
	Folder      *handlers.TestFolderHandler
	Case        *handlers.TestCaseHandler
	Set         *handlers.TestSetHandler
	RunTemplate *handlers.TestRunTemplateHandler
	Run         *handlers.TestRunHandler
	Summary     *handlers.TestSummaryHandler
}

// ChannelHandlers groups channel-related handlers.
type ChannelHandlers struct {
	ChannelCategory      *handlers.EnumHandler
	Channel              *handlers.ChannelHandler
	Notification         *handlers.NotificationHandler
	NotificationTemplate *handlers.NotificationTemplateHandler
	Webhook              *handlers.WebhookHandler
}

// PortalHandlers groups portal-related handlers.
type PortalHandlers struct {
	Portal         *handlers.PortalHandler
	PortalCustomer *handlers.PortalCustomersHandler
	ContactRole    *handlers.EnumHandler
}

// AssetHandlers groups asset management handlers.
type AssetHandlers struct {
	Asset    *handlers.AssetHandler
	Type     *handlers.AssetTypeHandler
	Category *handlers.AssetCategoryHandler
	Status   *handlers.AssetStatusHandler
}

// CollectionHandlers groups collection-related handlers.
type CollectionHandlers struct {
	Category     *handlers.EnumHandler
	Collection   *handlers.CollectionHandler
	BoardConfig  *handlers.BoardConfigurationHandler
	TestCoverage *handlers.TestCoverageHandler
}

// MiscHandlers groups miscellaneous handlers.
type MiscHandlers struct {
	Homepage     *handlers.HomepageHandler
	Review       *handlers.ReviewHandler
	CalendarFeed *handlers.CalendarFeedHandler
	CustomField  *handlers.CustomFieldHandler
}
