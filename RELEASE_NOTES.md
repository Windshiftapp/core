# Windshift v0.2.5

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

This release focuses on tightening the permission system, improving frontend accessibility, and expanding test coverage across handlers and services.

## New Features

- **Personal task full-page navigation.** The personal task modal now includes an "Open Full Details" button, allowing users to navigate from the modal to the full-page detail view.
- **Workspace lookup by key.** The GET workspace endpoint now accepts either a numeric ID or a workspace key. The `ws` CLI client uses this directly instead of listing all workspaces to resolve a key.
- **`ws` CLI binary in releases.** The release script now builds and packages the `ws` CLI tool alongside the server binary for all target platforms.
- **Notification settings redesign.** Notification settings now use a recipient picker UI with improved layout and controls.

## Improvements

- **Route-level permission middleware.** Permission checks for SCM and test management handlers have been moved from inline handler code to route-level middleware, simplifying handler logic and making access control more consistent. SCM read-only endpoints have been relaxed from workspace admin to item view permission.
- **Structured error responses.** Permission middleware now returns JSON error responses via `restapi.RespondError` instead of plain-text `http.Error`, bringing consistency to the API error format.
- **Frontend accessibility.** Replaced `div[role=button]` patterns with semantic `<button>` elements, added missing ARIA labels, used `<label>` elements for toggle controls, and fixed a11y warnings across components, dialogs, editors, pickers, and settings pages.
- **Svelte 5 migration.** Converted several legacy components to Svelte 5 patterns.
- **Hub and portal updates.** Improved hub hero, portal cards, portal sections, and customization panels.
- **Notification service improvements.** Fixed workspace admin query to use role-based permission tables, added warning type for item-deleted events, improved mention color in emails, and handled nullable `created_by` in notification settings queries.

## Bug Fixes

- **Customer permissions.** Fixed overly restrictive permissions that blocked valid customer access.
- **Recurrence repository.** Fixed query to use the correct `username` column.
- **Code scanning issues.** Addressed findings from GitHub code scanning and audit error checks.

## Testing

- Added handler tests for comments, configuration set notifications, custom fields, notification settings, notification templates, notifications, and recurrence.
- Added comprehensive tests for the notification service covering all event types.
- Updated existing tests to match the new structured error response format.
- Expanded workspace permission integration tests.
