# Windshift v0.5.0 — Clear Horizon

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## New Features

### Condition Sets

- **Rule-based transition restrictions** — Define conditions that control when workflow transitions are available. Supports role checks, group membership, field regex matching, and sandboxed JavaScript scripts.

### Recurring Tasks

- **RRULE-based recurrence** — Attach recurrence rules to items with configurable frequency (daily, weekly, monthly, yearly), lead time, and timezone.

### Public Boards

- **Shareable public links** — Share a read-only board view via public link. No login required for viewers.
- **Property display** — Shows status, priority, type, assignee, due date, story points, and labels on public items.
- **Public board attachments** — Embedded images in descriptions load on public boards via a new unauthenticated endpoint. Image-only, with path traversal protection.

### Internal Comments

- **Workspace setting** — New `internal_comments_enabled` workspace setting for internal/private notes outside portal requests.
- **Settings UI toggle** — Enable or disable internal comments from workspace settings.

### Custom Field Options Migration

- **ID-based options** — Select and multiselect custom fields now use ID-based options instead of raw strings.
- **Automatic migration** — Legacy string-array options are auto-migrated on startup. Stale references are cleaned up on option delete.

---

## Enhancements

### Performance

- **Rate limiter improvements** — Per-user keying on authenticated routes prevents shared-IP exhaustion. New `--disable-ip-rate-limit` flag for unauthenticated requests. AI endpoint limit raised to 20/min.
- **Logbook upload rate limiting** — Rate limits applied to logbook upload endpoints.

### Item Detail & Sidebar

- **Collapsible Scheduling section** — New collapsible section in the item detail sidebar for scheduling-related fields.
- **Revamped content layout** — Improved item detail sidebar structure and content organization.

### Collections & Roadmap

- **Roadmap fixes** — Fixed orphaned parent items, improved link fetching, and added a settings panel.
- **Collection breadcrumbs** — Improved breadcrumb navigation for collections.
- **Iteration timeline** — Iteration timeline widget for visualizing iteration progress.
- **Upcoming deadlines** — Enhanced upcoming deadlines widget.

---

## Security & Hardening

- **Fix user email exposure** — Resolved an issue where user emails were exposed in portal comments and the V1 REST API.
- **Public board item limit** — Reduced public board item limit from 1000 to 500.
- **Upload validation hardening** — Stricter file upload validation for attachments and logbook entries with additional content-type and size checks.
- **Permission hardening** — Additional permission checks across label, asset link, comment, and diagram handlers.

