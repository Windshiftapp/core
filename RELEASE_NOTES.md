# Windshift v0.5.6

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Features

### CLI onboarding

- `ws` can now complete its first-time authorization against a running server via a short-lived code exchange. Schema adds `cli_auth_codes`; the new `/cli/authorize` page confirms the pairing before issuing credentials.
- New `ws config` command groups the previously scattered configuration flags.

### Item transitions

- New item-transition endpoint captures status changes through a dedicated path so dependent rules (notifications, actions, workflow conditions) see a single, typed event instead of reverse-engineering intent from a generic update.

### Item context service

- `services/item_context` centralizes the "resolve an item and everything the rendering/notification code needs around it" lookup. Replaces several ad-hoc joins in handlers and action execution.

## Enhancements

### Email receiver

- Per-channel OAuth refresh is now serialized through a `sync.Map` of mutexes. Concurrent scheduler ticks can no longer both hit an expired token, both refresh, and both overwrite each other — which with Microsoft's rotating refresh tokens used to leave a dead token in the database.
- Encryption failures during token refresh now propagate instead of silently writing an empty ciphertext; a failure no longer wipes the stored refresh token and forces manual re-auth.
- Incoming HTML is now sanitized with `bluemonday` instead of a regex scrub of `<script>`/`<style>`. The previous implementation was trivially bypassed by case or whitespace tricks.
- Incoming items created from email now go through the same validation, type-allowlist, status-resolution and priority-resolution the REST API uses. The local duplicates (hardcoded "Open" fallback, no workspace filter) are gone.
- Subject, From.Name and To.Name headers are RFC 2047-decoded; `=?utf-8?Q?…?=` encoded-words render as native characters.
- Attachments are written through an atomic temp-file + rename; a crash mid-write can no longer leave a truncated file that the UI would later serve. If the database insert fails after the file lands, the orphan is removed.
- Portal-customer and processed-email upserts use `ON CONFLICT DO NOTHING RETURNING id`, so a race or retry against the unique constraints no longer surfaces as a hard failure.
- The poller now halts at the first message that fails to parse or process instead of logging it and moving on. A stuck UID holds up the queue (surfaced via `errorCount`/`last_error`) until it's addressed; previously a later success persisted `LastUID` past the failure and the bad message was searched past on the next tick.
- `UIDVALIDITY` is now tracked in `email_channel_state`. On a mismatch (mailbox restore, quota reset, folder migration) `sinceUID` resets to 0 so we neither skip new messages below the stale `LastUID` nor reprocess old ones.

### Security

- Integration OAuth `redirect_uri` is built exclusively from the configured `baseURL`. The `X-Forwarded-Host` / `Host` header fallback is removed: an unconfigured base URL now returns 503 on `StartOAuth` and a redirect-with-error on callback rather than silently generating a redirect through an attacker-controlled host.
- SCIM `PATCH` error responses no longer embed raw driver error text (constraint names, FK messages) in the SCIM body. The full error is logged server-side with the token prefix for IdP correlation; the client sees a generic `Patch operation failed`.
- Unknown SCIM PATCH paths emit an `<unsupported>` breadcrumb in the aggregate audit row instead of a silent no-op, so IdP misconfiguration leaves a grep-able trail.
- `asset_action_service.executeSetField` no longer interpolates field names into SQL via `fmt.Sprintf`. The whitelist is preserved but the write radius has no interpolation.
- The Milkdown link sanitizer now blocks protocol-relative URLs (`//evil.com`). The previous `isSafeUrl` returned `true` for any value without a colon, and browsers resolve protocol-relative URLs against the current scheme.

### SCIM audit trail

- Group `create`/`replace`/`patch` now emit per-member add/remove audit events, including which (if any) users failed FK or permission checks.
- User and group `PATCH` capture per-attribute old/new values in `details.changes` for forensic replay.
- When a SCIM request deactivates a user (`DELETE`, `PUT active=false`, `PATCH active=false`) the change cascades to owned agents, API tokens and app tokens. An in-app notification is raised for every active system admin so integrations can be re-pointed before credentials go stale.

### Hierarchy integrity

- Parent-id cycle detection now runs inside the same transaction that writes the new parent, using `SELECT … FOR UPDATE` on Postgres. Two concurrent reparents can no longer each pass their individual check and together create a cycle.
- `ItemFieldValidator` gains a cycle-check hook (wired up by default for user-facing updates) so parent changes made through `ValidateAndApplyUpdates` are now also rejected when they'd create a cycle or self-parent.
- Every recursive CTE in `HierarchyService` (`GetAncestors`, `GetDescendants`, `CountDescendants`, `GetRoot`) is capped at a shared depth ceiling. `GetRoot` now surfaces depth exhaustion as an error rather than a silent nil so callers cannot confuse it with "no parent".

### Frontend

- Added a shared `CopyButton` component and `utils/clipboard.js` utility with a legacy-browser fallback. Nine call sites (token views, settings, portal URL badges, form-integration panel, etc.) migrated to it; removes hand-rolled `navigator.clipboard.writeText` wrappers with inconsistent feedback and an incidental shared-state bug in the form integration panel.
- Ten hand-rolled empty states migrated to the shared `EmptyState` component (email log, test sets, form builder, organisation detail, notification tray, execution trace modal, chat panel, Security credentials and API tokens, test template detail, SSO provider list, repository picker).
- Four hand-rolled alert banners migrated to `AlertBox` (theme manager, hierarchy-level manager, channel SMTP/webhook test-result panels).
- Asset relationship graph now themes the Svelte Flow chrome (background, controls, minimap, attribution, edge labels) with design-system tokens instead of the library's bright-white defaults.
- `BoardConfiguration.GetByCollection` at the workspace-default path returns an empty default configuration on first load instead of 404.
- AI Features save no longer fails when a feature had no prior config — `setConnectionId` and `setSchedule` now default `mode` to the same value the UI renders.
- Dropped a no-op "Help" button from the WorkItemFilter QL panel.

### Backend / internal

- Permission middleware: `RequireGlobalPermission`, `RequireWorkspacePermission` and `RequireAnyWorkspacePermission` now share a single `requireWithCheck` scaffold.
- `actionutil.UpdateActionGraph` wraps the "begin tx + UPDATE row + replace node/edge graph + commit" transaction used by the action, asset-action and logbook-action repositories.
- LLM clients (`httpClient` for llama.cpp, `openaiClient`) share a `baseChatBody` request assembler and a `postChatCompletion` marshal+POST helper. Each client only adds its provider-specific field (grammar for llama.cpp, `response_format` for OpenAI).
- `scm.refreshItemSCMLink` unifies `RefreshItemSCMLink` and `RefreshItemSCMLinkForUser`; credential resolution picks the workspace or user strategy off an optional `userID`.
- `middleware.requireWithCheck`, `HandlerPlugins.invokeEnabledPlugin`, `BaseHandler.requireWorkspaceIDAndID(ForWrite)`, `CommentHandler.requireEditableComment`, `AssetTypeHandler.requireAssetTypeViewAccess`, `IntegrationItemLinksHandler.requireItemEditAuth`, `MilestoneHandler.requireMilestoneMutateAccess`, `scanTestRun`, `scanProvider`, `scanLinkIDs`, `queryCapabilities`, `respondConditionSets`, `respondTimeProjects`, `resolvePortalBySlug`, `resolveRuleForItem`, `resolveActionableToken`, `queryProviders`, `appendCustomScreenFields`, `applyGitHubAppCredentials`, `applyRequestTypeVisibility`, `unmarshalIntIDs`: new shared helpers replacing per-handler copy-paste scaffolds.
- Plugin manager: shared types and `With*` options moved to `manager_common.go` so the real and `noplugins` stub builds don't diverge. Fixes a pre-existing build break where `go build -tags noplugins ./...` failed because `manager.go` lacked its `!noplugins` build tag.
- Repository: dropped an unused duplicate `DynamicUpdateBuilder` type.
- `AvailableField` + `appendCustomScreenFields` hoisted to `internal/handlers/base.go` so `asset_reports.GetAvailableFields` and `request_types.GetAvailableFields` don't each carry the same inline type and 30-line screen-fields SELECT.

### CLI

- `ws init` can now complete authorization interactively against a running server.
- `ws config` groups the previously scattered configuration flags.

## Upgrade notes

- The email-receiver schema adds a `uid_validity` column to `email_channel_state` (`INTEGER` on SQLite, `BIGINT` on Postgres). Both fresh-install schemas and the existing-database migration lists carry it.
- The CLI onboarding flow adds a `cli_auth_codes` table; migration is automatic.
- `noplugins` builds: if you build with `-tags noplugins`, this is the first release in which that build is again functional.
