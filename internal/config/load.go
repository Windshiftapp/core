package config

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
)

// Load parses CLI flags (via the default flag.CommandLine), reads env vars,
// resolves all fallbacks and validates required values. It fatals (os.Exit(1))
// on missing required values so the rest of the app can treat every populated
// field as valid.
//
// The caller supplies the embedded frontend FS and a shutdown channel — these
// are wire-time values, not env-driven.
// last review: ser, 210426
func Load(frontend embed.FS, shutdownChan chan os.Signal) Config {
	// Flag definitions mirror the historic main.go flags verbatim.
	var (
		portFlag                    = flag.String("port", "8080", "Port to run the HTTP server on")
		portShort                   = flag.String("p", "8080", "Port to run the HTTP server on (shorthand)")
		dbPath                      = flag.String("db", "windshift.db", "Database file path (SQLite)")
		postgresConn                = flag.String("postgres-connection-string", "", "PostgreSQL connection string")
		postgresConnShort           = flag.String("pg-conn", "", "PostgreSQL connection string (shorthand)")
		attachmentPath              = flag.String("attachment-path", "", "Path to store attachments")
		disableCSRF                 = flag.Bool("no-csrf", false, "Disable CSRF protection (development only)")
		allowLocalConnections       = flag.Bool("allow-local-connections", true, "Allow server-side HTTP clients (SCM, Jira, LLM, webhooks) to reach loopback/private IPs; set to false to block local connections")
		allowedHosts                = flag.String("allowed-hosts", "", "Comma-separated allowed hostnames for CSRF")
		allowedPort                 = flag.String("allowed-port", "", "Port for CORS/WebAuthn trusted origins")
		useProxy                    = flag.Bool("use-proxy", false, "Enable proxy mode (trust X-Forwarded-Proto from private IPs)")
		allowInsecureHTTP           = flag.Bool("allow-insecure-http", false, "Allow browser access via plain http on non-localhost origins — trusted LANs/testing only")
		baseURL                     = flag.String("base-url", "", "Public URL for the server")
		contextPath                 = flag.String("context-path", "", "Public context path to serve Windshift under, e.g. /windshift")
		additionalProxies           = flag.String("additional-proxies", "", "Additional proxy IPs to trust")
		enableSSH                   = flag.Bool("ssh", false, "Enable SSH TUI server")
		enableMCP                   = flag.Bool("mcp", false, "Enable MCP server at /mcp")
		sshPort                     = flag.String("ssh-port", "23234", "SSH server port")
		sshHost                     = flag.String("ssh-host", "localhost", "SSH server host")
		sshKeyPath                  = flag.String("ssh-key", ".ssh/windshift_host_key", "SSH host key file path")
		maxReadConns                = flag.Int("max-read-conns", 30, "Max read connections (per pool; sum across pools × replicas must stay under Postgres max_connections)")
		postgresReplicaCount        = flag.Int("postgres-replica-count", 1, "Number of Windshift replicas sharing PostgreSQL for aggregate pool budgeting")
		postgresHeadroom            = flag.Int("postgres-connection-headroom", 10, "PostgreSQL connections reserved for migrations, administration, and other clients")
		maxUserConcurrency          = flag.Int("max-user-concurrency", 16, "Max simultaneous in-flight /api requests per authenticated user (0 disables)")
		maxTemplateSeedItems        = flag.Int("max-template-seed-items", 1000, "Maximum seed items copied when creating a workspace from a template")
		maxWriteConns               = flag.Int("max-write-conns", 1, "Max write connections")
		dbRequestTimeout            = flag.Duration("db-request-timeout", 12*time.Second, "Maximum database-work duration for ordinary HTTP requests")
		logLevel                    = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		logFormat                   = flag.String("log-format", "text", "Log format (text, json, logfmt)")
		tlsCertPath                 = flag.String("tls-cert", "", "TLS certificate file path")
		tlsKeyPath                  = flag.String("tls-key", "", "TLS key file path")
		disablePlugins              = flag.Bool("disable-plugins", false, "Disable the plugin system")
		disableIPRateLimit          = flag.Bool("disable-ip-rate-limit", false, "Disable IP-based rate limiting")
		enableAdminFallback         = flag.Bool("enable-fallback", false, "Enable admin password fallback")
		enableCodingAgent           = flag.Bool("enable-coding-agent", false, "Enable the coding-agent harness")
		activateDurableActions      = flag.Bool("activate-durable-actions", false, "Record the one-way cutover to canonical durable item actions")
		activateDurableAssetActions = flag.Bool("activate-durable-asset-actions", false, "Record the one-way cutover to canonical durable asset actions")
		llmProvidersFile            = flag.String("llm-providers", "", "Path to custom LLM providers JSON file")
		aiPromptsDir                = flag.String("ai-prompts-dir", "", "Directory of custom AI prompt override files")
		memoryLimitMB               = flag.Int("memory-limit-mb", DefaultMemoryLimitMB, "Total Windshift process memory budget in MiB")
	)
	flag.Parse()

	// Reconcile the shorthand flags with their long forms.
	port := *portFlag
	if *portShort != "" && *portShort != "8080" {
		port = *portShort
	}
	pgConn := *postgresConn
	if pgConn == "" {
		pgConn = *postgresConnShort
	}

	// Apply env overrides for every flag that supports env variants.
	port = firstNonEmpty(os.Getenv("PORT"), port)
	pgConn = firstNonEmpty(os.Getenv("POSTGRES_CONNECTION_STRING"), pgConn)
	sqlitePath := firstNonEmpty(os.Getenv("DB_PATH"), *dbPath)
	attachPath := firstNonEmpty(os.Getenv("ATTACHMENT_PATH"), *attachmentPath)
	resolvedLogLevel := firstNonEmpty(os.Getenv("LOG_LEVEL"), *logLevel)
	resolvedLogFormat := firstNonEmpty(os.Getenv("LOG_FORMAT"), *logFormat)

	// Postgres fallback via individual env vars, matching legacy behavior:
	// only trigger the builder when DB_TYPE=postgres and no conn string was
	// supplied directly.
	if pgConn == "" && os.Getenv("DB_TYPE") == "postgres" {
		pgConn = database.BuildPostgresConnString(postgresEnv())
	}

	resolvedAllowedHosts := *allowedHosts
	if resolvedAllowedHosts == "" {
		resolvedAllowedHosts = os.Getenv("ALLOWED_HOSTS")
	}

	resolvedBaseURL := *baseURL
	if resolvedBaseURL == "" {
		resolvedBaseURL = os.Getenv("BASE_URL")
	}

	resolvedContextPath := *contextPath
	if resolvedContextPath == "" {
		resolvedContextPath = os.Getenv("WINDSHIFT_CONTEXT_PATH")
	}
	resolvedContextPath = normalizeContextPath(resolvedContextPath)

	resolvedFormEmbedOrigins := os.Getenv("FORM_EMBED_ORIGINS")

	// Booleans: flag OR env.
	sshEnabled := *enableSSH || parseBoolEnv("SSH_ENABLED")
	mcpEnabled := *enableMCP || parseBoolEnv("MCP_ENABLED")
	pluginsDisabled := *disablePlugins || parseBoolEnv("DISABLE_PLUGINS")
	ipRateLimitDisabled := *disableIPRateLimit || parseBoolEnv("DISABLE_IP_RATE_LIMIT")
	adminFallbackEnabled := *enableAdminFallback || parseBoolEnv("ENABLE_ADMIN_FALLBACK")
	codingAgentEnabled := *enableCodingAgent || parseBoolEnv("CODING_AGENT_ENABLED")

	resolvedSSHPort := firstNonEmpty(os.Getenv("SSH_PORT"), *sshPort)
	resolvedSSHHost := firstNonEmpty(os.Getenv("SSH_HOST"), *sshHost)

	// Proxy: track explicit (flag OR env) because downstream auto-detection
	// treats "user said so" differently from "defaulted off".
	useProxyExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "use-proxy" {
			useProxyExplicit = true
		}
	})
	proxyVal := *useProxy
	if parseBoolEnv("USE_PROXY") {
		proxyVal = true
		useProxyExplicit = true
	}
	resolvedAdditionalProxies := firstNonEmpty(os.Getenv("ADDITIONAL_PROXIES"), *additionalProxies)

	resolvedPluginDir := firstNonEmpty(os.Getenv("PLUGIN_DIR"), "")
	licensePubKey := os.Getenv("PLUGIN_LICENSE_PUBKEY")
	var extraPluginDirs []string
	if envDirs := os.Getenv("PLUGIN_DIRS"); envDirs != "" {
		for _, dir := range strings.Split(envDirs, ",") {
			if dir = strings.TrimSpace(dir); dir != "" {
				extraPluginDirs = append(extraPluginDirs, dir)
			}
		}
	}

	resolvedLLMProviders := *llmProvidersFile
	if resolvedLLMProviders == "" {
		resolvedLLMProviders = os.Getenv("LLM_PROVIDERS_FILE")
	}
	resolvedAIPromptsDir := *aiPromptsDir
	if resolvedAIPromptsDir == "" {
		resolvedAIPromptsDir = os.Getenv("AI_PROMPTS_DIR")
	}
	resolvedAllowLocalConnections := *allowLocalConnections
	if os.Getenv("ALLOW_LOCAL_CONNECTIONS") != "" {
		resolvedAllowLocalConnections = parseBoolEnv("ALLOW_LOCAL_CONNECTIONS")
	}

	// Auth: SSO_SECRET with SESSION_SECRET fallback. Fatal if both empty.
	sessionSecret := firstNonEmpty(os.Getenv("SSO_SECRET"), os.Getenv("SESSION_SECRET"))
	if sessionSecret == "" {
		slog.Error("FATAL: SSO_SECRET (or SESSION_SECRET) must be set for session signing and SSO credential encryption")
		os.Exit(1)
	}

	// Session IP binding: validated here so a typo fails at startup rather than
	// silently weakening (or hardening) session validation at runtime.
	rawSessionIPBinding := os.Getenv("SESSION_IP_BINDING")
	resolvedSessionIPBinding, sessionIPBindingErr := resolveSessionIPBinding(rawSessionIPBinding)
	if sessionIPBindingErr != nil {
		slog.Error("FATAL: invalid SESSION_IP_BINDING",
			"session_ip_binding", rawSessionIPBinding, "error", sessionIPBindingErr)
		os.Exit(1)
	}

	// WebAuthn: use the browser-visible BASE_URL hostname by default. This is
	// especially important in containers, where os.Hostname() is normally an
	// internal container ID that browsers can never use as a relying-party ID.
	// Explicit WEBAUTHN_RP_ID remains available for split-host deployments. Accept
	// a full http(s) URL there as a convenience, but retain only its hostname: the
	// WebAuthn protocol requires an RP ID rather than an origin.
	rpID := resolveWebAuthnRPID(os.Getenv("WEBAUTHN_RP_ID"), resolvedBaseURL, os.Hostname)
	rpName := firstNonEmpty(os.Getenv("WEBAUTHN_RP_NAME"), "Windshift")

	// Web Push (VAPID). Both keys must be set to enable push; subject defaults
	// to the BaseURL so the VAPID JWT carries a valid contact URL.
	vapidSubject := firstNonEmpty(os.Getenv("VAPID_SUBJECT"), resolvedBaseURL)

	resolvedMemoryLimitMB := *memoryLimitMB
	if raw := os.Getenv("WINDSHIFT_MEMORY_LIMIT_MB"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			slog.Error("FATAL: WINDSHIFT_MEMORY_LIMIT_MB must be an integer number of MiB", "value", raw)
			os.Exit(1)
		}
		resolvedMemoryLimitMB = parsed
	}
	if _, memoryErr := ResolveMemoryBudget(resolvedMemoryLimitMB); memoryErr != nil {
		slog.Error("FATAL: invalid memory limit", "error", memoryErr)
		os.Exit(1)
	}

	return Config{
		Port:              port,
		BaseURL:           resolvedBaseURL,
		ContextPath:       resolvedContextPath,
		AllowedHosts:      resolvedAllowedHosts,
		AllowedPort:       *allowedPort,
		FormEmbedOrigins:  resolvedFormEmbedOrigins,
		UseProxy:          proxyVal,
		UseProxyExplicit:  useProxyExplicit,
		AdditionalProxies: resolvedAdditionalProxies,
		TLSCertPath:       *tlsCertPath,
		TLSKeyPath:        *tlsKeyPath,
		DisableCSRF:       *disableCSRF,
		AllowInsecureHTTP: *allowInsecureHTTP || parseBoolEnv("ALLOW_INSECURE_HTTP"),

		AllowLocalConnections: resolvedAllowLocalConnections,

		DB: DBConfig{
			PostgresConn:       pgConn,
			SQLitePath:         sqlitePath,
			MaxReadConns:       parseIntEnv("MAX_READ_CONNS", *maxReadConns),
			MaxWriteConns:      parseIntEnv("MAX_WRITE_CONNS", *maxWriteConns),
			ReplicaCount:       parseIntEnv("POSTGRES_REPLICA_COUNT", *postgresReplicaCount),
			ConnectionHeadroom: parseIntEnv("POSTGRES_CONNECTION_HEADROOM", *postgresHeadroom),
			RequestTimeout:     parseDurationEnv("DB_REQUEST_TIMEOUT", *dbRequestTimeout),
		},
		SSH: SSHConfig{
			Enabled: sshEnabled,
			Host:    resolvedSSHHost,
			Port:    resolvedSSHPort,
			KeyPath: *sshKeyPath,
		},
		Auth: AuthConfig{
			SessionSecret:             sessionSecret,
			SessionValidationCacheTTL: parseDurationEnv("SESSION_VALIDATION_CACHE_TTL", 5*time.Second),
			SessionIPBinding:          resolvedSessionIPBinding,
		},
		WebAuthn: WebAuthnConfig{
			RPID:   rpID,
			RPName: rpName,
		},
		Logging: LoggingConfig{
			Level:  resolvedLogLevel,
			Format: resolvedLogFormat,
		},
		Plugins: PluginsConfig{
			Disabled:      pluginsDisabled,
			Dir:           resolvedPluginDir,
			ExtraDirs:     extraPluginDirs,
			LicensePubKey: licensePubKey,
		},
		LLM: LLMConfig{
			Endpoint:      os.Getenv("LLM_ENDPOINT"),
			ProvidersFile: resolvedLLMProviders,
			PromptsDir:    resolvedAIPromptsDir,
		},
		CodingAgent: CodingAgentConfig{
			Enabled:  codingAgentEnabled,
			WSAPIURL: os.Getenv("CODING_AGENT_WS_API_URL"),
		},
		Logbook: LogbookConfig{
			Endpoint: os.Getenv("LOGBOOK_ENDPOINT"),
		},
		Notification: NotificationConfig{
			FlushInterval: parseDurationEnv("NOTIFICATION_FLUSH_INTERVAL", 0),
			BatchSize:     parseIntEnv("NOTIFICATION_BATCH_SIZE", 0),
			SyncInterval:  parseDurationEnv("NOTIFICATION_SYNC_INTERVAL", 0),
			BatchInterval: parseDurationEnv("WINDSHIFT_NOTIFICATION_BATCH_INTERVAL", 0),
		},
		OutboundTLS: OutboundTLSConfig{
			SkipVerify: parseBoolEnv("TLS_SKIP_VERIFY"),
		},
		Push: PushConfig{
			VAPIDPublicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
			VAPIDPrivateKey: os.Getenv("VAPID_PRIVATE_KEY"),
			VAPIDSubject:    vapidSubject,
		},
		Jira: JiraConfig{
			CapturePayloadsDir: os.Getenv("JIRA_CAPTURE_PAYLOADS"),
		},
		Memory: MemoryConfig{LimitMB: resolvedMemoryLimitMB},

		AttachmentPath:              attachPath,
		EnableAdminFallback:         adminFallbackEnabled,
		DisableIPRateLimit:          ipRateLimitDisabled,
		MaxUserConcurrency:          parseIntEnv("MAX_USER_CONCURRENCY", *maxUserConcurrency),
		MaxTemplateSeedItems:        parseIntEnv("MAX_TEMPLATE_SEED_ITEMS", *maxTemplateSeedItems),
		MCPEnabled:                  mcpEnabled,
		RecoverUser:                 os.Getenv("RECOVER_USER"),
		ActivateDurableActions:      *activateDurableActions || parseBoolEnv("WINDSHIFT_ACTIVATE_DURABLE_ACTIONS"),
		ActivateDurableAssetActions: *activateDurableAssetActions || parseBoolEnv("WINDSHIFT_ACTIVATE_DURABLE_ASSET_ACTIONS"),

		FrontendFiles: frontend,
		ShutdownChan:  shutdownChan,
	}
}

func resolveWebAuthnRPID(explicit, baseURL string, fallbackHostname func() (string, error)) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if hostname, ok := webAuthnRPIDHostnameFromURL(explicit); ok {
			return hostname
		}
		return explicit
	}

	if parsed, err := url.Parse(baseURL); err == nil &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Hostname() != "" {
		return parsed.Hostname()
	}

	if fallbackHostname != nil {
		if hostname, err := fallbackHostname(); err == nil {
			return hostname
		}
	}
	return ""
}

func webAuthnRPIDHostnameFromURL(value string) (string, bool) {
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", false
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return "", false
	}
	return parsed.Hostname(), true
}

// postgresEnv reads the POSTGRES_* family for use by database.BuildPostgresConnString.
// Kept here rather than in database/ so only this package touches env directly.
func postgresEnv() database.PostgresEnv {
	return database.PostgresEnv{
		Host:     firstNonEmpty(os.Getenv("POSTGRES_HOST"), "postgres"),
		Port:     firstNonEmpty(os.Getenv("POSTGRES_PORT"), "5432"),
		User:     firstNonEmpty(os.Getenv("POSTGRES_USER"), "windshift"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
		Database: firstNonEmpty(os.Getenv("POSTGRES_DB"), "windshift"),
		SSLMode:  postgresSSLMode(),
	}
}

// postgresSSLMode resolves POSTGRES_SSLMODE for the split-variable Postgres
// path. Validated here rather than left to the driver: a typo like "required"
// otherwise surfaces only as an opaque `pq: unsupported sslmode` at first
// connect, which reads like a server problem rather than a config one.
//
// Deployments using POSTGRES_CONNECTION_STRING put sslmode in the URL instead
// and never reach this.
func postgresSSLMode() string {
	raw := os.Getenv("POSTGRES_SSLMODE")
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return database.DefaultPostgresSSLMode
	}
	switch mode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return mode
	}
	slog.Error("FATAL: POSTGRES_SSLMODE must be one of disable, allow, prefer, require, verify-ca, verify-full",
		"sslmode", raw)
	os.Exit(1)
	return ""
}

// resolveSessionIPBinding validates SESSION_IP_BINDING and returns the mode to
// store in AuthConfig. It is pure — the os.Exit for an invalid value stays in
// Load — so the accepted/rejected sets are testable without a fatal path.
//
// Empty (unset, or whitespace only) resolves to DefaultSessionIPBinding rather
// than failing: the variable is optional.
func resolveSessionIPBinding(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return DefaultSessionIPBinding, nil
	}
	switch mode {
	case SessionIPBindingLog, SessionIPBindingStrict, SessionIPBindingOff:
		return mode, nil
	}
	return "", fmt.Errorf("SESSION_IP_BINDING must be one of %s, %s, %s; got %q",
		SessionIPBindingLog, SessionIPBindingStrict, SessionIPBindingOff, raw)
}

func normalizeContextPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	if p == "/" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasSuffix(p, "/") {
		slog.Error("FATAL: WINDSHIFT_CONTEXT_PATH / --context-path must be a non-root absolute path without a trailing slash", "context_path", raw)
		os.Exit(1)
	}
	if strings.ContainsAny(p, "?#\\") || strings.Contains(p, "//") {
		slog.Error("FATAL: WINDSHIFT_CONTEXT_PATH / --context-path must be a clean URL path", "context_path", raw)
		os.Exit(1)
	}
	decoded, err := url.PathUnescape(p)
	if err != nil || decoded != p || strings.Contains(decoded, "..") {
		slog.Error("FATAL: WINDSHIFT_CONTEXT_PATH / --context-path must not contain encoded bytes or traversal", "context_path", raw)
		os.Exit(1)
	}
	return p
}
