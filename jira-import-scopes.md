# Jira Import — API Token Setup

How to create a Jira API token that works with the Windshift importer.

## Atlassian Cloud

Create a token at <https://id.atlassian.com/manage-profile/security/api-tokens>.
Either button works:

- **"Create API token"** — legacy unscoped. Inherits the account's full
  Jira permissions. Nothing else to configure.
- **"Create API token with scopes"** — scoped. Select these classic
  scopes:
  - `read:jira-work` — projects, issues, fields, search
  - `read:jira-user` — resolve assignees / reporters
  - `read:me` — required by the importer's routing detector
  - `read:jira-software` *(optional)* — boards & sprints

In the connection wizard, paste your site URL
(`https://<your-site>.atlassian.net`) and the token. The importer
detects which token type you used and routes accordingly.

## Account permissions

Scope grants don't override Jira's per-project permissions. The account
that owns the token must have **Browse Projects** on every project you
want to import. Sign in to the Jira UI as that account first — if the
project switcher is empty there, scopes don't matter.

## Atlassian Data Center

Use a Personal Access Token (PAT). PATs have no scopes; the account
needs **Browse Projects** + **View Issues** on each project, plus
application access to Jira Software for board/sprint import.

## When something fails

The Windshift server logs every Jira error verbatim under the `jira`
component, and the UI surfaces Jira's own message. The most common
shapes:

| Jira says | Fix |
|---|---|
| `Client must be authenticated to access this resource.` | Scoped token is missing `read:me`. Add it, or use a legacy token. |
| `You do not have the permission to see the specified issue.` | Token is fine; the account lacks Browse Projects on that project. |
| Empty project list with no error | Server log will show which routing was chosen — open an issue if "site URL" with a scoped token. |

## References

- [Manage API tokens](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/) — official Atlassian guide for token creation
- [Jira OAuth 2.0 scopes](https://developer.atlassian.com/cloud/jira/platform/scopes-for-oauth-2-3LO-and-forge-apps/) — canonical scope list, including granular alternatives
