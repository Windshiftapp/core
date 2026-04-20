# Windshift v0.5.3

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Features

### Agent Management

- User-managed agents: create, edit and assign permissions to agents through the UI. These agents inherit the users permissions and can be tied to user accounts, providing a clear identification trail for any changes.
- Permission invalidation propagates to agents when roles or groups of the connected user change
- Schema and runtime migrations add the required columns on upgrade

### Asset reports

- New form mode for asset reports with configurable report fields
- Customise which fields are exposed per report type

### Public REST API

- Added a v1 milestone progress endpoint

### Editor

- Code blocks in the Milkdown editor are now syntax-highlighted via Shiki

### Portal Hub

- Open-request count now appears as a badge on the hub Inbox button, matching the badge shown on individual portals

## Enhancements

### Frontend

- Replaced remaining native `alert()` calls with the shared error toast
- Replaced the theme cycle button with an explicit accordion picker; new accordion type available in the DropdownMenu component
- Localized status-category delete errors and fixed a plural-syntax issue in translations
- Assorted modal and layout fixes

### Backend

- Consolidated all asset SQL behind AssetRepository
- Routed the remaining workspace, homepage, personal, portal, configuration-set and asset-link handlers through ItemRepository, removing the last inline item SQL
- Extracted dedicated repositories for test coverage, test runs, test sets, test folders, test run templates and test summaries
- Repository extensions for CQL lookup, everyone-role helpers and item consolidation queries
- Dropped a broken `asset-reports` route and tidied hub.go imports
- Refactored the analytics linear-forecast path for clarity

### CLI

- Several improvements to `cmd/ws`
