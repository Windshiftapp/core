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
	PortalAuthMiddleware *middleware.PortalAuthMiddleware

	// Rate limiters
	LoginRateLimiter      RateLimiter
	RunnerRegisterLimiter RateLimiter
	AuthRateLimiter       RateLimiter
	FIDORateLimiter       RateLimiter
	SSORateLimiter        RateLimiter // Rate limiter for SSO login/callback endpoints
	SCIMRateLimiter       RateLimiter // Rate limiter for SCIM provisioning endpoints (10 req/sec)
	PortalSubmitLimiter   RateLimiter
	PortalSearchLimiter   RateLimiter
	PortalAuthLimiter     RateLimiter // Rate limiter for portal magic link requests (3 req/min per IP)
	OAuthTokenLimiter     RateLimiter // IP-keyed limiter for the unauthenticated OAuth /token endpoint (never honors DisableIPRateLimit)
	EmailVerifyLimiter    RateLimiter
	SetupLimiter          RateLimiter
	AIRateLimiter         RateLimiter // Rate limiter for AI/LLM endpoints (5 req/min per IP)
	UploadLimiter         RateLimiter // Rate limiter for file uploads (10 req/min per IP)
	WebhookLimiter        RateLimiter // Rate limiter for webhook triggers (10 req/min per IP)
	CalendarFeedLimiter   RateLimiter // Rate limiter for public calendar feeds (10 req/min per IP)
	PublicBoardLimiter    RateLimiter // Shared IP limiter for public board, item, and attachment reads

	// Public handler (no auth)
	PublicBoard *handlers.PublicBoardHandler

	// Handler groups organized by domain
	Auth         AuthHandlers
	SCIM         SCIMHandlers
	SCM          SCMHandlers
	Items        ItemHandlers
	Workspaces   WorkspaceHandlers
	Users        UserHandlers
	Admin        AdminHandlers
	Planning     PlanningHandlers
	TimeTracking TimeTrackingHandlers
	Channels     ChannelHandlers
	Portal       PortalHandlers
	Assets       AssetHandlers
	AI           AIHandlers
	Misc         MiscHandlers
	Teams        TeamHandlers
	Integrations IntegrationHandlers
	Pages        PageHandlers

	// Standalone handlers (no domain group)
	Push *handlers.PushHandler
}

// AuthHandlers groups authentication-related handlers.
type AuthHandlers struct {
	Auth       *handlers.AuthHandler
	SSO        *handlers.SSOHandler
	WebAuthn   *handlers.WebAuthnHandler
	Invitation *handlers.InvitationHandler
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
	IssueSync     *handlers.IssueSyncHandler
}

// ItemHandlers groups item-related handlers.
type ItemHandlers struct {
	Item               *handlers.ItemHandler
	Attachment         *handlers.AttachmentHandler         // May be nil if attachments disabled
	AttachmentSettings *handlers.AttachmentSettingsHandler // May be nil
}

// WorkspaceHandlers groups workspace-related handlers.
type WorkspaceHandlers struct {
	Workspace             *handlers.WorkspaceHandler
	Bootstrap             *handlers.WorkspaceBootstrapHandler
	Screen                *handlers.ScreenHandler
	ConfigSet             *handlers.ConfigurationSetHandler
	ConfigSetNotification *handlers.ConfigurationSetNotificationHandler
	NotificationSettings  *handlers.NotificationSettingsHandler
	HierarchyLevel        *handlers.EnumHandler
	RequestType           *handlers.RequestTypeHandler
	Actions               *handlers.ActionsHandler
	ActionCredentials     *handlers.ActionCredentialsHandler
	ActionTemplates       *handlers.ActionTemplatesHandler
	Analytics             *handlers.AnalyticsHandler
	AgentBinding          *handlers.WorkspaceAgentBindingHandler
	RunnerControl         *handlers.RunnerControlHandler
	RunnerBroker          *handlers.RunnerBrokerHandler
}

// UserHandlers groups user-related handlers.
type UserHandlers struct {
	User          *handlers.UserHandler
	Group         *handlers.GroupHandler
	Permission    *handlers.PermissionHandler
	PermissionSet *handlers.PermissionSetHandler
	WorkspaceRole *handlers.WorkspaceRoleHandler
	Credential    *handlers.CredentialHandler
	APIToken      *handlers.APITokenHandler
	Agent         *handlers.AgentHandler
	CLIAuth       *handlers.CLIAuthHandler
	OAuth         *handlers.OAuthHandler
}

// AdminHandlers groups admin-related handlers.
type AdminHandlers struct {
	SecuritySettings     *handlers.SecuritySettingsHandler
	AuthPolicy           *handlers.AuthPolicyHandler
	Theme                *handlers.ThemeHandler
	UserPreferences      *handlers.UserPreferencesHandler
	JiraImport           *handlers.JiraImportHandler
	Plugin               *handlers.PluginHandler
	Setup                *handlers.SetupHandler
	System               *handlers.SystemHandler
	AuditLog             *handlers.AuditLogHandler
	LDAP                 *handlers.LDAPHandler
	Features             *handlers.FeaturesHandler
	ShellBootstrap       *handlers.ShellBootstrapHandler
	OAuthClients         *handlers.AdminOAuthClientHandler
	Diagnostics          *handlers.DiagnosticsHandler
	AgentSecurity        *handlers.AgentSecurityHandler
	AgentTemplateCatalog *handlers.AdminAgentTemplateCatalogHandler
	ObjectTranslation    *handlers.ObjectTranslationHandler
}

// PlanningHandlers groups planning-related handlers.
type PlanningHandlers struct {
	MilestoneCategory *handlers.EnumHandler
	IterationType     *handlers.EnumHandler
	PersonalLabel     *handlers.PersonalLabelHandler
}

// TimeTrackingHandlers groups time tracking handlers.
type TimeTrackingHandlers struct {
	Customer           *handlers.TimeCustomerHandler
	Project            *handlers.TimeProjectHandler
	CustomerPermission *handlers.CustomerOrganisationPermissionHandler
}

// ChannelHandlers groups channel-related handlers.
type ChannelHandlers struct {
	ChannelCategory *handlers.EnumHandler
	Channel         *handlers.ChannelHandler
	Notification    *handlers.NotificationHandler
	EmailTemplate   *handlers.EmailTemplateHandler
	Webhook         *handlers.WebhookHandler
	AssetReport     *handlers.AssetReportHandler
}

// PortalHandlers groups portal-related handlers.
type PortalHandlers struct {
	Portal         *handlers.PortalHandler
	PortalAuth     *handlers.PortalAuthHandler
	PortalWebAuthn *handlers.PortalWebAuthnHandler
	PortalCustomer *handlers.PortalCustomersHandler
	ContactRole    *handlers.EnumHandler
	Hub            *handlers.HubHandler
	Form           *handlers.FormHandler
}

// AssetHandlers groups asset management handlers.
type AssetHandlers struct {
	Asset  *handlers.AssetHandler
	Action *handlers.AssetActionHandler
}

// AIHandlers groups AI-related handlers.
type AIHandlers struct {
	AI                *handlers.AIHandler
	LLMConnection     *handlers.LLMConnectionHandler
	WorkItemStaleness *handlers.WorkItemStalenessHandler
}

// TeamHandlers groups team, leave, and on-call handlers.
type TeamHandlers struct {
	Team   *handlers.TeamHandler
	Leave  *handlers.LeaveHandler
	OnCall *handlers.OnCallHandler
}

// PageHandlers groups workspace knowledge-page handlers.
type PageHandlers struct {
	KnowledgeSearch *handlers.KnowledgeSearchHandler
}

// MiscHandlers groups miscellaneous handlers.
type MiscHandlers struct {
	Homepage      *handlers.HomepageHandler
	Review        *handlers.ReviewHandler
	CalendarFeed  *handlers.CalendarFeedHandler
	CustomField   *handlers.CustomFieldHandler
	RunnerInstall *handlers.RunnerInstallHandler
}
