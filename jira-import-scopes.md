# Jira Import — API Token Setup

A reference for administrators creating an Atlassian Cloud API token (or a
Data Center PAT) for the Windshift Jira importer.

## TL;DR — Atlassian Cloud

The importer accepts **both legacy unscoped tokens and scoped tokens**.
The host routing required for scoped tokens is detected automatically; no
operator action beyond picking the right scopes is needed.

Create either via <https://id.atlassian.com/manage-profile/security/api-tokens>:

- **"Create API token"** (plain) — unscoped/legacy. Inherits the account's
  full Jira permissions, no scope picker.
- **"Create API token with scopes"** — scoped. Pick the scopes listed
  below. **Must include `read:me`** or the importer's auto-probe can't
  recognise the routing (see [Edge case](#edge-case-scoped-tokens-without-readme)
  below if your security policy forbids it).

The connection wizard requires only the site URL
(`https://<your-site>.atlassian.net`) and the token; the importer
discovers the cloud ID and switches transports as needed.

## Recommended scopes — scoped tokens

Picked in the Atlassian token UI:

| Classic scope (UI label) | Why the importer needs it |
|---|---|
| **`read:jira-work`** ("Read Jira project data") | Projects, issues, fields, JQL search, bulkfetch, project versions |
| **`read:jira-user`** ("View Jira user info") | Resolve assignees / reporters / creators back to email & display name |
| **`read:me`** ("View your Atlassian account") | Identity probe used by the routing auto-detector |
| **`read:jira-software`** *(optional)* | Boards & sprints (`/rest/agile/1.0/...`) |

Granular alternative (recommended by Atlassian for new integrations) —
see [Atlassian's scope reference](https://developer.atlassian.com/cloud/jira/platform/scopes-for-oauth-2-3LO-and-forge-apps/)
for the canonical list. The importer minimally needs:

- `read:project:jira`, `read:project-version:jira`
- `read:issue:jira`, `read:issue-meta:jira`,
  `read:issue.changelog:jira`, `read:issue.comment:jira`,
  `read:issue.attachment:jira`, `read:issue.worklog:jira`
- `read:issue-type:jira`, `read:status:jira`, `read:field:jira`
- `read:user:jira`
- `read:me` (for the routing probe)
- `read:board-scope:jira-software`, `read:sprint:jira-software`
  (for boards/sprints)

The importer is **read-only**. Don't grant `write:*` or `manage:*` —
they're unused and broaden the blast radius of a leaked token.

## Why two URL hosts exist

Atlassian rolled out scoped API tokens in 2024 with a quirk that catches
every long-standing integration: **scoped tokens cannot be used against
`https://<site>.atlassian.net/...` directly**. Atlassian silently
downgrades the request to anonymous instead of returning 401. Endpoints
that allow anonymous reads return generic data; permission-filtered
endpoints return empty arrays; only `/myself` surfaces the 401. The
wizard's project list ends up empty with no actionable error.

Atlassian's documented routing for scoped tokens is:

> "You need to call the Atlassian API to use API tokens with scopes for
> Jira `https://api.atlassian.com/ex/jira/{cloudId}` or Confluence
> `https://api.atlassian.com/ex/confluence/{cloudId}`."
>
> — [Manage API tokens for your Atlassian account](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/)

Legacy unscoped tokens, by contrast, must keep using the site URL.

**How the importer handles both** (`internal/jira/client.go::cloudRoutingProbe`):

1. Fetch the unauthenticated `<site>/_edge/tenant_info` to learn the
   cloud ID.
2. Hit `https://api.atlassian.com/ex/jira/{cloudId}/rest/api/3/myself`
   with the token.
3. If that returns 200, the token is scoped and the importer uses the
   gateway URLs for every subsequent call. Otherwise it falls back to
   the site URL (legacy token, or `tenant_info` unreachable).

Decision is logged at info level (`Jira cloud routing: using
api.atlassian.com gateway` or `… using site URL`).

## Edge case: scoped tokens without read:me

The probe above uses `/myself` because it's the only endpoint that
reliably distinguishes "authenticated" from "anonymous-because-silently-downgraded".
A scoped token created without the `read:me` scope returns 401 on
`/myself` even when routed correctly, so the probe will fall back to the
site URL — where the token also fails.

If org policy forbids `read:me`, use a legacy unscoped token instead. We
can switch the probe to a different endpoint (e.g.
`POST /rest/api/3/permissions/check`) if this becomes a recurring
constraint.

## Account-level Jira permissions

Scope grants are necessary but not sufficient:

> "Jira permissions also control access to data and aren't overridden by
> scopes. For example, if a user does not have the Browse projects
> permission then the Get project operation won't be able to access
> project data even if the app has the manage:jira-project and other
> required scopes."
>
> — [Atlassian scopes reference](https://developer.atlassian.com/cloud/jira/platform/scopes-for-oauth-2-3LO-and-forge-apps/)

The account that owns the token must have **Browse Projects** on every
project to import, plus **View Issues** in the project's Permission
Scheme. Verify by signing in to the Jira UI as that account and checking
the project switcher — if it's empty there, scopes don't matter.

## Diagnosing failures

Every Jira 4xx response is logged at warn on the Windshift server and
surfaced to the UI with Jira's verbatim error text:

| Jira says | Means |
|---|---|
| `Client must be authenticated to access this resource.` | Routing probe likely fell back to site URL with a scoped token. Confirm `read:me` is on the token's scope list, or switch to a legacy unscoped token. |
| `You do not have the permission to see the specified issue.` | Token routing OK; the account lacks per-project Browse permission. Adjust the project's Permission Scheme. |
| `Unbounded JQL queries are not allowed here.` | Importer bug, not a token problem. |
| Empty `[]` from `/project`, `/issuetype`, etc. with no error | Should no longer happen since the auto-probe shipped. If it does: check the server log for `Jira cloud routing:` — if it says "using site URL" with a scoped token, the probe failed. |

## Data Center

Atlassian Data Center uses **Personal Access Tokens (PATs)**. PATs have
no selectable scopes — they inherit the **creating user's** Jira
permissions verbatim, use Basic auth against the standard site URL, and
the auto-probe is skipped entirely (it's Cloud-only). The PAT account
needs:

- **Browse Projects** on every project to import
- **View Issues** in the relevant Permission Scheme
- **View Development Tools** if importing Software boards / sprints
- Application access to **Jira Software** for `/rest/agile/1.0/*`
- Application access to **Jira Service Management** + Insight read for Assets

## Sources

- [Manage API tokens for your Atlassian account](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/) — official, scoped-token gateway routing requirement
- [Manage API tokens for service accounts](https://support.atlassian.com/user-management/docs/manage-api-tokens-for-service-accounts/) — official, includes the "Check your endpoint format" troubleshooting section
- [Jira scopes for OAuth 2.0 (3LO) and Forge apps](https://developer.atlassian.com/cloud/jira/platform/scopes-for-oauth-2-3LO-and-forge-apps/) — canonical classic + granular scope list
- [Jira Cloud platform REST API v3](https://developer.atlassian.com/cloud/jira/platform/rest/v3/) — per-operation scope requirements
