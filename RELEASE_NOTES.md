# Windshift v0.4.8

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### Portal Improvements

- **Unified header across all views** — Replaced the separate hamburger menu and compact request-view header with a single persistent header bar showing logo, title, settings, and profile across both the portal home and My Requests views.
- **Inline edit mode** — Added a fixed edit bar at the top of the viewport with a "Done" button. The portal header shifts down automatically to avoid overlap. Edit mode can also be toggled from the customize panel sidebar.
- **Post-submission redirect** — After submitting a request through the portal, users are now automatically taken to the request detail view instead of staying on the form.
- **Open request badge** — The "My Requests" button in the header now shows a count of open (non-completed) requests. Badge count loads on page load rather than only when navigating to the requests view.
- **Status badges in My Requests** — Request lists and detail views now display colored status badges (using status category colors) instead of plain text labels. Completed requests appear dimmed in the list.
- **Customize panel updates** — Edit mode toggle added to the customize panel sidebar. The modal backdrop is hidden while in edit mode so the portal remains interactive. An info banner indicates when edit mode is active.

### Security

- **Input sanitization** — Added `SanitizeTitle()` and `SanitizeCommentContent()` to 8 backend handlers: statuses, priorities, link types, projects, iterations, test sets, time customers, and personal labels.
- **XSS fixes** — Replaced inline HTML injection with shared `escapeHtml()` in TestRunDetail and TimeReports print templates. Added server-side sanitization for test run names.
- **CSRF Origin/Referer fallback** — CSRF middleware now falls back to checking Origin and Referer headers when the `Sec-Fetch-Site` header is stripped by reverse proxies. Extracted a shared `buildAllowedOrigins` helper with full test coverage.
