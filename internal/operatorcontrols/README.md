# Operator controls

Use `--operator-policy /run/windshift/operator-policy.json` or
`WINDSHIFT_OPERATOR_POLICY=/run/windshift/operator-policy.json` to load an
operator-owned JSON file at startup. The environment variable takes precedence.
Unknown fields, invalid capabilities and negative ceilings reject startup.
Omitting the file preserves self-managed behavior; zero ceilings add no limit.
Changes require a controlled restart. Do not mount this file writable by the
application or give organization administrators access to the process/container.

An example restrictive policy (numbers are illustrative, **not capacity claims**):

```json
{
  "deny": [
    "instance_lifecycle", "maintenance", "runtime_diagnostics", "plugin_exec",
    "ldap", "server_scripts", "imports", "generic_mail", "ai",
    "ai_connection_management"
  ],
  "limits": {
    "attachment_bytes": 10485760,
    "blob_bytes": 1073741824,
    "concurrent_requests": 16,
    "concurrent_streams": 4,
    "requests_per_minute": 600,
    "automation_steps_per_day": 10000,
    "mail_recipients_per_day": 1000
  },
  "mail": { "mode": "disabled" }
}
```

Continue to configure existing startup controls separately: disable plugins,
keep local/private network connections and TLS verification bypass disabled,
leave SSH/MCP/coding-agent execution opt-in, and set memory/cache, database
deadline, per-user admission, polling and outbox limits. This policy does not
replace those controls or turn workspaces into isolation boundaries. Each
customer instance retains its own application, database and storage boundary.

## Authority matrix

| Operation | Authority when restricted |
| --- | --- |
| Shutdown | Deployment/process supervisor; no tenant operator API |
| Rank migration, domain-event replay/skip, scheduler/webhook history purge | Operator maintenance only |
| Runtime cache/database/runner/scheduler/domain-event diagnostics | Operator only |
| Action logs, customer delivery errors, ordinary organization administration | Existing user permissions; not blanket disabled |
| Plugin CLI execution | Operator gate intersects persisted organization preference |
| LDAP writes, test and sync | Operator gate; existing records retained for migration |
| Script authoring/import/execution | Operator gate; read/remove definitions remains possible |
| Attachment preference | Organization may tighten operator per-object ceiling |
| Bulk imports | Operator gate and optional host admission |
| Mail transports and AI connections | Operator gate; normal permissions still required |

First-admin setup continues to use the existing one-time provisioning mechanism.
Operator controls do not introduce a second setup path or a privileged account.
Read effective controls at `/api/v2/operator-controls` (session) or
`/rest/api/v2/operator-controls` (bearer), requiring system administrator
permission. Bearer tokens additionally require `admin:operator-controls:read`.
The admin page and shell discovery never return relay credentials, storage
locations or the import admission directory. There is no policy write API.

## Accounting and failure behavior

- Request/stream concurrency is instance-wide and released on handler return,
  including cancellation/panic. Static assets, health and metrics are outside
  admission so deployment recovery remains possible. HTTP rate attempts use
  fixed UTC minutes; automation steps, mail recipient attempts and AI tokens use
  fixed UTC days. Counters persist in `operator_usage` on SQLite/PostgreSQL;
  restart does not grant a fresh allowance. Only the current interval is read;
  old counter rows may be removed by operator database maintenance.
- Automation reserves one unit before each dispatched node, including nested
  iterator work and asset flows. Exhaustion stops dispatch and records the
  failure in the execution trace. Already completed side effects are not rolled
  back. Durable event failures remain in the existing retry/terminal-delivery
  workflow; inspect before replaying partially completed effects.
- Blob quotas count current physical files/objects, including staged/orphaned
  objects, not attachment rows. Inventory is rebuilt after restart or ambiguous
  write/delete failure. Writes/deletes serialize within the store. The instance
  must exclusively own its local root or S3 prefix and be its **only writer**.
  S3 requires `ListObjectsV2`, `HeadObject`, `PutObject` and `DeleteObject` access.
  Versions, backups, database/WAL, import staging and temporary space are not a
  complete disk quota: enforce filesystem/container/object-store limits too.
- Denied capabilities return `403 operator_restricted`; storage ceilings return
  `413 operator_limit_exceeded`; work admission returns
  `429 operator_limit_exceeded`. HTTP overload includes `Retry-After: 60`.
  Unavailable required budget storage fails closed. Legacy adapters retain
  their legacy error envelope; v2 uses its standard envelope.

## Optional host import admission

Remove `imports` from `deny` and add:

```json
"imports": { "admission_directory": "/run/windshift-imports", "slots": 1, "wait_seconds": 300 }
```

Provision the same operator-owned **local host directory** into all participating
instances. Do not unlink/replace the slot files while any instance is running.
OS file locks release on process death; no shared application database is used.
This is not cross-host fencing and is not qualified for network filesystems.
The worker reports `admission_waiting`, holds a bounded job lease while waiting,
then either executes with a permit or reports `admission_denied`. At most one
pending import request per instance is admitted locally. Other bulk importers
are disabled when this admission policy is required, unless nested under an
already admitted Jira import. Fleet failover/placement remains external.
An injected `ImportAdmission` can read non-secret `ImportEstimate` inputs from
its context (project/schema counts, worker and storage ceilings). Total issue
and blob estimates are explicitly unknown until readiness has established them;
an external admission implementation must not interpret unknown totals as zero.

## Optional managed mail

`mail.mode` is `tenant`, `disabled` or `managed`; `suspended: true` stops sends.
Managed mode requires `host`, `port`, verified TLS `encryption` (`starttls`,
`tls` or `ssl`), fixed bare `from_address`, optional `from_name`, `username`
and `password`, plus `relay_abuse_controls: true`. That confirmation means the
operator has actually configured and qualified suppression, bounce and complaint
handling at the relay. The application cannot verify those external controls.
**Do not launch shared delivery until that qualification is complete.**
Tenant credentials cannot change managed transport or choose another sender.
Mail attempts share the daily recipient budget, including tests and auth mail;
there is no undocumented security-mail exemption. Failed/ambiguous attempts
remain charged. Existing outbox retry/diagnostic behavior remains in use;
operators must monitor delayed security mail and restore capacity deliberately.
`generic_mail` can be denied independently of approved OAuth mailbox intake.

## Optional AI

The safe unqualified default is to deny `ai` and `ai_connection_management`.
For qualified models, `ai` accepts `allowed_models`, `denied_features`,
`max_output_tokens` and `call_token_reservation`; `limits` accepts
`ai_tokens_per_day` and `concurrent_ai`. The reservation must cover the complete
qualified provider-call envelope, including retries, framing, media and hidden
tokens, and fit within the daily allowance. The application additionally bounds
serialized input and clamps output before upstream calls. A byte estimate is
**not** a universal tokenizer/media/pricing guarantee. Do not offer funded models
until their envelope has been qualified externally. Unknown or failed usage
retains the full reservation; known usage refunds only verified unused units.
Per-run broker quotas and conversation admission remain separate controls.

Timeout/concurrency ceilings for enabled scripts (`script_timeout_ms`,
`concurrent_scripts`) are not heap/process isolation. Keep untrusted scripts
disabled until a separately bounded worker design is available.
