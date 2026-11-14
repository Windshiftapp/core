// Package operatorcontrols enforces deployment-owned restrictions independently
// of organization settings and user permissions.
package operatorcontrols

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"slices"
)

type Capability string

const (
	Lifecycle          Capability = "instance_lifecycle"
	Maintenance        Capability = "maintenance"
	RuntimeDiagnostics Capability = "runtime_diagnostics"
	PluginExec         Capability = "plugin_exec"
	LDAP               Capability = "ldap"
	Scripts            Capability = "server_scripts"
	Imports            Capability = "imports"
	MailTransport      Capability = "mail_transport_management"
	GenericMail        Capability = "generic_mail"
	AI                 Capability = "ai"
	AIConnections      Capability = "ai_connection_management"
	MailSend           Capability = "mail"
)

var capabilities = []Capability{Lifecycle, Maintenance, RuntimeDiagnostics, PluginExec, LDAP, Scripts, Imports, MailTransport, GenericMail, AI, AIConnections, MailSend}

// Policy is operator input, not an application preference. Zero values preserve
// unrestricted self-managed behavior. Limits of zero mean no additional ceiling.
type Policy struct {
	Deny    []Capability  `json:"deny,omitempty"`
	Limits  Limits        `json:"limits,omitempty"`
	Mail    *MailPolicy   `json:"mail,omitempty"`
	AI      *AIPolicy     `json:"ai,omitempty"`
	Imports *ImportPolicy `json:"imports,omitempty"`
}

type ImportPolicy struct {
	AdmissionDirectory string `json:"admission_directory"`
	Slots              int    `json:"slots"`
	WaitSeconds        int    `json:"wait_seconds"`
}

type AIPolicy struct {
	AllowedModels   []string `json:"allowed_models,omitempty"`
	DeniedFeatures  []string `json:"denied_features,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	// CallTokenReservation must cover the qualified model's complete input and
	// output envelope. Unknown usage retains this full amount.
	CallTokenReservation int64 `json:"call_token_reservation,omitempty"`
}

// MailPolicy selects an operator relay responsible for suppression, bounce
// and complaint handling. Credentials never enter tenant persistence/discovery.
type MailPolicy struct {
	Mode               string `json:"mode"`
	Suspended          bool   `json:"suspended,omitempty"`
	Host               string `json:"host,omitempty"`
	Port               int    `json:"port,omitempty"`
	Encryption         string `json:"encryption,omitempty"`
	Username           string `json:"username,omitempty"`
	Password           string `json:"password,omitempty"`
	FromAddress        string `json:"from_address,omitempty"`
	FromName           string `json:"from_name,omitempty"`
	RelayAbuseControls bool   `json:"relay_abuse_controls,omitempty"`
}

type Limits struct {
	AttachmentBytes       int64 `json:"attachment_bytes,omitempty"`
	BlobBytes             int64 `json:"blob_bytes,omitempty"`
	ConcurrentRequests    int   `json:"concurrent_requests,omitempty"`
	ConcurrentStreams     int   `json:"concurrent_streams,omitempty"`
	RequestsPerMinute     int64 `json:"requests_per_minute,omitempty"`
	ScriptTimeoutMS       int   `json:"script_timeout_ms,omitempty"`
	ConcurrentScripts     int   `json:"concurrent_scripts,omitempty"`
	AutomationStepsPerDay int64 `json:"automation_steps_per_day,omitempty"`
	MailRecipientsPerDay  int64 `json:"mail_recipients_per_day,omitempty"`
	AITokensPerDay        int64 `json:"ai_tokens_per_day,omitempty"`
	ConcurrentAI          int   `json:"concurrent_ai,omitempty"`
}

func (p Policy) Validate() error {
	if p.Imports != nil && (p.Imports.AdmissionDirectory == "" || p.Imports.Slots < 1 || p.Imports.Slots > 64 || p.Imports.WaitSeconds < 1 || p.Imports.WaitSeconds > 3600) {
		return errors.New("import admission requires an operator directory, 1–64 slots and a 1–3600 second wait bound")
	}
	if p.AI != nil {
		if p.AI.MaxOutputTokens < 0 || p.AI.CallTokenReservation < 0 {
			return errors.New("AI operator bounds must not be negative")
		}
		for _, model := range p.AI.AllowedModels {
			if model == "" {
				return errors.New("operator AI model must not be empty")
			}
		}
	}
	if p.Limits.AITokensPerDay > 0 && (p.AI == nil || len(p.AI.AllowedModels) == 0 || p.AI.CallTokenReservation <= 0 || p.AI.MaxOutputTokens <= 0 || p.AI.CallTokenReservation > p.Limits.AITokensPerDay || p.AI.CallTokenReservation <= int64(p.AI.MaxOutputTokens) || p.AI.CallTokenReservation-int64(p.AI.MaxOutputTokens) <= 8192) {
		return errors.New("AI daily allowance requires an affordable qualified call reservation and output cap")
	}
	if p.Mail != nil {
		switch p.Mail.Mode {
		case "tenant", "disabled":
		case "managed":
			m := p.Mail
			if m.Host == "" || m.Port < 1 || m.Port > 65535 || (m.Encryption != "tls" && m.Encryption != "ssl" && m.Encryption != "starttls") {
				return errors.New("managed mail requires a host, port and verified TLS transport")
			}
			if !m.RelayAbuseControls {
				return errors.New("managed mail requires relay suppression, bounce and complaint controls")
			}
			address, err := mail.ParseAddress(m.FromAddress)
			if err != nil || address.Address != m.FromAddress {
				return errors.New("managed mail requires a fixed bare sender address")
			}
		default:
			return errors.New("invalid operator mail mode")
		}
	}
	for _, capability := range p.Deny {
		if !slices.Contains(capabilities, capability) {
			return fmt.Errorf("unknown operator capability %q", capability)
		}
	}
	limits := p.Limits
	for name, value := range map[string]int64{
		"attachment_bytes": limits.AttachmentBytes, "blob_bytes": limits.BlobBytes,
		"concurrent_requests": int64(limits.ConcurrentRequests), "concurrent_streams": int64(limits.ConcurrentStreams),
		"requests_per_minute": limits.RequestsPerMinute, "script_timeout_ms": int64(limits.ScriptTimeoutMS),
		"concurrent_scripts": int64(limits.ConcurrentScripts), "automation_steps_per_day": limits.AutomationStepsPerDay,
		"mail_recipients_per_day": limits.MailRecipientsPerDay, "ai_tokens_per_day": limits.AITokensPerDay,
		"concurrent_ai": int64(limits.ConcurrentAI),
	} {
		if value < 0 {
			return fmt.Errorf("operator limit %s must not be negative", name)
		}
	}
	return nil
}

// Load rejects unknown fields and trailing data so configuration typos fail closed.
func Load(path string) (Policy, error) {
	if path == "" {
		return Policy{}, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator-owned startup path
	if err != nil {
		return Policy{}, fmt.Errorf("read operator policy: %w", err)
	}
	if len(data) > 64*1024 {
		return Policy{}, errors.New("operator policy exceeds 64 KiB")
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return Policy{}, errors.New("operator policy must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("decode operator policy: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Policy{}, errors.New("operator policy contains trailing data")
	}
	return policy, policy.Validate()
}

type DeniedError struct{ Capability Capability }

func (e *DeniedError) Error() string {
	return fmt.Sprintf("%s is disabled by operator policy", e.Capability)
}

// From follows the instance's injected carrier rather than global process state.
func From(source any) *Controls {
	if carrier, ok := source.(interface{ OperatorControls() *Controls }); ok {
		return carrier.OperatorControls()
	}
	return nil
}
