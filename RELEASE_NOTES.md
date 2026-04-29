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
