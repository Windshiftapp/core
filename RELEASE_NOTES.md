# Windshift v0.5.0-rc2

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## What's New in rc2

### Security

- **Fix user email exposure** — Resolved an issue where user emails were exposed in portal comments and the V1 REST API.
- **Public board item limit** — Reduced the public board item limit from 1000 to 500.

### Internationalization

- **Chinese locale support** — Added Chinese (zh-CN) locale.

### Time Tracking

- **Customer required for projects** — Customer is now a required field when creating or editing projects.

### UI Fixes

- **Dark mode fix in asset manager** — Corrected styling issues in the asset manager under dark mode.
- **Public board improvements** — Various polish and usability improvements to public boards.

### Internal

- **Workspace handler refactor** — Workspace handler refactored to the repository pattern.
- **About page scope reduced** — Trimmed the about page to essential information.

---

## Highlights (rc1)

### Workspace Analytics

- **Analytics dashboard** — New workspace analytics page with cumulative flow, cycle time, and velocity charts.
- **Monte Carlo forecast** — Forecast panel using Monte Carlo simulation to project completion dates based on historical throughput.

### Public Boards

- **Shareable public links** — Share a read-only board view via public link. No login required for viewers.
- **Item detail modal** — Two-column layout with description and comments on the left, properties sidebar on the right.
- **Property display** — Shows status, priority, type, assignee, due date, story points, and labels on public items.

### Rate Limiter Improvements

- **Per-user keying** — Authenticated routes now key rate limits by user ID instead of IP address, preventing shared-IP users (NAT, office networks) from exhausting each other's buckets.
- **Configurable IP limiting** — New `--disable-ip-rate-limit` flag to disable IP-based rate limiting for unauthenticated requests.
- **AI rate limit increase** — AI endpoint rate limit raised from 5/min to 20/min.

### Internal Comments

- **Workspace setting** — New `internal_comments_enabled` workspace setting for internal/private notes outside portal requests.
- **Settings UI toggle** — Enable or disable internal comments from workspace settings.
- **Plugin comment creation** — Host functions for plugin comment creation with `SuppressNotifications` option.

### Upload Validation Hardening

- **Stricter upload checks** — Hardened file upload validation for attachments and logbook entries with additional content-type and size checks.

### Collections & Navigation

- **Collection breadcrumbs** — Improved breadcrumb navigation for collections.
- **Iteration timeline** — New iteration timeline widget for visualizing iteration progress.
- **Upcoming deadlines** — Enhanced upcoming deadlines widget.

### Permission Hardening

- **Broader permission coverage** — Additional permission checks across label, asset link, comment, and diagram handlers.
