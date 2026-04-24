<p align="center">
  <img src=".github/assets/readme-splash.svg" alt="Windshift — a self-hosted work management platform for teams" width="100%">
</p>

<p align="center">
  <a href="https://windshift.sh/download"><img src="https://img.shields.io/badge/download-latest-2e7dbd?style=flat-square" alt="Download"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-2e7dbd?style=flat-square" alt="AGPL-3.0 License"></a>
  <a href="https://windshift.sh/docs"><img src="https://img.shields.io/badge/docs-windshift.sh-2e7dbd?style=flat-square" alt="Documentation"></a>
</p>

---

## Overview

Windshift is a comprehensive highly optimized work management platform that combines task tracking, workflow automation, and team collaboration in a single self-hosted application. Built with Go and Svelte, it offers enterprise-grade features while remaining easy to deploy and maintain.

## Screenshots

<table>
  <tr>
    <td width="50%"><img src=".github/assets/screenshots/hero-board.webp" alt="Kanban board"></td>
    <td width="50%"><img src=".github/assets/screenshots/hero-dashboard.webp" alt="Dashboard"></td>
  </tr>
  <tr>
    <td align="center"><sub><b>Boards</b> — drag, rank, filter</sub></td>
    <td align="center"><sub><b>Dashboards</b> — widgets for every surface</sub></td>
  </tr>
  <tr>
    <td width="50%"><img src=".github/assets/screenshots/milestone-detail.webp" alt="Milestone detail"></td>
    <td width="50%"><img src=".github/assets/screenshots/hero-tree.webp" alt="Hierarchy tree"></td>
  </tr>
  <tr>
    <td align="center"><sub><b>Milestones</b> — plan and track delivery</sub></td>
    <td align="center"><sub><b>Hierarchy</b> — nest work to any depth</sub></td>
  </tr>
</table>

## Features

**Work Management**
- Workspaces and Task Management with custom fields and workflows
- Configurable statuses, priorities, screens and item types
- Rich text descriptions with mentions and attachments
- Recurring tasks with flexible scheduling

**Collaboration**
- Comments with activity tracking
- Multi-channel notifications (email, webhooks)
- Customer portal for external submissions
- Public boards for external stakeholders
- Team workspaces with role-based access

**Integrations**
- SSO/OIDC authentication (PocketID, Authentik, etc.)
- WebAuthn/FIDO2 passwordless login
- SCM integration (GitHub, Gitea)
- Jira project import

**Additional Modules**
- Test management (cases, runs, results)
- Time tracking with project billing and customer management
- Asset management / CMDB
- Collections and saved searches

## Getting started

Download the Windshift binaries from https://windshift.sh/download — you can find the quick start guide [here](https://windshift.sh/docs/01-getting-started/02-quick-start).

## Help wanted

**Important**: If you are viewing this on Github, this repository is a push mirror for https://codeberg.org/realigned/windshift-core. Code contributions can only be made on Codeberg.

If you would like to contribute to this project, we are looking for help in the following areas:

#### Early bug reports
Let us know if you encounter any bug or uncertainties about a feature via Github Issues.

#### OIDC Providers
If you can connect Windshift to an OIDC Provider, let us know how it goes via Discussion. Both positive and negative feedback helps us here. We have tested OIDC with PocketID from our side.

## Tech Stack

- **Backend**: Go 1.25+
- **Frontend**: Svelte 5, Vite, Tailwind CSS
- **Database**: SQLite (default) or PostgreSQL
- **Authentication**: Sessions, JWT, WebAuthn, OIDC

Windshift is built on a minimalist philosophy: a lean frontend and backend while maintaining a large set of features. We try to keep the memory and resource consumption as low as possible. You can run Windshift on a Raspberry Pi easily.

## Documentation

- [BUILD.md](BUILD.md) — Build instructions
- [CONTRIBUTING.md](CONTRIBUTING.md) — Contributing guide
- [LOGGING.md](LOGGING.md) — Logging configuration

## License

See [LICENSE](LICENSE) for details.
