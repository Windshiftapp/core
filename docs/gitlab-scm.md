# GitLab SCM integration

Windshift supports GitLab.com and self-managed GitLab as an SCM provider. The
integration uses GitLab REST API v4 and supports nested namespaces such as
`group/subgroup/project`.

## Provider setup

Create an SCM provider in **Admin → Source Control** and choose **GitLab**.

- For GitLab.com, leave Base URL empty or use `https://gitlab.com`.
- For self-managed GitLab, enter the instance root URL, for example
  `https://gitlab.example.com`. A trailing `/api/v4` is accepted and normalized.
- OAuth applications should grant the `api` scope. Register the callback URL
  shown by Windshift in the GitLab application.
- PAT providers require a token with API access and sufficient project role for
  the operations users will perform.

After an administrator creates the provider, a workspace administrator can
connect it, authorize with OAuth or PAT, and link one or more projects.

## Supported project data

The provider supports project discovery, branches, commits, merge requests,
MR notes and review discussions, tags, compare ranges, and GitLab Releases.
Windshift uses GitLab merge-request terminology in provider data while keeping
the provider-neutral `pull_request` value in its internal SCM link model.

Repository polling remains the recovery mechanism. A workspace administrator
can also open a linked GitLab project's webhook settings in Windshift, generate
a callback URL and one-time secret, then configure that webhook in GitLab for:

- push and tag push;
- merge request;
- note;
- release.

Webhook requests are token-validated, project-validated, rate-limited, and
deduplicated before they trigger a repository sync.

## Milestone releases

When releasing a milestone, users can either create a new SCM release or attach
an existing Git tag/GitLab Release. Stored release data includes the project,
tag and release URLs, release status and date, downloadable assets, and the last
successful synchronization time.

Normal repository sync refreshes attached release metadata. If a remote release
is removed but its tag remains, the record becomes `tag_only`; if both disappear,
it becomes `missing` rather than silently deleting the milestone history.
