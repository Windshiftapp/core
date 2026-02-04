// Package server provides a reusable HTTP server for windshift.
// This allows the server to be started both from the main binary
// and in-process for integration tests.
package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/handlers"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/plugins"
	"windshift/internal/restapi"
	v1 "windshift/internal/restapi/v1"
	"windshift/internal/router"
	"windshift/internal/routes"
	"windshift/internal/scheduler"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/smtp"
	"windshift/internal/utils"
	"windshift/internal/webauthn"
	"windshift/internal/webhook"
)

// Config holds all configuration options for the server.
type Config struct {
	// Port to run the HTTP server on (e.g., "8080" or "0" for random)
	Port string
	// DBPath is the SQLite database file path
	DBPath string
	// PostgresConn is the PostgreSQL connection string (if using Postgres)
	PostgresConn string
	// DisableCSRF disables CSRF protection (for development/testing)
	DisableCSRF bool
	// AttachmentPath is the path to store attachments (empty disables attachments)
	AttachmentPath string
	// AllowedHosts is a comma-separated list of allowed hostnames for CSRF
	AllowedHosts string
	// AllowedPort is the port for CSRF trusted origins
	AllowedPort string
	// UseProxy enables proxy mode (trust X-Forwarded-Proto from private IPs)
	UseProxy bool
	// AdditionalProxies is a comma-separated list of additional proxy IPs to trust
	AdditionalProxies string
	// MaxReadConns is the maximum number of read connections
	MaxReadConns int
	// MaxWriteConns is the maximum number of write connections
	MaxWriteConns int
	// TLSCertPath is the path to the TLS certificate file
	TLSCertPath string
	// TLSKeyPath is the path to the TLS key file
	TLSKeyPath string
	// DisablePlugins disables the plugin system
	DisablePlugins bool
	// EnableAdminFallback enables admin password fallback for restrictive auth policies
	EnableAdminFallback bool
	// BaseURL is the external URL for the server (used for email links, etc.)
	BaseURL string
	// FrontendFiles is the embedded filesystem containing frontend assets
	FrontendFiles embed.FS

	// Testing-specific options
	// ShutdownChan allows external control of server shutdown (for testing)
	ShutdownChan chan os.Signal
	// SilentMode suppresses all log output (for testing)
	SilentMode bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Port:          "8080",
		DBPath:        "windshift.db",
		MaxReadConns:  120,
		MaxWriteConns: 1,
	}
}

// Server represents a windshift HTTP server instance.
type Server struct {
	config     Config
	httpServer *http.Server
	db         database.Database
	listener   net.Listener

	// Services that need cleanup
	notificationManager   *handlers.NotificationManager
	notificationService   *services.NotificationService
	notificationScheduler *scheduler.NotificationScheduler
	recurrenceScheduler   *scheduler.RecurrenceScheduler
	actionService         *services.ActionService
	emailScheduler        *scheduler.EmailScheduler
	activityTracker       *services.ActivityTracker
	tokenTracker          *services.TokenTracker
	scmSyncStopChan       chan struct{}
	cleanupStopChan       chan struct{}
	cleanupTicker         *time.Ticker
	pluginManager         *plugins.Manager

	// Rate limiters that need cleanup
	loginRateLimiter    *middleware.RateLimiter
	fidoRateLimiter     *middleware.RateLimiter
	authRateLimiter     *middleware.RateLimiter
	scimRateLimiter     *middleware.RateLimiter
	portalSubmitLimiter *middleware.RateLimiter
	portalSearchLimiter *middleware.RateLimiter
	emailVerifyLimiter  *middleware.RateLimiter
	setupLimiter        *middleware.RateLimiter
	ssoRateLimiter      *middleware.RateLimiter
	portalAuthLimiter   *middleware.RateLimiter

	// Server state
	actualPort   int
	started      bool
	shuttingDown bool
}

// New creates a new Server instance with the given configuration.
// It initializes all services and handlers but does not start listening.
func New(cfg Config) (*Server, error) {
	s := &Server{
		config:          cfg,
		scmSyncStopChan: make(chan struct{}),
		cleanupStopChan: make(chan struct{}),
	}

	if err := s.initialize(); err != nil {
		s.cleanup()
		return nil, err
	}

	return s, nil
}

// initialize sets up all services and handlers.
func (s *Server) initialize() error {
	cfg := s.config

	// Suppress all logging in silent mode (for testing)
	if cfg.SilentMode {
		logger.SetSilent(true)
	}

	// Determine which database to use
	var err error
	if cfg.PostgresConn != "" {
		slog.Info("connecting to PostgreSQL database")
		s.db, err = database.NewDatabase("postgres", cfg.PostgresConn, cfg.MaxReadConns, cfg.MaxWriteConns)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
		}
		slog.Info("PostgreSQL database initialized", "max_read_conns", cfg.MaxReadConns, "max_write_conns", cfg.MaxWriteConns)
	} else {
		slog.Info("connecting to SQLite database", "path", cfg.DBPath)
		s.db, err = database.NewDatabase("sqlite3", cfg.DBPath, cfg.MaxReadConns, cfg.MaxWriteConns)
		if err != nil {
			return fmt.Errorf("failed to connect to SQLite database: %w", err)
		}
		slog.Info("SQLite database initialized", "max_read_conns", cfg.MaxReadConns, "max_write_conns", cfg.MaxWriteConns, "mode", "WAL")
	}

	if err := s.db.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Ensure default notification settings exist
	if err := s.db.EnsureDefaultNotificationSettings(); err != nil {
		slog.Warn("failed to ensure notification settings", "error", err)
	}

	// Determine setup status
	setupCompleted, err := checkSetupStatusWithRetry(s.db, 5, time.Second)
	if err != nil {
		return fmt.Errorf("failed to determine setup status: %w", err)
	}

	// Initialize permission service
	permService, err := services.NewPermissionService(s.db, services.PermissionCacheConfig{
		TTL:          15 * time.Minute,
		MaxCacheSize: 512,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize permission service: %w", err)
	}

	// Initialize activity tracker
	s.activityTracker, err = services.NewActivityTracker(s.db, services.DefaultActivityTrackerConfig())
	if err != nil {
		return fmt.Errorf("failed to initialize activity tracker: %w", err)
	}

	// Start activity cleanup scheduler
	s.cleanupTicker = time.NewTicker(24 * time.Hour)
	go s.runActivityCleanup()

	// Determine HTTPS mode
	enableHTTPS := cfg.TLSCertPath != "" && cfg.TLSKeyPath != ""

	// Parse additional proxies
	var additionalProxyList []string
	if cfg.AdditionalProxies != "" {
		additionalProxyList = strings.Split(cfg.AdditionalProxies, ",")
	}

	// Create IP extractor
	ipExtractor := utils.NewIPExtractor(cfg.UseProxy, additionalProxyList)

	// Authentication management
	sessionManager := auth.NewSessionManager(s.db, enableHTTPS, cfg.UseProxy, additionalProxyList)

	// Determine effective port for CORS
	effectivePort := cfg.Port
	if cfg.AllowedPort != "" {
		effectivePort = cfg.AllowedPort
	}

	// Initialize WebAuthn
	isDevelopment := cfg.DisableCSRF
	webAuthnConfig, err := webauthn.NewConfig("", "", nil, isDevelopment, cfg.AllowedHosts, effectivePort, enableHTTPS, cfg.UseProxy)
	if err != nil {
		return fmt.Errorf("failed to initialize WebAuthn configuration: %w", err)
	}
	slog.Info("WebAuthn configuration initialized",
		"rp_id", webAuthnConfig.RPID,
		"rp_name", webAuthnConfig.RPName,
		"development_mode", isDevelopment)

	// Create rate limiters
	s.loginRateLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.fidoRateLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.authRateLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList)
	s.scimRateLimiter = middleware.NewRateLimiter(10.0, 100, cfg.UseProxy, additionalProxyList)
	s.portalSubmitLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.portalSearchLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.emailVerifyLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.setupLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.ssoRateLimiter = middleware.NewRateLimiter(10.0/60.0, 5, cfg.UseProxy, additionalProxyList)
	s.portalAuthLimiter = middleware.NewRateLimiter(3.0/60.0, 3, cfg.UseProxy, additionalProxyList)

	// Initialize token tracker
	s.tokenTracker = services.NewTokenTracker(s.db, services.DefaultTokenTrackerConfig())

	// Create token manager
	tokenManager := auth.NewTokenManager(s.db, s.tokenTracker)

	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(sessionManager, tokenManager, s.db, cfg.UseProxy, additionalProxyList, setupCompleted)

	// Parse additional proxy IPs
	var additionalProxyIPs []net.IP
	for _, proxyStr := range additionalProxyList {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalProxyIPs = append(additionalProxyIPs, ip)
		}
	}

	mux := http.NewServeMux()

	// Initialize notification manager
	s.notificationManager, err = handlers.NewNotificationManager(s.db)
	if err != nil {
		return fmt.Errorf("failed to create notification manager: %w", err)
	}

	// Initialize notification service
	s.notificationService = services.NewNotificationService(
		s.db,
		s.notificationManager,
		services.DefaultNotificationServiceConfig(),
	)

	// Initialize SMTP and schedulers
	smtpSender := smtp.NewNotificationSMTPSender(s.db)
	s.notificationScheduler = scheduler.NewNotificationScheduler(s.db, smtpSender)
	s.notificationScheduler.Start()
	slog.Info("notification scheduler started")

	s.recurrenceScheduler = scheduler.NewRecurrenceScheduler(s.db)
	s.recurrenceScheduler.Start()
	slog.Info("recurrence scheduler started")

	// Initialize action service
	s.actionService = services.NewActionService(s.db, services.DefaultActionServiceConfig())
	s.actionService.SetNotificationService(s.notificationService)
	slog.Info("action service initialized")

	// Determine base URL
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("PUBLIC_URL")
	}
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", cfg.Port)
	}

	// Initialize email verification service
	emailVerificationService := services.NewEmailVerificationService(s.db, smtpSender, baseURL)

	// Initialize portal session manager
	portalSessionManager := auth.NewPortalSessionManager(s.db, enableHTTPS, cfg.UseProxy, additionalProxyList)

	// Initialize magic link service
	magicLinkService := services.NewMagicLinkService(s.db, smtpSender, baseURL)

	// Initialize handlers
	itemHandler := handlers.NewItemHandler(s.db, permService, s.activityTracker, s.notificationService)
	customFieldHandler := handlers.NewCustomFieldHandler(s.db)
	workspaceFieldReqHandler := handlers.NewWorkspaceFieldRequirementHandler(s.db)
	workspaceHandler := handlers.NewWorkspaceHandler(s.db, permService, s.activityTracker)
	screenHandler := handlers.NewScreenHandler(s.db)
	configSetHandler := handlers.NewConfigurationSetHandler(s.db, s.notificationService)
	itemTypeHandler := handlers.NewItemTypeHandler(s.db)
	priorityHandler := handlers.NewPriorityHandler(s.db)

	// Generic enum handlers
	hierarchyLevelHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewHierarchyLevelConfig()),
		func() interface{} { return &models.HierarchyLevel{} })
	requestTypeHandler := handlers.NewRequestTypeHandler(s.db)
	statusCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewStatusCategoryConfig()),
		func() interface{} { return &models.StatusCategory{} })
	statusHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewStatusConfig()),
		func() interface{} { return &models.Status{} })
	statusHandlerLegacy := handlers.NewStatusHandler(s.db)
	workflowHandler := handlers.NewWorkflowHandler(s.db)
	userHandler := handlers.NewUserHandler(s.db, permService)
	groupHandler := handlers.NewGroupHandler(s.db, permService)
	credentialHandler := handlers.NewCredentialHandler(s.db, permService)
	webAuthnHandler := handlers.NewWebAuthnHandler(s.db, permService, sessionManager, webAuthnConfig, ipExtractor)
	appTokenHandler := handlers.NewAppTokenHandler(s.db, permService)
	collectionHandler := handlers.NewCollectionHandler(s.db)
	boardConfigHandler := handlers.NewBoardConfigurationHandler(s.db)
	testCoverageHandler := handlers.NewTestCoverageHandler(s.db, permService)
	permissionHandler := handlers.NewPermissionHandlerWithCache(s.db, permService)
	apiTokenHandler := handlers.NewApiTokenHandler(s.db, tokenManager, permService)

	// SCIM handlers
	scimTokenManager := auth.NewSCIMTokenManager(s.db)
	scimAuthMiddleware := middleware.NewSCIMAuthMiddleware(scimTokenManager)
	scimHandler := handlers.NewSCIMHandler(s.db, baseURL)
	scimTokenHandler := handlers.NewSCIMTokenHandler(scimTokenManager)

	permissionSetHandler := handlers.NewPermissionSetHandlerWithPool(s.db, permService)
	workspaceRoleHandler := handlers.NewWorkspaceRoleHandlerWithPool(s.db, permService)

	// Time tracking handlers
	timePermissionService := services.NewTimePermissionService(s.db, permService)
	timeCustomerHandler := handlers.NewTimeCustomerHandler(s.db, timePermissionService)
	timeProjectHandler := handlers.NewTimeProjectHandler(s.db, timePermissionService)
	timeProjectCategoryHandler := handlers.NewTimeProjectCategoryHandler(s.db)
	timeWorklogHandler := handlers.NewTimeWorklogHandler(s.db, permService, timePermissionService)
	activeTimerHandler := handlers.NewActiveTimerHandler(s.db)
	timeProjectPermissionHandler := handlers.NewTimeProjectPermissionHandler(timePermissionService)

	// Test management handlers
	testFolderHandler := handlers.NewTestFolderHandlerWithPool(s.db, permService)
	testCaseHandler := handlers.NewTestCaseHandlerWithPool(s.db, permService)
	testSetHandler := handlers.NewTestSetHandlerWithPool(s.db, permService)
	testRunTemplateHandler := handlers.NewTestRunTemplateHandlerWithPool(s.db, permService)
	testRunHandler := handlers.NewTestRunHandlerWithPool(s.db, permService)
	testSummaryHandler := handlers.NewTestSummaryHandlerWithPool(s.db, permService)

	// Link management handlers
	linkTypeHandler := handlers.NewLinkTypeHandler(s.db)
	itemLinkHandler := handlers.NewItemLinkHandler(s.db, s.notificationService)

	// Label handler
	labelHandler := handlers.NewLabelHandler(s.db)

	// Recurrence handler
	recurrenceHandler := handlers.NewRecurrenceHandler(s.db, s.recurrenceScheduler)

	// Actions handler
	actionsHandler := handlers.NewActionsHandler(s.db, s.actionService)

	milestoneCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewMilestoneCategoryConfig()),
		func() interface{} { return &models.MilestoneCategory{} })
	milestoneHandler := handlers.NewMilestoneHandler(s.db, permService)
	channelCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewChannelCategoryConfig()),
		func() interface{} { return &models.ChannelCategory{} })
	collectionCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewCollectionCategoryConfig()),
		func() interface{} { return &models.CollectionCategory{} })
	iterationTypeHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewIterationTypeConfig()),
		func() interface{} { return &models.IterationType{} })
	iterationHandler := handlers.NewIterationHandler(s.db, permService)
	personalLabelHandler := handlers.NewPersonalLabelHandler(s.db)
	commentHandler := handlers.NewCommentHandler(s.db, permService, s.activityTracker, s.notificationService)
	reviewHandler := handlers.NewReviewHandler(s.db)
	calendarFeedHandler := handlers.NewCalendarFeedHandler(s.db, permService)
	securitySettingsHandler := handlers.NewSecuritySettingsHandler(s.db, cfg.DisablePlugins)

	// Admin rate limiter
	var adminRateLimiter *middleware.AdminFallbackRateLimiter
	if cfg.EnableAdminFallback {
		adminRateLimiter = middleware.NewAdminFallbackRateLimiter(s.db)
		slog.Info("Admin password fallback enabled", slog.String("component", "auth"))
	}

	authPolicyHandler := handlers.NewAuthPolicyHandlerWithFallback(s.db, cfg.EnableAdminFallback)

	// Initialize auth handler
	authHandler := handlers.NewAuthHandler(s.db, sessionManager, s.loginRateLimiter, permService, emailVerificationService, ipExtractor, authPolicyHandler, adminRateLimiter)

	themeHandler := handlers.NewThemeHandler(s.db)
	userPreferencesHandler := handlers.NewUserPreferencesHandler(s.db)
	homepageHandler := handlers.NewHomepageHandler(s.db, s.activityTracker)

	// Notification handlers
	notificationHandler := handlers.NewNotificationHandler(s.notificationManager, s.notificationService)
	notificationTemplateHandler := handlers.NewNotificationTemplateHandlerWithPool(s.db)

	permissionMiddleware := middleware.NewPermissionMiddleware(s.db)
	csrfMiddleware := middleware.NewCSRFMiddleware()

	// Setup handler
	setupHandler := handlers.NewSetupHandler(s.db, sessionManager, authMiddleware)

	// SSO handler
	ssoHandler := handlers.NewSSOHandler(s.db, sessionManager, permService, emailVerificationService, cfg.AllowedHosts, cfg.DisableCSRF, ipExtractor, cfg.UseProxy, additionalProxyList)

	// SCM provider handler
	scmProviderHandler := handlers.NewSCMProviderHandler(s.db)
	scmWorkspaceHandler := handlers.NewSCMWorkspaceHandler(s.db, scmProviderHandler.GetEncryption(), scmProviderHandler)
	scmItemLinksHandler := handlers.NewSCMItemLinksHandler(s.db, scmProviderHandler.GetEncryption())
	userSCMTokenHandler := handlers.NewUserSCMTokenHandler(s.db, scmProviderHandler.GetEncryption())

	// Asset management handlers
	assetHandler := handlers.NewAssetHandler(s.db, permService)
	assetTypeHandler := handlers.NewAssetTypeHandler(s.db, permService)
	assetCategoryHandler := handlers.NewAssetCategoryHandler(s.db, permService)
	assetStatusHandler := handlers.NewAssetStatusHandler(s.db, permService)
	assetReportHandler := handlers.NewAssetReportHandler(s.db)

	// Jira import handler
	jiraImportHandler := handlers.NewJiraImportHandler(s.db)

	// Email provider handler
	emailProviderHandler := handlers.NewEmailProviderHandler(s.db, scmProviderHandler.GetEncryption(), baseURL)

	// Email scheduler
	emailCredManager := email.NewCredentialManager(s.db, scmProviderHandler.GetEncryption())
	s.emailScheduler = scheduler.NewEmailScheduler(s.db, emailCredManager, cfg.AttachmentPath)
	s.emailScheduler.Start()
	slog.Info("email scheduler started (IMAP polling)")

	// SCM sync service
	scmSyncService := scm.NewSyncService(s.db, scmProviderHandler.GetEncryption())

	// Start SCM sync scheduler
	go s.runSCMSync(scmSyncService)

	// Webhook sender
	webhookSender := webhook.NewWebhookSender(s.db)

	// Event coordinator
	eventCoordinator := services.NewEventCoordinator(s.db)
	eventCoordinator.SetNotificationService(s.notificationService)
	eventCoordinator.SetActivityTracker(s.activityTracker)
	eventCoordinator.SetWebhookDispatcher(webhookSender)
	eventCoordinator.SetActionService(s.actionService)
	slog.Info("event coordinator initialized")

	// Wire up services
	itemHandler.SetWebhookSender(webhookSender)
	itemHandler.SetEventCoordinator(eventCoordinator)
	commentHandler.SetWebhookSender(webhookSender)

	// Mention service
	mentionService := services.NewMentionService(s.db, s.notificationService)
	itemHandler.SetMentionService(mentionService)
	commentHandler.SetMentionService(mentionService)

	// Comment service
	commentService := services.NewCommentService(s.db)
	commentService.SetActivityTracker(s.activityTracker)
	commentService.SetNotificationService(s.notificationService)
	commentService.SetMentionService(mentionService)
	commentService.SetWebhookSender(webhookSender)
	commentHandler.SetCommentService(commentService)
	s.actionService.SetCommentService(commentService)
	slog.Info("comment service initialized")

	// Wire up action service
	itemHandler.SetActionService(s.actionService)
	itemLinkHandler.SetActionService(s.actionService)

	// Channel handler
	channelHandler := handlers.NewChannelHandler(s.db, permService, webhookSender)
	channelHandler.SetEmailScheduler(s.emailScheduler)
	channelHandler.SetEncryption(scmProviderHandler.GetEncryption())
	channelHandler.SetBaseURL(baseURL)
	channelHandler.SetSMTPSender(smtpSender)

	// Webhook handler
	webhookHandler := handlers.NewWebhookHandler(s.db, webhookSender, permService)
	portalHandler := handlers.NewPortalHandler(s.db, sessionManager, portalSessionManager, ipExtractor)
	portalAuthHandler := handlers.NewPortalAuthHandler(s.db, portalSessionManager, sessionManager, magicLinkService, ipExtractor)
	portalCustomersHandler := handlers.NewPortalCustomersHandler(s.db)
	contactRolesHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewContactRoleConfig()),
		func() interface{} { return &models.ContactRole{} })
	hubHandler := handlers.NewHubHandler(s.db, permService)

	// Notification settings
	notificationSettingsHandler := handlers.NewNotificationSettingsHandler(s.db)
	configSetNotificationHandler := handlers.NewConfigurationSetNotificationHandler(s.db)

	// Attachment handlers
	var attachmentHandler *handlers.AttachmentHandler
	var attachmentSettingsHandler *handlers.AttachmentSettingsHandler
	if cfg.AttachmentPath != "" {
		slog.Info("attachments enabled", "path", cfg.AttachmentPath)
		attachmentHandler = handlers.NewAttachmentHandler(s.db, cfg.AttachmentPath, permService)
		attachmentSettingsService := services.NewAttachmentSettingsService(s.db)
		if err := attachmentSettingsService.Initialize(cfg.AttachmentPath); err != nil {
			slog.Warn("failed to initialize attachment settings", "error", err)
		}
		attachmentSettingsHandler = handlers.NewAttachmentSettingsHandler(attachmentSettingsService)
	} else {
		slog.Info("attachments disabled (no attachment path specified)")
	}

	// Diagram handler
	diagramHandler := handlers.NewDiagramHandler(s.db)

	// Plugin system
	var pluginRouter *plugins.Router
	if !cfg.DisablePlugins {
		var pluginOpts []plugins.Option
		pluginOpts = append(pluginOpts, plugins.WithDatabase(s.db), plugins.WithSCMService(scmSyncService))

		if pluginDirsEnv := os.Getenv("PLUGIN_DIRS"); pluginDirsEnv != "" {
			var additionalDirs []string
			for _, dir := range strings.Split(pluginDirsEnv, ",") {
				dir = strings.TrimSpace(dir)
				if dir != "" && dir != "plugins" {
					additionalDirs = append(additionalDirs, dir)
				}
			}
			if len(additionalDirs) > 0 {
				slog.Info("loading plugins from additional directories", "dirs", additionalDirs)
				pluginOpts = append(pluginOpts, plugins.WithAdditionalPluginDirs(additionalDirs...))
			}
		}

		s.pluginManager = plugins.NewManager("plugins", pluginOpts...)
		slog.Info("initializing plugin system")
		if err := s.pluginManager.LoadPlugins(); err != nil {
			slog.Warn("failed to load plugins", "error", err)
		}

		// Create webhook dispatcher
		webhookDispatcher := plugins.NewWebhookDispatcher(s.pluginManager, s.db)
		webhookSender.SetPluginDispatcher(webhookDispatcher)

		// Register plugin webhooks
		ctx := context.Background()
		for _, plugin := range s.pluginManager.ListPlugins() {
			if err := s.pluginManager.RegisterPluginWebhooks(ctx, s.db, plugin); err != nil {
				slog.Warn("failed to register plugin webhooks", "plugin", plugin.Manifest.Name, "error", err)
			}
		}

		pluginRouter = plugins.NewRouter(s.pluginManager)
	} else {
		slog.Info("plugin system disabled")
	}

	pluginHandler := handlers.NewPluginHandler(s.db, s.pluginManager, cfg.DisablePlugins)

	// System handler
	shutdownChan := cfg.ShutdownChan
	if shutdownChan == nil {
		shutdownChan = make(chan os.Signal, 1)
	}
	systemHandler := handlers.NewSystemHandler(shutdownChan)

	// Build API middleware chain
	corsMiddleware := createCORSMiddleware(cfg.AllowedHosts, effectivePort, cfg.DisableCSRF, cfg.UseProxy)
	apiMiddleware := router.MiddlewareChain{corsMiddleware, authMiddleware.OptionalAuth}

	if !cfg.DisableCSRF {
		slog.Info("CSRF protection enabled with bearer token bypass")
		apiMiddleware = append(apiMiddleware, csrfMiddleware.AddCSRFTokenToContext, csrfMiddleware.CSRFProtection)
	} else {
		slog.Warn("CSRF protection disabled (development mode)")
	}

	// Create API route group
	api := router.NewRouteGroup(mux, "/api", apiMiddleware...)

	// SCIM routes
	scimMiddleware := router.MiddlewareChain{corsMiddleware, s.scimRateLimiter.Limit}
	scimGroup := router.NewRouteGroup(mux, "/scim/v2", scimMiddleware...)

	// Create portal auth middleware (accepts both internal and portal sessions)
	portalAuthMiddleware := middleware.NewPortalAuthMiddleware(sessionManager, portalSessionManager, cfg.UseProxy, additionalProxyList)

	// Build route dependencies
	routeDeps := &routes.Deps{
		API:       api,
		SCIMGroup: scimGroup,
		Mux:       mux,

		AuthMiddleware:       authMiddleware,
		PermissionMiddleware: permissionMiddleware,
		SCIMAuthMiddleware:   scimAuthMiddleware,
		CSRFMiddleware:       csrfMiddleware,
		PortalAuthMiddleware: portalAuthMiddleware,
		DisableCSRF:          cfg.DisableCSRF,

		LoginRateLimiter:    s.loginRateLimiter,
		AuthRateLimiter:     s.authRateLimiter,
		FIDORateLimiter:     s.fidoRateLimiter,
		SSORateLimiter:      s.ssoRateLimiter,
		SCIMRateLimiter:     s.scimRateLimiter,
		PortalSubmitLimiter: s.portalSubmitLimiter,
		PortalSearchLimiter: s.portalSearchLimiter,
		PortalAuthLimiter:   s.portalAuthLimiter,
		EmailVerifyLimiter:  s.emailVerifyLimiter,
		SetupLimiter:        s.setupLimiter,

		Auth: routes.AuthHandlers{
			Auth:     authHandler,
			SSO:      ssoHandler,
			WebAuthn: webAuthnHandler,
		},
		SCIM: routes.SCIMHandlers{
			SCIM:      scimHandler,
			SCIMToken: scimTokenHandler,
		},
		SCM: routes.SCMHandlers{
			Provider:      scmProviderHandler,
			Workspace:     scmWorkspaceHandler,
			ItemLinks:     scmItemLinksHandler,
			UserToken:     userSCMTokenHandler,
			EmailProvider: emailProviderHandler,
		},
		Items: routes.ItemHandlers{
			Item:               itemHandler,
			Recurrence:         recurrenceHandler,
			Comment:            commentHandler,
			Attachment:         attachmentHandler,
			AttachmentSettings: attachmentSettingsHandler,
			Diagram:            diagramHandler,
			ItemLink:           itemLinkHandler,
			LinkType:           linkTypeHandler,
			Label:              labelHandler,
		},
		Workspaces: routes.WorkspaceHandlers{
			Workspace:             workspaceHandler,
			FieldRequirement:      workspaceFieldReqHandler,
			Screen:                screenHandler,
			ConfigSet:             configSetHandler,
			ConfigSetNotification: configSetNotificationHandler,
			NotificationSettings:  notificationSettingsHandler,
			ItemType:              itemTypeHandler,
			Priority:              priorityHandler,
			HierarchyLevel:        hierarchyLevelHandler,
			RequestType:           requestTypeHandler,
			StatusCategory:        statusCategoryHandler,
			Status:                statusHandler,
			StatusLegacy:          statusHandlerLegacy,
			Workflow:              workflowHandler,
			Actions:               actionsHandler,
		},
		Users: routes.UserHandlers{
			User:          userHandler,
			Group:         groupHandler,
			Permission:    permissionHandler,
			PermissionSet: permissionSetHandler,
			WorkspaceRole: workspaceRoleHandler,
			Credential:    credentialHandler,
			AppToken:      appTokenHandler,
			APIToken:      apiTokenHandler,
		},
		Admin: routes.AdminHandlers{
			SecuritySettings: securitySettingsHandler,
			AuthPolicy:       authPolicyHandler,
			Theme:            themeHandler,
			UserPreferences:  userPreferencesHandler,
			JiraImport:       jiraImportHandler,
			Plugin:           pluginHandler,
			Setup:            setupHandler,
			System:           systemHandler,
		},
		Planning: routes.PlanningHandlers{
			MilestoneCategory: milestoneCategoryHandler,
			Milestone:         milestoneHandler,
			IterationType:     iterationTypeHandler,
			Iteration:         iterationHandler,
			PersonalLabel:     personalLabelHandler,
		},
		TimeTracking: routes.TimeTrackingHandlers{
			Customer:          timeCustomerHandler,
			ProjectCategory:   timeProjectCategoryHandler,
			Project:           timeProjectHandler,
			Worklog:           timeWorklogHandler,
			ActiveTimer:       activeTimerHandler,
			ProjectPermission: timeProjectPermissionHandler,
		},
		TestMgmt: routes.TestManagementHandlers{
			Folder:      testFolderHandler,
			Case:        testCaseHandler,
			Set:         testSetHandler,
			RunTemplate: testRunTemplateHandler,
			Run:         testRunHandler,
			Summary:     testSummaryHandler,
		},
		Channels: routes.ChannelHandlers{
			ChannelCategory:      channelCategoryHandler,
			Channel:              channelHandler,
			Notification:         notificationHandler,
			NotificationTemplate: notificationTemplateHandler,
			Webhook:              webhookHandler,
			AssetReport:          assetReportHandler,
		},
		Portal: routes.PortalHandlers{
			Portal:         portalHandler,
			PortalAuth:     portalAuthHandler,
			PortalCustomer: portalCustomersHandler,
			ContactRole:    contactRolesHandler,
			Hub:            hubHandler,
		},
		Assets: routes.AssetHandlers{
			Asset:    assetHandler,
			Type:     assetTypeHandler,
			Category: assetCategoryHandler,
			Status:   assetStatusHandler,
		},
		Collections: routes.CollectionHandlers{
			Category:     collectionCategoryHandler,
			Collection:   collectionHandler,
			BoardConfig:  boardConfigHandler,
			TestCoverage: testCoverageHandler,
		},
		Misc: routes.MiscHandlers{
			Homepage:     homepageHandler,
			Review:       reviewHandler,
			CalendarFeed: calendarFeedHandler,
			CustomField:  customFieldHandler,
		},
	}
	routes.RegisterAll(routeDeps)

	// Register plugin routes
	if pluginRouter != nil {
		pluginRouter.RegisterRoutes(mux)
	}

	// REST API v1
	restapi.SetupRoutes(mux, s.db, tokenManager, permService, v1.RegisterRoutes)

	// Frontend files
	if cfg.FrontendFiles != (embed.FS{}) {
		distFS, err := fs.Sub(cfg.FrontendFiles, "frontend/dist")
		if err != nil {
			slog.Warn("frontend files not found, serving API only")
		} else {
			fileServer := http.FileServer(http.FS(distFS))

			mux.Handle("GET /remoteEntry.js", fileServer)
			mux.Handle("GET /assets/", fileServer)
			mux.Handle("GET /windshift-3.svg", fileServer)

			mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
					http.NotFound(w, r)
					return
				}
				if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/rest" {
					http.NotFound(w, r)
					return
				}

				indexFile, err := distFS.Open("index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				defer indexFile.Close()

				w.Header().Set("Content-Type", "text/html")
				http.ServeContent(w, r, "index.html", time.Time{}, indexFile.(io.ReadSeeker))
			})
		}
	}

	// Apply middleware (recovery is outermost to catch all panics)
	securityMiddleware := createSecurityHeaders(enableHTTPS, cfg.UseProxy, additionalProxyIPs)
	compressionMiddleware := middleware.CreateCompressionMiddleware(cfg.UseProxy)
	handler := middleware.Recovery(compressionMiddleware(securityMiddleware(mux)))

	// Create HTTP server
	s.httpServer = &http.Server{
		Handler:        handler,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	return nil
}

// Start begins listening for HTTP requests.
// This method is non-blocking; the server runs in a goroutine.
// Use Shutdown to stop the server gracefully.
func (s *Server) Start() error {
	if s.started {
		return errors.New("server already started")
	}

	// Create listener
	addr := ":" + s.config.Port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Get actual port (important for port 0)
	tcpAddr := listener.Addr().(*net.TCPAddr)
	s.actualPort = tcpAddr.Port

	enableHTTPS := s.config.TLSCertPath != "" && s.config.TLSKeyPath != ""

	if enableHTTPS {
		slog.Info("HTTPS server starting", "port", s.actualPort)
		go func() {
			if err := s.httpServer.ServeTLS(s.listener, s.config.TLSCertPath, s.config.TLSKeyPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("HTTPS server error", "error", err)
			}
		}()
	} else {
		slog.Info("HTTP server starting", "port", s.actualPort)
		go func() {
			if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("HTTP server error", "error", err)
			}
		}()
	}

	s.started = true
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	// Prevent double shutdown
	if s.shuttingDown {
		return nil
	}
	s.shuttingDown = true

	slog.Info("starting graceful shutdown")

	// Stop schedulers first - use safeClose helper to avoid panics on already-closed channels
	safeClose := func(ch chan struct{}) {
		if ch != nil {
			defer func() { recover() }() // Ignore panic if already closed
			close(ch)
		}
	}

	safeClose(s.scmSyncStopChan)
	s.scmSyncStopChan = nil

	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
		s.cleanupTicker = nil
	}
	safeClose(s.cleanupStopChan)
	s.cleanupStopChan = nil

	if s.notificationScheduler != nil {
		slog.Info("stopping notification scheduler")
		s.notificationScheduler.Stop()
	}

	if s.recurrenceScheduler != nil {
		slog.Info("stopping recurrence scheduler")
		s.recurrenceScheduler.Stop()
	}

	if s.actionService != nil {
		slog.Info("stopping action service")
		s.actionService.Stop()
	}

	if s.emailScheduler != nil {
		slog.Info("stopping email scheduler")
		s.emailScheduler.Stop()
	}

	if s.notificationService != nil {
		slog.Info("stopping notification service")
		s.notificationService.Close()
	}

	if s.notificationManager != nil {
		slog.Info("stopping notification manager")
		s.notificationManager.Stop()
	}

	// Stop HTTP server
	if s.httpServer != nil {
		s.httpServer.SetKeepAlivesEnabled(false)
		slog.Info("shutting down HTTP server")
		if err := s.httpServer.Shutdown(ctx); err != nil {
			slog.Warn("HTTP server shutdown timed out, forcing close", "error", err)
			s.httpServer.Close()
		}
	}

	// Cleanup remaining resources
	s.cleanup()

	slog.Info("server shutdown complete")
	return nil
}

// cleanup releases all resources.
func (s *Server) cleanup() {
	// Stop rate limiters
	if s.loginRateLimiter != nil {
		s.loginRateLimiter.Stop()
	}
	if s.fidoRateLimiter != nil {
		s.fidoRateLimiter.Stop()
	}
	if s.authRateLimiter != nil {
		s.authRateLimiter.Stop()
	}
	if s.scimRateLimiter != nil {
		s.scimRateLimiter.Stop()
	}
	if s.portalSubmitLimiter != nil {
		s.portalSubmitLimiter.Stop()
	}
	if s.portalSearchLimiter != nil {
		s.portalSearchLimiter.Stop()
	}
	if s.emailVerifyLimiter != nil {
		s.emailVerifyLimiter.Stop()
	}
	if s.setupLimiter != nil {
		s.setupLimiter.Stop()
	}
	if s.ssoRateLimiter != nil {
		s.ssoRateLimiter.Stop()
	}
	if s.portalAuthLimiter != nil {
		s.portalAuthLimiter.Stop()
	}

	// Close activity tracker
	if s.activityTracker != nil {
		s.activityTracker.Close()
	}

	// Close token tracker
	if s.tokenTracker != nil {
		s.tokenTracker.Close()
	}

	// Close database
	if s.db != nil {
		s.db.Close()
	}
}

// BaseURL returns the server's base URL.
func (s *Server) BaseURL() string {
	if s.actualPort == 0 {
		return fmt.Sprintf("http://localhost:%s", s.config.Port)
	}
	return fmt.Sprintf("http://localhost:%d", s.actualPort)
}

// Port returns the actual port the server is listening on.
func (s *Server) Port() int {
	return s.actualPort
}

// DB returns the database instance (for testing).
func (s *Server) DB() database.Database {
	return s.db
}

// runActivityCleanup runs periodic activity cleanup.
func (s *Server) runActivityCleanup() {
	// Initial cleanup after 1 hour
	select {
	case <-time.After(1 * time.Hour):
		slog.Info("running initial activity cleanup")
		if err := s.activityTracker.CleanupExpiredActivities(); err != nil {
			slog.Error("failed to cleanup expired activities", "error", err)
		}
	case <-s.cleanupStopChan:
		return
	}

	// Then run daily
	for {
		select {
		case <-s.cleanupTicker.C:
			slog.Info("running scheduled activity cleanup")
			if err := s.activityTracker.CleanupExpiredActivities(); err != nil {
				slog.Error("failed to cleanup expired activities", "error", err)
			}
		case <-s.cleanupStopChan:
			return
		}
	}
}

// runSCMSync runs periodic SCM synchronization.
func (s *Server) runSCMSync(scmSyncService *scm.SyncService) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	slog.Info("SCM sync scheduler started (5-minute interval)")
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			if err := scmSyncService.SyncAllRepositories(ctx); err != nil {
				slog.Error("SCM sync error", "error", err)
			}
			if err := scmSyncService.RefreshAllPRLinkStates(ctx); err != nil {
				slog.Error("PR state refresh error", "error", err)
			}
			cancel()
		case <-s.scmSyncStopChan:
			slog.Info("SCM sync scheduler stopped")
			return
		}
	}
}
