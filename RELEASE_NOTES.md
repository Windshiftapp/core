# Windshift v0.6.2

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.

---

This release opens up two new ways to deploy and one new way to sign in. **Postgres** is now a first-class backend — the entire test suite passes on both engines, and the schema and CQL layer have been reworked for portability. **Portal customers** can register passkeys and sign in without an emailed magic link. The desktop app ships as a **signed macOS DMG** alongside the existing tarballs.

Items now belong to **multiple milestones** via a proper join table, **configuration sets** can be exported and imported as portable JSON bundles, and request types support a **title template** for portal submissions where the title field is hidden from the customer.

Underneath, there is a wide cleanup pass on long-running goroutines, request-context propagation, and the boundary between the database layer and domain packages.

## Features

### Postgres backend

Windshift can now be deployed against Postgres in addition to SQLite. The same schema runs on both engines, and the e2e suite passes on either driver. The work that made this real:

- **Forward FK ordering.** Schema files used to declare foreign keys to tables defined later in the load order. SQLite tolerates this; Postgres rejects it at `CREATE TABLE` time. The cross-section FKs are now added via late `ALTER` blocks guarded by `pg_constraint` checks, so a clean install and a rerun both succeed.
- **Boolean literals.** Inline `0`/`1` in `INSERT`/`UPDATE`/`WHERE` against `BOOLEAN` columns is replaced with `true`/`false` across user, SSO, LDAP, approvals, on-call, leave, planning, custom-field, permission-set, audit-log, and team paths. `lib/pq` refused the integer form; SQLite continues to accept both.
- **Auto-generated IDs.** Sites that used `Exec(...).LastInsertId()` for sequence assignment now use `QueryRow(... RETURNING id)`. Approval, approval-set, condition-set, logbook-action, workflow, and workspace-role handlers were silently storing `0` against Postgres and breaking downstream FKs.
- **JSONB nullability.** Two write paths (`approval_repository.WriteDecision`, `auth_policy` audit insert) used to write `string(nil)` (`""`) into JSONB columns when there was no payload. Postgres rejected the empty string as invalid JSON. Both callers now pass a Go `nil` interface, which maps to SQL `NULL`; the read path switched from `COALESCE(..., '')` to a nullable scan.
- **CQL milestone migration.** With items now linked to milestones through `item_milestones`, the CQL generator gained `EXISTS`-subquery handlers for `milestone[_id|name]` comparisons (mirroring the existing label handling). Inner queries in `childrenOf` / `linkedOf` and consumers in `workspace_repository` / `briefing_scheduler` follow the same shape. The stale `i.milestone_id` column was also dropped from the link-repository `SELECT`.
- **`cf_x != y` semantics.** Reverted a WIP rewrite that treated NULL custom fields as not-equal-to-anything. Standard SQL null semantics now apply consistently on both engines.
- **Streaming-tx safety.** `test_run_repository.CreateResultsFromSet` previously ran `tx.Exec` inside a `rows.Next()` loop on the same transaction. `lib/pq` pins a transaction to one connection and refuses new statements while a `Rows` cursor is open. Test-case IDs are now drained into a slice before the insert loop.

### Portal passwordless sign-in (passkeys / WebAuthn)

Portal customers can register passkeys and sign in without an emailed magic link.

- **Discoverable login.** The customer does not enter an email — the authenticator hands back a `userHandle`, which the resolver maps to a `portal_customer` and checks against the channel access list before letting the WebAuthn library finish the assertion.
- New endpoints under `/portal/{slug}/auth/webauthn/login/{start,complete}` for sign-in and `/portal/{slug}/credentials/webauthn/*` for credential management.
- New `PortalProfile` and `PortalPasskeyBanner` components, a "Sign in with passkey" button on the login modal, and a "Profile & security" link in the portal header. The banner can be dismissed; the dismissal is tracked per customer.
- New tables: `portal_webauthn_credentials`, `portal_webauthn_sessions`. `portal_customers` gains `dismissed_passkey_prompt_at`.
- The WebAuthn config picks up the actual server port at runtime, so the e2e suite can run against a random port without extra origin entries.

### Items can belong to multiple milestones

`items.milestone_id` has been replaced by an `item_milestones` join table. The migration is idempotent — startup runs an `INSERT OR IGNORE` backfill from the existing column, then `ALTER TABLE DROP COLUMN` once the data has moved.

- **Backend.** Item DTOs, services, repositories, validators, and history switch from `milestone_id (*int)` to `milestones ([]Milestone)` / `milestone_ids`. A new `milestone_attach_repository` handles the set-replace semantics used by create/update. AI tools and SCM integrations follow the same shape. OpenAPI spec regenerated.
- **Frontend.** `WorkItemForm` and the inline milestone picker are now multi-select. `ListCellRenderer`, `ItemDetailSidebar`, and `CollectionBoard` render the set with overflow as "+N" beyond the first chip. The legacy `itemUpdateService` is removed; the form store talks to the API directly.
- The `ws` CLI item payloads switch from `milestone_id` to `milestone_ids` to match the multi-milestone REST surface.

### Configuration set export and import

`GET /configuration-sets/{id}/export` and `POST /configuration-sets/import` round-trip a configuration set as a self-contained JSON bundle.

The bundle carries the workflow, condition set, approval set, and linked item types / priorities / screens, plus the full definitions of every custom field referenced anywhere in the set. References between sections are by name (or email for users), so a bundle exported from one instance can be imported into another. Statuses, item types, priorities, and custom fields match by name and are created if missing; workflow / condition / approval / configuration sets are always created fresh.

Identity references (roles, groups, users, status categories) must already exist on the target — the importer surfaces every unresolved one in a single 422 response so an operator can fix them in one pass.

**Default-entity protection.** The default configuration set cannot be exported (403 `default_not_exportable`); imports whose top-level set or any embedded workflow name collides with an `is_default=true` row are refused with 409 `default_entity_conflict`. Statuses / item types / etc. are unaffected because the importer reuses them by name without modification.

### Title templates for hidden-title portal requests

Request types now carry a `title_template` column. When the title field is hidden from the customer-facing portal form, the template is rendered server-side using `{{var}}` placeholders resolved against the submitted virtual fields. The template runs through a new `internal/services/template` substitution engine, and it is wired through the portal, email, and form channel configurators. The fields builder has been split out of `RequestTypeModal` into its own component for reuse across the configurators.

### Virtual fields on the item detail view

Virtual field values collected via request portal / form submissions were being written to `items.virtual_field_data` but never read back — `FindByIDWithWorkspaceStatus` omitted the column from its `SELECT`, and `ItemDetailSidebar` had no rendering branch. The single-item query now selects the column, the item-detail store fetches request type field metadata, and a read-only **Request form fields** section renders below Custom Fields.

### Diagram tools for AI / MCP and mermaid seeding

The aitools registry gains diagram CRUD (`list`, `get`, `create`, `update`, `delete`), and a tiny seed format lets an agent or CLI attach a diagram without bundling mermaid-to-Excalidraw conversion server-side:

```
{"type":"mermaid","source":"graph TD; A-->B"}
```

`DiagramModal` detects the wrapper on open, lazy-loads the `@excalidraw/mermaid-to-excalidraw` + `@excalidraw/excalidraw` bundles, and uses the converted scene as the editor's initial data. Saving replaces the wrapper with the full Excalidraw scene; the source string is not preserved, matching how Excalidraw's own mermaid panel behaves.

The `ws` CLI gains a matching `diagram` subcommand (`create` / `list` / `get` / `update` / `delete`) that uses the same seed format.

### `ws` CLI moved into `internal/wscli`

`cmd/ws/` shrinks to a thin wrapper that calls `wscli.Run`. The package move makes it possible to drive the CLI in-process from tests, adds a flag-state reset hook so sequential test invocations do not bleed into each other, and introduces a `WS_DEBUG_HTTP` switch for triaging server errors.

### macOS desktop app distributed as a signed DMG

`release.sh` now copies the existing `darwin/arm64` server and `ws` Go binaries into the desktop Tauri tree (renamed to the `aarch64-apple-darwin` triple), patches `tauri.conf.json` with the release version, and runs `cargo tauri build` to produce a DMG. The DMG is signed when `APPLE_*` env vars are present, dropped into `dist/releases` alongside the existing tarballs, and picked up by `gh release create`. A new `--skip-desktop` flag and a graceful no-op on non-Mac hosts keep CI on Linux unaffected.

## Bug fixes

### Portal auth + item-move error message

A regression that prevented portal customers from authenticating in some configurations is fixed, and item-move failures now surface the underlying server error to the user instead of a generic message.

### Agent auth flow scopes

The agent OAuth flow was missing required scopes for some tools, leaving newly minted agents unable to call endpoints their tools advertised. The required scopes are now requested during the flow; an existing portal-toggle bug introduced by the same path is also fixed.

### Inactive workspaces leaking into selectors

Inactive workspaces were appearing in the sidebar workspace switcher and homepage Quick Access widget for users with admin-style access, and in Recent Workspaces for anyone who had previously visited one. The filter now runs at the consumption sites, so **Manage Workspaces** still lists them with the Inactive lozenge.

### Profile labels manager scoped to personal labels

Profile → Labels used to be a dual-purpose surface that could mint shared labels (`user_id NULL`, visible to everyone) — a destructive editor in front of every user. The manager now hides the "share with everyone" toggle, filters the list to personal labels, and forces newly created labels to be personal. The picker on items still surfaces shared labels for selection; only this manager drops them.

### Theme listener for system colour scheme

The theme store is a process-wide singleton initialized from `App.svelte`'s `onMount`, outside any component-init scope. `runed`'s `useEventListener` requires that scope and silently no-ops here, so the system-theme-change listener was not firing. It now uses a plain `mediaQuery.addEventListener` whose lifetime matches the page, which is what a singleton wants.

### Markdown sanitiser regex (balanced parens)

`dangerousMarkdownURLRegex`'s URL body used `[^)]*`, stopping at the first `)` inside payloads like `javascript:alert(1)` and leaving the markdown link's closing `)` as residue in the sanitised output. The regex now allows one level of balanced parens in the URL body. Affects `SanitizeMarkdownURLs` / `SanitizeDescription` / `SanitizeCommentContent`.

## Reliability and hardening

A focused pass on long-running goroutines, request-context propagation, and audit-write correctness:

- **LDAP sync supervised.** `TriggerSync` previously spawned a bare goroutine: no `recover` (a panic in the LDAP client crashed the whole server), no `WaitGroup` (shutdown abandoned in-flight syncs), and the inner `SyncUsers` took no context. There is now a per-handler `WaitGroup` + stop channel, a `Stop(ctx)` method called from `Server.Shutdown`, and `QueryContext`-routed DB lookups inside the sync.
- **Logbook ingestion cancels cleanly.** `IngestFile` and `ReprocessDocument` run a sequence of expensive steps (extract → classify → article → chunk) where a cancel between steps would silently start the next one. A new `abortIfCanceled` check runs at each boundary; on cancel the document is marked errored with `canceled at <stage>: <reason>` so it surfaces in the UI instead of stuck in `processing`.
- **Rate-limit cleanup stoppable.** `time.Ticker.Stop` does not close the channel, so `for-range` on `cleanupTicker.C` blocked forever and `startCleanupLoop` leaked for the process lifetime. A `cleanupDone` channel and a `sync.Once`-guarded `Stop` make double-stop safe and let shutdown drain.
- **SAML audit log synchronous.** The fire-and-forget goroutine in the SAML callback dropped errors, had no `recover`, and no timeout — audit rows were silently lost on shutdown or DB blips. `LogAudit` now runs inline; failures are logged with `slog.Warn` so operators see the loss.
- **Request context through admin paths.** `admin_rate_limiter` (`RecordAttempt`, `IsAllowed`) and the channel-management / setup-not-complete checks in `middleware/permissions` now accept `context.Context` and route their DB calls through the `*Context` variants. The four call sites pass `r.Context()` so a client disconnect cancels the lookups instead of orphaning them on the connection pool.
- **`errors.Is` sweep.** 359 bare `== sql.ErrNoRows` comparisons across `internal/` were converted to `errors.Is` via codemod. The previous form happened to work because the driver returns the bare sentinel, but it breaks the moment any layer wraps the error.
- **Session sentinels via `errors.New`.** The auth package's session sentinels were declared with `errors.New` instead of `fmt.Errorf("...")` so they round-trip through `errors.Is` cleanly.

## Refactor / internal

Cuts in the cross-package coupling between the DB layer and domain packages:

- **`internal/logbookapi` split out of `internal/logbook`.** The logbook domain package used to import `internal/restapi` via its HTTP handlers — inverted layering flagged in review. Every HTTP-shaped file moves to a new `internal/logbookapi/` package; `internal/logbook/` is now seven pure-domain files (action_service, ingestion, permission, repository, schema, schema/, thumbnail) with zero HTTP imports.
- **`emailutil` dependency inverted.** `seedDefaultEmailTemplates` lived in `internal/database/` and imported `internal/emailutil/` for the template list — DB layer reaching into a domain helper. Moved to `internal/emailutil/seed.go` as `SeedTemplates(db)` and called from the server bootstrap right after `Initialize`.
- **Workspace item-sequence ops moved to `WorkspaceRepository`.** `CreateWorkspaceItemSequence` / `DropWorkspaceItemSequence` are now repo methods that branch on the driver name; the unused `NextWorkspaceItemNumber` method on the `Database` interface (which had zero callers) is dropped outright in favour of the existing `ItemRepository.GetNextWorkspaceItemNumber`.
- **`notification_settings.EnsureDefault` moved to its repository.** Same SQL was implemented twice on `SQLiteDB` and `PostgresDB` with placeholder-style differences only the rebinder cares about.
- **`MigrateSelectFieldOptions` off the `Database` interface.** The two impl methods just delegated to the package-level migration helper; exported as `database.MigrateSelectFieldOptions` and called directly from `server.go`.

Frontend cleanups in the same spirit:

- **`@lucide/svelte` + svelte-check.** The deprecated `lucide-svelte` package is swapped for `@lucide/svelte`, `svelte-check` is added as a dev dependency, and JSDoc type annotations are applied across components, dialogs, pickers, editors, settings, widgets, and pages so the type-check pass succeeds.
- **`runed` primitives.** Hand-rolled `addEventListener` / `matchMedia` / `ResizeObserver` / `setTimeout` wiring is replaced with `useEventListener`, `onClickOutside`, `useResizeObserver`, and `useDebounce` across ~10 files. Pointer-drag handlers use a getter target so the listener attaches and detaches reactively with the drag state, removing the manual `addEventListener` / `removeEventListener` pairs in `pointerdown` / `up`. Workspace layout auto-save uses `useDebounce` instead of a custom `saveTimeout` + `clearTimeout`.

## Upgrade notes

- **Items are migrated from `milestone_id` to `item_milestones`.** The backfill runs idempotently at startup (`INSERT OR IGNORE`), then drops the legacy column. Take a backup before upgrading. AI tool callers and the `ws` CLI must use `milestone_ids` going forward; the old `milestone_id` field is removed from item create/update payloads.
- **Postgres deployments are now supported.** The schema runs against either engine; if you have a long-running SQLite deployment and want to move, do it from a stopped backup rather than live, as there is no online migration path between engines. Run the test suite against your target driver before switching.
- **Default configuration sets cannot be exported or overwritten on import.** Calls to export against the default set return 403; an import whose top-level set or any embedded workflow collides with an existing `is_default=true` row returns 409 with `default_entity_conflict`.
- **Portal customers can register passkeys.** The banner prompts customers to enrol after a successful magic-link sign-in; dismissals are persisted per-customer. No operator action is needed to enable the feature, but the `portal_webauthn_*` tables are created on first start.
- **macOS desktop DMG.** Release assets now include `Windshift-vX.Y.Z-macos-arm64.dmg`. Existing tarball assets are unchanged. Code-signing requires `APPLE_*` env vars at release time; unsigned builds still produce a DMG.
- **No SQLite schema migrations beyond the milestone-id move.** Other schema changes (passkey tables, request-type `title_template`, virtual-field projection) ship as additive columns or new tables.

---

# Windshift v0.6.1

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.

---

A point release covering approval workflow fixes, surfacing approval activity in the item Comments and History tabs, attachment previews with a lightbox, consolidation of the in-app and MCP AI tool surfaces, and a permission tightening for MCP.

## Features

### Approval activity in Comments and History

Approval decisions and any comments attached to them now appear inline with regular comments and history.

- The History tab lists each approval decision (`requested`, `approve`, `reject`, `cancel`, `comment`, `delegate`, `escalate`, `completed`) in chronological order alongside field changes. The tab uses a one-line per entry layout with a smaller avatar and the full timestamp on hover.
- The Comments tab includes the comment text from approve, reject, comment, and cancel decisions. Approval-sourced rows show a Shield icon. Comments authored by users with `is_agent = true` show a Bot icon. Both icons have tooltips. Edit and delete are restricted to rows authored by a human.
- AI chat has a new `get_item_approvals` tool that returns the request status, the approver pool, and the decision audit trail with user names resolved.

### Attachment previews and lightbox

Image attachments render a 40x40 thumbnail using the existing `/api/attachments/{id}/thumbnail` endpoint. PDFs render a 40x40 tile labeled `PDF`. Clicking either opens a fullscreen lightbox: images at full resolution, PDFs in the browser's built-in viewer via iframe. Backdrop click, the close button, or the Escape key dismisses the lightbox.

### Approval card UX

Approve, Reject, and Cancel use the existing `ConfirmDialog` component instead of `window.confirm`. The decision buttons sit above the optional comment box and wrap on narrow sidebars. After a decision is recorded, the item is reloaded so `status_id` reflects the new server-side state.

### AI tool registry consolidation

The in-app LLM agent and the external MCP server now share a single tool registry under `internal/aitools/`. Each tool is defined once with a typed Args struct and a `Run` function. The two adapters translate that into their respective protocol shapes. Schemas are derived once at registration via `google/jsonschema-go`.

Tools added to the MCP surface in this release: `get_item_approvals`, `transition_item`, `list_milestones`, `list_iterations`, `list_custom_fields`, `list_recent_activity`, and `log_time` (string durations such as `"1h30m"`).

Tools added to the in-app LLM agent: `create_item`, `delete_item`, `get_item_children`, `list_labels`, `set_item_labels`, `start_timer`, `stop_timer`.

## Bug fixes

### On-leave approver with no substitute

When an approval step used `on_leave_strategy = use_substitute` and the only resolved approver was on an active leave without a configured `substitute_user_id`, the engine dropped the approver and snapshotted an empty pool. The request opened but no one could act on it. The engine now keeps the original approver in the pool when no substitute is configured. Existing stuck requests can be unstuck by cancelling them and re-triggering the transition.

## Security hardening

### MCP applies item.view per workspace

The MCP adapter built its workspace access list via `repository.GetAccessibleWorkspaceIDs`, which returns every active non-personal workspace unconditionally. For workspaces with explicit role assignments, a non-member would fail `permission_service.HasWorkspacePermission(item.view)` but the workspace was still in the list. Registry-driven read tools (`get_item`, `search_items`, `get_item_children`, `list_comments`, `get_item_approvals`) returned data the deleted MCP per-family handlers used to block via `canViewItem`.

The MCP env now lists active workspaces and keeps only those where `HasWorkspacePermission(item.view)` succeeds. Bearer tokens with `mcp:access` scope can no longer read items, comments, or approvals from gated workspaces they are not a member of.

## Upgrade notes

- No schema migrations and no config changes.
- MCP `tools/list` advertises additional tools. Existing clients are unaffected.

---

# Windshift v0.6.0 — "Formation"

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

This release is about organising the people who do the work. **Teams** ships its full UI on top of the backend that landed late in 0.5.x — on-call schedules, rotation layers, manual overrides, swap requests, and per-team identity. **OAuth 2.0** with authorisation-code-plus-PKCE turns Windshift into a proper identity provider for third-party integrations. **Labels** become first-class for personal and shared use. Underneath, the codebase gets its largest cleanup pass to date.

## Features

### Teams and on-call

The Teams backend that landed in 0.5.x is now wired end-to-end. `/teams` is a routed page in the main nav with list and detail views, gated by the `teams.manage` global permission and a per-team admin role.

The detail view is a tabbed shell:

- **Overview** — inline-edit name and description, plus an Identity card with icon picker, colour, and optional avatar upload.
- **Members** — direct members with role select, a `UserPicker` for staged adds, and a resolved-members table that flags who is on leave and surfaces their substitute.
- **Groups** — mapped groups with `GroupPicker` for staged attachments. Member counts now reflect direct + group-resolved correctly.
- **On-call** — full schedule CRUD with rotation layers, manual overrides, swap requests, and a "Currently on-call" card per schedule.

Escalation policies are deferred to a future release — they need notification-service wiring before they would actually dispatch.

### Profile leave with on-call substitute

`/profile` gets a **Leave** tab so any user can manage their own leave windows. The form takes start, end, optional notes, and an optional substitute. The substitute is the on-call coverage piece: when a member is on leave during their shift, the team's "Currently on-call" card resolves to whoever they nominated, so the on-call view always reflects who is actually reachable.

### OAuth 2.0 server

Windshift now stands up its own OAuth 2.0 server so third-party applications can act on a user's behalf via a Windshift-issued agent identity. The flow is authorisation-code with PKCE (mandatory for public clients), refresh-token rotation with hashed storage and replay-cascade revocation, and exact `redirect_uri` matching against per-client allowlists. Authorisation codes are short-lived and single-use; granted scopes are intersected with each client's allowlist so a client cannot request more than it was registered for.

A new sysadmin page at `/admin/oauth-clients` exposes the full client lifecycle — create, list, edit, rotate secret, delete. Secrets are bcrypt-hashed; the plaintext is shown exactly once on create or rotate and is never echoed back afterwards.

The user-facing consent page at `/oauth/authorize` shares its component with the existing CLI-authorise page, and both renderers escape the client display name, so an admin who can register a client cannot smuggle markup into the consent screen.

### Personal and shared labels

Labels are now a first-class organisational primitive, with separate personal and shared scopes. Personal labels are private to a user; shared labels live within a workspace and respect its permission model. Items, lists, and CQL queries all understand the new shape.

### Admin-editable email templates

Transactional emails are no longer hardcoded. Admins can edit subject, HTML body, and plain-text body for `magic_link`, `email_verification`, `invitation`, `portal_reply`, and `notification_batch`. Senders look up the template by name at send time and fall back to the embedded defaults if no row exists, so an empty install ships with sane copy. The admin **EmailTemplateManager** page renders a live preview that runs the same enrichment pipeline the production sender uses.

The same surface ships substantial channel-security hardening:

- A shared SSRF-safe dialer (extracted from the IMAP guard) is now used by SMTP dispatch, the channel-test endpoint, and webhook HTTP clients. This closes the validate-then-dial DNS-rebinding window.
- SMTP and IMAP passwords are encrypted at rest in the channel config. Legacy plaintext rows continue to work, so deployments can encrypt rolling without re-issuing every channel.
- Webhook URLs are validated on save (defense-in-depth on top of the existing send-time check).
- An empty or typoed encryption-mode is rejected with a clear error rather than silently degrading to plaintext AUTH PLAIN.

### Homepage dashboard widgets

The homepage now has a proper dashboard layout. A new **Personal Tasks** widget surfaces items from the user's personal workspace alongside the existing cross-workspace **Assigned to me** widget. The outer section-card wrapper is removed so widgets sit directly on the page surface — the card-in-card nesting from the previous refactor is gone, and the dashboard reads as one cohesive view.

### Collections visual builder state

Collections now persist their visual builder state separately from the CQL string. A collection saved in builder mode reopens in builder mode without best-effort reparsing the raw query. Legacy collections still open in raw mode, with a "Reset to builder" toggle when needed. `iteration` is also added to the default system screen fields so the iteration picker appears on the default item-detail screen without manual screen configuration.

## Security hardening

### Fail-closed primitives

Three code paths that used to swallow setup or configuration errors now fail closed:

- **Sessions** reject the request when there is no client IP, instead of skipping the IP-binding check.
- **OIDC state lookup** rejects expired or missing state, instead of proceeding as though validation had passed.
- **Failed-login audit rows** hash the attempted identifier rather than logging it in the clear, so the audit table cannot itself be a source of credential leakage.

### SSO and SAML

- The SSO secret-encryption key is now derived via HKDF rather than raw SHA-256, with the SHA-256 path retained as a fallback so existing encrypted configurations keep working through a rolling rotation.

### API token expiry default

API tokens minted by non-admin users now default to a 90-day expiry when the request omits one. Admin-issued tokens and any token with an explicit expiry are unchanged. The change closes the case where a user could create a perpetual token by simply not setting an expiry.

### Tests page

- **"All Tests" replaces "No Folder" as the lead entry.** The previous lead counted only unassigned cases and was offset slightly to the left, making sibling folders look nested. "All Tests" counts every case in the workspace and aligns with root folders.
- **Folder collapse fixed.** Folders now collapse and expand reliably, and the collapse chevron is no longer an invalid nested button.

### App and infrastructure

- **`setup_completed` cached in `sessionStorage`.** Every cold app load was hitting the rate-limited status endpoint just to check whether the install was past first-run. The flag now caches after the first hit, dropping a request from every navigation.
- **Welcome page hotkeys** use the standard `keyboardHint` prop on `Button`, so the rendering matches the rest of the app.
- **Channels: handler logic pushed into the service layer.** The same operations are now callable from internal flows without going through HTTP.
- **Jira import — round-1 bug-hunt fixes.** Field mapping, attachment fetch, and worklog import all behave more predictably on real exports.
- **E2E: dialog/picker z-index layering and Playwright-targetable testids.** Pickers no longer disappear behind their host dialogs, and the headline frontend surfaces have stable test hooks.

## Upgrade notes

- **Logbook is not bundled in this release.** This is because of a license change on the Kreuzberg Library used by us. The Docker image and `docker-compose.yml` no longer include the logbook binary. Existing deployments that rely on the bundled logbook should pin to v0.5.9 until logbook ships again.
- **OAuth 2.0 server is enabled by default but has no clients out of the box.** Visit **Admin → OAuth Clients** to register clients. Sysadmin permission is required.
- **API token default expiry for non-admin tokens is now 90 days.** Existing tokens already in the database are unchanged on upgrade.
- **`is_active` is no longer accepted on create-user.** Integration scripts that set it should drop the field — it is silently ignored either way.
- **Five unused tables dropped from the schema.** No migration runs against existing databases; rows (if any) remain in place but the application no longer references them.
- **Email channel passwords are encrypted at rest going forward.** Existing plaintext rows continue to work; new and edited channels are written encrypted. No manual rotation step is required.
