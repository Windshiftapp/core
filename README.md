<p align="center">
  <img src="frontend/public/windshift-3.svg" alt="Windshift" width="120" height="120">
</p>

<h1 align="center">Windshift</h1>

<p align="center">
  A self-hosted work management platform for teams
</p>

---

## Overview

Windshift is a comprehensive highly optimized work management platform that combines task tracking, workflow automation, and team collaboration in a single self-hosted application. Built with Go and Svelte, it offers enterprise-grade features while remaining easy to deploy and maintain.

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

Download the Windshift binaries from https://windshift.sh/download - you can find the quick start guide [here](https://windshift.sh/docs/01-getting-started/02-quick-start).

## Help wanted

**Important**: If you are viewing this on Github, this repository is a push mirror for https://codeberg.org/realigned/windshift-core. Code contributions can only be made on Codeberg.

If you would like to contribute to this project, we are looking for help in the following areas:

#### Early bug reports 
Let us know if you encounter any bug or uncertainties about a feature via Github Issues

#### OIDC Providers
If you can connect Windshift to an OIDC Provider, let us know how it goes via Discussion. Both positive and negative feedback helps us here. We have tested OIDC with PocketID from our side.

## Tech Stack

- **Backend**: Go 1.25+
- **Frontend**: Svelte 5, Vite, Tailwind CSS
- **Database**: SQLite (default) or PostgreSQL
- **Authentication**: Sessions, JWT, WebAuthn, OIDC

Windshift is built on a minimalist philosophy: Lean frontend and backend while maintaining a large set of features. We try to keep the memory and resource consumption as low as possible. You can run Windshift on a Rasperry Pi easily.

## Documentation

- [BUILD.md](BUILD.md) - Build instructions
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contributing guide
- [LOGGING.md](LOGGING.md) - Logging configuration

## License

See LICENSE file for details.
