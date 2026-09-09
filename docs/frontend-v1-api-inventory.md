# Frontend v1 API inventory

Static inventory of frontend REST call sites that still target the legacy
`/api` (v1) routes instead of `/api/v2`, as of 2026-09-07. Companion to
`docs/frontend-v2-contract-audit.md`, which covers v2 contract correctness.

## Public API review scope (2026-09-09)

Browser-UI-only routes remain on `/api` for now. This inventory is evidence
for identifying gaps in the public bearer API, not a requirement to migrate
every frontend call. The outdated `docs/api-v2-contract.md` was removed;
its historical scope decisions are not authoritative for this review.

For each capability, decide whether public clients need it, whether a current
bearer-accessible v2 operation already provides it, and whether to add it or
intentionally leave it out. A session-only v2 route does not establish public
API coverage. The tables below retain the original snapshot and counts;
ongoing implementation changes require checking live registrations as well
as generated metadata before declaring a gap closed.

Approved for release 0.8.9: **WI-1306 — Expose configuration provisioning in
public API v2**. Scope includes custom-field mutations/settings, screens and
screen-field configuration, configuration sets, hierarchy levels, and workspace
roles. Browser-only routes remain on `/api`.

Remaining public capability candidates for discussion, not approved implementation:

- Product resources: teams, logbook documents,
  on-call schedules, leave, and forms.
- Developer integrations: item SCM links, repository associations, and sync
  controls, assessed separately from provider administration and OAuth flows.
- Search and automation: knowledge search and agent profile/binding operations,
  assessed separately from browser chat, pickers, and setup flows.

Authentication screens, UI bootstrap, presentation settings, and other
browser-only operations are not public API gaps merely because they use v1.
Mixed modules must be reviewed operation by operation. Public exposure of
administration, notifications, integrations, and translations remains an
explicit decision rather than an assumption based on module names.

## Method and totals

Counted call sites where a v1 URL is formed in `frontend/src`:

- 403 direct `fetchAPI(...)` calls (excludes the `fetchAPI` definition and the
  `get`/`post`/`put`/`del` wrapper bodies in `api/core.js`).
- 60 calls through the legacy generic helpers `get`/`post`/`put`/`del`
  (`ai.js`: 56, `hub.js`: 4), which always route to `API_BASE`.
- 3 raw `fetch(`${API_BASE}...`)` uploads (`misc.js` attachments,
  `configuration.js` imports, `logbook.js` documents).
- 9 hardcoded `fetch('/api/...')` / `publicBoardFetch('/api/...')` calls
  outside the shared transport.

**Original snapshot total: 475 v1 REST call sites.** Additionally, 22 `createCrudClient(...)`
instances still default to v1 reads/writes, and one SSE stream
(`useItemEventStream.svelte.js`) and several URL builders/iframe `src`s
reference v1 paths without issuing fetch calls themselves.

The "v2 equivalent" column reflects whether the v2 contract
(`internal/restapi/v2/contract-metadata.json`) already exposes a matching
surface.

Cleanup removed seven direct v1 call sites and two legacy CRUD clients from
that snapshot (projects, project field requirements, and defects). The module
table below remains historical; these figures are not a fresh whole-tree count.

## API layer (`frontend/src/lib/api/`)

### Projects, reviews, and defects review (2026-09-09)

Static source review found that these module names do not represent three
missing public resource families:

| Capability | Current evidence | Agreed disposition |
| --- | --- | --- |
| Projects | Public v2 exposes `/time/projects` CRUD, workspace-visible project lists, categories, members, and managers. Workspace bootstrap returns time projects. The legacy `/projects` client had no matching backend route. Its remaining create/delete callers were in an unreachable regular-workspace branch of the personal workspace page. | Removed the legacy client, unused project field-requirement helpers, and unreachable UI branch. Use existing time-project APIs; do not add a second project resource. |
| Reviews | Six session routes provide personal daily/weekly review CRUD and completed-item lookup. `PersonalReview.svelte` uses them. Reviews belong to the authenticated user; v2 has no review resource. | Intentionally browser-only: keep all six routes on `/api`. These are not approval or code reviews. |
| Test defects | No backend `/defects` routes or frontend consumers of the exported legacy defect client were found. Public v2 supports ordinary item creation, listing/linking/unlinking result items, and step-result updates with `item_id`. The test execution UI uses item links. | Removed the unused defect client and barrel export. Use existing items and test-management operations; no separate defect CRUD resource is needed for the current workflow. |

Relevant implementation: `internal/restapi/v2/time_projects.go`,
`internal/restapi/v2/test_management.go`, `internal/routes/misc.go`,
`internal/handlers/reviews.go`, and `internal/handlers/workspace_bootstrap.go`.
Public exposure was checked against the route builder and mount filtering in
`internal/restapi/v2/router.go`, not inferred only from path presence.

Review completed-item lookup uses completion history and a date range. Do not
assume ordinary item status filtering is an exact replacement if this helper
is later exposed publicly. This was a source review, not runtime validation.

| Module | v1 call sites | v2 equivalent exists? |
| --- | --- | --- |
| `ai.js` (via `get/post/put/del`) | 56 | No |
| `scm.js` | 43 | No |
| `portal.js` (+3 CRUD clients) | 37 | No |
| `misc.js` (+1 upload, `/projects`, `/reviews` CRUD) | 34 | Partial: labels, diagrams, comments, attachments already v2 |
| `users.js` | 27 | Partial: v2 only covers `/users`, `/users/me`, `/admin/users` |
| `channels.js` (+2 CRUD) | 25 | No |
| `agentBindings.js` | 20 | No |
| `diagnostics.js` | 19 | No |
| `admin.js` (+3 CRUD) | 17 | No |
| `oncall.js` | 14 | No |
| `permissions.js` | 13 | No |
| `logbook.js` (+1 upload) | 14 | No |
| `integrations.js` (+1 CRUD) | 13 | No |
| `auth.js` | 13 | No |
| `workspaces.js` (+`/workspace-roles` CRUD) | 12 | Partial: workspace CRUD itself is already v2 |
| `notifications.js` (+1 CRUD) | 12 | No |
| `configuration.js` (+1 import; `/screens`, `/configuration-sets`, `/hierarchy-levels` CRUD) | 13 | Partial: custom-field reads are v2, writes still v1 admin |
| `sso.js` | 9 | No |
| `logbookActions.js` | 9 | No |
| `assetActions.js` | 9 | No |
| `forms.js` | 8 | No |
| `teams.js` (+1 CRUD) | 7 | No |
| `agentSecurity.js` | 5 | No |
| `hub.js` (via `get/put`) | 4 | No |
| `objectTranslations.js` | 4 | No |
| `leave.js` | 4 | No |
| `email-templates.js` | 4 | No |
| `oauth.js` | 3 | No |
| `workflows.js:59` (`/workflows?include_transitions=true`) | 1 | Yes: `GET /api/v2/workflows` |
| `items.js:304` (legacy `/items` query) | 1 | Yes: `GET /api/v2/items` |
| `pages.js:178` (knowledge search) | 1 | No |
| `analytics.js` | 1 | No |
| `tests/defects.js` (+`/defects` CRUD) | 1 | No v2 defects surface |

## Outside the API layer

| File | v1 call sites | v2 equivalent exists? |
| --- | --- | --- |
| `stores/capabilities.svelte.js:65` (`/api/features`) | 1 | No |
| `stores/extensions.svelte.js:18` (`/api/plugins/extensions`) | 1 | No |
| `settings/ModuleSettings.svelte` (plugins, +upload) | 5 | No |
| `mobile/pushClient.js` | 4 | No |
| `features/time/WeeklyCalendar.svelte` (schedule/calendar) | 4 | No |
| `features/personal/PlanMyDay.svelte:75` | 1 | No |
| `pages/SetPassword.svelte` (invitation verify/accept) | 2 | No |
| `editors/ExcalidrawBlockView.svelte:29` (`/api/attachments/{id}/download`) | 1 | Yes: `/api/v2/attachments/{id}/content` |
| `features/pages/PageDiagramModal.svelte:63` (same download route) | 1 | Yes: `/api/v2/attachments/{id}/content` |
| `api/publicBoard.js` (`/api/public/board/...`) | 2 | No |

## Not counted as REST call sites

- SSE stream: `composables/useItemEventStream.svelte.js:48`
  (`/api/items/{itemId}/events`).
- URL builders: `MilkdownEditor.svelte` and `LazyMilkdownEditor.svelte`
  default `downloadUrlBase='/api/attachments'`,
  `features/logbook/DocumentDetail.svelte` uses `/api/logbook/attachments`,
  `logbook.js` thumbnail/preview/file URL getters, and
  `configuration.js` export URL.
- Plugin admin iframe `src`s in `pages/Admin.svelte` and
  `settings/SSOContainer.svelte`.
- Display-only callback/login strings in SSO, SCM, integration, and OAuth
  settings screens.

## Migration backlog

Frontend adapter work is separate from the public API gap review. Candidate
migrations and limitations:

1. `editors/ExcalidrawBlockView.svelte:29` and
   `features/pages/PageDiagramModal.svelte:63`:
   `/api/attachments/{id}/download` -> `/api/v2/attachments/{id}/content`.
2. `api/items.js:304`: legacy `/items` query -> `GET /api/v2/items`.
3. `api/workflows.js:59`: `/workflows?include_transitions=true` ->
   `GET /api/v2/workflows` plus transition reads and response adaptation;
   the old call requests embedded transitions.
4. `api/misc.js`: only item-owned uploads can use
   `POST /api/v2/items/{id}/attachments`. Generic upload callers include
   avatars and workspace/portal images, so this is not a universal replacement.
5. `api/configuration.js`: custom-field writes still routed through
   `/admin/custom-fields` while reads already use `GET /api/v2/custom-fields`;
   current v2 metadata exposes list/get only. Mutations are a backend gap,
   subject to the public-scope decision above.
