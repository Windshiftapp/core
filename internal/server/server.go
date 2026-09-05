// Package server provides a reusable HTTP server for windshift.
// This allows the server to be started both from the main binary
// and in-process for integration tests.
package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"windshift/internal/aitools"
	"windshift/internal/auth"
	"windshift/internal/authz"
	"windshift/internal/config"
	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/emailutil"
	"windshift/internal/events"
	"windshift/internal/handlers"
	"windshift/internal/health"
	"windshift/internal/ldap"
	"windshift/internal/llm"
	"windshift/internal/logger"
	mcpserver "windshift/internal/mcp"
	appmetrics "windshift/internal/metrics"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/plugins"
	"windshift/internal/portalwebauthn"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	v1 "windshift/internal/restapi/v1"
	v2 "windshift/internal/restapi/v2"
	"windshift/internal/router"
	"windshift/internal/routes"
	"windshift/internal/scheduler"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/smtp"
	"windshift/internal/standardagent"
	"windshift/internal/utils"
	"windshift/internal/webauthn"
	"windshift/internal/webhook"
)

type planningReleaseProviderAdapter struct {
	provider scm.ReleaseProvider
}

func (a planningReleaseProviderAdapter) CreateRelease(ctx context.Context, owner, repo string, input services.ExternalReleaseOptions) (*services.ExternalRelease, error) {
	release, err := a.provider.CreateRelease(ctx, owner, repo, scm.CreateReleaseOptions{
		TagName: input.TagName, TargetCommitish: input.TargetCommitish, Name: input.Name,
		Body: input.Body, IsDraft: input.IsDraft, IsPrerelease: input.IsPrerelease,
	})
	if err != nil {
		return nil, err
	}
	return &services.ExternalRelease{ID: release.ID, URL: release.URL, TagName: release.TagName}, nil
}

func (a planningReleaseProviderAdapter) ListReleases(ctx context.Context, owner, repo string) ([]services.ExternalRelease, error) {
	releases, err := a.provider.ListReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	result := make([]services.ExternalRelease, len(releases))
	for i := range releases {
		result[i] = services.ExternalRelease{ID: releases[i].ID, URL: releases[i].URL, TagName: releases[i].TagName}
	}
	return result, nil
}

// Config is an alias to config.Config — the canonical, fully-resolved runtime
// configuration. All resolution of env vars and CLI flags happens in
// internal/config/Load; this package only consumes the result.
type Config = config.Config

const (
	databasePoolBudgetWarningPercent = 90
	maxRequestHeaderValueCount       = 128
)

// Server represents a windshift HTTP server instance.
type Server struct {
	config     Config
	httpServer *http.Server
	db         database.Database
	listener   net.Listener

	ldapHandler                  *handlers.LDAPHandler
	notificationManager          *handlers.NotificationManager
	notificationService          *services.NotificationService
	notificationScheduler        *scheduler.NotificationScheduler
	recurrenceScheduler          *scheduler.RecurrenceScheduler
	cfvCleanupScheduler          *scheduler.CFVCleanupScheduler
	todoistSyncScheduler         *scheduler.TodoistSyncScheduler
	runnerLeaseReaper            *scheduler.RunnerLeaseReaper
	globalRankMigrationScheduler *scheduler.GlobalRankMigrationScheduler
	codingRunService             *services.RunService
	standardAgentDispatcher      *standardagent.Dispatcher
	workflowService              *services.WorkflowService
	actionService                *services.ActionService
	assetActionService           *services.AssetActionService
	eventEngine                  *events.Engine
	approvalEscalationSweeper    *services.ApprovalEscalationSweeper
	emailScheduler               *scheduler.EmailScheduler
	emailTrackingRetention       *scheduler.EmailTrackingRetentionSweeper
	briefingScheduler            *scheduler.BriefingScheduler
	pluginScheduleScheduler      *scheduler.PluginScheduleScheduler
	activityTracker              *services.ActivityTracker
	tokenTracker                 *services.TokenTracker
	webhookSender                *webhook.WebhookSender
	scmSyncStopChan              chan struct{}
	issueSyncStopChan            chan struct{}
	magicLinkStopChan            chan struct{}
	cleanupStopChan              chan struct{}
	jiraHostStopChan             chan struct{}
	cleanupTicker                *time.Ticker
	pluginManager                *plugins.Manager
	databaseDiagRepo             *repository.DatabaseDiagnosticsRepository
	databasePoolMonitor          *services.DatabasePoolMonitor
	channelService               *services.ChannelService
	memoryBudget                 config.MemoryBudget
	metrics                      *appmetrics.Metrics

	loginRateLimiter      *middleware.RateLimiter
	runnerRegisterLimiter *middleware.RateLimiter
	fidoRateLimiter       *middleware.RateLimiter
	authRateLimiter       *middleware.RateLimiter
	scimRateLimiter       *middleware.RateLimiter
	portalSubmitLimiter   *middleware.RateLimiter
	portalSearchLimiter   *middleware.RateLimiter
	emailVerifyLimiter    *middleware.RateLimiter
	setupLimiter          *middleware.RateLimiter
	ssoRateLimiter        *middleware.RateLimiter
	portalAuthLimiter     *middleware.RateLimiter
	oauthTokenLimiter     *middleware.RateLimiter
	aiRateLimiter         *middleware.RateLimiter
	uploadLimiter         *middleware.RateLimiter
	webhookLimiter        *middleware.RateLimiter
	searchLimiter         *middleware.RateLimiter
	calendarFeedLimiter   *middleware.RateLimiter
	publicBoardLimiter    *middleware.RateLimiter
	userConcurrency       *middleware.UserConcurrencyLimiter

	actualPort   int
	started      bool
	shuttingDown bool
}

// New creates a new Server instance with the given configuration.
// It initializes all services and handlers but does not start listening.
func New(cfg Config) (*Server, error) {
	memoryBudget, err := config.ResolveMemoryBudget(cfg.Memory.LimitMB)
	if err != nil {
		return nil, fmt.Errorf("resolve memory budget: %w", err)
	}
	s := &Server{
		config:            cfg,
		scmSyncStopChan:   make(chan struct{}),
		issueSyncStopChan: make(chan struct{}),
		magicLinkStopChan: make(chan struct{}),
		cleanupStopChan:   make(chan struct{}),
		jiraHostStopChan:  make(chan struct{}),
		memoryBudget:      memoryBudget,
	}

	if err := s.initialize(); err != nil {
		s.cleanup()
		return nil, err
	}

	return s, nil
}

// initialize sets up all services and handlers.
func (s *Server) initialize() error {
	// FIXME: split initialization into focused builders and lifecycle registries.
	cfg := s.config
	utils.SetSkipTLSVerify(cfg.OutboundTLS.SkipVerify)
	if cfg.OutboundTLS.SkipVerify {
		slog.Warn("outbound TLS certificate verification is disabled; self-signed certificates will be accepted without server identity verification")
	}

	if cfg.SilentMode {
		logger.SetSilent(true)
	}

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

	s.databaseDiagRepo = repository.NewDatabaseDiagnosticsRepository(s.db)
	if cfg.DB.PostgresConn != "" {
		replicas := cfg.DB.ReplicaCount
		if replicas <= 0 {
			slog.Warn("invalid PostgreSQL replica count; capacity budget assumes one replica", "configured_replica_count", replicas)
			replicas = 1
		}
		headroom := cfg.DB.ConnectionHeadroom
		if headroom < 0 {
			slog.Warn("invalid PostgreSQL connection headroom; capacity budget assumes zero", "configured_headroom", headroom)
			headroom = 0
		}
		auxiliaryConnections := 0
		if cfg.SSH.Enabled {
			auxiliaryConnections = config.SSHDatabaseMaxConnections
		}
		budgetCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		budget, budgetErr := s.databaseDiagRepo.LoadPostgresCapacityBudget(
			budgetCtx,
			s.db,
			replicas,
			headroom,
			auxiliaryConnections,
		)
		cancel()
		if budgetErr != nil {
			slog.Warn("unable to evaluate PostgreSQL connection capacity budget", "error", budgetErr)
		} else {
			logDatabaseCapacityBudget(budget)
		}
	}
	s.databasePoolMonitor = services.NewDatabasePoolMonitor(s.databaseDiagRepo, services.DefaultDatabasePoolMonitorConfig())

	if err = s.db.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	if err = database.ValidateCanonicalSchemaCheckpoint(s.db); err != nil {
		return fmt.Errorf("database startup refused: %w", err)
	}
	objectTranslationService := objecttranslation.NewService(s.db)
	if err := objectTranslationService.SyncSystem(context.Background(), objecttranslation.ShippedSystemTranslations()); err != nil {
		return fmt.Errorf("sync shipped object translations: %w", err)
	}
	s.eventEngine = events.NewEngine(s.db, events.DefaultConfig())

	if err = emailutil.SeedTemplates(s.db); err != nil {
		slog.Warn("failed to seed default email templates", "error", err)
	}

	if err = repository.NewNotificationSettingsRepository(s.db).EnsureDefault(); err != nil {
		slog.Warn("failed to ensure notification settings", "error", err)
	}

	if cfg.RecoverUser != "" {
		s.recoverUser(cfg.RecoverUser)
	}

	setupCompleted, err := checkSetupStatusWithRetry(s.db, 5, time.Second)
	if err != nil {
		return fmt.Errorf("failed to determine setup status: %w", err)
	}

	permService, err := services.NewPermissionService(s.db, services.PermissionCacheConfig{
		TTL:          15 * time.Minute,
		MaxCacheSize: s.memoryBudget.PermissionCacheMB,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize permission service: %w", err)
	}

	// Shared channel service used by ChannelHandler, WebhookHandler,
	// FormHandler, RequestTypeHandler, and AssetReportHandler for the
	// "user manages channel C" gate.
	channelService := services.NewChannelService(s.db, permService)
	s.channelService = channelService

	activityConfig := services.DefaultActivityTrackerConfig()
	activityConfig.MaxCacheSize = s.memoryBudget.ActivityCacheMB
	s.activityTracker, err = services.NewActivityTracker(s.db, activityConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize activity tracker: %w", err)
	}

	s.cleanupTicker = time.NewTicker(24 * time.Hour)
	go s.runActivityCleanup()

	enableHTTPS := cfg.TLSCertPath != "" && cfg.TLSKeyPath != ""

	var additionalProxyList []string
	if cfg.AdditionalProxies != "" {
		additionalProxyList = strings.Split(cfg.AdditionalProxies, ",")
	}

	ipExtractor := utils.NewIPExtractor(cfg.UseProxy, additionalProxyList)

	primarySessionCacheMB, _ := config.SplitSSHCacheBudget(s.memoryBudget.SessionCacheMB, cfg.SSH.Enabled)
	sessionManager := auth.NewSessionManagerWithValidationCacheTTL(
		s.db,
		enableHTTPS,
		cfg.UseProxy,
		additionalProxyList,
		cfg.Auth.SessionSecret,
		cfg.Auth.SessionIPBinding,
		cfg.Auth.SessionValidationCacheTTL,
		primarySessionCacheMB,
	)

	effectivePort := cfg.Port
	if cfg.AllowedPort != "" {
		effectivePort = cfg.AllowedPort
	}

	// WebAuthn settings are resolved by config.Load; development may override RPID.
	isDevelopment := cfg.DisableCSRF
	webAuthnConfig, portalWebAuthnConfig, err := initializeWebAuthnConfigs(cfg, isDevelopment, effectivePort, enableHTTPS)
	if err != nil {
		return err
	}

	var userKeyedOpts []middleware.RateLimiterOption
	userKeyedOpts = append(userKeyedOpts, middleware.WithUserKeyed())
	if cfg.DisableIPRateLimit {
		userKeyedOpts = append(userKeyedOpts, middleware.WithDisableIPLimit())
	}

	s.loginRateLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.runnerRegisterLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.fidoRateLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.scimRateLimiter = middleware.NewRateLimiter(10.0, 100, cfg.UseProxy, additionalProxyList)
	s.portalSubmitLimiter = middleware.NewRateLimiter(5.0/60.0, 10, cfg.UseProxy, additionalProxyList)
	s.portalSearchLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.emailVerifyLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	s.setupLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList)
	s.ssoRateLimiter = middleware.NewRateLimiter(10.0/60.0, 5, cfg.UseProxy, additionalProxyList)
	s.portalAuthLimiter = middleware.NewRateLimiter(3.0/60.0, 3, cfg.UseProxy, additionalProxyList)
	s.calendarFeedLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList)
	// Public boards are anonymous and can trigger substantial query and file IO.
	// Keep this limiter IP-keyed even when authenticated IP limiting is disabled.
	s.publicBoardLimiter = middleware.NewRateLimiter(30.0/60.0, 60, cfg.UseProxy, additionalProxyList)
	// OAuth /token is unauthenticated (server-to-server), so it must stay
	// IP-keyed and must NOT honor DisableIPRateLimit — otherwise enabling that
	// flag for NAT deployments would silently remove all brute-force protection
	// on client_secret/code guessing. Kept separate from the user-keyed
	// authRateLimiter for exactly this reason.
	s.oauthTokenLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList)
	s.authRateLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.aiRateLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.uploadLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.webhookLimiter = middleware.NewRateLimiter(10.0/60.0, 15, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	s.searchLimiter = middleware.NewRateLimiter(20.0/60.0, 30, cfg.UseProxy, additionalProxyList, userKeyedOpts...)
	// Per-user in-flight concurrency cap for the whole /api surface — bounds how
	// many shared DB-pool connections one user can hold so a runaway client
	// can't starve the others. Applied to the api group below.
	s.userConcurrency = middleware.NewUserConcurrencyLimiter(cfg.MaxUserConcurrency)

	if cfg.MaxTemplateSeedItems > 0 {
		services.MaxTemplateSeedItems = cfg.MaxTemplateSeedItems
	}

	s.tokenTracker = services.NewTokenTracker(s.db, services.DefaultTokenTrackerConfig())

	apiTokenCacheMB, _ := config.SplitSSHCacheBudget(s.memoryBudget.APITokenCacheMB, cfg.SSH.Enabled)
	tokenManager := auth.NewTokenManager(s.db, s.tokenTracker, apiTokenCacheMB)
	if cleaned, cleanupErr := tokenManager.CleanupExpiredTokens(); cleanupErr != nil {
		slog.Warn("failed to cleanup expired api tokens on startup", "error", cleanupErr)
	} else if cleaned > 0 {
		slog.Info("cleaned expired api tokens on startup", "count", cleaned)
	}
	userDeactivationService := services.NewUserDeactivationService(
		s.db,
		services.UserDeactivationInvalidators{
			Tokens:   tokenManager.InvalidateTokens,
			Sessions: sessionManager.InvalidateUserSessionValidation,
			Permissions: func(userID int) {
				if err := permService.InvalidateUserCache(userID); err != nil {
					slog.Warn("failed to invalidate permissions after user deactivation",
						slog.Int("user_id", userID),
						slog.Any("error", err))
				}
			},
		},
	)

	authMiddleware := middleware.NewAuthMiddleware(sessionManager, tokenManager, s.db, cfg.UseProxy, additionalProxyList, setupCompleted)

	var additionalProxyIPs []net.IP
	for _, proxyStr := range additionalProxyList {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalProxyIPs = append(additionalProxyIPs, ip)
		}
	}

	mux := http.NewServeMux()
	healthHandler := health.NewHandler(s.db.GetDB())
	mux.HandleFunc("GET /healthz", healthHandler.Liveness)
	mux.HandleFunc("GET /readyz", healthHandler.Readiness)
	s.metrics = appmetrics.New(s.db)
	mux.Handle("GET /metrics", s.metrics.Handler())

	nmCfg := handlers.DefaultNotificationManagerConfig()
	nmCfg.MaxCacheSize = s.memoryBudget.NotificationCacheMB
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
	notificationAssetPermissions := services.NewAssetPermissionService(repository.NewAssetRepository(s.db), permService)
	notificationAuthorizer := services.NewNotificationAuthorizer(s.db, permService, notificationAssetPermissions)

	s.notificationService = services.NewNotificationService(
		s.db,
		s.notificationManager,
		permService,
		services.DefaultNotificationServiceConfig(),
		notificationAssetPermissions,
	)
	if err := services.PrepareDurableNotificationEngine(context.Background(), s.eventEngine, s.notificationService); err != nil {
		return fmt.Errorf("prepare durable notification consumers: %w", err)
	}

	smtpSender := smtp.NewNotificationSMTPSender(s.db)
	s.notificationScheduler = scheduler.NewNotificationScheduler(s.db, smtpSender, cfg.Notification.BatchInterval, s.notificationService)
	s.notificationScheduler.Start()
	slog.Info("notification scheduler started")

	// WorkflowService is constructed here (moved up from later in bootstrap) so the
	// recurrence scheduler can resolve a workspace+item-type's initial status the
	// same way the rest of the system does. The handler-side instance below reuses
	// the same pointer, so the in-memory cache is shared.
	s.workflowService = services.NewWorkflowService(s.db)
	s.recurrenceScheduler = scheduler.NewRecurrenceScheduler(s.db, s.workflowService)
	s.recurrenceScheduler.Start()

	// Drains pending_custom_field_cleanups: when a custom field is
	// deleted, items' cfv JSON still carries the deleted key. This
	// scheduler scrubs them in batches so the Delete request returns
	// immediately even when the workspace has millions of items.
	s.cfvCleanupScheduler = scheduler.NewCFVCleanupScheduler(s.db)
	s.cfvCleanupScheduler.Start()
	// Liveness backstop for remote agent runs (WI-141): fail runs whose
	// runner's heartbeat went stale and revoke the dead runner instances.
	s.runnerLeaseReaper = scheduler.NewRunnerLeaseReaper(
		repository.NewAgentRunRepository(s.db),
		repository.NewRunnerRepository(s.db),
	)
	s.runnerLeaseReaper.Start()
	globalRankHostname, hostnameErr := os.Hostname()
	if hostnameErr != nil || globalRankHostname == "" {
		globalRankHostname = "unknown-host"
	}
	globalRankOwner := fmt.Sprintf("global-rank-%s-%d", globalRankHostname, os.Getpid())
	s.globalRankMigrationScheduler = scheduler.NewGlobalRankMigrationScheduler(s.db, globalRankOwner)
	s.globalRankMigrationScheduler.Start()
	slog.Info("global rank migration scheduler started", "owner", globalRankOwner)
	slog.Info("recurrence scheduler started")

	chainStore := services.NewExecutionChainStore()

	s.actionService = services.NewActionService(s.db, services.DefaultActionServiceConfig(), chainStore)
	s.actionService.SetNotificationService(s.notificationService)
	s.actionService.SetPermissionService(permService)
	if err := services.PrepareDurableActionEngine(context.Background(), s.eventEngine, s.actionService, cfg.ActivateDurableActions); err != nil {
		return fmt.Errorf("prepare durable action consumers: %w", err)
	}
	slog.Info("action service initialized")

	s.assetActionService = services.NewAssetActionService(s.db, services.DefaultActionServiceConfig(), chainStore)
	s.assetActionService.SetNotificationService(s.notificationService)
	s.assetActionService.SetPermissionService(permService)
	if err := services.PrepareDurableAssetActionEngine(context.Background(), s.eventEngine, s.assetActionService, cfg.ActivateDurableAssetActions); err != nil {
		return fmt.Errorf("prepare durable asset action consumers: %w", err)
	}
	slog.Info("asset action service initialized")

	// Determine base URL — cfg.BaseURL is already resolved by config.Load
	// from the --base-url flag or BASE_URL env; only the localhost fallback
	// remains here because it needs cfg.Port.
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s%s", cfg.Port, cfg.ContextPath)
	}

	emailVerificationService := services.NewEmailVerificationService(s.db, smtpSender, baseURL)

	portalSessionManager := auth.NewPortalSessionManager(s.db, enableHTTPS, cfg.UseProxy, additionalProxyList, cfg.Auth.SessionSecret, cfg.Auth.SessionIPBinding)

	magicLinkService := services.NewMagicLinkService(s.db, smtpSender, baseURL)

	invitationService := services.NewInvitationService(s.db, smtpSender, baseURL)

	workspaceKeyCache := handlers.NewWorkspaceKeyCache(repository.NewWorkspaceRepository(s.db))
	authorizationCacheInvalidator := services.NewAuthorizationCacheInvalidator(permService, workspaceKeyCache)

	transitionMatrixService := services.NewTransitionMatrixService(s.db)
	bulkOperationMetrics := services.NewBulkOperationMetrics()
	itemHandler := handlers.NewItemHandler(s.db, permService, s.activityTracker, s.notificationService, s.memoryBudget.ItemCacheMB)
	itemHandler.SetDBRequestTimeout(s.config.DB.RequestTimeout)
	customFieldHandler := handlers.NewCustomFieldHandler(s.db)
	workspaceHandler := handlers.NewWorkspaceHandler(s.db, permService, s.activityTracker, workspaceKeyCache, authorizationCacheInvalidator)
	screenHandler := handlers.NewScreenHandler(s.db).WithObjectTranslations(objectTranslationService)
	configSetHandler := handlers.NewConfigurationSetHandler(s.db, s.notificationService, permService).WithObjectTranslations(objectTranslationService)

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

	hierarchyLevelConfig := services.NewHierarchyLevelConfig()
	hierarchyLevelConfig.AuditEmit = enumAuditEmit
	hierarchyLevelHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, hierarchyLevelConfig),
		func() any { return &models.HierarchyLevel{} }).WithObjectTranslations(objectTranslationService, "hierarchy_level")
	requestTypeHandler := handlers.NewRequestTypeHandler(
		repository.NewRequestTypeRepository(s.db),
		repository.NewChannelRepository(s.db),
		repository.NewScreenRepository(s.db),
		repository.NewItemTypeRepository(s.db),
		logger.NewAuditor(s.db),
		channelService,
	)
	workflowService := s.workflowService
	userHandler := handlers.NewUserHandler(
		repository.NewUserRepository(s.db),
		logger.NewAuditor(s.db),
		permService,
		invitationService,
		services.NewUserReadService(s.db),
		func(id int) error {
			tokenIDs, err := services.OffboardUser(s.db, id, s.notificationService, authorizationCacheInvalidator)
			tokenManager.InvalidateTokens(tokenIDs)
			sessionManager.InvalidateUserSessionValidation(id)
			return err
		},
		userDeactivationService.DeactivateUser,
		sessionManager.InvalidateUserSessionValidation,
	)
	groupHandler := handlers.NewGroupHandler(repository.NewGroupRepository(s.db), permService, logger.NewAuditor(s.db), authorizationCacheInvalidator)
	credentialHandler := handlers.NewCredentialHandler(repository.NewCredentialRepository(s.db), logger.NewAuditor(s.db), permService, cfg.SSH.Enabled)
	var webAuthnHandler *handlers.WebAuthnHandler
	if webAuthnConfig != nil {
		webAuthnHandler = handlers.NewWebAuthnHandler(s.db, permService, sessionManager, webAuthnConfig, ipExtractor)
	}
	publicBoardHandler := handlers.NewPublicBoardHandler(s.db, permService, cfg.AttachmentPath)
	permissionHandler := handlers.NewPermissionHandlerWithCache(repository.NewPermissionRepository(s.db), permService, logger.NewAuditor(s.db))
	apiTokenHandler := handlers.NewAPITokenHandler(
		tokenManager,
		repository.NewAPITokenPolicyRepository(s.db),
		repository.NewWorkspaceRepository(s.db),
		logger.NewAuditor(s.db),
		permService,
	)
	agentHandler := handlers.NewAgentHandler(s.db, permService)

	scimTokenManager := auth.NewSCIMTokenManager(s.db, s.memoryBudget.SCIMTokenCacheMB)
	scimAuthMiddleware := middleware.NewSCIMAuthMiddleware(scimTokenManager)
	scimHandler := handlers.NewSCIMHandler(
		repository.NewSCIMRepository(s.db),
		baseURL,
		permService,
		logger.NewAuditor(s.db),
		userDeactivationService.DeactivateUser,
		func() ([]int, error) {
			return services.ActiveSystemAdminIDs(s.db)
		},
		s.notificationService,
		authorizationCacheInvalidator,
	)
	scimTokenHandler := handlers.NewSCIMTokenHandler(scimTokenManager, logger.NewAuditor(s.db))

	permissionSetHandler := handlers.NewPermissionSetHandlerWithPool(repository.NewPermissionSetRepository(s.db), permService, logger.NewAuditor(s.db))
	workspaceRoleHandler := handlers.NewWorkspaceRoleHandlerWithPool(repository.NewWorkspaceRoleRepository(s.db), permService, logger.NewAuditor(s.db)).WithObjectTranslations(objectTranslationService)

	timePermissionService := services.NewTimePermissionService(s.db, permService)
	customerOrgPermissionService := services.NewCustomerOrganisationPermissionService(s.db, permService, timePermissionService)
	timeCustomerHandler := handlers.NewTimeCustomerHandler(repository.NewCustomerOrganisationRepository(s.db), logger.NewAuditor(s.db), timePermissionService, customerOrgPermissionService)
	timeProjectHandler := handlers.NewTimeProjectHandler(s.db, timePermissionService, customerOrgPermissionService)
	timeWorklogService := services.NewTimeWorklogService(s.db)
	activeTimerRepo := repository.NewActiveTimerRepository(s.db)
	timerService := services.NewTimerService(activeTimerRepo, repository.NewItemRepository(s.db), timePermissionService, permService)
	customerOrgPermissionHandler := handlers.NewCustomerOrganisationPermissionHandler(logger.NewAuditor(s.db), customerOrgPermissionService)

	itemLinkService := services.NewItemLinkService(s.db).
		WithPermissionService(permService).
		WithNotificationEmitter(s.notificationService)

	pageLabelRepo := repository.NewPageLabelRepository(s.db)
	pageService := services.NewPageService(s.db)
	pageService.SetPageLabelRepository(pageLabelRepo)
	pagePermissionService := services.NewPagePermissionService(s.db, permService)
	itemLinkService.WithPagePermissionChecker(pagePermissionService)
	pageApplication := services.NewPageApplicationService(pageService, pagePermissionService)
	pageDiagramService := services.NewPageDiagramService(
		s.db,
		cfg.AttachmentPath,
		pageApplication,
		pagePermissionService,
		permService,
	)
	knowledgeRetrieval := services.NewKnowledgeRetrievalService(s.db, pagePermissionService)
	knowledgeSearchHandler := handlers.NewKnowledgeSearchHandler(knowledgeRetrieval)
	pageLabelService := services.NewPageLabelService(pageLabelRepo, logger.NewAuditor(s.db))

	recurrenceService := services.NewRecurrenceService(repository.NewRecurrenceRepository(s.db), s.recurrenceScheduler, logger.NewAuditor(s.db))

	actionsHandler := handlers.NewActionsHandler(
		repository.NewActionRepository(s.db),
		repository.NewActionCredentialRepository(s.db),
		logger.NewAuditor(s.db),
		workspaceKeyCache,
	)
	actionCredentialService := services.NewActionCredentialService(repository.NewActionCredentialRepository(s.db), cfg.Auth.SessionSecret)
	actionCredentialsHandler := handlers.NewActionCredentialsHandler(actionCredentialService, permService, workspaceKeyCache, logger.NewAuditor(s.db))
	// Wire credential resolution into the action runtime so HTTP capabilities
	// can reference tokens by ID. The service shares the same SSO_SECRET via
	// a domain-separated HKDF label (ActionCredentialEncryptionInfo).
	credentialSvc := services.NewActionCredentialService(
		repository.NewActionCredentialRepository(s.db),
		cfg.Auth.SessionSecret,
	)
	s.actionService.SetCredentialService(credentialSvc)
	// Lets container_run nodes dispatch to a remote runner pool (WI-146).
	s.actionService.SetAgentRunRepository(repository.NewAgentRunRepository(s.db))
	// One-shot scanner: warn about any legacy capability whose
	// default_headers still holds a sensitive header value. The scanner logs
	// capability ID + header name only — never the value.
	services.ScanLegacyInlineSecrets(s.db)

	// Team handlers
	teamRepo := repository.NewTeamRepository(s.db)
	leaveRepo := repository.NewLeaveRepository(s.db)
	onCallRepo := repository.NewOnCallRepository(s.db)
	teamService := services.NewTeamService(s.db, teamRepo, leaveRepo)
	onCallService := services.NewOnCallService(s.db, onCallRepo, leaveRepo)
	teamHandler := handlers.NewTeamHandler(teamRepo, leaveRepo, permService, logger.NewAuditor(s.db))
	leaveHandler := handlers.NewLeaveHandler(leaveRepo, repository.NewUserRepository(s.db), permService)
	onCallHandler := handlers.NewOnCallHandler(onCallRepo, teamRepo, onCallService, permService, logger.NewAuditor(s.db))
	s.actionService.SetTeamService(teamService)

	milestoneCategoryConfig := services.NewMilestoneCategoryConfig()
	milestoneCategoryConfig.AuditEmit = enumAuditEmit
	milestoneCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, milestoneCategoryConfig),
		func() any { return &models.MilestoneCategory{} }).WithGlobalMutationPermission(permService, models.PermissionMilestoneCreate)
	channelCategoryConfig := services.NewChannelCategoryConfig()
	channelCategoryConfig.AuditEmit = enumAuditEmit
	channelCategoryHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, channelCategoryConfig),
		func() any { return &models.ChannelCategory{} })
	iterationTypeConfig := services.NewIterationTypeConfig()
	iterationTypeConfig.AuditEmit = enumAuditEmit
	iterationTypeHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, iterationTypeConfig),
		func() any { return &models.IterationType{} }).WithGlobalMutationPermission(permService, models.PermissionIterationManage)
	personalLabelHandler := handlers.NewPersonalLabelHandler(s.db, permService)
	reviewHandler := handlers.NewReviewHandler(s.db, permService)
	calendarFeedHandler := handlers.NewCalendarFeedHandler(s.db, permService, cfg.BaseURL)
	securitySettingsHandler := handlers.NewSecuritySettingsHandler(repository.NewSystemSettingRepository(s.db), logger.NewAuditor(s.db), cfg.Plugins.Disabled)

	// WI-87/88/89/90 coding-agent harness stack lands later in the
	// constructor — see the block right after the SCM handlers are
	// built, since scm.CredentialResolver needs scmProviderHandler.GetEncryption().

	var adminRateLimiter *middleware.AdminFallbackRateLimiter
	if cfg.EnableAdminFallback {
		adminRateLimiter = middleware.NewAdminFallbackRateLimiter(s.db)
		slog.Info("Admin password fallback enabled", slog.String("component", "auth"))
	}

	authPolicyHandler := handlers.NewAuthPolicyHandlerWithFallback(s.db, cfg.EnableAdminFallback, logger.NewAuditor(s.db))
	if webAuthnHandler != nil {
		webAuthnHandler.SetAuthPolicyHandler(authPolicyHandler)
	}

	authHandler := handlers.NewAuthHandler(
		repository.NewUserRepository(s.db),
		repository.NewCredentialRepository(s.db),
		logger.NewAuditor(s.db),
		sessionManager,
		s.loginRateLimiter,
		permService,
		emailVerificationService,
		ipExtractor,
		authPolicyHandler,
		adminRateLimiter,
	)

	invitationHandler := handlers.NewInvitationHandler(invitationService)

	themeHandler := handlers.NewThemeHandler(services.NewThemeService(repository.NewThemeRepository(s.db)), logger.NewAuditor(s.db)).WithObjectTranslations(objectTranslationService)
	objectTranslationHandler := handlers.NewObjectTranslationHandler(objectTranslationService)
	userPreferencesService := services.NewUserPreferencesService(repository.NewUserPreferencesRepository(s.db), repository.NewThemeRepository(s.db), permService)
	userPreferencesHandler := handlers.NewUserPreferencesHandler(userPreferencesService)
	homepageHandler := handlers.NewHomepageHandler(
		repository.NewWorkspaceRepository(s.db),
		repository.NewItemRepository(s.db),
		services.NewItemCRUDService(s.db),
		services.NewPlanningService(s.db),
		s.activityTracker,
		permService,
		userPreferencesService,
	)

	notificationHandler := handlers.NewNotificationHandler(s.notificationManager, s.notificationService, permService)
	notificationHandler.SetNotificationAuthorizer(notificationAuthorizer)
	emailTemplateHandler := handlers.NewEmailTemplateHandler(repository.NewEmailTemplateRepository(s.db), logger.NewAuditor(s.db))

	// Push dispatches every notification; VAPID config resolves env, persisted,
	// then generated keys.
	pushCfg := services.ResolveVAPIDConfig(s.db, cfg.Push, slog.Default())
	pushService := services.NewPushService(s.db, pushCfg, permService)
	pushService.SetNotificationAuthorizer(notificationAuthorizer)
	pushHandler := handlers.NewPushHandler(pushService)
	s.notificationManager.SetPushDispatcher(pushService)
	if pushService.Enabled() {
		slog.Info("Web Push enabled")
	}

	permissionMiddleware := middleware.NewPermissionMiddleware(s.db, permService)

	setupHandler := handlers.NewSetupHandler(s.db, sessionManager, authMiddleware)

	ssoHandler := handlers.NewSSOHandler(s.db, sessionManager, permService, emailVerificationService, s.pluginManager, cfg.Auth.SessionSecret, baseURL, cfg.AllowedHosts, cfg.DisableCSRF, ipExtractor, cfg.UseProxy, additionalProxyList)

	scmProviderHandler := handlers.NewSCMProviderHandler(s.db, cfg.Auth.SessionSecret, baseURL)
	scmWorkspaceRepo := repository.NewSCMWorkspaceRepository(s.db)
	scmWorkspaceHandler := handlers.NewSCMWorkspaceHandler(scmWorkspaceRepo, scmProviderHandler.GetEncryption(), scmProviderHandler, scm.NewCredentialResolver(s.db, scmProviderHandler.GetEncryption()), permService, baseURL)
	scmItemLinksHandler := handlers.NewSCMItemLinksHandler(s.db, scmProviderHandler.GetEncryption(), permService)
	userSCMTokenHandler := handlers.NewUserSCMTokenHandler(repository.NewUserSCMTokenRepository(s.db), scmProviderHandler.GetEncryption())
	milestonePlanningService := services.NewPlanningService(s.db)
	milestonePlanningService.SetSCMWorkspaceRepository(scmWorkspaceRepo)

	// The optional coding-agent harness queues and finalizes remote runner-pool
	// work; disabled mode retains bindings without starting runs.
	agentSecurityRepo := repository.NewAgentSecurityRepository(s.db)
	agentIdentitySvc, _ := services.NewAgentActingIdentityService(services.NewUserReadService(s.db), agentSecurityRepo)
	agentBindingRepo := repository.NewWorkspaceAgentBindingRepository(s.db)
	scmCredResolver := scm.NewCredentialResolver(s.db, scmProviderHandler.GetEncryption())

	// AI handlers and agents share embedded or configured prompt overrides.
	promptStore := llm.NewPromptStore(cfg.LLM.PromptsDir)
	// System-admin-overridable Agent Studio catalog (WI-922): configured rows
	// overlay or disable embedded defaults.
	agentTemplateCatalogRepo := repository.NewAgentTemplateCatalogRepository(s.db)
	templateCatalog := llm.NewTemplateCatalog(promptStore, agentTemplateCatalogRepo)
	agentTemplateCatalogHandler := handlers.NewAdminAgentTemplateCatalogHandler(agentTemplateCatalogRepo, permService, logger.NewAuditor(s.db))
	agentTemplateCatalogHandler.SetDefaults(promptStore)

	// Bindings and AI handlers share the provider registry.
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

	fallbackLLMClient := llm.NewClient(llm.Config{Endpoint: cfg.LLM.Endpoint})
	if fallbackLLMClient.Available() {
		slog.Info("LLM fallback service configured", slog.String("endpoint", cfg.LLM.Endpoint))
	} else {
		slog.Info("LLM fallback service not configured")
	}
	llmManager := llm.NewConnectionManager(s.db, scmProviderHandler.GetEncryption(), fallbackLLMClient)
	llmModelCache := llm.NewModelCache(s.db)
	llmManager.SetModelCache(llmModelCache) // freshest vision-capability resolution
	llmModelRefresher := llm.NewModelRefresher(llmModelCache)

	var codingRunSvc *services.RunService
	if cfg.CodingAgent.Enabled {
		var bootErr error
		codingRunSvc, bootErr = bootCodingAgentRunService(s.db, tokenManager, agentBindingRepo, scmCredResolver, promptStore.Get(llm.PromptCodingAgentInitial))
		if bootErr != nil {
			slog.Warn("coding-agent harness disabled: failed to construct RunService",
				slog.String("component", "coding-agent"),
				slog.Any("error", bootErr),
			)
		}
	}
	// Retain the service so shutdown can drain local runs.
	s.codingRunService = codingRunSvc

	agentAPIURL := cfg.CodingAgent.WSAPIURL
	if agentAPIURL == "" {
		// Agent broker URLs require the API suffix, not the SPA base URL.
		agentAPIURL = strings.TrimRight(baseURL, "/") + "/api"
	}
	agentSkillRepo := repository.NewWorkspaceAgentSkillRepository(s.db)
	standardCapabilityGroups := aitools.StandardCapabilityGroups(aitools.Default)
	standardCapabilityKeys := make([]string, 0, len(standardCapabilityGroups))
	for _, group := range standardCapabilityGroups {
		standardCapabilityKeys = append(standardCapabilityKeys, string(group.Key))
	}
	bindingSvc, _ := services.NewBindingService(services.BindingServiceOptions{
		DB:                       s.db,
		Repo:                     agentBindingRepo,
		Identity:                 agentIdentitySvc,
		Permissions:              permService,
		Prompts:                  templateCatalog,
		StandardCapabilityGroups: standardCapabilityKeys,
		Runs:                     codingRunSvc,
		SCMCreds:                 &scmCredsAdapter{cr: scmCredResolver},
		LLMRuntime:               llmManager,
		RunContext:               agentBindingRepo,
		Pools:                    repository.NewActionRepository(s.db),
		Skills:                   agentSkillRepo,
		Continuations:            &itemPRContinuationResolver{db: s.db, cr: scmCredResolver},
		APIURL:                   agentAPIURL,
	})
	// Wire remote-claim enrichment after construction to break the service cycle.
	if codingRunSvc != nil && bindingSvc != nil {
		codingRunSvc.SetBindingInputsResolver(bindingSvc)
	}
	agentBindingHandler := handlers.NewWorkspaceAgentBindingHandler(bindingSvc, agentIdentitySvc, permService, logger.NewAuditor(s.db))
	agentBindingHandler.SetSkillsRepo(agentSkillRepo)
	agentBindingHandler.SetPromptStore(promptStore)
	agentBindingHandler.SetTemplateCatalog(templateCatalog)
	agentBindingHandler.SetInitialPrompt(promptStore.Get(llm.PromptCodingAgentInitial))
	// Remote-runner control plane (WI-141). Constructed unconditionally;
	// the handler 503s when the registry/run service is unavailable (i.e.
	// CodingAgent.Enabled is off).
	runnerRegistry := services.NewRunnerRegistryService(repository.NewRunnerRepository(s.db), nil)
	runnerControlHandler := handlers.NewRunnerControlHandler(runnerRegistry, repository.NewAgentRunRepository(s.db), codingRunSvc, repository.NewActionRepository(s.db), nil, baseURL)
	agentBindingHandler.SetRunnerOnboarding(runnerRegistry, baseURL)
	// Agent presence for workspace rosters (WI-272): ready binding → pool →
	// heartbeat-fresh runner count, surfaced as online/offline/local.
	agentPresenceService := services.NewAgentPresenceService(agentBindingRepo, repository.NewRunnerRepository(s.db))
	workspaceUsers := services.NewWorkspaceUserResolver(s.db, permService)
	userHandler.SetWorkspaceUserResolver(workspaceUsers)
	agentBindingHandler.SetPresenceService(agentPresenceService)
	workspaceBootstrapHandler := handlers.NewWorkspaceBootstrapHandler(workspaceHandler, userHandler, milestonePlanningService, permService, timeProjectHandler)
	// Secretless access layer (WI-144): brokers a granted credential to a
	// running job without it ever living on the runner host.
	runnerBrokerHandler := handlers.NewRunnerBrokerHandler(tokenManager, repository.NewAgentRunRepository(s.db), credentialSvc, llmManager, &scmCredsAdapter{cr: scmCredResolver})
	runnerBrokerHandler.SetUsageRepository(repository.NewLLMUsageRepository(s.db)) // meter LLM token/cost at the broker (WI-493)
	if bindingSvc != nil {
		// Registers the coding-agent assignee trigger inside the item
		// create/update services, so every surface that sets an assignee
		// (cookie handlers, REST v1, MCP/AI tools, automation actions,
		// recurrence) starts runs — not just the cookie update handler.
		services.SetItemAssigneeTrigger(bindingSvc)
	}

	assetHandler := handlers.NewAssetHandler(s.db, permService, cfg.AttachmentPath)
	assetApplication := services.NewAssetApplicationService(s.db, permService, assetHandler.AssetService(), assetHandler.AssetPermissionService()).
		WithLinks(itemLinkService).
		WithImportStorage(cfg.AttachmentPath)
	s.actionService.SetAssetNodeServices(assetHandler.AssetService(), assetHandler.AssetPermissionService())
	if n, err := assetApplication.ReconcileInterruptedImports(); err != nil {
		slog.Warn("failed to reconcile interrupted asset imports", slog.Any("error", err))
	} else if n > 0 {
		slog.Info("reconciled interrupted asset imports", slog.Int("count", n))
	}
	go s.runAssetImportRecovery(assetApplication)
	itemLinkService.WithAssetPermissionChecker(assetHandler)
	assetRepo := repository.NewAssetRepository(s.db)
	assetReportHandler := handlers.NewAssetReportHandler(
		repository.NewAssetReportRepository(s.db),
		repository.NewChannelRepository(s.db),
		repository.NewScreenRepository(s.db),
		logger.NewAuditor(s.db),
		channelService,
		services.NewAssetPermissionService(assetRepo, permService),
	)
	assetActionHandler := handlers.NewAssetActionHandler(repository.NewAssetActionRepository(s.db), assetHandler, s.assetActionService, logger.NewAuditor(s.db))

	jiraImportHandler := handlers.NewJiraImportHandler(s.db, cfg.Auth.SessionSecret, cfg.Jira.CapturePayloadsDir).
		WithAuthorizationCacheInvalidator(authorizationCacheInvalidator)

	// Share one credential manager so every in-process refresh/callback path
	// uses the same per-channel lock and CAS config writer.
	emailCredManager := email.NewCredentialManager(s.db, scmProviderHandler.GetEncryption())

	emailProviderHandler := handlers.NewEmailProviderHandler(s.db, scmProviderHandler.GetEncryption(), baseURL, channelService)
	emailProviderHandler.SetCredentialManager(emailCredManager)

	s.emailScheduler = scheduler.NewEmailScheduler(s.db, emailCredManager, cfg.AttachmentPath)
	s.emailScheduler.Start()
	slog.Info("email scheduler started (IMAP polling)")

	// Daily retention sweep for email_message_tracking. Per-channel
	// retention comes from ChannelConfig.EmailTrackingRetentionDays; anchors
	// referenced by in_reply_to are preserved past the cutoff.
	s.emailTrackingRetention = scheduler.NewEmailTrackingRetentionSweeper(s.db)
	s.emailTrackingRetention.Start()

	integrationProviderHandler := handlers.NewIntegrationProviderHandler(repository.NewIntegrationProviderRepository(s.db), scmProviderHandler.GetEncryption(), logger.NewAuditor(s.db))
	integrationOAuthHandler := handlers.NewIntegrationOAuthHandler(s.db, scmProviderHandler.GetEncryption(), baseURL)
	integrationItemLinksHandler := handlers.NewIntegrationItemLinksHandler(s.db, scmProviderHandler.GetEncryption(), permService)
	todoistSyncHandler := handlers.NewTodoistSyncHandler(s.db, scmProviderHandler.GetEncryption())
	s.todoistSyncScheduler = scheduler.NewTodoistSyncScheduler(s.db, scmProviderHandler.GetEncryption())
	s.todoistSyncScheduler.Start()

	scmSyncService := scm.NewSyncService(s.db, scmProviderHandler.GetEncryption())

	issueSyncService := scm.NewIssueSyncService(s.db, scmProviderHandler.GetEncryption())
	issueSyncService.SetUserService(services.NewUserReadService(s.db))

	go s.runIssueSync(issueSyncService)

	go s.runMagicLinkCleanup(magicLinkService)

	webhookSender := webhook.NewWebhookSender(s.db, scmProviderHandler.GetEncryption())
	s.webhookSender = webhookSender

	eventCoordinator := services.NewEventCoordinator(s.db)
	eventCoordinator.SetNotificationService(s.notificationService)
	eventCoordinator.SetActivityTracker(s.activityTracker)
	eventCoordinator.SetWebhookDispatcher(webhookSender)
	eventCoordinator.SetActionService(s.actionService)
	eventCoordinator.SetMagicLinkService(magicLinkService)
	s.actionService.SetEventCoordinator(eventCoordinator)
	s.assetActionService.SetAssetPermissionChecker(assetHandler)
	s.assetActionService.SetEventCoordinator(eventCoordinator)
	slog.Info("event coordinator initialized")

	// Wire up services
	itemHandler.SetWebhookSender(webhookSender)
	itemHandler.SetEventCoordinator(eventCoordinator)
	s.actionService.SetItemUpdateApplicationService(itemHandler.ItemUpdateApplicationService())
	s.assetActionService.SetItemCreationService(itemHandler.ItemCreationService())

	// Item live-update stream (WI-484): register the in-memory SSE hub as the
	// process-wide item-change publisher (WI-483 installed a no-op default), and
	// give the item handler the hub so GET /items/{id}/events can subscribe.
	sseHub := services.NewSSEHub()
	services.SetItemChangePublisher(sseHub)
	itemHandler.SetSSEHub(sseHub)

	mentionService := services.NewMentionService(s.db, s.notificationService, permService)
	mentionService.SetWorkspaceUserResolver(workspaceUsers)
	itemHandler.SetMentionService(mentionService)

	commentService := services.NewCommentService(s.db)
	commentService.SetActivityTracker(s.activityTracker)
	commentService.SetNotificationService(s.notificationService)
	commentService.SetMentionService(mentionService)
	commentService.SetWebhookSender(webhookSender)
	commentService.SetIssueSync(issueSyncService)
	if bindingSvc != nil {
		// @mentioning a binding's acting user in a comment starts a run
		// (WI-264), same machinery as the assignee-change trigger.
		commentService.SetAgentMentionTrigger(bindingSvc)
	}
	s.actionService.SetCommentService(commentService)

	// Wire email reply service for bidirectional email threading
	emailReplyService := services.NewEmailReplyService(s.db, smtpSender)
	if err := services.PrepareDurableEmailReplyEngine(context.Background(), s.eventEngine, emailReplyService); err != nil {
		return fmt.Errorf("prepare durable email reply consumer: %w", err)
	}
	commentService.SetEmailReplyService(emailReplyService)
	s.notificationScheduler.SetEmailReplyOutbox(emailReplyService)

	// Wire CommentService into email processor for unified comment creation
	s.emailScheduler.SetCommentService(commentService)

	// Wire EventCoordinator into email processor so inbound-email-created items
	// emit the same notifications/webhooks/action events as REST-created ones.
	s.emailScheduler.SetEventCoordinator(eventCoordinator)

	slog.Info("comment service initialized")

	itemHandler.SetActionService(s.actionService)
	itemLinkService.WithActionEmitter(s.actionService)

	scriptEngine := services.NewScriptEngine()
	conditionService := services.NewConditionService(s.db, permService, scriptEngine)

	approvalService := services.NewApprovalService(s.db, leaveRepo, workflowService)
	approvalService.SetEventCoordinator(eventCoordinator)
	approvalSetService := services.NewApprovalSetService(s.db)
	commentService.SetApprovalService(approvalService)
	s.actionService.SetApprovalService(approvalService)
	workspaceRoleHandler.SetApprovalService(approvalService)

	// Standard profiles execute in-process through the canonical aitools
	// registry. Wiring is intentionally late because their final comments and
	// approval tools use the fully configured application services above.
	if bindingSvc != nil {
		standardDispatcher, err := standardagent.New(standardagent.Options{
			DB:                     s.db,
			Runs:                   repository.NewAgentRunRepository(s.db),
			Bindings:               agentBindingRepo,
			LLMs:                   llmManager,
			Permissions:            permService,
			TimePermissions:        timePermissionService,
			Timers:                 timerService,
			Comments:               commentService,
			Approvals:              approvalService,
			Notifications:          s.notificationService,
			ActionService:          s.actionService,
			PageApplicationService: pageApplication,
			PageDiagramService:     pageDiagramService,
			Registry:               aitools.Default,
		})
		if err != nil {
			return fmt.Errorf("construct Standard agent runtime: %w", err)
		}
		bindingSvc.SetStandardRunDispatcher(standardDispatcher)
		s.standardAgentDispatcher = standardDispatcher
		if err := standardDispatcher.Resume(context.Background()); err != nil {
			return fmt.Errorf("resume Standard agent runtime: %w", err)
		}
	}

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
	// Outbound "@agent" PR-comment continuation trigger (WI-426): the sync poller
	// hands detected comments to the binding service to continue the PR. Nil-safe
	// when the coding-agent harness is disabled (bindingSvc may be nil).
	if bindingSvc != nil {
		scmSyncService.SetContinuationStarter(bindingSvc)
	}

	// Wire the SCM-driven milestone automation:
	//  1) sync emits ActionEvents for new tags / release branches,
	//  2) the create_milestone node executor consumes them and upserts
	//     by external_key (with optional release attach + commit-issue
	//     attachment via the scm.MilestoneAttacher adapter).
	scmSyncService.SetActionEvents(s.actionService)
	scmSyncService.SetDurableActionEvents(s.actionService)
	milestoneItemUpdater := services.NewItemUpdateApplicationService(s.db, permService)
	milestoneItemUpdater.SetEmitter(eventCoordinator)
	milestoneAttacher := scm.NewMilestoneAttacher(
		scmSyncService,
		repository.NewMilestoneAttachRepository(s.db),
	).WithItemUpdater(milestoneItemUpdater)
	s.actionService.RegisterNodeExecutor(
		services.NewCreateMilestoneExecutor(services.NewPlanningService(s.db), s.actionService).
			WithCommitAttacher(milestoneAttacher),
	)

	go s.runSCMRepoSync(scmSyncService)
	go s.runSCMLinkRefresh(scmSyncService)
	go s.runSCMOAuthStateCleanup()

	channelRepoForHandler := repository.NewChannelRepository(s.db)
	channelHandler := handlers.NewChannelHandler(
		channelRepoForHandler,
		repository.NewUserRepository(s.db),
		channelService,
		permService,
		webhookSender,
		logger.NewAuditor(s.db),
	)
	channelHandler.SetEmailScheduler(s.emailScheduler)
	channelHandler.SetEncryption(scmProviderHandler.GetEncryption())
	channelHandler.SetBaseURL(baseURL)
	channelHandler.SetSMTPSender(smtpSender)
	channelHandler.SetCredentialManager(emailCredManager)
	// Wire at-rest decryption into the SMTP sender so dispatch can decrypt
	// SMTPPassword before AUTH PLAIN. Done here (after scmProviderHandler is
	// initialized) rather than at smtpSender construction time because the
	// scheduler/notification wiring above can't depend on the encryption
	// service yet.
	smtpSender.SetEncryption(scmProviderHandler.GetEncryption())

	// Webhook handler
	webhookHandler := handlers.NewWebhookHandler(repository.NewChannelRepository(s.db), repository.NewItemRepository(s.db), webhookSender, permService, channelService, logger.NewAuditor(s.db))
	portalHandler := handlers.NewPortalHandler(s.db, sessionManager, portalSessionManager, ipExtractor, cfg.AttachmentPath)
	portalHandler.SetApprovalService(approvalService)
	portalHandler.SetEventCoordinator(eventCoordinator)
	portalAuthHandler := handlers.NewPortalAuthHandler(repository.NewPortalAuthRepository(s.db), portalSessionManager, sessionManager, magicLinkService, ipExtractor)
	var portalWebAuthnHandler *handlers.PortalWebAuthnHandler
	if portalWebAuthnConfig != nil {
		portalWebAuthnHandler = handlers.NewPortalWebAuthnHandler(
			portalSessionManager,
			portalWebAuthnConfig,
			portalwebauthn.NewSessionStore(s.db),
			portalwebauthn.NewCredentialStore(s.db),
			portalwebauthn.NewPortalLookupStore(s.db),
			ipExtractor,
		)
	}
	portalCustomersHandler := handlers.NewPortalCustomersHandler(s.db, permService, customerOrgPermissionService)
	contactRoleConfig := services.NewContactRoleConfig()
	contactRoleConfig.AuditEmit = enumAuditEmit
	contactRolesHandler := handlers.NewEnumHandler(
		services.NewEnumService(s.db, contactRoleConfig),
		func() any { return &models.ContactRole{} })
	hubHandler := handlers.NewHubHandler(s.db, permService, logger.NewAuditor(s.db))
	formHandler := handlers.NewFormHandler(s.db, sessionManager, portalSessionManager, ipExtractor, channelService)
	formHandler.SetEventCoordinator(eventCoordinator)

	notificationSettingsHandler := handlers.NewNotificationSettingsHandler(repository.NewNotificationSettingsRepository(s.db), logger.NewAuditor(s.db), s.notificationService).WithObjectTranslations(objectTranslationService)
	configSetNotificationHandler := handlers.NewConfigurationSetNotificationHandler(repository.NewConfigurationSetRepository(s.db), s.notificationService, logger.NewAuditor(s.db))

	var attachmentHandler *handlers.AttachmentHandler
	var attachmentSettingsHandler *handlers.AttachmentSettingsHandler
	if cfg.AttachmentPath != "" {
		slog.Info("attachments enabled", "path", cfg.AttachmentPath)
		attachmentHandler = handlers.NewAttachmentHandler(s.db, cfg.AttachmentPath, permService)
		attachmentHandler.SetApprovalService(approvalService)
		attachmentHandler.SetPagePermissionService(pagePermissionService)
		attachmentHandler.SetChannelService(channelService)
		attachmentSettingsService := services.NewAttachmentSettingsService(s.db)
		if err := attachmentSettingsService.Initialize(cfg.AttachmentPath); err != nil {
			slog.Warn("failed to initialize attachment settings", "error", err)
		}
		formHandler.SetItemAttachmentService(services.NewItemAttachmentService(s.db, cfg.AttachmentPath, permService))
		attachmentSettingsHandler = handlers.NewAttachmentSettingsHandler(attachmentSettingsService, logger.NewAuditor(s.db))
	} else {
		slog.Info("attachments disabled (no attachment path specified)")
	}

	itemDiagramService := services.NewItemDiagramService(repository.NewDiagramRepository(s.db))

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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		for _, plugin := range s.pluginManager.ListPlugins() {
			if err := s.pluginManager.RegisterPluginWebhooks(ctx, s.db, plugin); err != nil {
				slog.Warn("failed to register plugin webhooks", "plugin", plugin.Manifest.Name, "error", err)
			}
		}
		cancel()

		pluginRouter = plugins.NewRouter(s.pluginManager)

		// Plugin schedule scheduler — invokes plugin handlers on their declared
		// interval (manifest `schedules:` field). Must start after LoadPlugins
		// so the in-memory schedule registry is populated for the first tick.
		s.pluginScheduleScheduler = scheduler.NewPluginScheduleScheduler(s.pluginManager, s.db)
		s.pluginScheduleScheduler.Start()
	} else {
		slog.Info("plugin system disabled")
	}
	ssoHandler.SetPluginManager(s.pluginManager)

	pluginHandler := handlers.NewPluginHandler(s.pluginManager, repository.NewPluginRegistryRepository(s.db), logger.NewAuditor(s.db), cfg.Plugins.Disabled)

	auditLogHandler := handlers.NewAuditLogHandler(repository.NewAuditLogRepository(s.db))

	// LDAP handler — keep on Server so Shutdown can drain in-flight syncs.
	ldapSyncService := ldap.NewSyncService(s.db, ssoHandler.GetEncryption(), func(userID int) error {
		_, err := userDeactivationService.DeactivateUser(userID)
		return err
	})
	s.ldapHandler = handlers.NewLDAPHandler(s.db, ldapSyncService, ssoHandler.GetEncryption())
	ldapHandler := s.ldapHandler

	featuresHandler := handlers.NewFeaturesHandler(s.pluginManager, cfg.SSH.Enabled, cfg.Logbook.Endpoint != "")

	shutdownChan := cfg.ShutdownChan
	if shutdownChan == nil {
		shutdownChan = make(chan os.Signal, 1)
	}
	systemHandler := handlers.NewSystemHandler(shutdownChan)

	llmConnHandler := handlers.NewLLMConnectionHandler(llmManager, logger.NewAuditor(s.db), llmModelCache, llmModelRefresher)
	workItemStalenessHandler := handlers.NewWorkItemStalenessHandler(
		services.NewWorkItemStalenessService(s.db),
		logger.NewAuditor(s.db),
	)
	aiHandler := handlers.NewAIHandler(
		s.db,
		llmManager,
		permService,
		timePermissionService,
		timerService,
		promptStore,
		s.actionService,
		pageApplication,
		pageDiagramService,
	)
	aiHandler.SetConversationDependencies(commentService, approvalService)
	conversationRepo := repository.NewAgentConversationRepository(s.db)
	if _, err := conversationRepo.FailInterruptedRuns(context.Background()); err != nil {
		return fmt.Errorf("repair interrupted agent conversations: %w", err)
	}
	auditLogHandler.SetAgentTranscriptRepositories(
		conversationRepo,
		repository.NewAgentRunRepository(s.db),
	)
	shellBootstrapHandler := handlers.NewShellBootstrapHandler(
		featuresHandler,
		setupHandler,
		attachmentSettingsHandler,
		aiHandler,
		assetApplication,
		hubHandler,
		channelService,
		workItemStalenessHandler,
	)

	s.briefingScheduler = scheduler.NewBriefingScheduler(s.db, llmManager, permService, timePermissionService, services.NewUserReadService(s.db), promptStore)
	s.briefingScheduler.Start()

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
			nodeExecHandler := handlers.NewLogbookNodeExecutionHandler(
				ssoSecret,
				permService,
				assetHandler,
				itemHandler.ItemCreationService(),
			)
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
	csrfOrigins := buildAllowedOrigins(cfg.AllowedHosts, effectivePort, corsScheme, cfg.UseProxy)
	corsMiddleware := createCORSMiddleware(cfg.AllowedHosts, effectivePort, corsScheme, cfg.DisableCSRF, cfg.UseProxy, cfg.AllowInsecureHTTP)
	apiCORSMiddleware := createFormEmbedCORSMiddleware(cfg.FormEmbedOrigins, csrfOrigins, corsMiddleware)
	apiMiddleware := router.MiddlewareChain{
		apiCORSMiddleware,
		authMiddleware.OptionalAuth,
		middleware.LimitJSONRequestBody(
			restapi.DefaultJSONRequestBodyLimit,
			"/api/llm-proxy/",
			"/api/http-proxy/",
			"/api/git-proxy/",
		),
	}

	if !cfg.DisableCSRF {
		slog.Info("CSRF protection enabled (Sec-Fetch-Site + Origin/Referer fallback)", "allowed_origins", csrfOrigins)
		apiMiddleware = append(apiMiddleware, middleware.CSRFProtection(csrfOrigins))
	} else {
		slog.Warn("CSRF protection disabled (development mode)")
	}

	// Per-user concurrency cap goes last so it is the innermost wrapper: the
	// slot is held only around the handler, and OptionalAuth (earlier in the
	// chain) has already put the user in context for keying. Cheap rejections
	// (CORS/CSRF/auth failures) never consume a slot.
	apiMiddleware = append(apiMiddleware, s.userConcurrency.Limit)
	if cfg.MaxUserConcurrency > 0 {
		slog.Info("per-user API concurrency cap enabled", "max_in_flight_per_user", cfg.MaxUserConcurrency)
	}

	// Create API route group
	api := router.NewRouteGroup(mux, "/api", apiMiddleware...)

	// SCIM routes
	scimMiddleware := router.MiddlewareChain{corsMiddleware}
	scimGroup := router.NewRouteGroup(mux, "/scim/v2", scimMiddleware...)

	// Create portal auth middleware (accepts both internal and portal sessions)
	portalAuthMiddleware := middleware.NewPortalAuthMiddleware(sessionManager, portalSessionManager, cfg.UseProxy, additionalProxyList)
	oauthHandler := handlers.NewOAuthHandler(
		s.db,
		agentHandler,
		tokenManager,
		apiTokenHandler,
		permService,
		handlers.OAuthServerConfig{IssuerURL: baseURL, MCPEnabled: cfg.MCPEnabled},
	)

	// Build route dependencies
	routeDeps := &routes.Deps{
		API:       api,
		SCIMGroup: scimGroup,
		Mux:       mux,

		AuthMiddleware:       authMiddleware,
		PermissionMiddleware: permissionMiddleware,
		SCIMAuthMiddleware:   scimAuthMiddleware,
		PortalAuthMiddleware: portalAuthMiddleware,

		LoginRateLimiter:      s.loginRateLimiter,
		RunnerRegisterLimiter: s.runnerRegisterLimiter,
		AuthRateLimiter:       s.authRateLimiter,
		FIDORateLimiter:       s.fidoRateLimiter,
		SSORateLimiter:        s.ssoRateLimiter,
		SCIMRateLimiter:       s.scimRateLimiter,
		PortalSubmitLimiter:   s.portalSubmitLimiter,
		PortalSearchLimiter:   s.portalSearchLimiter,
		PortalAuthLimiter:     s.portalAuthLimiter,
		OAuthTokenLimiter:     s.oauthTokenLimiter,
		EmailVerifyLimiter:    s.emailVerifyLimiter,
		SetupLimiter:          s.setupLimiter,
		AIRateLimiter:         s.aiRateLimiter,
		UploadLimiter:         s.uploadLimiter,
		WebhookLimiter:        s.webhookLimiter,
		SearchLimiter:         s.searchLimiter,
		CalendarFeedLimiter:   s.calendarFeedLimiter,
		PublicBoardLimiter:    s.publicBoardLimiter,

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
			IssueSync:     handlers.NewIssueSyncHandler(issueSyncService, permService, logger.NewAuditor(s.db)),
		},
		Items: routes.ItemHandlers{
			Item:               itemHandler,
			Attachment:         attachmentHandler,
			AttachmentSettings: attachmentSettingsHandler,
		},
		Workspaces: routes.WorkspaceHandlers{
			Workspace:             workspaceHandler,
			Bootstrap:             workspaceBootstrapHandler,
			Screen:                screenHandler,
			ConfigSet:             configSetHandler,
			ConfigSetNotification: configSetNotificationHandler,
			NotificationSettings:  notificationSettingsHandler,
			HierarchyLevel:        hierarchyLevelHandler,
			RequestType:           requestTypeHandler,
			Actions:               actionsHandler,
			ActionCredentials:     actionCredentialsHandler,
			ActionTemplates:       handlers.NewActionTemplatesHandler(services.NewActionTemplateService(s.db), s.actionService, workspaceKeyCache, logger.NewAuditor(s.db)),
			Analytics:             handlers.NewAnalyticsHandler(services.NewAnalyticsService(s.db), permService, workspaceKeyCache),
			AgentBinding:          agentBindingHandler,
			RunnerControl:         runnerControlHandler,
			RunnerBroker:          runnerBrokerHandler,
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
			CLIAuth:       handlers.NewCLIAuthHandler(repository.NewCLIAuthRepository(s.db), logger.NewAuditor(s.db), agentHandler, tokenManager, apiTokenHandler, permService),
			OAuth:         oauthHandler,
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
			ShellBootstrap:   shellBootstrapHandler,
			OAuthClients:     handlers.NewAdminOAuthClientHandler(s.db, tokenManager, permService),
			Diagnostics: handlers.NewDiagnosticsHandler(
				sessionManager,
				s.databaseDiagRepo,
				repository.NewActionRepository(s.db),
				repository.NewWebhookDeliveryRepository(s.db),
				repository.NewSchedulerRunRepository(s.db),
				repository.NewFracIndexRepository(s.db),
				repository.NewAIRepository(s.db),
				llmManager,
				llmModelCache,
				logger.NewAuditor(s.db),
				repository.NewRunnerRepository(s.db),
				repository.NewAgentRunRepository(s.db),
				s.webhookSender,
				transitionMatrixService,
				bulkOperationMetrics,
				repository.NewRecurrenceRepository(s.db),
				repository.NewSystemSettingRepository(s.db),
				s.globalRankMigrationScheduler,
				s.memoryBudget,
				s.eventEngine.Store(),
				repository.NewSCMHealthRepository(s.db),
			),
			AgentSecurity: handlers.NewAgentSecurityHandler(
				agentSecurityRepo,
				services.NewUserReadService(s.db),
				permService,
				logger.NewAuditor(s.db),
			),
			AgentTemplateCatalog: agentTemplateCatalogHandler,
			ObjectTranslation:    objectTranslationHandler,
		},
		Planning: routes.PlanningHandlers{
			MilestoneCategory: milestoneCategoryHandler,
			IterationType:     iterationTypeHandler,
			PersonalLabel:     personalLabelHandler,
		},
		TimeTracking: routes.TimeTrackingHandlers{
			Customer:           timeCustomerHandler,
			Project:            timeProjectHandler,
			CustomerPermission: customerOrgPermissionHandler,
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
			PortalWebAuthn: portalWebAuthnHandler,
			PortalCustomer: portalCustomersHandler,
			ContactRole:    contactRolesHandler,
			Hub:            hubHandler,
			Form:           formHandler,
		},
		Assets: routes.AssetHandlers{
			Asset:  assetHandler,
			Action: assetActionHandler,
		},
		PublicBoard: publicBoardHandler,
		AI: routes.AIHandlers{
			AI:                aiHandler,
			LLMConnection:     llmConnHandler,
			WorkItemStaleness: workItemStalenessHandler,
		},
		Misc: routes.MiscHandlers{
			Homepage:      homepageHandler,
			Review:        reviewHandler,
			CalendarFeed:  calendarFeedHandler,
			CustomField:   customFieldHandler,
			RunnerInstall: handlers.NewRunnerInstallHandler(baseURL),
		},
		Teams: routes.TeamHandlers{
			Team:   teamHandler,
			Leave:  leaveHandler,
			OnCall: onCallHandler,
		},
		Integrations: routes.IntegrationHandlers{
			Provider:    integrationProviderHandler,
			OAuth:       integrationOAuthHandler,
			ItemLinks:   integrationItemLinksHandler,
			TodoistSync: todoistSyncHandler,
		},
		Pages: routes.PageHandlers{
			KnowledgeSearch: knowledgeSearchHandler,
		},
		Push: pushHandler,
	}
	routes.RegisterAll(routeDeps)

	// Test-only endpoint: gated on WINDSHIFT_E2E_TEST_HOOKS=1. Lets the
	// Playwright suite inject a synthetic SCM ref ActionEvent — same
	// payload shape the sync layer emits — so the create_milestone
	// action chain can be exercised end-to-end without standing up a
	// real GitHub or pushing real refs. Production never sets this env.
	if os.Getenv("WINDSHIFT_E2E_TEST_HOOKS") == "1" {
		mux.Handle("POST /api/test/scm/setup-mock-repo", handlers.NewTestSetupMockRepo(services.NewTestSCMHookService(s.db, nil)))
		mux.Handle("POST /api/test/scm/inject-ref", handlers.NewTestSCMInjectRef(services.NewTestSCMHookService(s.db, s.actionService)))
		slog.Warn("WINDSHIFT_E2E_TEST_HOOKS enabled — test hook routes are mounted; never enable in production")
	}

	// Register plugin routes
	if pluginRouter != nil {
		pluginRouter.RegisterRoutes(mux)
	}

	// REST API v1
	restapi.SetupRoutes(restapi.Deps{
		Mux:                            mux,
		DB:                             s.db,
		TokenManager:                   tokenManager,
		PermissionService:              permService,
		ActionService:                  s.actionService,
		AttachmentPath:                 cfg.AttachmentPath,
		ItemLinkService:                itemLinkService,
		AssetPermissionService:         assetHandler.AssetPermissionService(),
		AssetService:                   assetHandler.AssetService(),
		CommentService:                 commentService,
		ItemCreationService:            itemHandler.ItemCreationService(),
		ItemUpdateApplicationService:   itemHandler.ItemUpdateApplicationService(),
		ItemDeletionApplicationService: itemHandler.ItemDeletionApplicationService(),
		PageApplicationService:         pageApplication,
		PageDiagramService:             pageDiagramService,
		ObjectTranslationService:       objectTranslationService,
		AuthorizationCacheInvalidator:  authorizationCacheInvalidator,
	}, v1.RegisterRoutes)
	v2CSRF := middleware.NewDisabledCSRFValidator()
	if !cfg.DisableCSRF {
		v2CSRF = middleware.NewCSRFValidator(csrfOrigins)
	}
	v2Access := authz.New(s.db, permService)
	planningCredentialResolver := scm.NewCredentialResolver(s.db, scmProviderHandler.GetEncryption())
	planningApplication := services.NewPlanningApplicationService(
		milestonePlanningService,
		permService,
		services.NewIterationCompletionService(s.db),
		func(ctx context.Context, connectionID, userID int) (services.PlanningReleaseProvider, error) {
			provider, err := planningCredentialResolver.GetProviderForUser(ctx, connectionID, userID)
			if err != nil {
				return nil, err
			}
			releaseProvider, ok := provider.(scm.ReleaseProvider)
			if !ok {
				return nil, errors.New("SCM provider does not support releases")
			}
			return planningReleaseProviderAdapter{provider: releaseProvider}, nil
		},
	)
	agentRunApplication := services.NewAgentRunReadService(
		repository.NewItemRepository(s.db),
		repository.NewAgentRunRepository(s.db),
		permService,
	).WithRuntime(codingRunSvc, bindingSvc).WithUsage(repository.NewLLMUsageRepository(s.db))
	actionApplication := services.NewActionApplicationService(
		s.db,
		repository.NewActionRepository(s.db),
		repository.NewItemRepository(s.db),
		permService,
		s.actionService,
		assetHandler.AssetService(),
	)
	itemApplication := services.NewItemApplicationService(
		s.db,
		permService,
		itemHandler.ItemCreationService(),
		itemHandler.ItemUpdateApplicationService(),
		itemHandler.ItemDeletionApplicationService(),
	).WithReads(s.activityTracker, approvalService).
		WithCache(itemHandler.ItemCacheService()).
		WithWorkflow(conditionService, eventCoordinator, issueSyncService).
		WithMutationEffects(mentionService)
	itemDetailApplication := services.NewItemDetailApplicationService(s.db, itemApplication, itemLinkService, permService).
		WithContextReaders(screenHandler, requestTypeHandler, actionApplication)
	catalogMutations := services.NewCatalogMutationService(s.db, permService, workflowService)
	governanceApplication := services.NewGovernanceApplicationService(s.db, permService, approvalSetService, approvalService)
	if err := v2.RegisterRoutes(v2.Deps{
		Mux:                mux,
		Tokens:             tokenManager,
		Users:              services.NewUserReadService(s.db),
		Statuses:           services.NewStatusService(s.db),
		Workflows:          workflowService,
		Configuration:      services.NewConfigReadService(s.db),
		ObjectTranslations: objectTranslationService,
		Catalog:            services.NewCatalogReadService(s.db, permService, v2Access, s.activityTracker),
		CatalogMutations:   catalogMutations,
		Workspaces:         services.NewWorkspaceApplicationService(s.db, v2Access, authorizationCacheInvalidator),
		ItemTemplates:      services.NewItemTemplateApplicationService(s.db, v2Access),
		Labels:             services.NewLabelApplicationService(s.db),
		Items:              repository.NewItemRepository(s.db),
		Access:             v2Access,
		Preferences:        userPreferencesService,
		Recurrence:         recurrenceService,
		ItemDiagrams:       itemDiagramService,
		Pages:              pageService,
		PageApplication:    pageApplication,
		PageDiagrams:       pageDiagramService,
		PageAccess:         pagePermissionService,
		PageLabels:         pageLabelService,
		Worklogs:           timeWorklogService,
		TimeAccess:         timePermissionService,
		TimeProjects:       services.NewTimeProjectApplicationService(s.db, timePermissionService, v2Access),
		Timers:             timerService,
		SystemAdmins:       permService,
		Groups:             groupHandler.Application(),
		AdminUsers:         services.NewUserReadService(s.db),
		Comments:           commentService,
		CommentAccess:      permService,
		Attachments:        services.NewItemAttachmentService(s.db, cfg.AttachmentPath, permService),
		PageAttachments:    services.NewPageAttachmentUploadService(s.db, cfg.AttachmentPath, permService, pagePermissionService),
		Collections:        services.NewCollectionApplicationService(s.db, permService),
		Planning:           planningApplication,
		Links:              itemLinkService,
		AgentRuns:          agentRunApplication,
		AgentSkills:        services.NewAgentSkillApplicationService(s.db, permService),
		ConditionSets:      services.NewConditionSetApplicationService(s.db, permService),
		Governance:         governanceApplication,
		Actions:            actionApplication,
		TestManagement:     services.NewTestManagementApplicationService(s.db, permService),
		Assets:             assetApplication,
		ItemApplication:    itemApplication,
		ItemDetail:         itemDetailApplication,
		SessionMiddleware:  authMiddleware.OptionalAuth,
		CORS:               v2.NewCORS(csrfOrigins, cfg.DisableCSRF, !cfg.DisableCSRF),
		CSRF:               v2CSRF,
		Concurrency:        s.userConcurrency,
	}); err != nil {
		return fmt.Errorf("register REST API v2: %w", err)
	}

	// MCP Server (Model Context Protocol) — opt-in via --mcp or MCP_ENABLED=true
	if cfg.MCPEnabled {
		mcpServer := mcpserver.NewMCPServer(mcpserver.Deps{
			DB:           s.db,
			TokenManager: tokenManager,
			Auth: mcpserver.AuthConfig{
				ResourceURI:         oauthHandler.MCPResourceURI(),
				ResourceMetadataURI: oauthHandler.MCPProtectedResourceMetadataURI(),
			},
			PermissionService:      permService,
			TimePermissionService:  timePermissionService,
			TimerService:           timerService,
			CommentService:         commentService,
			ItemDeletionService:    itemHandler.ItemDeletionApplicationService(),
			PageApplicationService: pageApplication,
			PageDiagramService:     pageDiagramService,
			ActionService:          s.actionService,
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

			// Vite emits content-hashed filenames under /_app/, so those bytes
			// never change for a given URL — cache them aggressively. The other
			// static entry points have stable filenames whose contents can change
			// between builds, so force revalidation.
			immutableAssets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				fileServer.ServeHTTP(w, r)
			})
			revalidatingAssets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				fileServer.ServeHTTP(w, r)
			})

			mux.Handle("GET /remoteEntry.js", revalidatingAssets)
			mux.Handle("GET /_app/", immutableAssets)
			mux.Handle("GET /windshift-3.svg", revalidatingAssets)
			mux.Handle("GET /favicon-32x32.png", revalidatingAssets)
			mux.Handle("GET /apple-touch-icon.png", revalidatingAssets)
			mux.Handle("GET /apple-touch-startup-image-1170x2532.png", revalidatingAssets)
			mux.Handle("GET /apple-touch-startup-image-1290x2796.png", revalidatingAssets)
			mux.Handle("GET /apple-touch-startup-image-2048x2732.png", revalidatingAssets)
			mux.Handle("GET /forms/widget.js", revalidatingAssets)
			mux.Handle("GET /embed/", revalidatingAssets)

			// PWA entry points. These need explicit routes (the SPA fallback at
			// "GET /" would otherwise serve index.html for them) and explicit
			// content types — Go has no registered MIME for .webmanifest, and the
			// service worker needs a root scope grant + no-cache so updates land.
			mux.Handle("GET /manifest.webmanifest", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/manifest+json")
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				fileServer.ServeHTTP(w, r)
			}))
			mux.Handle("GET /service-worker.js", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/javascript")
				w.Header().Set("Service-Worker-Allowed", "/")
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				fileServer.ServeHTTP(w, r)
			}))

			// Read index.html once at startup for nonce injection
			indexHTML, err := fs.ReadFile(distFS, "index.html")
			if err != nil {
				slog.Warn("could not read index.html from embedded FS", "error", err)
			}

			contextPath := cfg.ContextPath
			mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				// Anything under an API root that hasn't matched a specific
				// route is a 404 — don't fall through to the SPA shell.
				// The prefixes must be path-segment scoped so client routes
				// like /api-docs aren't shadowed by the /api check.
				if isAPIPath(r.URL.Path) {
					http.NotFound(w, r)
					return
				}

				if indexHTML == nil {
					http.NotFound(w, r)
					return
				}

				// Inject CSP nonce into the inline theme script tag and expose the
				// externally visible context path for the SPA translation layer.
				nonce := CSPNonceFromContext(r.Context())
				html := prepareIndexHTML(indexHTML, nonce, contextPath)

				w.Header().Set("Content-Type", "text/html")
				// Force the SPA shell to revalidate on every load so that a
				// new desktop/web build is picked up without users having to
				// force-quit the Tauri WebView or hard-refresh the browser.
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(html))
			})
		}
	}

	// Maintain a small in-memory list of configured Jira instance origins so the
	// CSP `img-src` directive allows project avatars served from each tenant.
	jiraHosts := NewJiraHostAllowlist(s.db, 60*time.Second)
	go jiraHosts.Start(s.jiraHostStopChan)

	// Recovery converts panics before the metrics layer records the final status.
	securityMiddleware := createSecurityHeaders(enableHTTPS, cfg.UseProxy, additionalProxyIPs, jiraHosts.Allowed, securitySettingsHandler.ExternalImagesAllowed)
	compressionMiddleware := middleware.CreateCompressionMiddleware(cfg.UseProxy)
	applicationHandler := middleware.Recovery(compressionMiddleware(securityMiddleware(s.metrics.CaptureRoutePattern(mux))))
	handler := s.metrics.Instrument(applicationHandler)
	handler = withContextPath(handler, cfg.ContextPath)

	// Create HTTP server
	s.httpServer = &http.Server{
		Handler:             handler,
		ReadTimeout:         15 * time.Second,
		WriteTimeout:        30 * time.Second,
		IdleTimeout:         60 * time.Second,
		MaxHeaderBytes:      1 << 20,
		MaxHeaderValueCount: maxRequestHeaderValueCount,
	}
	if err := s.eventEngine.Start(context.Background()); err != nil {
		return fmt.Errorf("start domain event engine: %w", err)
	}

	return nil
}

// initializeWebAuthnConfigs builds the internal and portal passkey
// configurations. Passkeys are optional, so a relying party that cannot be
// described by the current settings disables them and returns nil
// configurations rather than stopping an otherwise healthy installation.
// Remaining errors stay fatal because they indicate a broken WebAuthn setup.
func initializeWebAuthnConfigs(cfg Config, isDevelopment bool, effectivePort string, enableHTTPS bool) (internalConfig *webauthn.Config, portalConfig *portalwebauthn.Config, err error) {
	rpID := cfg.WebAuthn.RPID
	if isDevelopment {
		rpID = ""
	}

	webAuthnConfig, err := webauthn.NewConfig(webauthn.Options{
		RPID:          rpID,
		RPName:        cfg.WebAuthn.RPName,
		BaseURL:       cfg.BaseURL,
		AllowedHosts:  cfg.AllowedHosts,
		Port:          effectivePort,
		IsDevelopment: isDevelopment,
		EnableHTTPS:   enableHTTPS,
		UseProxy:      cfg.UseProxy,
	})
	if err != nil {
		var invalidRPIDErr *webauthn.InvalidRPIDError
		var missingOriginsErr *webauthn.MissingOriginsError

		switch {
		// A single-label hostname is common in Docker, homelab DNS, and local
		// test installations, but it cannot be used as an RP ID by the current
		// WebAuthn implementation. Keep the application healthy and make the
		// limitation actionable; passkeys remain available when the operator
		// uses localhost, an IP address, or a dotted hostname.
		case errors.As(err, &invalidRPIDErr):
			slog.Warn("WebAuthn disabled because the configured RP ID is not valid; set BASE_URL or WEBAUTHN_RP_ID to localhost, an IP address, or a dotted hostname to enable passkeys",
				"rp_id", invalidRPIDErr.RPID,
				"error", invalidRPIDErr)
		case errors.As(err, &missingOriginsErr):
			slog.Warn("WebAuthn disabled because no browser-visible origin is configured; set BASE_URL to the URL users open to enable passkeys",
				"error", missingOriginsErr)
		default:
			return nil, nil, fmt.Errorf("failed to initialize WebAuthn configuration: %w", err)
		}

		return nil, nil, nil
	}

	// Log the origins: a passkey ceremony that fails in the browser is almost
	// always an origin the relying party does not accept, and this is the only
	// place the resolved list is visible to an operator.
	slog.Info("WebAuthn configuration initialized",
		"rp_id", webAuthnConfig.RPID,
		"rp_name", webAuthnConfig.RPName,
		"rp_origins", webAuthnConfig.RPOrigins,
		"development_mode", isDevelopment)

	// Portal passkeys reuse the relying-party settings but require resident
	// keys so customers can sign in passwordlessly (BeginDiscoverableLogin).
	portalWebAuthnConfig, err := portalwebauthn.NewConfig(webAuthnConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize portal WebAuthn configuration: %w", err)
	}

	return webAuthnConfig, portalWebAuthnConfig, nil
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
	_, err = s.db.ExecWrite(`UPDATE users SET is_active = true, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
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
	s.databasePoolMonitor.Start()

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
	if s.databasePoolMonitor != nil {
		s.databasePoolMonitor.Stop()
	}

	// Stop schedulers first - use safeClose helper to avoid panics on already-closed channels
	safeClose := func(ch chan struct{}) {
		if ch != nil {
			defer func() { recover() }() //nolint:errcheck // Intentionally ignoring recover() return; used to suppress panics from closing already-closed channels
			close(ch)
		}
	}

	// Close, but do NOT nil, the stop channels: background schedulers select
	// on these fields in a loop, so the nil-write races with their reads (and
	// a select on a nil channel blocks forever, leaking the goroutine).
	// Double-close safety comes from safeClose's recover, not from nil-ing.
	safeClose(s.scmSyncStopChan)
	safeClose(s.issueSyncStopChan)
	safeClose(s.magicLinkStopChan)

	if s.cleanupTicker != nil {
		// Stop, but do NOT nil: runActivityCleanup selects on cleanupTicker.C
		// in a loop and the nil-write races with that read.
		s.cleanupTicker.Stop()
	}
	safeClose(s.cleanupStopChan)
	safeClose(s.jiraHostStopChan)

	if s.notificationScheduler != nil {
		slog.Info("stopping notification scheduler")
		s.notificationScheduler.Stop()
	}

	if s.recurrenceScheduler != nil {
		slog.Info("stopping recurrence scheduler")
		s.recurrenceScheduler.Stop()
	}

	if s.cfvCleanupScheduler != nil {
		slog.Info("stopping cfv cleanup scheduler")
		s.cfvCleanupScheduler.Stop()
	}

	if s.todoistSyncScheduler != nil {
		slog.Info("stopping todoist sync scheduler")
		s.todoistSyncScheduler.Stop()
	}

	if s.runnerLeaseReaper != nil {
		slog.Info("stopping runner lease reaper")
		s.runnerLeaseReaper.Stop()
	}

	if s.globalRankMigrationScheduler != nil {
		slog.Info("stopping global rank migration scheduler")
		s.globalRankMigrationScheduler.Stop()
	}

	if s.codingRunService != nil {
		slog.Info("shutting down coding-agent run service")
		// Stops admission, drains still-queued local runs as canceled, and
		// cancels in-flight runs so their workers finalize a terminal status
		// (WI-332). Bounded by the shutdown ctx like the LDAP drain below.
		if err := s.codingRunService.Shutdown(ctx); err != nil {
			slog.Warn("coding-agent run service shutdown did not drain in time", "error", err)
		}
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

	if s.emailTrackingRetention != nil {
		slog.Info("stopping email tracking retention sweeper")
		s.emailTrackingRetention.Stop()
	}

	if s.briefingScheduler != nil {
		slog.Info("stopping briefing scheduler")
		s.briefingScheduler.Stop()
	}

	if s.pluginScheduleScheduler != nil {
		slog.Info("stopping plugin schedule scheduler")
		s.pluginScheduleScheduler.Stop()
	}

	if s.standardAgentDispatcher != nil {
		slog.Info("draining Standard agent runtime")
		if err := s.standardAgentDispatcher.Close(ctx); err != nil {
			slog.Warn("Standard agent runtime did not drain in time", "error", err)
		}
	}

	if s.notificationService != nil {
		slog.Info("stopping notification service")
		_ = s.notificationService.Close()
	}

	if s.ldapHandler != nil {
		slog.Info("draining LDAP sync goroutines")
		s.ldapHandler.Stop(ctx)
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

	if s.webhookSender != nil {
		slog.Info("draining webhook dispatch queue")
		if err := s.webhookSender.Shutdown(ctx); err != nil {
			slog.Warn("webhook dispatch queue did not drain in time", "error", err)
		}
	}

	if s.eventEngine != nil {
		slog.Info("stopping domain event engine")
		if err := s.eventEngine.Shutdown(ctx); err != nil {
			slog.Warn("domain event engine did not stop in time", "error", err)
		}
	}

	// Cleanup remaining resources
	s.cleanup()

	slog.Info("server shutdown complete")
	return nil
}

// isAPIPath reports whether p falls under an API root (and so should be a
// hard 404 when no specific route matched) rather than the SPA shell. The
// match is path-segment scoped — `/api-docs` is *not* an API path even
// though it starts with the four bytes `/api`.
func isAPIPath(p string) bool {
	for _, root := range []string{"/api", "/rest", "/scim"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

// cleanup releases all resources.
func (s *Server) cleanup() {
	if s.databasePoolMonitor != nil {
		s.databasePoolMonitor.Stop()
	}
	if s.eventEngine != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.eventEngine.Shutdown(ctx); err != nil {
			slog.Warn("domain event engine cleanup did not stop in time", "error", err)
		}
		cancel()
	}
	// initialize may fail after this scheduler has started. Stop and join it
	// before closing the database so an in-flight rank batch cannot race cleanup.
	if s.globalRankMigrationScheduler != nil {
		s.globalRankMigrationScheduler.Stop()
	}
	// Stop rate limiters
	if s.loginRateLimiter != nil {
		s.loginRateLimiter.Stop()
	}
	if s.runnerRegisterLimiter != nil {
		s.runnerRegisterLimiter.Stop()
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
	if s.oauthTokenLimiter != nil {
		s.oauthTokenLimiter.Stop()
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
	if s.publicBoardLimiter != nil {
		s.publicBoardLimiter.Stop()
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

// RegisterDatabasePool makes a process-local auxiliary SQL pool visible to
// admin diagnostics and threshold monitoring.
func (s *Server) RegisterDatabasePool(name string, db database.Database) error {
	if s.databaseDiagRepo == nil {
		return fmt.Errorf("database diagnostics are not initialized")
	}
	return s.databaseDiagRepo.RegisterPool(name, db)
}

func logDatabaseCapacityBudget(budget repository.DatabaseCapacityBudget) {
	args := []any{
		"component", "database_pool",
		"event", "database_pool_capacity_budget",
		"server_max_connections", budget.ServerMaxConnections,
		"main_connections_per_replica", budget.MainConnectionsPerReplica,
		"auxiliary_connections_per_replica", budget.AuxiliaryConnectionsPerReplica,
		"connections_per_replica", budget.ConnectionsPerReplica,
		"replica_count", budget.ReplicaCount,
		"headroom_connections", budget.HeadroomConnections,
		"required_connections", budget.RequiredConnections,
		"remaining_connections", budget.RemainingConnections,
		"utilization_percent", budget.UtilizationPercent,
	}
	if !budget.Safe {
		slog.Warn("declared PostgreSQL pool budget exceeds server capacity", args...)
		return
	}
	if budget.UtilizationPercent >= databasePoolBudgetWarningPercent {
		slog.Warn("declared PostgreSQL pool budget leaves little server headroom", args...)
		return
	}
	slog.Info("PostgreSQL connection capacity budget", args...)
}

// BaseURL returns the server's base URL.
// deadcode-keep: called by core-tests/tests/helpers.go
func (s *Server) BaseURL() string {
	if s.actualPort == 0 {
		return fmt.Sprintf("http://localhost:%s%s", s.config.Port, s.config.ContextPath)
	}
	return fmt.Sprintf("http://localhost:%d%s", s.actualPort, s.config.ContextPath)
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

func (s *Server) runAssetImportRecovery(assets *services.AssetApplicationService) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.cleanupStopChan:
			return
		case <-ticker.C:
			if _, err := assets.ReconcileInterruptedImports(); err != nil {
				slog.Error("asset import recovery failed", "error", err)
			}
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
			started := time.Now()
			err := scmSyncService.SyncAllRepositories(ctx)
			s.metrics.ObserveSCMPoll("repository_sync", time.Since(started), err)
			if err != nil {
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
			started := time.Now()
			err := scmSyncService.RefreshAllPRLinkStates(ctx)
			s.metrics.ObserveSCMPoll("pull_request_refresh", time.Since(started), err)
			if err != nil {
				slog.Error("PR state refresh error", "error", err)
			}
			cancel()
		case <-s.scmSyncStopChan:
			slog.Info("SCM PR link refresh scheduler stopped")
			return
		}
	}
}

// runSCMOAuthStateCleanup expires OAuth state and restores reconnecting email
// channels across both database backends.
func (s *Server) runSCMOAuthStateCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	slog.Info("OAuth state cleanup scheduler started (1-minute interval)")
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := s.restoreExpiredEmailOAuthChannels(ctx); err != nil {
				slog.Error("expired email OAuth channel restore failed", slog.Any("error", err))
			}
			if _, err := s.db.ExecWriteContext(ctx, `DELETE FROM email_oauth_state WHERE expires_at < CURRENT_TIMESTAMP`); err != nil {
				slog.Error("email_oauth_state cleanup failed", slog.Any("error", err))
			}
			res, err := s.db.ExecWriteContext(ctx, `DELETE FROM scm_oauth_state WHERE expires_at < CURRENT_TIMESTAMP`)
			cancel()
			if err != nil {
				slog.Error("scm_oauth_state cleanup failed", slog.Any("error", err))
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				slog.Debug("scm_oauth_state cleanup", slog.Int64("deleted", n))
			}
		case <-s.scmSyncStopChan:
			slog.Info("OAuth state cleanup scheduler stopped")
			return
		}
	}
}

// restoreExpiredEmailOAuthChannels restores only currently ingestion-ready channels.
func (s *Server) restoreExpiredEmailOAuthChannels(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT channel_id
		FROM email_oauth_state
		WHERE expires_at < CURRENT_TIMESTAMP
		  AND restore_channel_enabled = true
		  AND channel_id IS NOT NULL
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var channelIDs []int
	for rows.Next() {
		var channelID int
		if err := rows.Scan(&channelID); err != nil {
			return err
		}
		channelIDs = append(channelIDs, channelID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, channelID := range channelIDs {
		channel, err := s.channelService.GetByID(ctx, channelID)
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		configJSON, err := s.channelService.GetConfig(ctx, channelID)
		if err != nil {
			return err
		}
		var channelConfig models.ChannelConfig
		if err := json.Unmarshal([]byte(configJSON), &channelConfig); err != nil {
			slog.Warn("invalid email config while restoring expired OAuth state", "channel_id", channelID, "error", err)
			continue
		}
		if err := email.ValidateConfigForEnable(channel, &channelConfig); err != nil {
			slog.Warn("expired OAuth left email channel disabled because config is not ready", "channel_id", channelID, "error", err)
			continue
		}
		updated, err := s.channelService.SetStatusIfConfigUnchanged(ctx, channelID, "enabled", configJSON)
		if err != nil {
			return err
		}
		if !updated {
			slog.Warn("email config changed during expired OAuth restoration; leaving disabled", "channel_id", channelID)
		}
	}
	return nil
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
			started := time.Now()
			err := issueSyncService.SyncAll(ctx)
			s.metrics.ObserveSCMPoll("issue_sync", time.Since(started), err)
			if err != nil {
				slog.Error("Issue sync error", "error", err)
			}
			cancel()
		case <-s.issueSyncStopChan:
			slog.Info("Issue sync scheduler stopped")
			return
		}
	}
}

// scmCredsAdapter avoids a services-to-scm import cycle.
type scmCredsAdapter struct {
	cr *scm.CredentialResolver
}

// ResolveForRun resolves SCM credentials for run-start Git authentication.
func (a *scmCredsAdapter) ResolveForRun(ctx context.Context, connectionID int) (token, providerType, baseURL string, err error) {
	creds, err := a.cr.GetCredentialsByConnectionID(ctx, connectionID)
	if err != nil {
		return "", "", "", err
	}
	return a.tokenFromCreds(ctx, connectionID, creds)
}

// ResolveForRunAsUser requires a triggering user's OAuth token while retaining
// PAT and GitHub App connection-level resolution.
func (a *scmCredsAdapter) ResolveForRunAsUser(ctx context.Context, connectionID, userID int) (token, providerType, baseURL string, err error) {
	creds, err := a.cr.GetCredentialsForUser(ctx, connectionID, userID)
	if err != nil {
		if errors.Is(err, scm.ErrUserSCMNotConnected) {
			return "", "", "", fmt.Errorf("user %d on connection %d: %w", userID, connectionID, services.ErrTriggerUserSCMNotConnected)
		}
		return "", "", "", err
	}
	return a.tokenFromCreds(ctx, connectionID, creds)
}

// tokenFromCreds selects OAuth, PAT, then GitHub App credentials.
func (a *scmCredsAdapter) tokenFromCreds(ctx context.Context, connectionID int, creds *scm.ProviderCredentials) (token, providerType, baseURL string, err error) {
	switch {
	case creds.OAuthAccessToken != "":
		token = creds.OAuthAccessToken
	case creds.PersonalAccessToken != "":
		token = creds.PersonalAccessToken
	case creds.GitHubAppID != "" && creds.GitHubAppPrivateKey != "" && creds.GitHubAppInstallationID != "":
		// Per-run GitHub App tokens need no cache or refresh.
		t, terr := a.mintGitHubAppToken(ctx, creds)
		if terr != nil {
			return "", "", "", fmt.Errorf("mint GitHub App installation token: %w", terr)
		}
		token = t
	default:
		return "", "", "", fmt.Errorf("connection %d has no usable auth (no OAuth, no PAT, no complete GitHub App config)", connectionID)
	}
	return token, string(creds.ProviderType), creds.BaseURL, nil
}

// mintGitHubAppToken builds a transient scm.GitHubAppProvider from the
// stored App credentials and asks it for an installation token. Used by
// ResolveForRun for the git-CLI auth path; CreatePullRequest goes through
// the same provider already (it just calls GetInstallationAccessToken
// internally on the first authenticated request).
func (a *scmCredsAdapter) mintGitHubAppToken(ctx context.Context, creds *scm.ProviderCredentials) (string, error) {
	provider, err := scm.NewProvider(scm.ProviderConfig{
		ProviderType:            creds.ProviderType,
		AuthMethod:              creds.AuthMethod,
		BaseURL:                 creds.BaseURL,
		GitHubAppID:             creds.GitHubAppID,
		GitHubAppPrivateKey:     creds.GitHubAppPrivateKey,
		GitHubAppInstallationID: creds.GitHubAppInstallationID,
	})
	if err != nil {
		return "", fmt.Errorf("build provider: %w", err)
	}
	appProvider, ok := provider.(scm.GitHubAppProvider)
	if !ok {
		return "", fmt.Errorf("provider for connection is not a GitHubAppProvider (type %T)", provider)
	}
	installationID, err := strconv.ParseInt(creds.GitHubAppInstallationID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse installation id %q: %w", creds.GitHubAppInstallationID, err)
	}
	token, _, err := appProvider.GetInstallationAccessToken(ctx, installationID)
	if err != nil {
		return "", err
	}
	return token, nil
}

// openPRViaCredentialResolver implements services.OpenPRFn. Builds a
// scm.Provider for the connection, calls CreatePullRequest, and lifts
// the result into the orchestrator's OpenedPR shape. When the request
// carries a UserID (the run's triggering user, WI-275), credentials
// resolve per-user — on OAuth connections the PR is authored by that
// user; PAT / GitHub App connections resolve identically either way.
// permanentOpenPRErrors are the scm sentinel failures a retry can't fix: the
// request reached the provider and was refused (bad/expired credentials,
// forbidden, repo not found, a PR that already exists, an unsupported provider).
// Everything else — a timeout, a 5xx, a dropped connection, a rate-limit — is
// transient and left bare so AgentPRService's retry loop re-attempts it.
var permanentOpenPRErrors = []error{
	scm.ErrInvalidCredentials,
	scm.ErrNotAuthenticated,
	scm.ErrTokenExpired,
	scm.ErrRefreshTokenInvalid,
	scm.ErrForbidden,
	scm.ErrNotFound,
	scm.ErrAlreadyExists,
	scm.ErrUserSCMNotConnected,
	scm.ErrUnsupportedProvider,
}

// classifyOpenPRError wraps the scm errors that must not be retried so the
// AgentPRService retry loop surfaces them immediately; transient errors pass
// through unwrapped and stay retryable. ErrRateLimited is deliberately omitted
// from the permanent set — the retry loop's backoff is the right response to it.
func classifyOpenPRError(err error) error {
	for _, sentinel := range permanentOpenPRErrors {
		if errors.Is(err, sentinel) {
			return services.NewPermanentOpenPRError(err)
		}
	}
	return err
}

func openPRViaCredentialResolver(cr *scm.CredentialResolver) services.OpenPRFn {
	return func(ctx context.Context, req services.OpenPRRequest) (*services.OpenedPR, error) {
		var creds *scm.ProviderCredentials
		var err error
		if req.UserID > 0 {
			creds, err = cr.GetCredentialsForUser(ctx, req.ConnectionID, req.UserID)
		} else {
			creds, err = cr.GetCredentialsByConnectionID(ctx, req.ConnectionID)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve connection %d: %w", req.ConnectionID, err)
		}
		provider, err := cr.CreateProvider(creds)
		if err != nil {
			return nil, fmt.Errorf("build provider: %w", err)
		}
		pr, err := provider.CreatePullRequest(ctx, req.Owner, req.Repo, scm.CreatePROptions{
			Title:      req.Title,
			Body:       req.Body,
			HeadBranch: req.HeadBranch,
			BaseBranch: req.BaseBranch,
			Draft:      req.Draft,
		})
		if err != nil {
			return nil, classifyOpenPRError(err)
		}
		authorName := pr.Author.Username
		if authorName == "" {
			authorName = pr.Author.Name
		}
		return &services.OpenedPR{
			ID:     fmt.Sprintf("%d", pr.ID),
			Number: pr.Number,
			URL:    pr.URL,
			Title:  pr.Title,
			State:  pr.State,
			Author: authorName,
		}, nil
	}
}

// commentPRViaCredentialResolver implements services.CommentPRFn. Builds a
// scm.Provider for the connection and posts a comment on the PR via
// IssueCommentProvider.CreateIssueComment (a PR is an issue on both GitHub and Gitea).
// Credentials resolve per-user when a UserID is present (WI-275), matching the
// open-PR path. Returns an error if the provider lacks issue-comment support.
func commentPRViaCredentialResolver(cr *scm.CredentialResolver) services.CommentPRFn {
	return func(ctx context.Context, req services.PRCommentRequest) error {
		var creds *scm.ProviderCredentials
		var err error
		if req.UserID > 0 {
			creds, err = cr.GetCredentialsForUser(ctx, req.ConnectionID, req.UserID)
		} else {
			creds, err = cr.GetCredentialsByConnectionID(ctx, req.ConnectionID)
		}
		if err != nil {
			return fmt.Errorf("resolve connection %d: %w", req.ConnectionID, err)
		}
		provider, err := cr.CreateProvider(creds)
		if err != nil {
			return fmt.Errorf("build provider: %w", err)
		}
		issues, ok := provider.(scm.IssueCommentProvider)
		if !ok {
			return fmt.Errorf("provider %s does not support issue comments", creds.ProviderType)
		}
		_, err = issues.CreateIssueComment(ctx, req.Owner, req.Repo, req.Number, req.Body)
		return err
	}
}

// itemPRContinuationResolver implements services.ItemPRContinuationResolver: it
// finds an item's most-recently-updated open linked PR and resolves its head
// branch via the SCM provider, so the @mention trigger can land commits on that
// PR instead of opening a new one. Read-only, so it resolves connection-level
// credentials (no per-user principal needed just to read a PR head).
type itemPRContinuationResolver struct {
	db database.Database
	cr *scm.CredentialResolver
}

func (r *itemPRContinuationResolver) ContinuationForItem(ctx context.Context, itemID int) (*services.ContinuationTarget, error) {
	return r.ContinuationForItemAsUser(ctx, itemID, 0, nil)
}

func (r *itemPRContinuationResolver) ContinuationForItemAsUser(ctx context.Context, itemID, userID int, allowedRepoSlugs []string) (*services.ContinuationTarget, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT l.external_id, wr.repository_name, wr.workspace_scm_connection_id
		FROM item_scm_links l
		JOIN workspace_repositories wr ON l.workspace_repository_id = wr.id
		WHERE l.item_id = ? AND l.link_type = 'pull_request' AND lower(l.state) = 'open'
		ORDER BY l.updated_at DESC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("query open PR links: %w", err)
	}
	defer func() { _ = rows.Close() }()
	allowed := make(map[string]bool, len(allowedRepoSlugs))
	for _, slug := range allowedRepoSlugs {
		allowed[slug] = true
	}
	for rows.Next() {
		var externalID, repoName string
		var connectionID int
		if err := rows.Scan(&externalID, &repoName, &connectionID); err != nil {
			return nil, err
		}
		if len(allowed) > 0 && !allowed[repoName] {
			continue
		}
		number, err := strconv.Atoi(externalID)
		if err != nil {
			continue
		}
		owner, repo, ok := strings.Cut(repoName, "/")
		if !ok || owner == "" || repo == "" {
			continue
		}
		var creds *scm.ProviderCredentials
		if userID > 0 {
			creds, err = r.cr.GetCredentialsForUser(ctx, connectionID, userID)
		} else {
			creds, err = r.cr.GetCredentialsByConnectionID(ctx, connectionID)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve connection %d: %w", connectionID, err)
		}
		provider, err := r.cr.CreateProvider(creds)
		if err != nil {
			return nil, fmt.Errorf("build provider: %w", err)
		}
		pr, err := provider.GetPullRequest(ctx, owner, repo, number)
		if err != nil {
			return nil, fmt.Errorf("get PR %s/%s#%d: %w", owner, repo, number, err)
		}
		if pr.IsMerged || strings.EqualFold(pr.State, "closed") || pr.HeadBranch == "" {
			continue
		}
		return &services.ContinuationTarget{PRNumber: number, RepoSlug: repoName, HeadBranch: pr.HeadBranch}, nil
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// bootCodingAgentRunService configures token minting, PR hooks, and remote-run
// orchestration without an in-process worker. Errors leave the harness disabled.
func bootCodingAgentRunService(
	db database.Database,
	tm *auth.TokenManager,
	bindings *repository.WorkspaceAgentBindingRepository,
	cr *scm.CredentialResolver,
	initialPrompt string,
) (*services.RunService, error) {
	tokens, err := services.NewRunTokenService(tm)
	if err != nil {
		return nil, fmt.Errorf("coding-agent token service: %w", err)
	}

	// The post-run hook shares binding and SCM resolution with run startup.
	prSvc, err := services.NewAgentPRService(services.AgentPRServiceOptions{
		Bindings:  bindings,
		OpenPR:    openPRViaCredentialResolver(cr),
		CommentPR: commentPRViaCredentialResolver(cr),
		DB:        db,
	})
	if err != nil {
		return nil, fmt.Errorf("coding-agent pr service: %w", err)
	}

	runRepo := repository.NewAgentRunRepository(db)
	// Fail queued local runs orphaned by a previous in-process worker.
	if n, recErr := runRepo.ReapOrphanedLocalRuns(context.Background(), time.Now().UTC()); recErr != nil {
		slog.Warn("coding-agent: reconcile orphaned local runs",
			slog.String("component", "coding-agent"),
			slog.Any("error", recErr),
		)
	} else if n > 0 {
		slog.Info("coding-agent: failed local runs orphaned by a previous process",
			slog.String("component", "coding-agent"),
			slog.Int("count", n),
		)
	}
	runSvc, err := services.NewRunService(runRepo, services.RunServiceOptions{
		Tokens:        tokens,
		PostRunHook:   prSvc,
		InitialPrompt: initialPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("coding-agent run service: %w", err)
	}
	return runSvc, nil
}
