# Windshift v0.4.5

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### Improved Onboarding for Non-Admin Users
The onboarding flow now handles non-admin users who already have access to a workspace. Previously, these users could get stuck in the onboarding process; they are now guided directly into their available workspace.

### Searchable Multi-Select for Iteration Filters
The CQL iteration `IN` / `NOT IN` filter UI has been upgraded from a plain checkbox list to a searchable multi-select dropdown, making it much easier to work with long lists of iterations.

### Resizable Collections Sidebar
The collections sidebar can now be resized by dragging a handle on its edge, giving you control over how much screen space it occupies.

### Workspace Key Cache
Workspace key resolution no longer requires a database lookup on every request. Keys are now cached in memory, improving response times for all workspace-scoped API calls.

---

## Bug Fixes

- **CQL Iteration Filter:** Fixed iteration `IN` / `NOT IN` queries and corrected user group name resolution in CQL
- **PostgreSQL Notification Settings:** Fixed notification settings queries and the config set admin page failing on PostgreSQL

## Other Changes

- Split large handler files into smaller, focused modules (planning, Jira importer, SCM, portal, AI)
