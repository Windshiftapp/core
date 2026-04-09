# Windshift v0.4.9

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### Issue Sync (GitHub)

- **Bi-directional GitHub issue sync** — Sync GitHub issues into Windshift workspaces as work items. Status changes in Windshift push back to GitHub, and vice versa.
- **Comment sync** — GitHub issue comments pull into Windshift items. Comments added in Windshift push back to GitHub with author attribution.
- **Status mapping** — Map GitHub issue states (open/closed) to Windshift statuses with configurable reverse mappings for push-back.
- **Label sync** — Three modes: none, mapped (explicit label mapping), or mirror (auto-create matching labels).
- **Filtering** — Restrict sync scope to issues with specific GitHub labels.
- **Assignee and milestone mapping** — Map GitHub usernames to Windshift users and GitHub milestones to Windshift iterations.
- **Manual and automatic sync** — Trigger syncs manually or let them run on a schedule. Sync status and errors are visible in the settings UI.

### Collection Improvements

- **Stale data fix** — Navigating back to a collection after editing its query now shows fresh results instead of stale cached data. The route guard, refresh merge strategy, and collection cache have all been reworked.
- **Server-side sorting** — Collection list view now supports sortable columns. Sort requests are sent to the backend for correct ordering across pages.
- **Pagination safety** — The list view no longer shows a stale pagination bar (e.g., "337 results") when the items array is empty.
- **Reactive board and backlog** — Board and backlog views now derive data directly from the central collection store instead of maintaining local copies, eliminating an entire class of sync bugs.

### Capability Manager

- **New admin UI** — Manage action capabilities including Docker container environments (image, memory, CPU, network), HTTP client configuration (URL patterns, headers, timeouts), and LLM connection settings.

### AI Safety

- **Prompt injection defense** — AI-powered features now wrap user-provided content with injection defense markers, instructing the LLM to treat wrapped content as data extraction targets rather than executable instructions.
