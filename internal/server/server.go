// Package server provides a reusable HTTP server for windshift.
// This allows the server to be started both from the main binary
// and in-process for integration tests.
package server

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/config"
	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/handlers"
	"windshift/internal/ldap"
	"windshift/internal/llm"
	"windshift/internal/logger"
	mcpserver "windshift/internal/mcp"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/plugins"
	"windshift/internal/repository"
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

// Config is an alias to config.Config — the canonical, fully-resolved runtime
// configuration. All resolution of env vars and CLI flags happens in
// internal/config/Load; this package only consumes the result.
type Config = config.Config

// Server represents a windshift HTTP server instance.
type Server struct {
	config     Config
	httpServer *http.Server
	db         database.Database
	listener   net.Listener

	// Services that need cleanup
	notificationManager       *handlers.NotificationManager
	notificationService       *services.NotificationService
	notificationScheduler     *scheduler.NotificationScheduler
	recurrenceScheduler       *scheduler.RecurrenceScheduler
	workflowService           *services.WorkflowService
	actionService             *services.ActionService
	assetActionService        *services.AssetActionService
	approvalEscalationSweeper *services.ApprovalEscalationSweeper
	emailScheduler            *scheduler.EmailScheduler
	briefingScheduler         *scheduler.BriefingScheduler
	activityTracker           *services.ActivityTracker
	tokenTracker              *services.TokenTracker
	scmSyncStopChan           chan struct{}
	issueSyncStopChan         chan struct{}
	magicLinkStopChan         chan struct{}
	cleanupStopChan           chan struct{}
	cleanupTicker             *time.Ticker
	pluginManager             *plugins.Manager

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
	aiRateLimiter       *middleware.RateLimiter
	uploadLimiter       *middleware.RateLimiter
	webhookLimiter      *middleware.RateLimiter
	searchLimiter       *middleware.RateLimiter
	calendarFeedLimiter *middleware.RateLimiter

	// Server state
	actualPort   int
	started      bool
	shuttingDown bool
}

// New creates a new Server instance with the given configuration.
// It initializes all services and handlers but does not start listening.
func New(cfg Config) (*Server, error) {
	s := &Server{
		config:            cfg,
		scmSyncStopChan:   make(chan struct{}),
		issueSyncStopChan: make(chan struct{}),
		magicLinkStopChan: make(chan struct{}),
		cleanupStopChan:   make(chan struct{}),
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
	if cfg.DB.PostgresConn != "" {
		slog.Info("connecting to PostgreSQL database")
		s.db, err = database.NewDatabase("postgres", cfg.DB.PostgresConn, cfg.DB.MaxReadConns, cfg.DB.MaxWriteConns)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
		}
		slog.Info("PostgreSQL database initialized", "max_read_conns", cfg.DB.MaxReadConns, "max_write_conns", cfg.DB.MaxWriteConns)
	} else {
		slog.Info("connecting to SQLite database", "path", cfg.DB.SQLitePath)
		s.db, err = database.NewDatabase("sqlite3", cfg.DB.SQLitePath, cfg.DB.MaxReadConns, cfg.DB.MaxWriteConns)
		if err != nil {
			return fmt.Errorf("failed to connect to SQLite database: %w", err)
		}
		slog.Info("SQLite database initialized", "max_read_conns", cfg.DB.MaxReadConns, "max_write_conns", cfg.DB.MaxWriteConns, "mode", "WAL")
	}

	if err = s.db.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Ensure default notification settings exist
	if err = s.db.EnsureDefaultNotificationSettings(); err != nil {
		slog.Warn("failed to ensure notification settings", "error", err)
	}

	// Migrate legacy select field options to ID-based format
	if err = s.db.MigrateSelectFieldOptions(); err != nil {
		slog.Warn("failed to migrate select field options", "error", err)
	}

	if cfg.RecoverUser != "" {
		s.recoverUser(cfg.RecoverUser)
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
	sessionManager := auth.NewSessionManager(s.db, enableHTTPS, cfg.UseProxy, additionalProxyList, cfg.Auth.SessionSecret)

	// Determine effective port for CORS
	effectivePort := cfg.Port
	if cfg.AllowedPort != "" {
		effectivePort = cfg.AllowedPort
	}

	// Initialize WebAuthn — RPID/RPName are pre-resolved by config.Load;
	// webauthn only overrides RPID when in development mode.
	isDevelopment := cfg.DisableCSRF
	rpID := cfg.WebAuthn.RPID
	if isDevelopment {
		rpID = ""
	}
	webAuthnConfig, err := webauthn.NewConfig(rpID, cfg.WebAuthn.RPName, nil, isDevelopment, cfg.AllowedHosts, effectivePort, enableHTTPS, cfg.UseProxy)
	if err != nil {
		return fmt.Errorf("failed to initialize WebAuthn configuration: %w", err)
	}
	slog.Info("WebAuthn configuration initialized",
		"rp_id", webAuthnConfig.RPID,
		"rp_name", webAuthnConfig.RPName,
		"development_mode", isDevelopment)

	// Build options for user-keyed rate limiters (authenticated endpoints)
	var userKeyedOpts []middleware.RateLimiterOption
	userKeyedOpts = append(userKeyedOpts, middleware.WithUserKeyed())
	if cfg.DisableIPRateLimit {
		userKeyedOpts = append(userKeyedOpts, middleware.WithDisableIPLimit())
	}

	// Create rate limiters
	// IP-only limiters (pre-auth / unauthenticated endpoints)
	s.loginRateLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.fidoRateLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.scimRateLimiter = middleware.NewRateLimiter(10.0, 100, cfg.UseProxy, additionalProxyList)
	s.portalSubmitLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.portalSearchLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.emailVerifyLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.setupLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList)
	s.ssoRateLimiter = middleware.NewRateLimiter(10.0/60.0, 5, cfg.UseProxy, additionalProxyList)
	s.portalAuthLimiter = middleware.NewRateLimiter(3.0/60.0, 3, cfg.UseProxy, additionalProxyList)
	s.calendarFeedLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	// User-keyed limiters (authenticated endpoints — key by user ID, optionally skip IP)
	s.authRateLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.aiRateLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.uploadLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.webhookLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.searchLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList, userKeyedOpts...)

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
	nmCfg := handlers.DefaultNotificationManagerConfig()
	if cfg.Notification.FlushInterval > 0 {
		nmCfg.FlushInterval = cfg.Notification.FlushInterval
	}
	if cfg.Notification.BatchSize > 0 {
		nmCfg.MaxBatchSize = cfg.Notification.BatchSize
	}
	if cfg.Notification.SyncInterval > 0 {
		nmCfg.SyncInterval = cfg.Notification.SyncInterval
	}
	s.notificationManager, err = handlers.NewNotificationManager(s.db, nmCfg)
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

	// WorkflowService is constructed here (moved up from later in bootstrap) so the
	// recurrence scheduler can resolve a workspace+item-type's initial status the
	// same way the rest of the system does. The handler-side instance below reuses
	// the same pointer, so the in-memory cache is shared.
	s.workflowService = services.NewWorkflowService(s.db)
	s.recurrenceScheduler = scheduler.NewRecurrenceScheduler(s.db, s.workflowService)
	s.recurrenceScheduler.Start()
	slog.Info("recurrence scheduler started")

	// Initialize shared execution chain store for cross-application loop prevention
	chainStore := services.NewExecutionChainStore()

	// Initialize action service
	s.actionService = services.NewActionService(s.db, services.DefaultActionServiceConfig(), chainStore)
	s.actionService.SetNotificationService(s.notificationService)
	s.actionService.SetPermissionService(permService)
	slog.Info("action service initialized")

	// Initialize asset action service (shared chain store for cross-application loop prevention)
	s.assetActionService = services.NewAssetActionService(s.db, services.DefaultActionServiceConfig(), chainStore)
	s.assetActionService.SetNotificationService(s.notificationService)
	slog.Info("asset action service initialized")

	// Determine base URL — cfg.BaseURL is already resolved by config.Load
	// from the --base-url flag or BASE_URL env; only the localhost fallback
	// remains here because it needs cfg.Port.
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", cfg.Port)
	}

	// Initialize email verification service
	emailVerificationService := services.NewEmailVerificationService(s.db, smtpSender, baseURL)

	// Initialize portal session manager
	portalSessionManager := auth.NewPortalSessionManager(s.db, enableHTTPS, cfg.UseProxy, additionalProxyList, cfg.Auth.SessionSecret)

	// Initialize magic link service
	magicLinkService := services.NewMagicLinkService(s.db, smtpSender, baseURL)

	// Initialize invitation service
	invitationService := services.NewInvitationService(s.db, smtpSender, baseURL)

	// Initialize workspace key cache (resolves workspace keys to IDs without DB lookups)
	workspaceKeyCache := handlers.NewWorkspaceKeyCache(repository.NewWorkspaceRepository(s.db))

	// Initialize handlers
	itemHandler := handlers.NewItemHandler(s.db, permService, s.activityTracker, s.notificationService)
	customFieldHandler := handlers.NewCustomFieldHandler(s.db)
	workspaceHandler := handlers.NewWorkspaceHandler(s.db, permService, s.activityTracker, workspaceKeyCache)
	screenHandler := handlers.NewScreenHandler(s.db)
	configSetHandler := handlers.NewConfigurationSetHandler(s.db, s.notificationService, permService)
	itemTypeHandler := handlers.NewItemTypeHandler(s.db)
	priorityHandler := handlers.NewPriorityHandler(s.db)

	// Shared audit emitter for enum services
	enumAuditEmit := services.AuditEmitFunc(func(db database.Database, r *http.Request, actionType, resourceType string, entityID int, entityName string) {
		currentUser := utils.GetCurrentUser(r)
		if currentUser == nil {
			return
		}
		_ = logger.LogAudit(db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   actionType,
			ResourceType: resourceType,
			ResourceID:   &entityID,
			ResourceName: entityName,
			Success:      true,
		})
	})

	// Generic enum handlers
	hierarchyLevelConfig := services.NewHierarchyLevelConfig()
	hierarchyLevelConfig.AuditEmit = enumAuditEmit
	hierarchyLevelHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, hierarchyLevelConfig),
		func() interface{} { return &models.HierarchyLevel{} })
	requestTypeHandler := handlers.NewRequestTypeHandler(s.db)
	statusCategoryConfig := services.NewStatusCategoryConfig()
	statusCategoryConfig.AuditEmit = enumAuditEmit
	statusCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, statusCategoryConfig),
		func() interface{} { return &models.StatusCategory{} })
	statusHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, services.NewStatusConfig()),
		func() interface{} { return &models.Status{} })
	statusHandlerLegacy := handlers.NewStatusHandler(repository.NewStatusRepository(s.db), repository.NewItemRepository(s.db), logger.NewAuditor(s.db))
	workflowService := s.workflowService
	workflowHandler := handlers.NewWorkflowHandler(s.db)
	workflowHandler.SetWorkflowService(workflowService)
	userHandler := handlers.NewUserHandler(s.db, permService, invitationService)
	groupHandler := handlers.NewGroupHandler(s.db, permService)
	credentialHandler := handlers.NewCredentialHandler(s.db, permService, cfg.SSH.Enabled)
	webAuthnHandler := handlers.NewWebAuthnHandler(s.db, permService, sessionManager, webAuthnConfig, ipExtractor)
	collectionHandler := handlers.NewCollectionHandler(s.db, permService)
	boardConfigHandler := handlers.NewBoardConfigurationHandler(s.db, permService)
	testCoverageHandler := handlers.NewTestCoverageHandler(s.db, permService)
	publicBoardHandler := handlers.NewPublicBoardHandler(s.db, permService, cfg.AttachmentPath)
	permissionHandler := handlers.NewPermissionHandlerWithCache(s.db, permService)
	apiTokenHandler := handlers.NewAPITokenHandler(s.db, tokenManager, permService)
	agentHandler := handlers.NewAgentHandler(s.db, permService)

	// SCIM handlers
	scimTokenManager := auth.NewSCIMTokenManager(s.db)
	scimAuthMiddleware := middleware.NewSCIMAuthMiddleware(scimTokenManager)
	scimHandler := handlers.NewSCIMHandler(s.db, baseURL, permService)
	scimTokenHandler := handlers.NewSCIMTokenHandler(scimTokenManager, logger.NewAuditor(s.db))

	permissionSetHandler := handlers.NewPermissionSetHandlerWithPool(s.db, permService)
	workspaceRoleHandler := handlers.NewWorkspaceRoleHandlerWithPool(s.db, permService)

	// Time tracking handlers
	timePermissionService := services.NewTimePermissionService(s.db, permService)
	timeCustomerHandler := handlers.NewTimeCustomerHandler(repository.NewCustomerOrganisationRepository(s.db), logger.NewAuditor(s.db), timePermissionService)
	timeProjectHandler := handlers.NewTimeProjectHandler(s.db, timePermissionService, workspaceKeyCache)
	timeProjectCategoryHandler := handlers.NewTimeProjectCategoryHandler(repository.NewTimeProjectCategoryRepository(s.db), logger.NewAuditor(s.db))
	timeWorklogHandler := handlers.NewTimeWorklogHandler(s.db, permService, timePermissionService)
	activeTimerHandler := handlers.NewActiveTimerHandler(s.db, timePermissionService)
	timeProjectPermissionHandler := handlers.NewTimeProjectPermissionHandler(s.db, timePermissionService)

	// Test management handlers
	testFolderHandler := handlers.NewTestFolderHandlerWithPool(s.db)
	testCaseHandler := handlers.NewTestCaseHandlerWithPool(s.db)
	workspaceResourceRepo := repository.NewWorkspaceResourceRepository(s.db)
	testSetHandler := handlers.NewTestSetHandlerWithPool(repository.NewTestSetRepository(s.db), workspaceResourceRepo, logger.NewAuditor(s.db))
	testRunTemplateHandler := handlers.NewTestRunTemplateHandlerWithPool(repository.NewTestRunTemplateRepository(s.db), workspaceResourceRepo)
	testRunHandler := handlers.NewTestRunHandlerWithPool(s.db)
	testSummaryHandler := handlers.NewTestSummaryHandlerWithPool(repository.NewTestSummaryRepository(s.db))

	// Link management handlers
	linkTypeHandler := handlers.NewLinkTypeHandler(repository.NewLinkTypeRepository(s.db), logger.NewAuditor(s.db))
	itemLinkHandler := handlers.NewItemLinkHandler(s.db, s.notificationService, permService)

	// Label handler
	labelHandler := handlers.NewLabelHandler(s.db, permService)

	// Recurrence handler
	recurrenceHandler := handlers.NewRecurrenceHandler(s.db, s.recurrenceScheduler, permService)

	// Actions handler
	actionsHandler := handlers.NewActionsHandler(s.db, s.actionService, permService, workspaceKeyCache)

	// Team handlers
	teamRepo := repository.NewTeamRepository(s.db)
	leaveRepo := repository.NewLeaveRepository(s.db)
	onCallRepo := repository.NewOnCallRepository(s.db)
	teamService := services.NewTeamService(s.db, teamRepo, leaveRepo)
	onCallService := services.NewOnCallService(s.db, onCallRepo, leaveRepo)
	teamHandler := handlers.NewTeamHandler(s.db, teamRepo, leaveRepo, permService)
	leaveHandler := handlers.NewLeaveHandler(leaveRepo, permService)
	onCallHandler := handlers.NewOnCallHandler(s.db, onCallRepo, teamRepo, onCallService, permService)
	s.actionService.SetTeamService(teamService)

	milestoneCategoryConfig := services.NewMilestoneCategoryConfig()
	milestoneCategoryConfig.AuditEmit = enumAuditEmit
	milestoneCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, milestoneCategoryConfig),
		func() interface{} { return &models.MilestoneCategory{} })
	channelCategoryConfig := services.NewChannelCategoryConfig()
	channelCategoryConfig.AuditEmit = enumAuditEmit
	channelCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, channelCategoryConfig),
		func() interface{} { return &models.ChannelCategory{} })
	collectionCategoryConfig := services.NewCollectionCategoryConfig()
	collectionCategoryConfig.AuditEmit = enumAuditEmit
	collectionCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, collectionCategoryConfig),
		func() interface{} { return &models.CollectionCategory{} })
	iterationTypeConfig := services.NewIterationTypeConfig()
	iterationTypeConfig.AuditEmit = enumAuditEmit
	iterationTypeHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, iterationTypeConfig),
		func() interface{} { return &models.IterationType{} })
	iterationHandler := handlers.NewIterationHandler(s.db, permService)
	personalLabelHandler := handlers.NewPersonalLabelHandler(s.db, permService)
	commentHandler := handlers.NewCommentHandler(s.db, permService, s.activityTracker, s.notificationService)
	reviewHandler := handlers.NewReviewHandler(s.db)
	calendarFeedHandler := handlers.NewCalendarFeedHandler(s.db, permService)
	securitySettingsHandler := handlers.NewSecuritySettingsHandler(repository.NewSystemSettingRepository(s.db), logger.NewAuditor(s.db), cfg.Plugins.Disabled)

	// Admin rate limiter
	var adminRateLimiter *middleware.AdminFallbackRateLimiter
	if cfg.EnableAdminFallback {
		adminRateLimiter = middleware.NewAdminFallbackRateLimiter(s.db)
		slog.Info("Admin password fallback enabled", slog.String("component", "auth"))
	}

	authPolicyHandler := handlers.NewAuthPolicyHandlerWithFallback(s.db, cfg.EnableAdminFallback)

	// Initialize auth handler
	authHandler := handlers.NewAuthHandler(s.db, sessionManager, s.loginRateLimiter, permService, emailVerificationService, ipExtractor, authPolicyHandler, adminRateLimiter)

	// Initialize invitation handler
	invitationHandler := handlers.NewInvitationHandler(invitationService)

	themeHandler := handlers.NewThemeHandler(s.db, s.db)
	userPreferencesHandler := handlers.NewUserPreferencesHandler(s.db)
	homepageHandler := handlers.NewHomepageHandler(s.db, s.activityTracker)

	// Notification handlers
	notificationHandler := handlers.NewNotificationHandler(s.notificationManager, s.notificationService)
	emailTemplateHandler := handlers.NewEmailTemplateHandler(repository.NewEmailTemplateRepository(s.db), logger.NewAuditor(s.db))

	permissionMiddleware := middleware.NewPermissionMiddleware(s.db, permService)

	// Setup handler
	setupHandler := handlers.NewSetupHandler(s.db, sessionManager, authMiddleware)

	// SSO handler
	ssoHandler := handlers.NewSSOHandler(s.db, sessionManager, permService, emailVerificationService, s.pluginManager, cfg.Auth.SessionSecret, baseURL, cfg.AllowedHosts, cfg.DisableCSRF, ipExtractor, cfg.UseProxy, additionalProxyList)

	// SCM provider handler
	scmProviderHandler := handlers.NewSCMProviderHandler(s.db, cfg.Auth.SessionSecret, baseURL)
	scmWorkspaceHandler := handlers.NewSCMWorkspaceHandler(s.db, scmProviderHandler.GetEncryption(), scmProviderHandler, permService, baseURL)
	scmItemLinksHandler := handlers.NewSCMItemLinksHandler(s.db, scmProviderHandler.GetEncryption(), permService)
	userSCMTokenHandler := handlers.NewUserSCMTokenHandler(s.db, scmProviderHandler.GetEncryption())
	milestoneHandler := handlers.NewMilestoneHandler(s.db, permService, scm.NewCredentialResolver(s.db, scmProviderHandler.GetEncryption()))

	// Asset management handlers
	assetHandler := handlers.NewAssetHandler(s.db, permService, cfg.AttachmentPath)
	assetHandler.SetAssetActionService(s.assetActionService)
	if n, err := assetHandler.ReconcileInterruptedImports(); err != nil {
		slog.Warn("failed to reconcile interrupted asset imports", slog.Any("error", err))
	} else if n > 0 {
		slog.Info("reconciled interrupted asset imports", slog.Int("count", n))
	}
	itemLinkHandler.SetAssetPermissionChecker(assetHandler)
	assetTypeHandler := handlers.NewAssetTypeHandler(s.db, permService)
	assetCategoryHandler := handlers.NewAssetCategoryHandler(s.db, permService)
	assetStatusHandler := handlers.NewAssetStatusHandler(s.db, permService)
	assetReportHandler := handlers.NewAssetReportHandler(s.db)
	assetActionHandler := handlers.NewAssetActionHandler(s.db, assetHandler, s.assetActionService)

	// Jira import handler
	jiraImportHandler := handlers.NewJiraImportHandler(s.db, cfg.Auth.SessionSecret, cfg.Jira.CapturePayloadsDir)

	// Email provider handler
	emailProviderHandler := handlers.NewEmailProviderHandler(s.db, scmProviderHandler.GetEncryption(), baseURL)

	// Email scheduler
	emailCredManager := email.NewCredentialManager(s.db, scmProviderHandler.GetEncryption())
	s.emailScheduler = scheduler.NewEmailScheduler(s.db, emailCredManager, cfg.AttachmentPath)
	s.emailScheduler.Start()
	slog.Info("email scheduler started (IMAP polling)")

	// Integration provider handlers
	integrationProviderHandler := handlers.NewIntegrationProviderHandler(repository.NewIntegrationProviderRepository(s.db), scmProviderHandler.GetEncryption())
	integrationOAuthHandler := handlers.NewIntegrationOAuthHandler(s.db, scmProviderHandler.GetEncryption(), baseURL)
	integrationItemLinksHandler := handlers.NewIntegrationItemLinksHandler(s.db, scmProviderHandler.GetEncryption(), permService)

	// SCM sync service (started below once smart-commit dependencies exist)
	scmSyncService := scm.NewSyncService(s.db, scmProviderHandler.GetEncryption())

	// Issue sync service
	issueSyncService := scm.NewIssueSyncService(s.db, scmProviderHandler.GetEncryption())
	issueSyncService.SetUserService(services.NewUserReadService(s.db))

	// Start issue sync scheduler
	go s.runIssueSync(issueSyncService)

	// Start magic link cleanup scheduler
	go s.runMagicLinkCleanup(magicLinkService)

	// Webhook sender
	webhookSender := webhook.NewWebhookSender(s.db)

	// Event coordinator
	eventCoordinator := services.NewEventCoordinator(s.db)
	eventCoordinator.SetNotificationService(s.notificationService)
	eventCoordinator.SetActivityTracker(s.activityTracker)
	eventCoordinator.SetWebhookDispatcher(webhookSender)
	eventCoordinator.SetActionService(s.actionService)
	eventCoordinator.SetAssetActionService(s.assetActionService)
	eventCoordinator.SetMagicLinkService(magicLinkService)
	s.actionService.SetAssetActionService(s.assetActionService)
	s.actionService.SetEventCoordinator(eventCoordinator)
	s.actionService.SetAssetPermissionChecker(assetHandler)
	s.assetActionService.SetEventCoordinator(eventCoordinator)
	slog.Info("event coordinator initialized")

	// Wire up services
	itemHandler.SetWebhookSender(webhookSender)
	itemHandler.SetEventCoordinator(eventCoordinator)
	commentHandler.SetWebhookSender(webhookSender)

	// Mention service
	mentionService := services.NewMentionService(s.db, s.notificationService, permService)
	itemHandler.SetMentionService(mentionService)
	commentHandler.SetMentionService(mentionService)

	// Comment service
	commentService := services.NewCommentService(s.db)
	commentService.SetActivityTracker(s.activityTracker)
	commentService.SetNotificationService(s.notificationService)
	commentService.SetMentionService(mentionService)
	commentService.SetWebhookSender(webhookSender)
	commentHandler.SetCommentService(commentService)
	commentHandler.SetIssueSyncService(issueSyncService)
	s.actionService.SetCommentService(commentService)

	// Wire email reply service for bidirectional email threading
	emailReplyService := services.NewEmailReplyService(s.db, smtpSender)
	commentService.SetEmailReplyService(emailReplyService)

	// Wire CommentService into email processor for unified comment creation
	s.emailScheduler.SetCommentService(commentService)

	slog.Info("comment service initialized")

	// Wire up action service
	itemHandler.SetActionService(s.actionService)
	itemHandler.SetIssueSyncService(issueSyncService)
	itemLinkHandler.SetActionService(s.actionService)

	// Wire up condition service for workflow transition conditions
	scriptEngine := services.NewScriptEngine()
	conditionService := services.NewConditionService(s.db, permService, scriptEngine)
	itemHandler.SetConditionService(conditionService)

	// Wire up approval service for status-bound approvals (sibling of conditions).
	approvalService := services.NewApprovalService(s.db, permService, leaveRepo, workflowService)
	approvalService.SetEventCoordinator(eventCoordinator)
	approvalSetService := services.NewApprovalSetService(s.db)
	itemHandler.SetApprovalService(approvalService)
	commentHandler.SetApprovalService(approvalService)
	s.actionService.SetApprovalService(approvalService)
	workspaceRoleHandler.SetApprovalService(approvalService)

	// Background sweeper drives time-based escalation for pending approval steps.
	s.approvalEscalationSweeper = services.NewApprovalEscalationSweeper(s.db, approvalService, services.DefaultApprovalEscalationSweeperConfig())
	s.approvalEscalationSweeper.Start()

	// Wire smart-commit dependencies into the SCM sync service and start its
	// scheduler. Must be done after commentService and conditionService exist.
	scmSyncService.SetSmartCommitServices(
		workflowService, commentService, permService, conditionService,
		repository.NewItemRepository(s.db),
	)
	scmSyncService.SetApprovalService(approvalService)
	go s.runSCMRepoSync(scmSyncService)
	go s.runSCMLinkRefresh(scmSyncService)
	go s.runSCMOAuthStateCleanup()

	// Channel handler
	channelHandler := handlers.NewChannelHandler(s.db, permService, webhookSender)
	channelHandler.SetEmailScheduler(s.emailScheduler)
	channelHandler.SetEncryption(scmProviderHandler.GetEncryption())
	channelHandler.SetBaseURL(baseURL)
	channelHandler.SetSMTPSender(smtpSender)
	// Wire at-rest decryption into the SMTP sender so dispatch can decrypt
	// SMTPPassword before AUTH PLAIN. Done here (after scmProviderHandler is
	// initialized) rather than at smtpSender construction time because the
	// scheduler/notification wiring above can't depend on the encryption
	// service yet.
	smtpSender.SetEncryption(scmProviderHandler.GetEncryption())

	// Webhook handler
	webhookHandler := handlers.NewWebhookHandler(repository.NewChannelRepository(s.db), repository.NewItemRepository(s.db), webhookSender, permService)
	portalHandler := handlers.NewPortalHandler(s.db, sessionManager, portalSessionManager, ipExtractor, cfg.AttachmentPath)
	portalHandler.SetApprovalService(approvalService)
	portalAuthHandler := handlers.NewPortalAuthHandler(s.db, portalSessionManager, sessionManager, magicLinkService, ipExtractor)
	portalCustomersHandler := handlers.NewPortalCustomersHandler(s.db, permService)
	contactRoleConfig := services.NewContactRoleConfig()
	contactRoleConfig.AuditEmit = enumAuditEmit
	contactRolesHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, contactRoleConfig),
		func() interface{} { return &models.ContactRole{} })
	hubHandler := handlers.NewHubHandler(s.db, permService)
	formHandler := handlers.NewFormHandler(s.db, sessionManager, portalSessionManager, ipExtractor)

	// Notification settings
	notificationSettingsHandler := handlers.NewNotificationSettingsHandler(s.db)
	configSetNotificationHandler := handlers.NewConfigurationSetNotificationHandler(s.db)

	// Attachment handlers
	var attachmentHandler *handlers.AttachmentHandler
	var attachmentSettingsHandler *handlers.AttachmentSettingsHandler
	if cfg.AttachmentPath != "" {
		slog.Info("attachments enabled", "path", cfg.AttachmentPath)
		attachmentHandler = handlers.NewAttachmentHandler(s.db, cfg.AttachmentPath, permService)
		attachmentHandler.SetApprovalService(approvalService)
		attachmentSettingsService := services.NewAttachmentSettingsService(s.db)
		if err := attachmentSettingsService.Initialize(cfg.AttachmentPath); err != nil {
			slog.Warn("failed to initialize attachment settings", "error", err)
		}
		attachmentSettingsHandler = handlers.NewAttachmentSettingsHandler(attachmentSettingsService, logger.NewAuditor(s.db))
	} else {
		slog.Info("attachments disabled (no attachment path specified)")
	}

	// Diagram handler
	diagramHandler := handlers.NewDiagramHandler(repository.NewDiagramRepository(s.db), repository.NewItemRepository(s.db), permService)

	// Plugin system
	var pluginRouter *plugins.Router
	if !cfg.Plugins.Disabled {
		var pluginOpts []plugins.Option
		pluginOpts = append(pluginOpts, plugins.WithDatabase(s.db), plugins.WithSCMService(scmSyncService), plugins.WithCommentService(commentService))

		pluginDir := cfg.Plugins.Dir
		if pluginDir == "" {
			pluginDir = "plugins"
		}

		// PLUGIN_DIRS additional dirs (pre-split by config.Load)
		var additionalDirs []string
		for _, dir := range cfg.Plugins.ExtraDirs {
			if dir != "" && dir != pluginDir {
				additionalDirs = append(additionalDirs, dir)
			}
		}
		if len(additionalDirs) > 0 {
			slog.Info("loading plugins from additional directories", "dirs", additionalDirs)
			pluginOpts = append(pluginOpts, plugins.WithAdditionalPluginDirs(additionalDirs...))
		}

		s.pluginManager = plugins.NewManager(pluginDir, pluginOpts...)
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

	pluginHandler := handlers.NewPluginHandler(s.db, s.pluginManager, cfg.Plugins.Disabled)

	// Audit log handler
	auditLogHandler := handlers.NewAuditLogHandler(repository.NewAuditLogRepository(s.db))

	// LDAP handler
	ldapSyncService := ldap.NewSyncService(s.db, ssoHandler.GetEncryption())
	ldapHandler := handlers.NewLDAPHandler(s.db, ldapSyncService, ssoHandler.GetEncryption())

	// Features handler
	featuresHandler := handlers.NewFeaturesHandler(s.pluginManager, cfg.SSH.Enabled)

	// System handler
	shutdownChan := cfg.ShutdownChan
	if shutdownChan == nil {
		shutdownChan = make(chan os.Signal, 1)
	}
	systemHandler := handlers.NewSystemHandler(shutdownChan)

	// Load LLM provider definitions
	if cfg.LLM.ProvidersFile != "" {
		if err := llm.LoadProviders(cfg.LLM.ProvidersFile); err != nil {
			slog.Error("failed to load custom LLM providers file, falling back to built-in defaults", "path", cfg.LLM.ProvidersFile, "error", err)
			llm.LoadDefaultProviders()
		} else {
			slog.Info("loaded custom LLM providers", "path", cfg.LLM.ProvidersFile)
		}
	} else {
		llm.LoadDefaultProviders()
	}

	// LLM connection manager and AI handler
	fallbackLLMClient := llm.NewClient(llm.Config{Endpoint: cfg.LLM.Endpoint})
	if fallbackLLMClient.Available() {
		slog.Info("LLM fallback service configured", slog.String("endpoint", cfg.LLM.Endpoint))
	} else {
		slog.Info("LLM fallback service not configured")
	}
	llmManager := llm.NewConnectionManager(s.db, scmProviderHandler.GetEncryption(), fallbackLLMClient)
	llmConnHandler := handlers.NewLLMConnectionHandler(llmManager, logger.NewAuditor(s.db))
	promptStore := llm.NewPromptStore(cfg.LLM.PromptsDir)
	aiHandler := handlers.NewAIHandler(s.db, llmManager, permService, timePermissionService, promptStore)

	// Briefing scheduler (generates daily briefings for all users)
	s.briefingScheduler = scheduler.NewBriefingScheduler(s.db, llmManager, permService, timePermissionService, services.NewUserReadService(s.db), promptStore)
	s.briefingScheduler.Start()

	// Logbook reverse proxy (optional sidecar)
	if cfg.Logbook.Endpoint != "" {
		proxyCfg := LogbookProxyConfig{
			Endpoint:          cfg.Logbook.Endpoint,
			AuthMiddleware:    authMiddleware,
			PermissionService: permService,
			UploadLimiter:     s.uploadLimiter,
			SharedSecret:      cfg.Auth.SessionSecret,
		}
		logbookProxy := NewLogbookProxy(proxyCfg)

		// Rate-limited upload routes (registered before the catch-all so they take priority)
		logbookUploadProxy := NewLogbookUploadProxy(proxyCfg)
		mux.Handle("POST /api/logbook/buckets/{bucketID}/documents/upload", logbookUploadProxy)
		mux.Handle("POST /api/logbook/documents/{documentID}/attachments", logbookUploadProxy)

		// All logbook routes (including actions) are proxied to the sidecar
		mux.Handle("GET /api/logbook/", logbookProxy)
		mux.Handle("POST /api/logbook/", logbookProxy)
		mux.Handle("PUT /api/logbook/", logbookProxy)
		mux.Handle("PATCH /api/logbook/", logbookProxy)
		mux.Handle("DELETE /api/logbook/", logbookProxy)
		slog.Info("logbook proxy enabled", "endpoint", cfg.Logbook.Endpoint)

		// Internal endpoints for sidecar → main server communication.
		// cfg.Auth.SessionSecret is already validated non-empty by config.Load,
		// so the guard is cosmetic — kept for defense-in-depth.
		if ssoSecret := cfg.Auth.SessionSecret; ssoSecret != "" {
			// LLM proxy for logbook article generation
			llmProxy := NewInternalLLMProxy(llmManager, ssoSecret)
			mux.Handle("POST /api/internal/llm/v1/chat/completions", llmProxy)
			mux.Handle("GET /api/internal/llm/health", NewInternalLLMHealthCheck(llmManager, ssoSecret))
			slog.Info("internal LLM proxy enabled for logbook article generation")

			// Node execution endpoint for logbook actions (create_item, create_asset on SQLite)
			nodeExecHandler := handlers.NewLogbookNodeExecutionHandler(s.db, ssoSecret, eventCoordinator, permService, assetHandler)
			mux.Handle("POST /api/internal/logbook/execute-node", http.HandlerFunc(nodeExecHandler.HandleNodeExecution))
			slog.Info("internal logbook node execution endpoint enabled")
		}
	}

	// Build API middleware chain
	// Derive scheme from BASE_URL for CORS origin construction
	corsScheme := ""
	if cfg.BaseURL != "" {
		if parsed, err := url.Parse(cfg.BaseURL); err == nil {
			corsScheme = parsed.Scheme
		}
	}
	corsMiddleware := createCORSMiddleware(cfg.AllowedHosts, effectivePort, corsScheme, cfg.DisableCSRF, cfg.UseProxy)
	apiMiddleware := router.MiddlewareChain{corsMiddleware, authMiddleware.OptionalAuth}

	if !cfg.DisableCSRF {
		csrfOrigins := buildAllowedOrigins(cfg.AllowedHosts, effectivePort, corsScheme, cfg.UseProxy)
		slog.Info("CSRF protection enabled (Sec-Fetch-Site + Origin/Referer fallback)", "allowed_origins", csrfOrigins)
		apiMiddleware = append(apiMiddleware, middleware.CSRFProtection(csrfOrigins))
	} else {
		slog.Warn("CSRF protection disabled (development mode)")
	}

	// Create API route group
	api := router.NewRouteGroup(mux, "/api", apiMiddleware...)

	// SCIM routes
	scimMiddleware := router.MiddlewareChain{corsMiddleware}
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
		PortalAuthMiddleware: portalAuthMiddleware,

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
		AIRateLimiter:       s.aiRateLimiter,
		UploadLimiter:       s.uploadLimiter,
		WebhookLimiter:      s.webhookLimiter,
		SearchLimiter:       s.searchLimiter,
		CalendarFeedLimiter: s.calendarFeedLimiter,

		Auth: routes.AuthHandlers{
			Auth:       authHandler,
			SSO:        ssoHandler,
			WebAuthn:   webAuthnHandler,
			Invitation: invitationHandler,
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
			IssueSync:     handlers.NewIssueSyncHandler(s.db, issueSyncService, permService),
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
			ActionTemplates:       handlers.NewActionTemplatesHandler(services.NewActionTemplateService(s.db), s.actionService, workspaceKeyCache, logger.NewAuditor(s.db)),
			Analytics:             handlers.NewAnalyticsHandler(services.NewAnalyticsService(s.db), permService, workspaceKeyCache),
			ConditionSet:          handlers.NewConditionSetHandler(s.db),
			ApprovalSet:           handlers.NewApprovalSetHandler(approvalSetService, logger.NewAuditor(s.db)),
			Approval:              handlers.NewApprovalHandler(s.db, permService, approvalService),
			TransitionGovernance:  handlers.NewTransitionGovernanceHandler(repository.NewTransitionRepository(s.db), approvalSetService),
		},
		Users: routes.UserHandlers{
			User:          userHandler,
			Group:         groupHandler,
			Permission:    permissionHandler,
			PermissionSet: permissionSetHandler,
			WorkspaceRole: workspaceRoleHandler,
			Credential:    credentialHandler,
			APIToken:      apiTokenHandler,
			Agent:         agentHandler,
			CLIAuth:       handlers.NewCLIAuthHandler(s.db, agentHandler, tokenManager, apiTokenHandler, permService),
			OAuth:         handlers.NewOAuthHandler(s.db, agentHandler, tokenManager, apiTokenHandler, permService),
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
			AuditLog:         auditLogHandler,
			LDAP:             ldapHandler,
			Features:         featuresHandler,
			OAuthClients:     handlers.NewAdminOAuthClientHandler(s.db, tokenManager, permService),
			Diagnostics:      handlers.NewDiagnosticsHandler(s.db),
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
			ChannelCategory: channelCategoryHandler,
			Channel:         channelHandler,
			Notification:    notificationHandler,
			EmailTemplate:   emailTemplateHandler,
			Webhook:         webhookHandler,
			AssetReport:     assetReportHandler,
		},
		Portal: routes.PortalHandlers{
			Portal:         portalHandler,
			PortalAuth:     portalAuthHandler,
			PortalCustomer: portalCustomersHandler,
			ContactRole:    contactRolesHandler,
			Hub:            hubHandler,
			Form:           formHandler,
		},
		Assets: routes.AssetHandlers{
			Asset:    assetHandler,
			Type:     assetTypeHandler,
			Category: assetCategoryHandler,
			Status:   assetStatusHandler,
			Action:   assetActionHandler,
		},
		PublicBoard: publicBoardHandler,
		Collections: routes.CollectionHandlers{
			Category:     collectionCategoryHandler,
			Collection:   collectionHandler,
			BoardConfig:  boardConfigHandler,
			TestCoverage: testCoverageHandler,
		},
		AI: routes.AIHandlers{
			AI:            aiHandler,
			LLMConnection: llmConnHandler,
		},
		Misc: routes.MiscHandlers{
			Homepage:     homepageHandler,
			Review:       reviewHandler,
			CalendarFeed: calendarFeedHandler,
			CustomField:  customFieldHandler,
		},
		Teams: routes.TeamHandlers{
			Team:   teamHandler,
			Leave:  leaveHandler,
			OnCall: onCallHandler,
		},
		Integrations: routes.IntegrationHandlers{
			Provider:  integrationProviderHandler,
			OAuth:     integrationOAuthHandler,
			ItemLinks: integrationItemLinksHandler,
		},
	}
	routes.RegisterAll(routeDeps)

	// Register plugin routes
	if pluginRouter != nil {
		pluginRouter.RegisterRoutes(mux)
	}

	// REST API v1
	restapi.SetupRoutes(mux, s.db, tokenManager, permService, v1.RegisterRoutes)

	// MCP Server (Model Context Protocol) — opt-in via --mcp or MCP_ENABLED=true
	if cfg.MCPEnabled {
		mcpServer := mcpserver.NewMCPServer(mcpserver.Deps{
			DB:                    s.db,
			TokenManager:          tokenManager,
			PermissionService:     permService,
			TimePermissionService: timePermissionService,
			CommentService:        commentService,
		})
		mux.Handle("GET /mcp", mcpServer.Handler())
		mux.Handle("POST /mcp", mcpServer.Handler())
		mux.Handle("DELETE /mcp", mcpServer.Handler())
		slog.Info("MCP server enabled", "path", "/mcp")
	}

	// Frontend files
	if cfg.FrontendFiles != (embed.FS{}) {
		distFS, err := fs.Sub(cfg.FrontendFiles, "frontend/dist")
		if err != nil {
			slog.Warn("frontend files not found, serving API only")
		} else {
			fileServer := http.FileServer(http.FS(distFS))

			mux.Handle("GET /remoteEntry.js", fileServer)
			mux.Handle("GET /_app/", fileServer)
			mux.Handle("GET /windshift-3.svg", fileServer)
			mux.Handle("GET /forms/widget.js", fileServer)

			// Read index.html once at startup for nonce injection
			indexHTML, err := fs.ReadFile(distFS, "index.html")
			if err != nil {
				slog.Warn("could not read index.html from embedded FS", "error", err)
			}

			mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
					http.NotFound(w, r)
					return
				}
				if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/rest" {
					http.NotFound(w, r)
					return
				}
				if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/scim" {
					http.NotFound(w, r)
					return
				}

				if indexHTML == nil {
					http.NotFound(w, r)
					return
				}

				// Inject CSP nonce into the inline theme script tag
				nonce := CSPNonceFromContext(r.Context())
				html := bytes.Replace(indexHTML, []byte("<script>"), []byte(`<script nonce="`+nonce+`">`), 1)

				w.Header().Set("Content-Type", "text/html")
				http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(html))
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

func (s *Server) recoverUser(username string) {
	var id int
	var userEmail string
	var isActive bool
	err := s.db.QueryRow(
		`SELECT id, email, is_active FROM users WHERE username = ?`, username,
	).Scan(&id, &userEmail, &isActive)
	if err != nil {
		slog.Error("RECOVER_USER: user not found", "username", username)
		return
	}
	if isActive {
		slog.Info("RECOVER_USER: user is already active, no action needed", "username", username, "email", userEmail)
		return
	}
	_, err = s.db.Exec(`UPDATE users SET is_active = true, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		slog.Error("RECOVER_USER: failed to re-enable user", "username", username, "error", err)
		return
	}
	slog.Warn("RECOVER_USER: re-enabled disabled user", "username", username, "email", userEmail, "id", id)
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
	tcpAddr := listener.Addr().(*net.TCPAddr) //nolint:errcheck // Type assertion is safe; net.Listen("tcp", ...) always returns *net.TCPAddr
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
			defer func() { recover() }() //nolint:errcheck // Intentionally ignoring recover() return; used to suppress panics from closing already-closed channels
			close(ch)
		}
	}

	safeClose(s.scmSyncStopChan)
	s.scmSyncStopChan = nil

	safeClose(s.issueSyncStopChan)
	s.issueSyncStopChan = nil

	safeClose(s.magicLinkStopChan)
	s.magicLinkStopChan = nil

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

	if s.approvalEscalationSweeper != nil {
		slog.Info("stopping approval escalation sweeper")
		s.approvalEscalationSweeper.Stop()
	}

	if s.assetActionService != nil {
		slog.Info("stopping asset action service")
		s.assetActionService.Stop()
	}

	if s.emailScheduler != nil {
		slog.Info("stopping email scheduler")
		s.emailScheduler.Stop()
	}

	if s.briefingScheduler != nil {
		slog.Info("stopping briefing scheduler")
		s.briefingScheduler.Stop()
	}

	if s.notificationService != nil {
		slog.Info("stopping notification service")
		_ = s.notificationService.Close()
	}

	// Stop HTTP server
	if s.httpServer != nil {
		s.httpServer.SetKeepAlivesEnabled(false)
		slog.Info("shutting down HTTP server")
		if err := s.httpServer.Shutdown(ctx); err != nil {
			slog.Warn("HTTP server shutdown timed out, forcing close", "error", err)
			_ = s.httpServer.Close()
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
	if s.aiRateLimiter != nil {
		s.aiRateLimiter.Stop()
	}
	if s.uploadLimiter != nil {
		s.uploadLimiter.Stop()
	}
	if s.webhookLimiter != nil {
		s.webhookLimiter.Stop()
	}
	if s.searchLimiter != nil {
		s.searchLimiter.Stop()
	}
	if s.calendarFeedLimiter != nil {
		s.calendarFeedLimiter.Stop()
	}

	// Stop notification manager (flush cached notifications to DB)
	if s.notificationManager != nil {
		slog.Info("stopping notification manager")
		s.notificationManager.Stop()
	}

	// Close activity tracker
	if s.activityTracker != nil {
		_ = s.activityTracker.Close()
	}

	// Close token tracker
	if s.tokenTracker != nil {
		_ = s.tokenTracker.Close()
	}

	// Close database
	if s.db != nil {
		_ = s.db.Close()
	}
}

// BaseURL returns the server's base URL.
// deadcode-keep: called by core-tests/tests/helpers.go
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
// deadcode-keep: called by core-tests/tests/helpers.go
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

// runMagicLinkCleanup runs periodic cleanup of expired magic link tokens.
func (s *Server) runMagicLinkCleanup(magicLinkService *services.MagicLinkService) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	slog.Info("magic link cleanup scheduler started (1-hour interval)")
	for {
		select {
		case <-ticker.C:
			if err := magicLinkService.CleanupExpiredMagicLinks(); err != nil {
				slog.Error("magic link cleanup error", "error", err)
			}
		case <-s.magicLinkStopChan:
			slog.Info("magic link cleanup scheduler stopped")
			return
		}
	}
}

// runSCMRepoSync periodically walks every active repo and upserts PR/branch
// SCM links. Runs on its own ticker so the slower runSCMLinkRefresh below
// can't push a sync tick off the end of the deadline.
func (s *Server) runSCMRepoSync(scmSyncService *scm.SyncService) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	slog.Info("SCM repo sync scheduler started (5-minute interval)")
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			if err := scmSyncService.SyncAllRepositories(ctx); err != nil {
				slog.Error("SCM sync error", "error", err)
			}
			cancel()
		case <-s.scmSyncStopChan:
			slog.Info("SCM repo sync scheduler stopped")
			return
		}
	}
}

// runSCMLinkRefresh periodically re-reads the state of every non-merged PR
// link. Runs on a slower cadence than the repo-level sync because each
// link costs one provider round-trip, and a stale "merged" badge is far
// less critical than a missed link discovery.
func (s *Server) runSCMLinkRefresh(scmSyncService *scm.SyncService) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	slog.Info("SCM PR link refresh scheduler started (15-minute interval)")
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if err := scmSyncService.RefreshAllPRLinkStates(ctx); err != nil {
				slog.Error("PR state refresh error", "error", err)
			}
			cancel()
		case <-s.scmSyncStopChan:
			slog.Info("SCM PR link refresh scheduler stopped")
			return
		}
	}
}

// runSCMOAuthStateCleanup periodically deletes expired rows from
// scm_oauth_state. Postgres has a stored function defined for this but
// nothing in the code or schema schedules it; SQLite has a probabilistic
// AFTER INSERT trigger that fires on ~1% of inserts. A unified Go-side
// periodic covers both backends and bounds table growth on Postgres.
func (s *Server) runSCMOAuthStateCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	slog.Info("SCM OAuth state cleanup scheduler started (1-hour interval)")
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			res, err := s.db.ExecContext(ctx, `DELETE FROM scm_oauth_state WHERE expires_at < CURRENT_TIMESTAMP`)
			cancel()
			if err != nil {
				slog.Error("scm_oauth_state cleanup failed", slog.Any("error", err))
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				slog.Debug("scm_oauth_state cleanup", slog.Int64("deleted", n))
			}
		case <-s.scmSyncStopChan:
			slog.Info("SCM OAuth state cleanup scheduler stopped")
			return
		}
	}
}

// runIssueSync runs periodic GitHub Issue synchronization.
func (s *Server) runIssueSync(issueSyncService *scm.IssueSyncService) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	slog.Info("Issue sync scheduler started (5-minute interval)")
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			if err := issueSyncService.SyncAll(ctx); err != nil {
				slog.Error("Issue sync error", "error", err)
			}
			cancel()
		case <-s.issueSyncStopChan:
			slog.Info("Issue sync scheduler stopped")
			return
		}
	}
}
