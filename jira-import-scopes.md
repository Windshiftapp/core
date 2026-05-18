# Jira Import — Required API Token Scopes

A reference for administrators creating an Atlassian Cloud API token (or a
Data Center PAT) for the Windshift Jira importer. Maps every Jira REST
endpoint the importer touches to the classic and granular OAuth scopes that
grant it.

## TL;DR — Atlassian Cloud

Pick a **classic scope** for simplicity:

```
read:jira-work
read:jira-user
```

That covers everything in the platform importer. Add `read:jira-software` if
you want sprints and boards. Add the Assets/CMDB scope if you also import
Insight.

**Do not add `read:me`.** The connection test no longer requires it (see
`internal/jira/client.go::TestConnection`).

## Why this matters

Atlassian Cloud rolled out **scoped API tokens** in 2024. When a token is
created with scopes, every Jira endpoint enforces them. A token that's
visibly "Read Jira project data" only will 401 against `/myself` even though
it succeeds against `/project`, `/field`, `/search`, etc. The importer used
to probe `/myself` for the connection test and would (incorrectly) report
the whole token as invalid in that case; that probe is now `/serverInfo`,
which any authenticated request reaches regardless of scope.

Tokens created before scoped-tokens rolled out act as **unscoped** legacy
credentials and need no scope selection — they inherit the full set of the
creating user's Jira permissions.

## Endpoint → scope map

All paths are relative to `https://<your-instance>.atlassian.net`.

### Platform (REST API v3)

| Importer call | Classic | Granular |
|---|---|---|
| `GET /rest/api/3/serverInfo` | (auth only) | (auth only) |
| `GET /rest/api/3/myself` *(optional enrichment)* | `read:jira-user` | `read:me` + `read:account` |
| `GET /rest/api/3/user/email?accountId=…` | `read:jira-user` | `read:user:jira` + `read:email-address:user-profile` |
| `GET /rest/api/3/project?expand=description` | `read:jira-work` | `read:project:jira` |
| `GET /rest/api/3/project/{key}` | `read:jira-work` | `read:project:jira` |
| `GET /rest/api/3/project/{key}/statuses` | `read:jira-work` | `read:issue-type:jira` + `read:status:jira` |
| `GET /rest/api/3/project/{key}/versions` | `read:jira-work` | `read:project-version:jira` |
| `GET /rest/api/3/issuetype` | `read:jira-work` | `read:issue-type:jira` |
| `GET /rest/api/3/field` | `read:jira-work` | `read:field:jira` |
| `GET /rest/api/3/status` | `read:jira-work` | `read:status:jira` |
| `GET /rest/api/3/statuscategory` | `read:jira-work` | `read:status:jira` |
| `GET /rest/api/3/search?jql=…` | `read:jira-work` | `read:issue:jira` + `read:issue-meta:jira` + `read:user:jira` |
| `POST /rest/api/3/search/jql` | `read:jira-work` | `read:issue:jira` + `read:issue-meta:jira` |
| `POST /rest/api/3/issue/bulkfetch` | `read:jira-work` | `read:issue:jira` |
| `GET /rest/api/3/issue/{key}?expand=…` | `read:jira-work` | `read:issue:jira` + `read:issue.comment:jira` + `read:issue.attachment:jira` + `read:issue.worklog:jira` + `read:issue.changelog:jira` |

### Jira Software (Agile)

| Importer call | Classic | Granular |
|---|---|---|
| `GET /rest/agile/1.0/board?projectKeyOrId=…` | `read:jira-software` | `read:board-scope:jira-software` + `read:project:jira` |
| `GET /rest/agile/1.0/board/{id}/sprint` | `read:jira-software` | `read:sprint:jira-software` + `read:board-scope:jira-software` |

### Assets / Insight (Jira Service Management)

| Importer call | Classic | Granular |
|---|---|---|
| `GET /rest/assets/1.0/objectschema/list` | `read:cmdb-object:jira` | `read:cmdb-object:jira` + `read:cmdb-schema:jira` |
| `GET /rest/assets/1.0/objectschema/{id}` | `read:cmdb-object:jira` | `read:cmdb-schema:jira` |
| `GET /rest/assets/1.0/objectschema/{id}/objecttypes/flat` | `read:cmdb-object:jira` | `read:cmdb-type:jira` |
| `GET /rest/assets/1.0/objecttype/{id}/attributes` | `read:cmdb-object:jira` | `read:cmdb-attribute:jira` |
| `POST /rest/assets/1.0/object/navlist/aql` | `read:cmdb-object:jira` | `read:cmdb-object:jira` |

## Recommended scope sets

Pick the set matching the data you want to import. Each builds on the one
above it.

**Core import** — projects, issues, fields, comments, attachments, worklogs:
```
read:jira-work
read:jira-user
```

**Add Software boards & sprints**:
```
read:jira-work
read:jira-user
read:jira-software
```

**Add Insight / Assets**:
```
read:jira-work
read:jira-user
read:jira-software
read:cmdb-object:jira
```

The importer is **read-only**. Do not add `write:*` or `manage:*` scopes —
they aren't used and broaden the blast radius of a leaked token.

## Diagnosing scope failures

After the connection-test fix, every Jira 4xx response is logged at warn on
the Windshift server and surfaced to the UI with Jira's verbatim error
text. Common shapes:

| Jira says | Means |
|---|---|
| `Client must be authenticated to access this resource.` | Token lacks the scope for this endpoint. Compare the failing path against the table above. |
| `OAuth 2.0 is not enabled for this method.` | Endpoint isn't reachable via the token type you used. Rare with API tokens. |
| `You do not have the permission to see the specified issue.` | Scope is fine; the account lacks project-level Jira permission. Grant Browse Projects in the project's Permission Scheme. |
| `Unbounded JQL queries are not allowed here.` | Importer bug, not a scope problem — JQL must include a restriction (e.g. `project = X`). |

The server log line is `Jira connection test failed` (warn) for the
connect step and `Failed to ...` (error) for in-flight import calls. Both
include the Jira response body when the upstream returns a non-2xx.

## Data Center

Atlassian Data Center uses **Personal Access Tokens (PATs)**. PATs have no
selectable scopes — they inherit the **creating user's** Jira permissions
verbatim. To make a PAT work end-to-end, the user account needs:

- **Browse Projects** on every project to import
- **View Issues** in the relevant Permission Scheme
- **View Development Tools** if importing Software boards / sprints
- Application access to **Jira Software** for `/rest/agile/1.0/*`
- Application access to **Jira Service Management** + Insight read for Assets
