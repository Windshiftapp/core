# Windshift v0.4.7

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### MCP Server (Model Context Protocol)

Windshift now ships a built-in MCP server at `/mcp`, enabling AI assistants and external tools to interact with your workspace programmatically. The server uses stateless Streamable HTTP transport with Bearer token authentication.

Available tool categories:

- **Items** — list, get, create, update, delete, search, and get children of work items
- **Workspaces** — list and inspect accessible workspaces
- **Comments** — list and add comments on work items (plain text or TipTap JSON)
- **Labels** — list workspace labels and assign them to items
- **Time Tracking** — list projects, manage worklogs, start/stop timers

### Integrated Terminal

The terminal panel has been overhauled into a full multi-tab terminal experience. In the Tauri desktop app it spawns native PTY sessions; drag-and-drop a work item into the terminal to generate a task prompt. The tab bar shows workspace info and `ws.toml` configuration status.

A new **WsTomlProvisioner** overlay guides first-time setup: one-click token generation, server URL configuration, and `ws.toml` creation for CLI authentication in project directories.

### Collection Navigation Sidebar

A new dedicated sidebar appears when viewing a collection. It shows the collection name, available views (Backlog, Board, List, Tree, Map, Roadmap), and a backlog count badge. The sidebar is collapsible to a 48px icon-only rail and resizable between 180–320px.

### Customer & Organisation Management

The Customers page has been restructured around organisations:

- **Organisation detail view** with tabs for Contacts, Files, and Tickets
- **Contact detail view** with tabs for Overview, Submissions, and Channels, plus inline editing and custom field support
- Left sidebar with organisation search and drag-and-drop contact-to-organisation assignment
- Document grid with thumbnails, status badges, and upload source tracking

---

## New Features

- **ChipPicker component** — compact, searchable pill-shaped dropdown with keyboard navigation, used in the create modal and elsewhere
- **Workspace path store** — tracks workspace folder paths with localStorage persistence and `ws.toml` detection status
- **Create modal improvements** — stepped navigation (Type → Workspace → Item type) using ChipPicker, parent item support for child creation
- **Collection view switcher** — tab-style switcher with backlog count badge and context-aware styling
- **Workspace data store auto-refresh** — automatic 5-minute refresh intervals with granular field invalidation and race condition protection on workspace switches
- **Logbook customer org filter** — `customer_organisation_id` query parameter on `GET /rest/api/v1/logbook/documents` filters documents by organisation

## Bug Fixes

- **Empty collection query fallthrough** — collections with no filter rules no longer fall through to workspace-level queries; they correctly return empty results
- **Logbook model completeness** — `customer_organisation_id` and `portal_customer_id` fields are now included in document queries and API responses

## API Changes

- Added `GET /mcp`, `POST /mcp`, `DELETE /mcp` endpoints for the MCP server
- Added `customer_organisation_id` query parameter to `GET /rest/api/v1/logbook/documents`
- Added `customer_organisation_id` and `portal_customer_id` fields to logbook document responses

## Removed

- **CompactWorkspaceSelector** — removed in favour of ChipPicker-based selection in the create modal
