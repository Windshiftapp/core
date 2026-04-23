# Windshift v0.5.9

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Features

### Smart commits on pull-request merge

When a linked pull request transitions to merged during the periodic SCM sync, Windshift now parses Jira-style smart-commit directives out of the PR body and each commit message, and applies them against the detected work items.

Two directives are recognised — `#comment <text>` posts the remainder of the line as a comment, and `#<transition-slug>` runs the named workflow transition (slugs match statuses case-insensitively with dashes in place of spaces). A single line can reference multiple item keys and multiple commands; the cross-product is applied, mirroring Jira's semantics. Hashes inside URLs are skipped via a word-boundary check.

Each command runs under the committer, not the triggering user. The committer's git email is resolved to an internal user with `item.edit` on the workspace; transitions fire through `WorkflowService.PerformTransition` so conditions are still enforced, and comments go through `CommentService.Create` with the normal mention + notification flow. If the committer email doesn't map to a user, or that user lacks `item.edit`, the directive is skipped.

Idempotency is persistent, not in-memory — the PR body is marked as processed via `item_scm_links.smart_commits_applied_at`, and each commit SHA is recorded in a new `scm_processed_commits` ledger so that a sync tick or container restart never re-applies the same action twice.

Because git does not authenticate the committer email (a contributor can set it to any value), smart-commit processing is off by default and must be enabled per SCM connection via **Workspace settings → SCM → Enable smart commits**. The toggle renders a yellow callout that spells out the trust assumption so the choice is knowing.

### Segmented SCM development panel

The work-item SCM panel used to render pull requests, branches, and commits as one flat list, which scanned badly once the item had more than a handful of links. The panel is now split into three labelled sub-sections with their own count chips, and a rollup status lozenge next to the pull-requests header (`OPEN` if any are open, else `MERGED` if any merged, else `DECLINED`). Mirrors the semantics of Jira's development panel.

### Live comments via shared notification bus

Comments on a work item used to refresh only on the item's own poll tick, so a comment or mention from another user could take up to 30s to show up. The notifications store now carries an app-wide adaptive poller (30s active / 5m idle / paused on hidden tab) and exposes a pub/sub bus that emits each newly-arrived notification exactly once. The comments view subscribes to that bus and refreshes the thread the moment a comment or mention notification arrives for the open item. If the user is scrolled away from the bottom when new comments land, a small count badge tracks the arrivals until it's clicked.

The underlying polling loop has been extracted into a reusable `usePoller` composable so the work-item poller and the new notification poller share a single cadence + activity-tracking implementation.

### SCIM explicit disconnect

Revoking a SCIM token used to leave every `scim_managed` user and group stuck in that state forever: admin-side edits and deletes were blocked by the SCIM guards, and no path cleared the flag at scale. Two new endpoints address this:

- `GET /admin/scim/disconnect-preview` returns the counts so the UI can render a confirmation such as "this will release N users and M groups".
- `POST /admin/scim/disconnect` transactionally revokes every active SCIM token, clears `scim_managed` / `scim_external_id` on every user, group, and group membership, and writes an audit entry.

Per-token revocation keeps its current semantics, so rotating credentials without disconnecting remains a single-token operation.

## Security hardening

Four write paths that previously trusted the request body for their scope have been rebound to URL-derived scope so a caller cannot cross workspace or channel boundaries by editing a payload.

### Channel-scoped request type and asset report writes

`PUT` / `DELETE` on request types, their fields, their visibility, and the asset-report equivalents were behind `auth()` only — any logged-in user could update the field schema of any request type, delete arbitrary asset reports, or re-point either resource at a different workspace via the body's `workspace_id`. `UpdateVisibility` was nominally wrapped in `channelMgmt`, but the middleware was reading the request-type / asset-report id as though it were a channel id, so the gate never fired correctly.

All writes now live under `/channels/{channel_id}/...`:

```
PUT | DELETE  /channels/{channel_id}/request-types/{id}
PUT           /channels/{channel_id}/request-types/{id}/fields
PUT           /channels/{channel_id}/request-types/{id}/visibility
PUT | DELETE  /channels/{channel_id}/asset-reports/{id}
PUT           /channels/{channel_id}/asset-reports/{id}/fields
PUT           /channels/{channel_id}/asset-reports/{id}/visibility
```

Each handler reads `channel_id` from the URL, scopes its existence check and SQL `UPDATE` / `DELETE` with `WHERE id = ? AND channel_id = ?`, and returns `404` on zero rows affected. `workspace_id` is dropped from every `SET` clause so a body cannot reposition the resource. Reads (`GET .../{id}` and friends) stay flat since the existing handlers don't expose a write surface.

### Workspace-scoped milestone and iteration writes

`PUT /milestones/{id}` and `PUT /iterations/{id}` decoded the request body and then checked workspace permission against the value the caller supplied — a user with edit on workspace B could send `PUT /milestones/{X_in_A}` with body `{"workspace_id": B}` and the permission check passed against B while the `UPDATE` ran on milestone X. The same body was then written back, so the milestone could also be relocated into the attacker's workspace.

Write paths now live under URL scopes the existing middleware already gates:

```
PUT /workspaces/{workspaceId}/milestones/{id}   (workspaceItemEdit)
PUT /global/milestones/{id}                     (RequireGlobalPermission)
```

Iterations mirror the same shape. `workspace_id` / `is_global` are ignored on the body and dropped from the `SET` clause, so moving a milestone or iteration between scopes is no longer possible through `Update`. Zero rows affected surfaces as `404`, collapsing the cross-scope hijack into the same response as a stale id.

### Form submission request-type enforcement + safe embed

`SubmitForm` previously accepted submissions without a `request_type_id` — the fallthrough created a generic item bypassing per-form `require_auth`, field validation, and item-type resolution. It now rejects a missing `request_type_id` with `400` and verifies the referenced request type belongs to the form's channel in a single query. An unparseable config JSON is treated as an empty config rather than failing the whole submission.

On the rendering side, `PublicFormPage` / `FormRenderer` now read brand, theme, and logo from the flattened public-channel fields the backend actually serves (`channel.theme`, `channel.brand_color`, `channel.logo_url`), and the submit button label comes from `formConfig.submit_button_text` so channel customisation actually flows through. When the form is loaded inside an iframe (`?embed=...`), the page measures its document height on every layout and posts `ws-form-resize` to the parent so the widget-host iframe can match its height to the content.

The security-headers middleware now sets `CSP frame-ancestors: *` and omits `X-Frame-Options` for the `/forms/*` path prefix so customers can embed those pages on their own websites. All other routes keep `SAMEORIGIN` framing.

### SCIM write guards on non-SCIM users

The SCIM handlers took an internal user id and happily operated on any row, including local or admin accounts the IdP never provisioned. That meant a leaked or misconfigured SCIM credential could deactivate any user by id. `DeleteUser`, `PatchUser`, and `ReplaceUser` now check `scim_managed` up front — non-managed users get `404` with an audit entry recording the refusal reason. POST's adoption-by-email path is untouched, so the legitimate route to bring a local user under SCIM management still works.

## Enhancements

### Time tracking

- Time Entry and Time Reports summary rows (**Total Time: Xh**) now sit on `--ds-surface` to match the table header, instead of the semi-transparent neutral background that read as a detached block and looked off in dark mode.
- The daily-hours chart pulls its line colour from `--ds-accent-blue` instead of a hard-coded `#3b82f6`, so the chart tracks the theme.

### Collections board

- On the board view, the `(N remaining)` count next to the Load more button is now hidden while a sprint filter is applied — the count reflects the unfiltered collection total, so showing it while filtering was misleading.

### Workspace context chrome

- When a workspace gradient is active, the navigation reads interactive-active and inactive text colours against the gradient rather than falling back through `--ds-text` (which becomes near-white-on-white in some gradient presets and near-invisible). The glass nav now uses explicit white + translucent-white values so legibility is consistent regardless of gradient.

### SCIM offboarding notification

- The admin notification for a SCIM-initiated deactivation always said "was deactivated via SCIM" regardless of whether the target was actually SCIM-provisioned. When a SCIM request deactivated a locally-managed user (a signal that something is off — IdP misconfig or a SCIM client reaching past the users it owns), the copy implied IdP provenance and hid that signal. For SCIM-managed users the copy is unchanged; for non-SCIM users the title and body now call out the anomaly and ask the admin to verify intent. `owner_scim_managed` is also added to notification metadata so downstream filters can find these events.

## Upgrade notes

- **`api_tokens` migration for existing databases.** The `api_tokens` table (introduced in an earlier release) was only created by the fresh-install schema, so existing SQLite and Postgres deployments provisioned before it landed were missing the table and every token insert failed with `500`. An idempotent `CREATE TABLE IF NOT EXISTS` has been added to the existing-DB branch for both drivers, plus the three indexes, ordered before `cli_auth_codes` so its FK to `api_tokens(id)` resolves on first boot. No manual action is required on upgrade.
- **New URL shape for channel-scoped writes.** Request type and asset-report updates, field edits, and visibility changes now live under `/channels/{channel_id}/...`. Built-in frontend callers have been updated. External clients that call the old flat paths (`PUT /request-types/{id}`, `PUT /asset-reports/{id}/fields`, etc.) must update to include the channel id in the URL.
- **New URL shape for milestone and iteration writes.** `PUT /milestones/{id}` and `PUT /iterations/{id}` are replaced by `PUT /workspaces/{workspaceId}/milestones/{id}` and `PUT /global/milestones/{id}` (and the iteration equivalents). `workspace_id` / `is_global` on the body are ignored; moving a milestone or iteration between scopes is no longer supported through update and must be modelled as delete + recreate.
- **Smart commits are off by default.** After upgrade, the `smart_commits_enabled` column on `workspace_scm_connections` defaults to `false`. Enable it per connection in **Workspace settings → SCM** for workspaces where you trust committer-email provenance; the toggle renders a yellow callout that spells out the trust assumption.
- **SCIM write paths reject non-managed users.** If you have integration scripts that call `PUT` / `PATCH` / `DELETE /scim/v2/Users/{id}` against internal user ids for accounts that were never SCIM-provisioned, those calls now return `404` with an audit entry. Legitimate flows should adopt a user via `POST` (email match) first, which marks them `scim_managed`.
