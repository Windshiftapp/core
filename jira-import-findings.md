# Jira Import Wizard — Bug Hunt Findings & Fix Plan

Bug hunt scope: Jira Cloud → Windshift data alignment. Methodology: code-level
verification of every hypothesis, with file:line evidence. Live capture
verification (Phase 1–3 of the approved plan) is the follow-up step the user
runs against their Jira instance to confirm symptoms in real data.

---

## Executive summary

| Severity | Count | Bug IDs |
|---|---|---|
| Critical | 4 | B11 (panic), B15 (created_at), B17 (silent reporter loss), B18 (FK violation) |
| High | 11 | B01, B02, B03, B04, B05, B07, B10, B12, B13, B19, B22 |
| Medium | 10 | B06, B08, B09, B14, B16, B20, B21, B23, B24, B27 |
| Low / Note | 3 | B25, B26 (no bug — UUID prefix), B28 (deferred follow-up) |

**Total verified bugs: 26.** B25 needs live data to confirm option storage. B26 was hypothesised but reading `jira_import_entities.go:713` shows filenames are UUID-prefixed — **not a bug**, marked NOT-A-BUG below.

Code-only verification status: 26 of 28 hypotheses are confirmed against the
code. B22 is structurally confirmed (the per-issue Comment/Worklog containers
carry pagination fields the importer ignores) but the actual truncation
threshold needs a live test issue with >50 comments to observe. B25 needs a
live capture to inspect what option payloads Jira returns.

---

## Bug catalogue

### B01 — Story points silently dropped
- **Severity:** High
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:265-310`
- **Symptom:** `items.story_points` is `NULL` for every imported issue, even when the wizard's mapping UI mapped Jira `customfield_*` (Story Points) → `number`.
- **Evidence:** `importIssue` builds `customFieldValues` only for `WindshiftType == "user" || "users"` (line 273-275). All other types fall through. `services.ItemCreationParams` exposes `StoryPoints *float64` (`internal/services/items.go:96`) but `importIssue` never sets it. The `items` table has the column (`internal/database/schema/items.sql:29`).
- **Root cause:** Custom-field switch is gated on user/users only.
- **Fix sketch:** Handler-only. Detect the Story Points field by mapping target (or by `WindshiftType == "number"` AND a per-mapping flag like `IsStoryPoints`), extract `value.(float64)`, set `params.StoryPoints`. For other numerics, write into `CustomFieldValuesJSON`.

### B02 — Sprint never imported as iteration
- **Severity:** High
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:206` (`importIssue` signature has no iteration mapping); analyzer does detect sprints (`jira_analyzer.go`, `HasSprints` flag) but the import path stops there.
- **Symptom:** `items.iteration_id` is `NULL` for all imported issues; no rows are added to `iterations`; sprints visible in Jira do not appear in Windshift.
- **Evidence:** `JiraIssueFields.Sprint` exists (`internal/jira/types.go:149`); `JiraSprint` struct exists (`types.go:241-250`); `client.GetBoardSprints` is implemented and routed through the recording client (`jira_capture.go:161`); but no code path reads `Fields.Sprint` and creates iterations.
- **Root cause:** Sprint is detected during analysis but no `ensureSprints`/`ensureIterations` step runs during import; `importIssue` doesn't pass an `IterationID` to `CreateItem`.
- **Fix sketch:** Add an `ensureIterations` step alongside `ensureMilestones` in `executeImport` (`jira_import_execution.go`). Pull active+future sprints via `client.GetBoardSprints` for each board returned by `client.ListBoards`. Build `sprintMap[jira_sprint_id] → windshift_iteration_id`. In `importIssue`, parse the sprint custom field on each issue (Jira returns it as `customfield_*` for company-managed projects, or as `Fields.Sprint` for some shapes — handle both), look up in `sprintMap`, set `params.IterationID`. See B28 for sprint state semantics.

### B03 — Labels never imported
- **Severity:** High
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:206` (importIssue) — no read of `Fields.Labels`.
- **Symptom:** Imported items have no labels in Windshift; `item_labels` table receives no rows from import.
- **Evidence:** `JiraIssueFields.Labels []string` exists (`types.go:138`). Windshift has `labels` and `item_labels` tables (`internal/database/schema/labels.sql:3-30`) with `UNIQUE(name, workspace_id)`. The importer never references either.
- **Root cause:** Not implemented.
- **Fix sketch:** Handler-only. After `services.CreateItem` succeeds, walk `issue.Fields.Labels`, upsert each into `labels` (workspace-scoped), insert into `item_labels`. Cache label-name → label-id within the import job to avoid repeated lookups.

### B04 — Components never imported
- **Severity:** High (depends on user model decision)
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:206` — no read of `Fields.Components`.
- **Symptom:** Components like `API`, `Backend`, `Frontend` set on Jira issues are lost.
- **Evidence:** `JiraIssueFields.Components []JiraComponent` exists (`types.go:139`). Windshift has **no** `components` table — confirmed via `grep "components" internal/database/schema/`.
- **Root cause:** No target schema in Windshift.
- **Fix sketch:** Design decision needed. Three options:
  1. **Map components to labels** with a `[component]` prefix or distinct color — simplest, no schema change.
  2. **Add a `components` workspace-scoped table + `item_components` join**, mirroring the labels schema.
  3. **Stuff into `custom_field_values` JSON** as `{"jira_components": ["API","Backend"]}` — preserves data, no UI.
  Recommendation: option 2 if components are first-class in Windshift's plans, else option 1 as a quick fix.

### B05 — Only first FixVersion used
- **Severity:** High
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:239`
- **Symptom:** Issues with multiple Fix Versions (e.g., `[v1.0, v1.1]`) only get the first one as milestone in Windshift; the rest are dropped silently.
- **Evidence:** `if len(issue.Fields.FixVersions) > 0 { ... versionMap[issue.Fields.FixVersions[0].ID] }` — line 239-243.
- **Root cause:** Windshift items have a single `milestone_id` column, but Jira allows N fix versions per issue.
- **Fix sketch:** Two layers. (a) Pick the first version as primary milestone (current behavior, but pick deterministically — e.g., earliest release date). (b) Add an `item_milestones` join table for additional versions, OR persist additional versions into `custom_field_values` JSON as `{"jira_extra_fix_versions": [...]}`. Document the choice in the findings.

### B06 — Affects Versions never read
- **Severity:** Medium
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:206` — no read of `Fields.Versions`.
- **Symptom:** Jira's "Affects Version/s" field is silently dropped on import.
- **Evidence:** `JiraIssueFields.Versions []JiraVersion` exists (`types.go:141`, comment "Affects versions").
- **Root cause:** Not implemented.
- **Fix sketch:** Same options as B04/B05 — a join table, a custom field bag, or treat as labels. Lower priority than fixVersions because fewer teams use it.

### B07 — Custom fields beyond user/users not persisted
- **Severity:** High
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:265-310`
- **Symptom:** The mapping wizard analyzes and lets users map text, textarea, select, multiselect, date, number, milestone, multiversion fields — but on import, every type other than `user` and `users` falls through silently. The mapped target receives no value.
- **Evidence:** Switch at line 273-275: `if mapping.WindshiftType != "user" && mapping.WindshiftType != "users" { continue }`. Field mapper supports the full type matrix (`internal/jira/field_mapper.go:39-98`).
- **Root cause:** Bare-bones implementation comment in code: `// Process custom fields (user/users types only for now)` (line 265).
- **Fix sketch:** Handler-only. Extend switch to:
  - `text`, `textarea`: stringify value (handle `value.(string)` and option-object `value.(map)["value"]`).
  - `select`, `multiselect`: store option `value` (string), not `id` (which doesn't exist in Windshift).
  - `date`: parse `2006-01-02` and `time.RFC3339Nano` shapes.
  - `number`: `value.(float64)`.
  - `milestone`: lookup in `versionMap`.
  Persist via `customFieldValues` JSON map already being assembled, then emit through `CustomFieldValuesJSON`. Verify `services.CreateItem` round-trips this into Windshift's custom field storage with the right field type IDs (the wizard's `mapping.WindshiftID` should already point at the created/mapped Windshift custom field).

### B08 — Worklog and TimeTracking never imported
- **Severity:** Medium
- **Class:** Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:206` — no read of `Fields.Worklog` or `Fields.TimeTracking`.
- **Symptom:** Time spent, original/remaining estimates, and individual worklog entries are all lost.
- **Evidence:** `JiraIssueFields.Worklog *JiraWorklogContainer` (`types.go:147`); `JiraIssueFields.TimeTracking *JiraTimeTracking` (line 148). Windshift has `time_worklogs` table with `user_id`, `item_id`, plus seconds/started_at columns (`internal/database/schema/time_worklogs_postgres.sql`).
- **Root cause:** Not implemented.
- **Fix sketch:** Handler-only for worklogs (insert into `time_worklogs` per `JiraWorklog` entry, mapping `Author` via `userMap`, parsing `Started`, using `TimeSpentSeconds`). For aggregate `TimeTracking` (originalEstimate/remainingEstimate), check whether `items` has columns for these — if not, store in `custom_field_values` JSON or add columns. Pagination caveat: `JiraWorklogContainer` is paged (B22) — fetch additional pages if `Total > len(Worklogs)`.

### B09 — Epic Link mapped to text, no parent semantics
- **Severity:** Medium
- **Class:** Wrong mapping
- **Location:** `internal/jira/field_mapper.go:79`
- **Symptom:** In company-managed Jira projects, the Epic→Story relationship is encoded as a custom field (`com.pyxis.greenhopper.jira:gh-epic-link`) holding the parent epic's key as a string. The mapper categorizes this as `FieldTypeText`, so even after B07 is fixed, the relationship lands as a text custom field, not as `parent_id`.
- **Evidence:** `field_mapper.go:79`: `"com.pyxis.greenhopper.jira:gh-epic-link": FieldTypeText`. `linkParents` (`jira_import_entities.go:384`) only walks `meta["parent_key"]` from `Fields.Parent`, which is set in team-managed projects; for company-managed, `Fields.Parent` may be nil while `Fields.Epic` and the epic-link custom field carry the relationship.
- **Root cause:** Mapper assumes single hierarchy mechanism (the `Fields.Parent` field).
- **Fix sketch:** In `importIssue`, before calling `recordMapping`, also check `issue.Fields.Epic` and `issue.Fields.CustomFields[<epic-link-field-id>]`. Persist as `parent_key` in metadata so `linkParents` resolves it. Make the epic-link field detection driven by `field_mapper.go` (return a special sentinel rather than `FieldTypeText` for that key).

### B10 — Priority bypasses SuggestPriorityMapping
- **Severity:** High
- **Class:** Wrong mapping
- **Location:** `internal/handlers/jira_import_entities.go:248`
- **Symptom:** Jira priority "Highest" lands as Windshift priority `NULL` (default applied) instead of "Critical". Same for "Lowest", "Blocker", "Major", "Minor", "Trivial".
- **Evidence:** Importer passes raw `issue.Fields.Priority.Name` (`entities.go:248`) to `params.Priority` (`entities.go:327`). Inside `services.CreateItem`, `mapTextPriorityToID` only matches `low|medium|high|critical|urgent` (`internal/services/items.go:48-66`). "Highest" doesn't match — `mapTextPriorityToID` returns `nil` — line 146-162 fallback fetches the workspace default priority. So "Highest" silently becomes "Medium" (or whatever the workspace default is). `SuggestPriorityMapping` (`internal/jira/field_mapper.go:345`) WOULD map "highest" → "Critical" but is never called by the importer.
- **Root cause:** The mapper has the synonym table but the importer skips it; the fallback path in `CreateItem` has a narrower table.
- **Fix sketch:** Handler-only. In `importIssue`, call `priorityName := jira.SuggestPriorityMapping(issue.Fields.Priority.Name)` before passing to `CreateItem`. Note: priorities are workspace-scoped via `configuration_set_priorities`, so the resulting name still has to match a workspace priority — the mapping call should target one of `Low|Medium|High|Critical` since those are the canonical Windshift priorities.

### B11 — ADF heading panic risk
- **Severity:** Critical
- **Class:** Crash risk
- **Location:** `internal/jira/field_mapper.go:396`
- **Symptom:** A heading node with missing or non-numeric `level` attr panics the import goroutine with a nil-map or wrong-type assertion. Brings down the whole job.
- **Evidence:** `level, _ := nodeMap["attrs"].(map[string]interface{})["level"].(float64)` — chained assertion. If `nodeMap["attrs"]` is missing → `nil.(map[...])["level"]` panics with "invalid memory address or nil pointer dereference". If `["level"]` is e.g. `int` (rare but possible from custom serializers) → assertion-with-`,_` form silently sets `level = 0` (writes `# title` heading with no `#`s, just a leading space — also a quiet bug).
- **Root cause:** Type assertion not guarded with `,ok`.
- **Fix sketch:** Pure correctness. Replace with two-step:
  ```go
  attrs, ok := nodeMap["attrs"].(map[string]interface{})
  if !ok { attrs = map[string]interface{}{} }
  levelF, _ := attrs["level"].(float64)
  level := int(levelF)
  if level < 1 || level > 6 { level = 1 }
  ```

### B12 — ADF unhandled node types silently dropped
- **Severity:** High
- **Class:** Silent drop / Wrong rendering
- **Location:** `internal/jira/field_mapper.go:392-454` (`convertADFNode` switch)
- **Symptom:** Issue descriptions that use Jira's full editor lose substantial content. Specifically silently dropped or rendered as empty text:
  - `media`, `mediaSingle`, `mediaGroup` (inline images, attached file references)
  - `inlineCard` (Atlassian smart links, very common in modern Jira)
  - `table` and children (`tableRow`, `tableHeader`, `tableCell`)
  - `panel` (info / warning / note / success / error blocks)
  - `taskList` / `taskItem` (checkboxes)
  - `expand` / `nestedExpand` (collapsible sections)
  - `decisionList` / `decisionItem`
  - `status` (the colored status lozenge)
  - `emoji`
  - `date` (inline date pill)
- **Evidence:** Switch at `field_mapper.go:392` lists `paragraph, heading, bulletList, orderedList, codeBlock, blockquote, rule, text, hardBreak, mention, default`. Default just calls `convertADFContent` which traverses children — for `media`, children are typically empty so the whole thing becomes "". For `table`, children render but lose all structural meaning (cells emit text without delimiters).
- **Root cause:** ADF coverage incomplete.
- **Fix sketch:** Feature-shaped (split into sub-tickets):
  - **B12a (high):** `table` → render as Markdown table with `|` delimiters and `--- |` header row separator. Highest user impact.
  - **B12b (high):** `panel` → render as `> [!INFO]` or `> [!WARNING]` GFM-style admonition; preserve panel type.
  - **B12c (medium):** `media` / `mediaSingle` → emit `![alt](attachment-id)` referencing imported attachment by ID; requires attachment import to run first and a way to lookup the imported attachment URL by Jira media ID.
  - **B12d (medium):** `taskList` / `taskItem` → render as `- [ ]` / `- [x]` markdown checkboxes.
  - **B12e (low):** `inlineCard` → render as `[<url>](<url>)` from `attrs.url`.
  - **B12f (low):** `expand` / `nestedExpand` → render as `<details><summary>...</summary>...</details>` HTML or as a heading.
  - **B12g (low):** `decisionList`, `status`, `emoji`, `date` → text representation.

### B13 — Comment timestamp parsing too narrow
- **Severity:** High
- **Class:** Silent drop / Metadata loss
- **Location:** `internal/handlers/jira_import_entities.go:485-489`
- **Symptom:** Most imported comments end up with `created_at = <import time>` instead of Jira's actual creation time, breaking timeline ordering.
- **Evidence:**
  ```go
  if parsed, err := time.Parse("2006-01-02T15:04:05.000-0700", comment.Created); err == nil {
      createdAt = &parsed
  } else if parsed, err := time.Parse("2006-01-02T15:04:05.000Z0700", comment.Created); err == nil {
      createdAt = &parsed
  }
  ```
  Jira Cloud commonly returns timestamps in shapes like `2024-01-15T10:23:45.123+0000` (4-digit zone, no colon — covered) but also `2024-01-15T10:23:45.000Z` (Z literal — NOT covered) and `2024-01-15T10:23:45.123456+02:00` (microseconds + colon zone — NOT covered). When neither layout matches, `createdAt` stays `nil` and `commentSvc.Create` falls back to `CURRENT_TIMESTAMP`.
- **Root cause:** Hardcoded layouts instead of `time.RFC3339Nano` plus a small fallback list.
- **Fix sketch:** Pure correctness. Use:
  ```go
  layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05.000Z0700"}
  for _, layout := range layouts {
      if parsed, err := time.Parse(layout, comment.Created); err == nil {
          createdAt = &parsed
          break
      }
  }
  ```
  Same fix needed for attachment/worklog `Created` timestamps once those imports are added (B08).

### B14 — ADF mention loses user link
- **Severity:** Medium
- **Class:** Wrong mapping
- **Location:** `internal/jira/field_mapper.go:445-449`
- **Symptom:** `@mentions` in descriptions and comments become plain `@<displayName>` text. The link to the imported Windshift user (which the wizard already mapped via `userMap`) is lost — no notification, no profile link, just a string.
- **Evidence:** `case "mention":` at line 445 only emits `"@" + text` from `attrs.text`. `attrs.id` (the Jira accountId) is also present in ADF mention nodes but not consulted.
- **Root cause:** The ADF converter has no access to `userMap` and produces text-only output.
- **Fix sketch:** Two-step. (a) Change `ConvertADFToMarkdown` signature to optionally accept `userMap map[string]int` (or wrap in a struct). (b) For mentions, look up `attrs["id"]` in `userMap` and emit something like `@user-<windshiftID>` (Windshift's mention syntax — verify what `mention_service.go` uses). Depends on B23 (deactivated user fallback) for unknown IDs.

### B15 — created_at / updated_at not preserved
- **Severity:** Critical
- **Class:** Metadata loss
- **Location:** `internal/services/items.go:114, 224-225`
- **Symptom:** Every imported item gets `created_at = <import time>`, so a 2019 issue and a 2024 issue both look like they were created today. Reports, charts, "recent" filters, and history are all wrong.
- **Evidence:** `now := time.Now()` at line 114; passed directly into the INSERT at lines 224-225. `ItemCreationParams` has no `CreatedAt`/`UpdatedAt` fields. The DB column has `DEFAULT CURRENT_TIMESTAMP` (`items.sql:45-46`) but the explicit value in the INSERT overrides the default.
- **Root cause:** `CreateItem` is the only insert path and was designed without an override.
- **Fix sketch:** Service-signature change (Class A — must land before importer fix).
  1. Add `CreatedAt *time.Time`, `UpdatedAt *time.Time` to `ItemCreationParams`.
  2. In `CreateItem`, use `params.CreatedAt` if non-nil, else `now` (same for updated).
  3. In `importIssue`, parse `issue.Fields.Created` / `issue.Fields.Updated` (use the `time.Parse` layout list from B13) and pass to `CreateItem`.

### B16 — Resolved date never imported
- **Severity:** Medium
- **Class:** Metadata loss
- **Location:** `internal/handlers/jira_import_entities.go:206` — no read of `Fields.Resolved`.
- **Symptom:** Issues that were resolved/closed in Jira have no resolution date in Windshift. Cycle-time and lead-time reports are wrong.
- **Evidence:** `JiraIssueFields.Resolved string` exists (`types.go:136`, `json:"resolutiondate"`). `items` table has **no `resolved_at` column** (verified via grep across schema; `resolved_at` exists only in `teams.sql:248` for incidents).
- **Root cause:** No target column.
- **Fix sketch:** Schema decision (Class A). Either:
  - **(a)** Add `resolved_at DATETIME` column to `items` via migration, expose via `ItemCreationParams.ResolvedAt`. Cleanest.
  - **(b)** Stuff into `custom_field_values` JSON. Loses sortability.
  Recommend (a). Then `importIssue` parses `Fields.Resolved` and sets it.

### B17 — Reporter silently nil when user mapping fails
- **Severity:** Critical
- **Class:** Silent drop / Metadata loss
- **Location:** `internal/handlers/jira_import_entities.go:230-235`
- **Symptom:** When the reporter's email isn't available (Jira Cloud restricts emails for many users), `userMap[GetIdentifier()]` lookup misses, `reporterID` stays `nil`, and the imported item shows no reporter. In Jira every issue has a reporter — this is a big information loss.
- **Evidence:** `ensureUsers` skips users with empty email (`entities.go:56-61`) — those users are intentionally not added to `userMap`. Then `importIssue` `reporterID = nil` if the reporter's accountID isn't in `userMap`. Combined with B23, this loses every issue's reporter for a typical Cloud instance where most users have GDPR-restricted emails.
- **Root cause:** Hard policy decision in `ensureUsers` to skip emailless users.
- **Fix sketch:** Class A (policy decision, may need schema):
  - Option 1: Allow inactive users with no email — generate a synthetic email from accountID like `<accountId>@imported.invalid` and create the user. Risk: clutters the user list.
  - Option 2: Add a `reporter_external_id TEXT` column to `items` for unmapped reporters; display "Imported user: <displayName>" in the UI when `reporter_id IS NULL AND reporter_external_id IS NOT NULL`.
  - Option 3: Create a single shared "Imported (unknown user)" user and use it as fallback; preserve the original Jira display name in `custom_field_values` or a dedicated metadata field.
  Recommend Option 1 with synthetic emails — keeps user identity round-trippable, easy to reconcile later if the user is invited.

### B18 — Comment author=0 violates FK
- **Severity:** Critical
- **Class:** Crash risk / Data corruption
- **Location:** `internal/handlers/jira_import_entities.go:475-480`; `internal/services/comment_service.go:139-143` (INSERT path).
- **Symptom:** When a comment author can't be mapped (deactivated/unmapped Jira user), `authorID` defaults to `0` and is passed to `commentSvc.Create`. The comments table has `FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE` (`internal/database/schema/content.sql:48`); `users.id=0` doesn't exist. On PostgreSQL with FK enforcement: INSERT fails, comment is silently lost (logged as error). On SQLite: depends on `PRAGMA foreign_keys=ON`; if off, you get an orphan comment with broken author_id=0.
- **Evidence:**
  - `entities.go:475`: `authorID := 0`
  - `entities.go:476-479`: only assigns when the user is in `userMap`
  - `entities.go:496`: `commentSvc.Create(... AuthorID: authorID, ...)` — even if 0
  - `comment_service.go:139-140`: `if params.AuthorID != 0 { authorID = params.AuthorID }` — local `authorID` retains its previous value (likely 0 by Go default)
  - `content.sql:41`: `author_id INTEGER` — column is **nullable**, so passing NULL is valid
- **Root cause:** Importer uses `0` as sentinel where the column expects NULL or a real user.
- **Fix sketch:** Pure correctness, but depends on B17 policy. Two-line fix in `importIssue/importComments`:
  ```go
  var authorID *int  // pointer; nil if no mapping
  if comment.Author != nil {
      if uid, ok := userMap[comment.Author.GetIdentifier()]; ok {
          authorID = &uid
      }
  }
  // pass *int through CreateCommentParams; comment_service inserts NULL when nil
  ```
  Requires changing `CreateCommentParams.AuthorID` from `int` to `*int` (a service-signature change — Class A).

### B19 — Inward-only links from non-imported projects dropped
- **Severity:** High
- **Class:** Silent drop / Hierarchy
- **Location:** `internal/handlers/jira_import_entities.go:586-590`
- **Symptom:** Suppose project A is imported, project B is not. An issue in A has an inward "is blocked by" link from an issue in B. The importer only processes outward links (`outward_key`), so this inward relationship is lost.
- **Evidence:** Comment at line 586: "We only process outward links to avoid duplicates". The dedup logic is correct **when both ends are imported** (avoids creating two link rows), but **wrong when only one end is imported and it's the inward side**.
- **Root cause:** Symmetry assumption that doesn't hold for partial imports.
- **Fix sketch:** Handler-only. Replace the simple "outward only" rule with: process outward; for each inward link, check if the corresponding outward issue is in the import scope (i.e., is in `jira_import_id_mappings` for this job). If yes, skip (already covered). If no, this inward link's source is non-imported — process it as the source of a synthetic outward link, OR record it in metadata for future reconciliation.

### B20 — Company-managed Epic Link not read
- **Severity:** Medium
- **Class:** Hierarchy / Silent drop
- **Location:** `internal/handlers/jira_import_entities.go:341-344`
- **Symptom:** In team-managed Jira, Epic→Story is `Fields.Parent`. In company-managed, it's a custom field `gh-epic-link`. The importer only writes `Fields.Parent.Key` into `meta["parent_key"]`, so company-managed epics don't link.
- **Evidence:** `entities.go:342-344`: only `issue.Fields.Parent` is consulted. `JiraIssueFields.Epic *JiraIssue` exists (`types.go:150`) but nothing reads it.
- **Root cause:** Single-source assumption.
- **Fix sketch:** Handler-only. In the metadata-build block, fall through:
  ```go
  if issue.Fields.Parent != nil { meta["parent_key"] = issue.Fields.Parent.Key }
  else if issue.Fields.Epic != nil { meta["parent_key"] = issue.Fields.Epic.Key }
  else if epicKey, ok := issue.Fields.CustomFields["customfield_<epic-link-id>"].(string); ok { meta["parent_key"] = epicKey }
  ```
  The custom field id varies across instances — discover it during analysis (look for type `gh-epic-link` and store the `customfield_*` ID into the import config).

### B21 — Reporter vs Creator distinction lost
- **Severity:** Medium
- **Class:** Metadata loss
- **Location:** `internal/handlers/jira_import_entities.go:230-235` — only Reporter is read; Creator is never looked at.
- **Symptom:** Windshift items have `creator_id = NULL` for all imported issues. Jira distinguishes "Reporter" (mutable, who is currently reporting) from "Creator" (immutable, who pressed Create) — both are real users worth preserving.
- **Evidence:** `JiraIssueFields.Creator *JiraUser` exists (`types.go:133`). `services.ItemCreationParams.CreatorID *int` exists (`items.go:88`). The importer never reads `Fields.Creator` and never sets `params.CreatorID`.
- **Root cause:** Not implemented.
- **Fix sketch:** Handler-only. Add a `creatorID` lookup parallel to `reporterID`/`assigneeID` and pass to `CreateItem`. Subject to B17 policy for unmapped users.

### B22 — Per-issue containers (Comments, Worklog, Attachments) truncated to first page
- **Severity:** High
- **Class:** Silent drop / Pagination
- **Location:** `internal/handlers/jira_import_entities.go:469` (comments loop), `680` (attachments loop). `BulkFetchIssues` itself doesn't paginate — `internal/jira/client.go:705-721` is a single POST.
- **Symptom:** Issues with > Jira's per-issue page limit (typically 50 comments, similar for worklogs) lose all comments after the first page.
- **Evidence:** `JiraCommentContainer` carries `MaxResults int`, `Total int`, `StartAt int` (`types.go:282-286`) — Jira pages these. The importer walks `issue.Fields.Comment.Comments` but never compares `len(Comments)` to `Total`. Same for `JiraWorklogContainer` (`types.go:299-304`). Attachments don't appear to be paginated by Jira (they're returned whole), but worth verifying.
- **Root cause:** Importer treats per-issue containers as complete arrays.
- **Fix sketch:** Handler change with a new client method. Add `client.GetIssueComments(ctx, issueKey, startAt) ([]JiraComment, total, error)` (Cloud endpoint: `GET /rest/api/3/issue/{key}/comment?startAt=N`). In `importComments`, after walking `issue.Fields.Comment.Comments`, if `Total > len(Comments)`, fetch additional pages until done. Same pattern for worklogs.

### B23 — Deactivated/deleted Jira users silently break user-dependent fields
- **Severity:** Medium
- **Class:** Silent drop
- **Location:** Cross-cutting — `entities.go:225, 232, 287, 298, 477, 750` (every `userMap[...]` lookup).
- **Symptom:** Any field referencing a deactivated Jira user (assignee, reporter, comment author, attachment uploader, custom user-picker, custom multi-user-picker, ADF mention) silently drops to `nil` / `0`. Combined with B17 and B18, this can silently break entire issues.
- **Evidence:** `JiraUser.GetIdentifier()` returns `""` if `AccountID`, `Name`, and `Key` are all empty (`types.go:212-220`). `userMap[""]` lookup misses. The importer treats a miss the same as "no user", which is wrong for a known-but-deactivated user.
- **Root cause:** No fallback policy for known-but-unresolvable users.
- **Fix sketch:** Tied to B17 fix. Whatever policy resolves "no email reporter" should also resolve "deactivated user that we know existed". Likely: synthetic users with `is_active=false` and a deterministic synthetic email (`<accountId>@imported.invalid`) so the same Jira user always maps to the same Windshift user across re-runs.

### B24 — Re-run idempotency gap
- **Severity:** Medium
- **Class:** Operational
- **Location:** `internal/handlers/jira_importer.go:124-191` (`StartImport`); dedup constraint at `internal/database/schema/jira_import.sql` `UNIQUE(job_id, entity_type, jira_id)`.
- **Symptom:** Running the same import twice (without first calling `DeleteImportedData`) creates duplicate items, duplicate comments, duplicate attachments. The unique constraint scopes by `job_id`, so a new job is a clean slate from the constraint's perspective.
- **Evidence:** Schema `UNIQUE(job_id, entity_type, jira_id)` — schema only protects within a single job. `StartImport` always creates a new job (`jira_importer.go:127-150`). `GetPreviousImports` exists (`jira_importer.go:307`) suggesting the UI can warn the user, but no enforcement.
- **Root cause:** Idempotency is operator-driven (delete-then-reimport), not enforced by code.
- **Fix sketch:** Two-layer:
  - **Backend guard:** Before starting import, query `jira_import_id_mappings` joined to `jira_import_jobs.connection_id` for the project's existing imports; if found and `status != 'data_deleted'`, return an error with the existing `job_id` so the UI can prompt to delete first.
  - **Frontend handling:** UI already calls `GetPreviousImports`; ensure it blocks the start unless explicitly confirmed.

### B25 — Custom-field option ID vs value (needs live capture)
- **Severity:** Pending verification
- **Class:** Wrong mapping
- **Location:** `internal/handlers/jira_import_entities.go:265-310` once B07 is implemented.
- **Symptom:** For `select`/`multiselect` custom fields, Jira returns option payloads like `{"self": "...", "value": "Backend", "id": "10001"}`. The naive implementation might extract `id` (which is meaningless to Windshift) or `value` (which can collide across fields).
- **Evidence:** Type `JiraIssueFields.CustomFields map[string]interface{}` (`types.go:151`), no schema for option payloads. Need live capture to confirm the exact shape across cascade-select, multi-select, single-select.
- **Root cause:** Not yet implemented (B07 dependency).
- **Fix sketch:** When implementing B07: extract the human-readable `value` (or `displayValue` for asset types). For cascading-select, recurse into `child`. Document the chosen rule in code comments.

### B26 — Attachment filename collisions (NOT A BUG)
- **Severity:** None
- **Class:** Verified safe
- **Location:** `internal/handlers/jira_import_entities.go:713`
- **Evidence:** `storedFilename := fmt.Sprintf("%s_%s", uuid.New().String(), filepath.Base(attachment.Filename))` — every stored file is UUID-prefixed, so two issues with `image.png` get distinct stored names. Original filename is preserved in `original_filename` column. Marked here for completeness so this doesn't get re-investigated.

### B27 — ADF in comments inherits B11 / B12
- **Severity:** Medium
- **Class:** Wrong rendering / Crash risk
- **Location:** `internal/handlers/jira_import_entities.go:470` calls `jira.ConvertADFToMarkdown(comment.Body)` — same converter as descriptions.
- **Symptom:** All ADF gaps in B11 and B12 also apply to comments — heading panic, dropped tables/panels/media/etc.
- **Evidence:** Single converter, used by both `importIssue` (description) and `importComments` (body).
- **Root cause:** Shared code path.
- **Fix sketch:** Fixing B11 and B12 fixes B27. Note in changelog that the fixes apply to both descriptions and comments.

### B28 — Sprint state semantics (deferred follow-up to B02)
- **Severity:** Low (deferred)
- **Class:** Mapping
- **Location:** Not yet relevant — depends on B02 implementation.
- **Symptom:** Once B02 is fixed, decide how Jira sprint states (`future`, `active`, `closed`) map to Windshift iteration states. Closed Jira sprints with no end date might land as active iterations etc.
- **Fix sketch:** Map `future`→pending, `active`→active, `closed`→closed/archived; preserve `JiraSprint.Goal` into iteration description.

---

## Cross-cutting concerns

### ADF coverage matrix

| ADF node | Currently supported? | Bug | Priority |
|---|---|---|---|
| paragraph | ✅ | — | — |
| heading | ⚠️ panic risk | B11 | Critical |
| bulletList / orderedList | ✅ | — | — |
| codeBlock | ✅ | — | — |
| blockquote | ✅ | — | — |
| rule | ✅ | — | — |
| text + marks (strong, em, code, strike, link) | ✅ | — | — |
| hardBreak | ✅ | — | — |
| mention | ⚠️ text only, no link | B14 | Medium |
| table / tableRow / tableHeader / tableCell | ❌ | B12a | High |
| panel | ❌ | B12b | High |
| media / mediaSingle / mediaGroup | ❌ | B12c | Medium |
| taskList / taskItem | ❌ | B12d | Medium |
| inlineCard | ❌ | B12e | Low |
| expand / nestedExpand | ❌ | B12f | Low |
| decisionList / status / emoji / date | ❌ | B12g | Low |

### Custom-field type matrix

| Field type | Mapper recognises | Wizard offers mapping | Importer persists |
|---|---|---|---|
| user | ✅ | ✅ | ✅ |
| users | ✅ | ✅ | ✅ |
| text / textarea | ✅ | ✅ | ❌ B07 |
| number | ✅ | ✅ | ❌ B07 |
| story-points (special-case number) | ✅ | ✅ | ❌ B01 |
| select / radiobuttons | ✅ | ✅ | ❌ B07/B25 |
| multiselect / multicheckboxes | ✅ | ✅ | ❌ B07/B25 |
| labels (custom field type) | ✅ | ✅ | ❌ B07 |
| date / datetime | ✅ | ✅ | ❌ B07 |
| version (single) | ✅ | ✅ | ❌ B07 |
| multiversion | ✅ | ✅ | ❌ B07 |
| gh-sprint | ✅ | ✅ (FieldTypeIteration) | ❌ B02 |
| gh-epic-link | ✅ as text | ✅ | ❌ B09/B20 (wrong target) |
| insight-object-field | ✅ | ✅ | ❌ B07 |
| cascade-select | ✅ | ✅ | ❌ B07 |
| Standard Labels (Fields.Labels) | n/a (top-level) | ❌ wizard doesn't ask | ❌ B03 |
| Standard Components | n/a (top-level) | ❌ wizard doesn't ask | ❌ B04 |
| FixVersions (multi) | ✅ | partial | ❌ B05 (only first) |
| Affects Versions | ✅ | ❌ wizard doesn't ask | ❌ B06 |

### User-mapping fallback policy (cross-cuts B17, B18, B23)

The current policy is "skip if no email" which silently drops users for the
typical Jira Cloud case (most accounts have GDPR-restricted emails). This
cascades into B17 (reporter loss), B18 (FK violation when comment author=0),
and B23 (deactivated users break every user field). All three should be
fixed by a single policy change in `ensureUsers`:

- Replace email-required filter with: create a Windshift user even without
  an email, using a synthetic email `<accountId>@imported.invalid` and
  `is_active = false`.
- Make `comment.author_id` and `attachment.uploaded_by` nullable in the
  service layer (they already are at the DB layer) so unmapped/orphan
  cases land as NULL, not 0.
- Document the policy in code comments and provide an admin migration
  helper to merge synthetic users with real accounts post-hoc.

### Out-of-scope / deferred

- Rate-limit and 429 retry behavior — current rate limiter is 10 req/s with
  no backoff. Fine for small projects, dangerous at scale. Punt to a
  follow-up performance pass.
- Custom-field schema creation (the wizard supports `action: 'create'` but
  the import path's behavior under that flag wasn't audited this round).
- Workflow transitions and screen schemes — partially imported, fidelity
  to Jira's transition rules not audited.
- Permissions / project-role mapping — out of scope for "data alignment".
- Changelog/history import (`JiraChangelog` exists in types but is unused).

---

## Fix plan

Order: A → B → C → D. Within each class, items can be parallelized.

### Class A — schema / service-signature (do first, others depend)

| Step | Bug(s) | Files | Notes |
|---|---|---|---|
| A1 | B15 | `internal/services/items.go:69-98` (params), `:114, 224-225` (use override) | Add `CreatedAt *time.Time`, `UpdatedAt *time.Time` to `ItemCreationParams`; pass through to INSERT, falling back to `now`. |
| A2 | B16 | new migration `internal/database/schema/<add resolved_at>` for SQLite + Postgres; `items.go` params | Add `resolved_at DATETIME` to `items`; expose `ResolvedAt *time.Time` in params. |
| A3 | B18 | `internal/services/comment_service.go:48-145` | Change `CreateCommentParams.AuthorID` from `int` to `*int`; INSERT uses NULL when nil. |
| A4 | B17, B23 | `internal/handlers/jira_import_entities.go:50-107` (`ensureUsers`) | Drop email-required filter; create users with synthetic email `<accountID>@imported.invalid`; mark `is_active=false`. |

### Class B — handler-only data passthrough (no signature changes once A is done)

| Step | Bug(s) | Files | Notes |
|---|---|---|---|
| B1 | B01, B07, B25 | `entities.go:265-310` | Extend custom-field switch to text/textarea/number/select/multiselect/date/milestone/multiversion. Detect Story Points field via mapping target. For options, store `value` not `id`. |
| B2 | B03 | `entities.go:374` (after `recordMapping`) | Walk `issue.Fields.Labels`, upsert to `labels`, link via `item_labels`. |
| B3 | B04 | `entities.go:374` | Implement chosen design (label-with-prefix, new components table, or JSON bag). |
| B4 | B05, B06 | `entities.go:237-243` | Pick first fixVersion deterministically; persist additional fixVersions and affects-versions to chosen target. |
| B5 | B10 | `entities.go:248` | Replace raw `Priority.Name` with `jira.SuggestPriorityMapping(Priority.Name)`. |
| B6 | B15 (consumer) | `entities.go:206-336` | After A1: parse `Fields.Created`, `Fields.Updated` via shared layout list, pass to `CreateItem`. |
| B7 | B16 (consumer) | `entities.go` | After A2: parse `Fields.Resolved`, pass to `CreateItem`. |
| B8 | B19 | `entities.go:518-639` (`importIssueLinks`) | Process inward links symmetrically: skip if outward end is in scope; else create from inward side. |
| B9 | B21 | `entities.go:230-235` | Add `creatorID` lookup, pass `params.CreatorID`. |
| B10 | B09, B20 | `entities.go:341-344` | Fall through Parent → Epic → epic-link custom field for `meta["parent_key"]`. |
| B11 | B02 | new `ensureIterations` in `jira_import_execution.go`; `entities.go` reads sprint custom field | Fetch sprints via `client.GetBoardSprints`, build `sprintMap`, pass `IterationID` to `CreateItem`. Subject to B28 sprint-state mapping. |

### Class C — pure correctness

| Step | Bug(s) | Files | Notes |
|---|---|---|---|
| C1 | B11 | `internal/jira/field_mapper.go:396` | Two-step type assertion; clamp level to 1–6. |
| C2 | B13 (and reuse for B6/B7/B8) | `entities.go:485-489`; extract a shared helper `parseJiraTimestamp(s string) *time.Time` | Layout list: `time.RFC3339Nano`, `time.RFC3339`, plus the existing two layouts. |
| C3 | B14 | `field_mapper.go:445-449` and converter signature | Plumb `userMap` into the converter; emit Windshift-mention syntax for known accountIds. Depends on B23/A4. |
| C4 | B18 (consumer) | `entities.go:475-498` | After A3: convert local `authorID int` to `*int`, pass through. |

### Class D — feature-shaped (independent tracks, can parallelize)

| Step | Bug(s) | Files | Notes |
|---|---|---|---|
| D1 | B22 | new client method `GetIssueComments` (and worklog equivalent); `entities.go:469, 680` | Iterate `startAt` until `Total ≤ count`. |
| D2 | B08 | `entities.go` (new `importWorklogs`); possibly new columns on `items` for time-tracking aggregates | Insert into `time_worklogs`; decide where TimeTracking goes. |
| D3 | B12a (table) | `field_mapper.go` switch + new `convertADFTable` helper | Highest user impact. |
| D4 | B12b (panel) | same | GFM admonition or `> *Info:* ...`. |
| D5 | B12c (media) | same; depends on attachment import providing media-id → URL mapping | Emit `![alt](url)`. |
| D6 | B12d–g | same | Lower priority, in order of impact. |
| D7 | B24 | `jira_importer.go:124-191`; frontend wizard | Detect existing project imports; require explicit confirmation. |

### Live capture verification (from Phase 1–3 of the approved plan)

The diff checklist remains the regression suite. After each fix lands:

1. `set -x JIRA_CAPTURE_PAYLOADS /tmp/jira-capture/run-N`, `mkdir -p $JIRA_CAPTURE_PAYLOADS`
2. Build + start `core`, run the wizard end-to-end against the test corpus
   (TEST-1..TEST-7 from the approved plan).
3. Diff captured `jira_responses.json` against the resulting Windshift DB
   rows, joining via `jira_import_id_mappings.job_id`. Checklist fields:
   title, description (ADF), status, item_type, priority, assignee_id,
   reporter_id, **creator_id**, due_date, milestone_id (first), additional
   fix versions, affects versions, **labels**, **components**,
   **story_points**, **iteration_id**, **resolved_at**, **created_at**,
   **updated_at**, parent_id, custom field values (every type), comment
   count + per-comment author/timestamp/body, attachment count,
   item_links (outward AND inward), worklog count, time_tracking values,
   ADF mention → @user-id linkage.
4. Mark each B0n entry from MISSING/WRONG to OK.

### Recommended next-round prerequisite (B00 — methodology)

Before implementing A1, add a `windshift_export.json` sibling to the
existing `jira_responses.json`. ~80 LOC inside `jira_import_execution.go`
after `defer rc.saveToFile(...)`. Snapshot the imported items / comments /
attachments / links keyed by `jira_key`. Once both sides are JSON, a small
~150-LOC Python diff at `scripts/jira_import_diff.py` replaces manual SQL
diffing and serves as the ongoing regression check after each fix.
