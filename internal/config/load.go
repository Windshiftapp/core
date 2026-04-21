package config

import (
	"embed"
	"flag"
	"log/slog"
	"os"
	"strings"

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
		portFlag            = flag.String("port", "8080", "Port to run the HTTP server on")
		portShort           = flag.String("p", "8080", "Port to run the HTTP server on (shorthand)")
		dbPath              = flag.String("db", "windshift.db", "Database file path (SQLite)")
		postgresConn        = flag.String("postgres-connection-string", "", "PostgreSQL connection string")
		postgresConnShort   = flag.String("pg-conn", "", "PostgreSQL connection string (shorthand)")
		attachmentPath      = flag.String("attachment-path", "", "Path to store attachments")
		disableCSRF         = flag.Bool("no-csrf", false, "Disable CSRF protection (development only)")
		allowedHosts        = flag.String("allowed-hosts", "", "Comma-separated allowed hostnames for CSRF")
		allowedPort         = flag.String("allowed-port", "", "Port for CORS/WebAuthn trusted origins")
		useProxy            = flag.Bool("use-proxy", false, "Enable proxy mode (trust X-Forwarded-Proto from private IPs)")
		baseURL             = flag.String("base-url", "", "Public URL for the server")
		additionalProxies   = flag.String("additional-proxies", "", "Additional proxy IPs to trust")
		enableSSH           = flag.Bool("ssh", false, "Enable SSH TUI server")
		enableMCP           = flag.Bool("mcp", false, "Enable MCP server at /mcp")
		sshPort             = flag.String("ssh-port", "23234", "SSH server port")
		sshHost             = flag.String("ssh-host", "localhost", "SSH server host")
		sshKeyPath          = flag.String("ssh-key", ".ssh/windshift_host_key", "SSH host key file path")
		maxReadConns        = flag.Int("max-read-conns", 120, "Max read connections")
		maxWriteConns       = flag.Int("max-write-conns", 1, "Max write connections")
		logLevel            = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		logFormat           = flag.String("log-format", "text", "Log format (text, json, logfmt)")
		tlsCertPath         = flag.String("tls-cert", "", "TLS certificate file path")
		tlsKeyPath          = flag.String("tls-key", "", "TLS key file path")
		disablePlugins      = flag.Bool("disable-plugins", false, "Disable the plugin system")
		disableIPRateLimit  = flag.Bool("disable-ip-rate-limit", false, "Disable IP-based rate limiting")
		enableAdminFallback = flag.Bool("enable-fallback", false, "Enable admin password fallback")
		llmProvidersFile    = flag.String("llm-providers", "", "Path to custom LLM providers JSON file")
		aiPromptsDir        = flag.String("ai-prompts-dir", "", "Directory of custom AI prompt override files")
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

	// Booleans: flag OR env.
	sshEnabled := *enableSSH || parseBoolEnv("SSH_ENABLED")
	mcpEnabled := *enableMCP || parseBoolEnv("MCP_ENABLED")
	pluginsDisabled := *disablePlugins || parseBoolEnv("DISABLE_PLUGINS")
	ipRateLimitDisabled := *disableIPRateLimit || parseBoolEnv("DISABLE_IP_RATE_LIMIT")
	adminFallbackEnabled := *enableAdminFallback || parseBoolEnv("ENABLE_ADMIN_FALLBACK")

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

	// Auth: SSO_SECRET with SESSION_SECRET fallback. Fatal if both empty.
	sessionSecret := firstNonEmpty(os.Getenv("SSO_SECRET"), os.Getenv("SESSION_SECRET"))
	if sessionSecret == "" {
		slog.Error("FATAL: SSO_SECRET (or SESSION_SECRET) must be set for session signing and SSO credential encryption")
		os.Exit(1)
	}

	// WebAuthn: RPID falls back to hostname in production; dev mode override
	// happens inside the webauthn package (it knows devMode). RPName defaults
	// to "Windshift" when WEBAUTHN_RP_NAME is unset.
	rpID := os.Getenv("WEBAUTHN_RP_ID")
	if rpID == "" {
		if hostname, err := os.Hostname(); err == nil {
			rpID = hostname
		}
	}
	rpName := firstNonEmpty(os.Getenv("WEBAUTHN_RP_NAME"), "Windshift")

	return Config{
		Port:              port,
		BaseURL:           resolvedBaseURL,
		AllowedHosts:      resolvedAllowedHosts,
		AllowedPort:       *allowedPort,
		UseProxy:          proxyVal,
		UseProxyExplicit:  useProxyExplicit,
		AdditionalProxies: resolvedAdditionalProxies,
		TLSCertPath:       *tlsCertPath,
		TLSKeyPath:        *tlsKeyPath,
		DisableCSRF:       *disableCSRF,

		DB: DBConfig{
			PostgresConn:  pgConn,
			SQLitePath:    sqlitePath,
			MaxReadConns:  parseIntEnv("MAX_READ_CONNS", *maxReadConns),
			MaxWriteConns: parseIntEnv("MAX_WRITE_CONNS", *maxWriteConns),
		},
		SSH: SSHConfig{
			Enabled: sshEnabled,
			Host:    resolvedSSHHost,
			Port:    resolvedSSHPort,
			KeyPath: *sshKeyPath,
		},
		Auth: AuthConfig{
			SessionSecret: sessionSecret,
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
			Disabled:  pluginsDisabled,
			Dir:       resolvedPluginDir,
			ExtraDirs: extraPluginDirs,
		},
		LLM: LLMConfig{
			Endpoint:      os.Getenv("LLM_ENDPOINT"),
			ProvidersFile: resolvedLLMProviders,
			PromptsDir:    resolvedAIPromptsDir,
		},
		Logbook: LogbookConfig{
			Endpoint: os.Getenv("LOGBOOK_ENDPOINT"),
		},
		Notification: NotificationConfig{
			FlushInterval: parseDurationEnv("NOTIFICATION_FLUSH_INTERVAL", 0),
			BatchSize:     parseIntEnv("NOTIFICATION_BATCH_SIZE", 0),
			SyncInterval:  parseDurationEnv("NOTIFICATION_SYNC_INTERVAL", 0),
		},
		Jira: JiraConfig{
			CapturePayloadsDir: os.Getenv("JIRA_CAPTURE_PAYLOADS"),
		},

		AttachmentPath:      attachPath,
		EnableAdminFallback: adminFallbackEnabled,
		DisableIPRateLimit:  ipRateLimitDisabled,
		MCPEnabled:          mcpEnabled,
		RecoverUser:         os.Getenv("RECOVER_USER"),

		FrontendFiles: frontend,
		ShutdownChan:  shutdownChan,
	}
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
	}
}
