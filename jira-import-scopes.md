# Jira Import — API Token Setup

A reference for administrators creating an Atlassian Cloud API token (or a
Data Center PAT) for the Windshift Jira importer.

## TL;DR — Atlassian Cloud

**Use a legacy (unscoped) API token.** Scoped tokens do not work with the
Windshift importer today.

Create one at <https://id.atlassian.com/manage-profile/security/api-tokens>
via the **"Create API token"** button (the plain one — *not* "Create API
token with scopes"). The token inherits the account's full Jira
permissions; no scope selection is needed or possible.

If your org policy forbids unscoped tokens, see [Scoped token support](#scoped-token-support-not-implemented)
below — the importer needs a code change before scoped tokens will work.

## Why scoped tokens don't work (yet)

Atlassian's scoped API tokens (rolled out 2024) cannot be used against the
standard site URL `https://<site>.atlassian.net/rest/api/3/...`. They must
be routed through `https://api.atlassian.com/ex/jira/{cloudId}/rest/api/3/...`.

If you hit the direct site URL with a scoped token, Atlassian silently
treats the request as **anonymous** instead of returning 401. Endpoints
that allow anonymous reads return generic data (e.g. `/serverInfo`,
`/field` system catalog); permission-filtered endpoints return empty
arrays (`/project`, `/project/search`, `/issuetype`, `/status`, `/search`);
endpoints that require an authenticated user return 401 (`/myself`).

The Windshift importer (`internal/jira/client.go::NewClient`) currently
hardcodes the direct site URL, so scoped tokens fail silently in exactly
this way: connection test now succeeds (we probe `/serverInfo`, which is
anonymous-readable), but no projects appear in the wizard.

Atlassian source confirming this:

> "You need to call the Atlassian API to use API tokens with scopes for
> Jira `https://api.atlassian.com/ex/jira/{cloudId}` or Confluence
> `https://api.atlassian.com/ex/confluence/{cloudId}`."
>
> — [Manage API tokens for your Atlassian account](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/)

## Scoped token support (not implemented)

To support scoped tokens, the importer would need:

1. At connect time, fetch the cloud ID from the well-known endpoint:
   ```
   GET https://<site>.atlassian.net/_edge/tenant_info
   → {"cloudId": "..."}
   ```
   This endpoint is unauthenticated and stable.

2. Store `cloudId` on the connection record.

3. Rewrite client base URLs from
   `https://<site>.atlassian.net/rest/api/3/...` to
   `https://api.atlassian.com/ex/jira/{cloudId}/rest/api/3/...`
   for every importer call.

4. Same change for the Agile (`/rest/agile/1.0/...`) and Assets
   (`/rest/assets/1.0/...`) clients.

5. Accept both URL shapes from the operator (we already accept the site
   URL; the cloudId comes from step 1).

When implemented, the per-endpoint scope requirements are documented
below for selecting scopes during token creation.

## Scope reference (for future scoped-token support)

Per [Atlassian's official scope docs](https://developer.atlassian.com/cloud/jira/platform/scopes-for-oauth-2-3LO-and-forge-apps/),
the classic scopes the importer would need are:

| Classic scope | Covers |
|---|---|
| `read:jira-work` | Read Jira project and issue data, search for issues and objects associated with issues like attachments and worklogs |
| `read:jira-user` | View user information in Jira that the user has access to, including usernames, email addresses, and avatars |

Add `read:jira-software` (Jira Software classic scope) if importing
boards/sprints, and the CMDB scope (`read:cmdb-object:jira` or similar)
if importing Insight / Assets.

**Important caveat from Atlassian**:

> "Jira permissions also control access to data and aren't overridden by
> scopes. For example, if a user does not have the Browse projects
> permission then the Get project operation won't be able to access
> project data even if the app has the manage:jira-project and other
> required scopes."

So scope grants are necessary but not sufficient — the underlying account
must also have project-level Browse permission in Jira's permission
schemes.

### Granular scopes (recommended for new integrations)

Atlassian recommends granular scopes for tighter least-privilege. The
endpoint → granular-scope mapping is documented per-operation in the REST
API reference at
<https://developer.atlassian.com/cloud/jira/platform/rest/v3/>.
The importer would minimally need:

- `read:project:jira` — list / fetch projects
- `read:project-version:jira` — fetch project versions (fix versions)
- `read:issue:jira` — fetch issues
- `read:issue-meta:jira` — issue metadata (also used by JQL search)
- `read:issue.changelog:jira`, `read:issue.comment:jira`, `read:issue.attachment:jira`, `read:issue.worklog:jira` — issue children
- `read:issue-type:jira` — list issue types
- `read:status:jira` — list statuses
- `read:field:jira` — list custom fields
- `read:user:jira` — resolve assignees / reporters / creators

For boards & sprints add `read:board-scope:jira-software` and
`read:sprint:jira-software`. For Insight / Assets add the corresponding
`read:cmdb-*:jira` scopes.

`read:me` is **not** required — the importer's `TestConnection` probes
`/serverInfo` (no scope needed) and treats `/myself` as best-effort
enrichment that's silently ignored on 401.

## Diagnosing failures

After the connection-test fix, every Jira 4xx response is logged at warn
on the Windshift server and surfaced to the UI with Jira's verbatim
error text. Common shapes:

| Jira says | Means |
|---|---|
| `Client must be authenticated to access this resource.` | Scoped token used against the direct site URL — Atlassian dropped your identity. Use an unscoped token, or wait for scoped-token routing support. |
| `You do not have the permission to see the specified issue.` | Token is valid; the account lacks per-project Browse permission in Jira's permission scheme. |
| `Unbounded JQL queries are not allowed here.` | Importer bug, not a token problem — JQL must include a restriction. |
| Empty `[]` from `/project`, `/issuetype`, etc. with no error | Almost certainly a scoped token used against the site URL. Atlassian routes the call as anonymous; everything permission-filtered returns empty. |

The server log line is `Jira connection test failed` (warn) for the
connect step and `Failed to ...` (error) for in-flight import calls.
Both include the Jira response body when the upstream returns a non-2xx.

## Data Center

Atlassian Data Center uses **Personal Access Tokens (PATs)**. PATs have
no selectable scopes — they inherit the **creating user's** Jira
permissions verbatim, and they use Basic auth against the standard site
URL with no special routing. To make a PAT work end-to-end, the user
account needs:

- **Browse Projects** on every project to import
- **View Issues** in the relevant Permission Scheme
- **View Development Tools** if importing Software boards / sprints
- Application access to **Jira Software** for `/rest/agile/1.0/*`
- Application access to **Jira Service Management** + Insight read for Assets

## Sources

- [Manage API tokens for your Atlassian account](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/) — official, explains scoped vs unscoped token endpoint requirements
- [Jira scopes for OAuth 2.0 (3LO) and Forge apps](https://developer.atlassian.com/cloud/jira/platform/scopes-for-oauth-2-3LO-and-forge-apps/) — canonical classic + granular scope list
- [Jira Cloud platform REST API v3](https://developer.atlassian.com/cloud/jira/platform/rest/v3/) — per-operation scope requirements
- [How to use a Scoped API key for user query](https://community.atlassian.com/forums/Jira-questions/How-to-use-a-Scoped-API-key-for-user-query/qaq-p/3114697) — community thread documenting the URL-routing requirement
