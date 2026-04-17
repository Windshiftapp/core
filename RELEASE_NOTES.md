# Windshift v0.5.1

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## New Features

### Form Channels

- Public form submission channel type — Create public-facing forms that submit directly into work items. Configure a slug, theme, brand color, logo, success message, and redirect URL per channel.
- Embeddable widget — A lightweight JavaScript widget for embedding forms on external websites.
- Portal integration — Reuses portal session and customer management for optional authenticated submissions.

### Notion Integration

- OAuth-based workspace integration — Connect Notion workspaces via OAuth with encrypted credential storage and CSRF-protected state tokens.
- Item linking — Link work items to Notion pages and databases from the item detail view.

### AI Assistant

- Comment tools — The AI chat assistant can now list and add comments on work items, with workspace access checks and audit trail.

---

## Enhancements

### Frontend

- Design system tokens — Replaced hardcoded dark mode color ternaries with centralized design system CSS variables across portal and hub components.
- Icon selector improvements — Added a `colorOnly` mode for standalone color selection, injectable icon maps, compact color-swatch trigger, and improved search.
- Color picker consolidation — Removed the standalone ColorPicker component in favor of the updated IconSelector with color-only mode.
- Command palette — Added additional navigation entries.
- Locale additions — New i18n strings for channels, forms, workspaces, and navigation.

### Backend

- Condition set fallback — Condition set lookups now fall back to the default configuration set (with item type override) when no workspace-specific set is configured.
- Analytics query — Cumulative flow chart now resolves the workspace workflow or falls back to the default workflow, fixing empty charts for workspaces using default configuration.
- Script engine — User-authored condition scripts with top-level `return` statements are automatically retried wrapped in an IIFE, preventing syntax errors.
- Condition filter logging — Condition filtering errors are now logged with item and condition set context instead of being silently swallowed.
- Condition config scanning — Fixed JSON deserialization of condition config when loading condition sets for editing.

### Code Quality

- Codebase deduplication — Systematic extraction of shared helpers across auth, SSO, WebAuthn, repositories, services, REST API handlers, CLI, plugins, and middleware. Reduced duplication while improving consistency.

---

## Security

- Personal workspace isolation — Personal workspaces are now excluded from implicit "Everyone" permissions. Previously, users could access other users' personal workspaces through the implicit Viewer/Editor/Tester grant. Personal workspace owners retain full access through dedicated ownership checks across permission caching, workspace listings, and fallback queries.

---
