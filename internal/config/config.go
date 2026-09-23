package config

import (
	"embed"
	"os"
	"time"
)

// Config is the fully-resolved runtime configuration for the main windshift
// server. It is produced by Load (reading flags + env) and consumed by
// server.New and every handler that previously read env vars directly.
// last review: ser, 210426
type Config struct {
	// HTTP / network
	Port              string
	BaseURL           string
	ContextPath       string
	AllowedHosts      string
	AllowedPort       string
	FormEmbedOrigins  string
	UseProxy          bool
	UseProxyExplicit  bool
	AdditionalProxies string
	TLSCertPath       string
	TLSKeyPath        string
	DisableCSRF       bool

	// AllowInsecureHTTP permits credentialed browser access from plain-http
	// origins other than localhost (e.g. http://lanhost:8080). Without it,
	// such a BASE_URL fails CORS middleware construction, because credentialed
	// CORS over insecure origins is interceptable. For trusted LANs and
	// testing only; unlike UseProxy it does not affect forwarded-header trust.
	AllowInsecureHTTP bool

	// AllowLocalConnections, when true, lets every server-side SSRF-safe HTTP
	// client/dialer reach loopback and private/RFC1918 destinations. It is the
	// single switch operators flip to run self-hosted SCM (Gitea / GitHub
	// Enterprise), Jira Data Center, or a local LLM gateway on a private
	// network — instead of allowlisting each endpoint's CIDR. On by default;
	// set the corresponding flag or environment variable to false to restore
	// private-address blocking.
	AllowLocalConnections bool

	// Sub-configs grouped by concern
	DB           DBConfig
	SSH          SSHConfig
	Auth         AuthConfig
	WebAuthn     WebAuthnConfig
	Logging      LoggingConfig
	Plugins      PluginsConfig
	LLM          LLMConfig
	CodingAgent  CodingAgentConfig
	Logbook      LogbookConfig
	Notification NotificationConfig
	OutboundTLS  OutboundTLSConfig
	Push         PushConfig
	Jira         JiraConfig
	Memory       MemoryConfig

	// Flat fields (no logical grouping)
	AttachmentPath              string
	EnableAdminFallback         bool
	DisableIPRateLimit          bool
	MaxUserConcurrency          int
	MaxTemplateSeedItems        int
	MCPEnabled                  bool
	RecoverUser                 string
	ActivateDurableActions      bool
	ActivateDurableAssetActions bool

	// Wire-time values (not env/flag-driven)
	FrontendFiles embed.FS
	ShutdownChan  chan os.Signal
	SilentMode    bool
}

// MemoryConfig declares the intended total memory budget for one Windshift
// process. The runtime heap target and cache allocations are derived from it.
type MemoryConfig struct {
	LimitMB int
}

// DBConfig holds database connection + pool configuration.
type DBConfig struct {
	// PostgresConn, when non-empty, selects Postgres; otherwise SQLitePath is used.
	PostgresConn       string
	SQLitePath         string
	MaxReadConns       int
	MaxWriteConns      int
	ReplicaCount       int
	ConnectionHeadroom int
	// RequestTimeout bounds database work owned by an ordinary HTTP request.
	// It stays below the server's 30s write timeout so SQL is canceled before
	// the connection can be severed while database work continues.
	RequestTimeout time.Duration
}

// SSHDatabaseMaxConnections is the fixed per-process auxiliary SQL pool used
// for SSH authentication and session lookups.
const SSHDatabaseMaxConnections = 5

// SSHConfig holds the SSH TUI server configuration.
type SSHConfig struct {
	Enabled bool
	Host    string
	Port    string
	KeyPath string
}

// Session IP-binding modes, resolved from SESSION_IP_BINDING into
// AuthConfig.SessionIPBinding. Deployments behind a load balancer with a
// changing egress IP, or with mobile clients that roam between networks, need
// a weaker binding than one serving a fixed office range.
const (
	// SessionIPBindingLog records a client-IP change on a session and serves
	// the request anyway. It is the default so the mismatch rate is observable
	// before an operator switches to strict.
	SessionIPBindingLog = "log"
	// SessionIPBindingStrict rejects a session presented from a client IP
	// other than the one it was created from.
	SessionIPBindingStrict = "strict"
	// SessionIPBindingOff skips the client-IP comparison entirely.
	SessionIPBindingOff = "off"

	// DefaultSessionIPBinding applies when SESSION_IP_BINDING is unset or empty.
	DefaultSessionIPBinding = SessionIPBindingLog
)

// AuthConfig holds session-signing / SSO credential-encryption secrets.
type AuthConfig struct {
	// SessionSecret is resolved from SSO_SECRET (preferred) with fallback to
	// SESSION_SECRET. Empty means neither env var was set — Load will fatal
	// before populating this field, so consumers can treat empty as unreachable.
	SessionSecret string
	// SessionValidationCacheTTL bounds local session/user-state staleness. Zero
	// disables retained validation entries; the default is five seconds.
	SessionValidationCacheTTL time.Duration
	// SessionIPBinding selects how a client-IP change on an existing session is
	// handled: SessionIPBindingLog, SessionIPBindingStrict or
	// SessionIPBindingOff. Load validates it, so consumers can treat any other
	// value as unreachable.
	SessionIPBinding string
}

// WebAuthnConfig holds WebAuthn relying-party identity.
type WebAuthnConfig struct {
	// RPID is the relying party ID (usually the hostname). In production it is
	// resolved from WEBAUTHN_RP_ID, then the BASE_URL hostname, with
	// os.Hostname() as a final fallback. In development the webauthn package
	// overrides this to "localhost".
	RPID string
	// RPName is the displayed RP name; defaults to "Windshift".
	RPName string
}

// LoggingConfig holds logger initialization parameters.
type LoggingConfig struct {
	Level  string // debug, info, warn, error
	Format string // text, json, logfmt
}

// PluginsConfig holds plugin-system configuration.
type PluginsConfig struct {
	Disabled  bool
	Dir       string
	ExtraDirs []string // from PLUGIN_DIRS (comma-separated)
	// LicensePubKey is the base64 Ed25519 public key that plugin license
	// tokens must verify against. Empty disables license enforcement.
	LicensePubKey string
}

// LLMConfig holds LLM-related configuration.
type LLMConfig struct {
	Endpoint      string
	ProvidersFile string
	PromptsDir    string
}

// CodingAgentConfig configures the coding-agent harness (WI-89). The harness is
// opt-in: set --enable-coding-agent (or CODING_AGENT_ENABLED=true) to activate
// it. When enabled, the server wires the orchestration surface — the
// BindingService trigger that queues runs, the /runner/* control plane that
// remote runner pools register/claim against, and the post-run PR hook — but it
// does NOT execute agents on the orchestrator host. All runs are dispatched to
// remote runner pools (windshift-runner); there is no in-process LLM loop.
//
// The agent reaches the model only through the llm-proxy broker, so no
// provider key or provider selection is injected; it needs only an
// LLM_BASE_URL (set per-run to the run-scoped proxy) and a model id, both
// derived from the binding's LLM connection at claim time.
type CodingAgentConfig struct {
	Enabled  bool
	WSAPIURL string // URL the runner reaches this Windshift API on; defaults to BASE_URL
}

// LogbookConfig holds the URL of the logbook sidecar (if any).
type LogbookConfig struct {
	Endpoint string
}

// NotificationConfig holds the notification WriteBatcher tuning knobs.
type NotificationConfig struct {
	FlushInterval time.Duration
	BatchSize     int
	SyncInterval  time.Duration
	// BatchInterval is the email-batch scheduler cadence
	// (WINDSHIFT_NOTIFICATION_BATCH_INTERVAL). Zero means the scheduler uses
	// its built-in default.
	BatchInterval time.Duration
}

// OutboundTLSConfig controls process-wide certificate verification for
// outbound TLS connections.
type OutboundTLSConfig struct {
	// SkipVerify permits self-signed or otherwise untrusted certificates. It is
	// intentionally disabled by default because enabling it removes server
	// identity verification from outbound TLS connections.
	SkipVerify bool
}

// PushConfig holds Web Push (VAPID) configuration. Push is enabled only when
// both keys are present; otherwise the push endpoints and dispatch are no-ops
// and the mobile UI hides the enable-notifications affordance.
type PushConfig struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	// VAPIDSubject is the "sub" claim in the VAPID JWT — a mailto: or https:
	// contact URL for the push service operator. Defaults to the BaseURL.
	VAPIDSubject string
}

// Enabled reports whether Web Push can be dispatched (both keys configured).
func (p PushConfig) Enabled() bool {
	return p.VAPIDPublicKey != "" && p.VAPIDPrivateKey != ""
}

// JiraConfig holds Jira-related runtime options.
type JiraConfig struct {
	// CapturePayloadsDir, when non-empty, makes the Jira import path write
	// request/response payloads to this directory for debugging.
	CapturePayloadsDir string
}

// LogbookSidecarConfig is the sibling of Config for the standalone logbook
// sidecar binary (cmd/logbook/main.go). The sidecar never parses CLI flags
// and has a narrower set of concerns than the main server.
type LogbookSidecarConfig struct {
	PostgresConn           string
	Port                   string
	StoragePath            string
	LLMEndpoint            string
	ArticleEndpoint        string
	MainServerURL          string
	MainServerSecret       string
	BaseURL                string
	ActivateDurableActions bool
	Logging                LoggingConfig
	OutboundTLS            OutboundTLSConfig
}
